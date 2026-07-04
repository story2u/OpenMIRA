// Package archiveingesttask adapts the legacy archive_ingest_tasks table.
package archiveingesttask

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"

	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Record mirrors Python ArchiveIngestTaskRecord.
type Record struct {
	TaskID       string
	EnterpriseID string
	Source       string
	Cursor       string
	SeqStart     int64
	SeqEnd       int64
	MessageCount int
	PayloadJSON  string
	Status       string
	AttemptCount int
	NextRetryAt  *time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// EnqueueBatchInput describes one pulled archive batch staged for ingest.
type EnqueueBatchInput struct {
	EnterpriseID    string
	Source          string
	Cursor          string
	MessagesPayload []map[string]any
}

// AfterEnqueueFunc is called after a staged archive ingest task is durably upserted.
type AfterEnqueueFunc func(ctx context.Context, record Record) error

// Scope identifies one enterprise/source queue partition.
type Scope struct {
	EnterpriseID string
	Source       string
}

// Repository persists staged archive ingest batches.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time

	RetryBackoffBaseSeconds int
	RetryBackoffMaxSeconds  int
	LeaseSeconds            int
	AfterEnqueue            AfterEnqueueFunc
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// GetTask returns one staged ingest task by id.
func (repository *Repository) GetTask(ctx context.Context, taskID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	record, found, err := repository.getTask(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &record, nil
}

// EnqueueBatch upserts a pulled archive batch and keeps task ids stable.
func (repository *Repository) EnqueueBatch(ctx context.Context, input EnqueueBatchInput) (Record, error) {
	if repository.DB == nil {
		return Record{}, fmt.Errorf("archive ingest task database is not configured")
	}
	ent := normalizeEnterpriseID(input.EnterpriseID)
	source := normalizeSource(input.Source)
	cursor := strings.TrimSpace(input.Cursor)
	messages := normalizeMessages(input.MessagesPayload)
	if len(messages) == 0 {
		return Record{}, fmt.Errorf("messages_payload is required")
	}
	seqStart, seqEnd := messageSeqRange(messages)
	archiveMsgIDs := messageArchiveMsgIDs(messages)
	taskID := buildTaskID(taskIDInput{
		EnterpriseID:      ent,
		Source:            source,
		Cursor:            cursor,
		SeqStart:          seqStart,
		SeqEnd:            seqEnd,
		MessageCount:      len(messages),
		FirstArchiveMsgID: firstString(archiveMsgIDs),
		LastArchiveMsgID:  lastString(archiveMsgIDs),
	})
	payloadJSON, err := normalizePayloadJSON(messages)
	if err != nil {
		return Record{}, err
	}
	status, found, err := repository.getStatus(ctx, taskID)
	if err != nil {
		return Record{}, err
	}
	nowParam := repository.dbNowParam()
	if !found {
		_, err = repository.DB.ExecContext(ctx, repository.insertSQL(),
			taskID,
			ent,
			source,
			cursor,
			seqStart,
			seqEnd,
			len(messages),
			payloadJSON,
			nowParam,
			nowParam,
		)
	} else {
		nextStatus := status
		if nextStatus != StatusSuccess && nextStatus != StatusRunning {
			nextStatus = StatusPending
		}
		_, err = repository.DB.ExecContext(ctx, repository.updateExistingSQL(),
			cursor,
			seqStart,
			seqEnd,
			len(messages),
			payloadJSON,
			nextStatus,
			nextStatus,
			nextStatus,
			nowParam,
			taskID,
		)
	}
	if err != nil {
		return Record{}, err
	}
	record, found, err := repository.getTask(ctx, taskID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, fmt.Errorf("archive ingest task upsert failed")
	}
	repository.notifyAfterEnqueue(ctx, record)
	return record, nil
}

func (repository *Repository) notifyAfterEnqueue(ctx context.Context, record Record) {
	if repository == nil || repository.AfterEnqueue == nil || strings.TrimSpace(record.TaskID) == "" {
		return
	}
	_ = repository.AfterEnqueue(ctx, record)
}

// ClaimTask marks a task running when it is currently claimable.
func (repository *Repository) ClaimTask(ctx context.Context, taskID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	record, found, err := repository.getTask(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	now := repository.now()
	if !repository.isClaimable(record, now) {
		return nil, nil
	}
	nowParam := repository.dbTimeParam(now)
	_, err = repository.DB.ExecContext(ctx, repository.claimTaskSQL(), record.AttemptCount+1, nowParam, nowParam, key)
	if err != nil {
		return nil, err
	}
	return repository.GetTask(ctx, key)
}

// ClaimNextScopeTask claims the oldest available task in one enterprise/source scope.
func (repository *Repository) ClaimNextScopeTask(ctx context.Context, enterpriseID string, source string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	ent := normalizeEnterpriseID(enterpriseID)
	src := normalizeSource(source)
	now := repository.now()
	nowParam := repository.dbTimeParam(now)
	staleBeforeParam := repository.dbTimeParam(now.Add(-time.Duration(repository.leaseSeconds()) * time.Second))
	rows, err := repository.DB.QueryContext(ctx, repository.claimableScopeSelectSQL(), ent, src, nowParam, staleBeforeParam)
	if err != nil {
		return nil, err
	}
	records, err := scanRecordRows(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	candidate := records[0]
	result, err := repository.DB.ExecContext(ctx, repository.claimNextScopeUpdateSQL(),
		candidate.AttemptCount+1,
		nowParam,
		nowParam,
		candidate.TaskID,
		nowParam,
		staleBeforeParam,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected <= 0 {
		return nil, nil
	}
	claimed, err := repository.GetTask(ctx, candidate.TaskID)
	if err != nil || claimed == nil {
		return claimed, err
	}
	if claimed.Status != StatusRunning {
		return nil, nil
	}
	return claimed, nil
}

// MarkSuccess records a completed staged ingest task.
func (repository *Repository) MarkSuccess(ctx context.Context, taskID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	now := repository.dbNowParam()
	if _, err := repository.DB.ExecContext(ctx, repository.markSuccessSQL(), now, now, key); err != nil {
		return nil, err
	}
	return repository.GetTask(ctx, key)
}

// MarkFailed records a failed ingest attempt and schedules the next retry.
func (repository *Repository) MarkFailed(ctx context.Context, taskID string, errText string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	existing, err := repository.GetTask(ctx, key)
	if err != nil || existing == nil {
		return nil, err
	}
	attemptCount := existing.AttemptCount
	if attemptCount < 1 {
		attemptCount = 1
	}
	retryAt := repository.computeRetryAt(attemptCount)
	if _, err := repository.DB.ExecContext(ctx, repository.markFailedSQL(),
		repository.dbTimeParam(retryAt),
		truncate(strings.TrimSpace(errText), 2000),
		repository.dbNowParam(),
		key,
	); err != nil {
		return nil, err
	}
	return repository.GetTask(ctx, key)
}

// CountPending counts claimable tasks in one enterprise/source scope.
func (repository *Repository) CountPending(ctx context.Context, enterpriseID string, source string) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive ingest task database is not configured")
	}
	now := repository.now()
	var total any
	err := repository.DB.QueryRowContext(ctx, repository.countPendingSQL(),
		normalizeEnterpriseID(enterpriseID),
		normalizeSource(source),
		repository.dbTimeParam(now),
		repository.dbTimeParam(now.Add(-time.Duration(repository.leaseSeconds())*time.Second)),
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(intValue(total)), nil
}

// ListPendingScopes lists enterprise/source scopes with claimable backlog.
func (repository *Repository) ListPendingScopes(ctx context.Context, limit int) ([]Scope, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	if limit < 1 {
		limit = 1
	}
	now := repository.now()
	rows, err := repository.DB.QueryContext(ctx, repository.listPendingScopesSQL(),
		repository.dbTimeParam(now),
		repository.dbTimeParam(now.Add(-time.Duration(repository.leaseSeconds())*time.Second)),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scopes := []Scope{}
	for rows.Next() {
		var enterpriseRaw any
		var sourceRaw any
		if err := rows.Scan(&enterpriseRaw, &sourceRaw); err != nil {
			return nil, err
		}
		scopes = append(scopes, Scope{
			EnterpriseID: normalizeEnterpriseID(textValue(enterpriseRaw)),
			Source:       normalizeSource(textValue(sourceRaw)),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
}

// LatestSeq returns MAX(seq_end) for one enterprise/source scope.
func (repository *Repository) LatestSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive ingest task database is not configured")
	}
	var maxSeq any
	err := repository.DB.QueryRowContext(ctx, repository.latestSeqSQL(), normalizeEnterpriseID(enterpriseID), normalizeSource(source)).Scan(&maxSeq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return intValue(maxSeq), nil
}

// PruneBefore deletes old successful tasks and preserves unfinished work.
func (repository *Repository) PruneBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive ingest task database is not configured")
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := repository.DB.QueryContext(ctx, repository.pruneSelectSQL(), repository.dbTimeParam(cutoff), limit)
	if err != nil {
		return 0, err
	}
	taskIDs, err := scanTaskIDs(rows)
	if err != nil {
		return 0, err
	}
	if len(taskIDs) == 0 {
		return 0, nil
	}
	args := stringsToAny(taskIDs)
	if _, err := repository.DB.ExecContext(ctx, "DELETE FROM archive_ingest_tasks WHERE task_id IN ("+placeholders(len(taskIDs))+")", args...); err != nil {
		return 0, err
	}
	return int64(len(taskIDs)), nil
}

func (repository *Repository) getStatus(ctx context.Context, taskID string) (string, bool, error) {
	var status any
	err := repository.DB.QueryRowContext(ctx, "SELECT status FROM archive_ingest_tasks WHERE task_id = ?", taskID).Scan(&status)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return normalizeStatus(textValue(status)), true, nil
}

func (repository *Repository) getTask(ctx context.Context, taskID string) (Record, bool, error) {
	row := repository.DB.QueryRowContext(ctx, "SELECT "+repository.recordColumnSQL("")+" FROM archive_ingest_tasks WHERE task_id = ?", taskID)
	record, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func (repository *Repository) insertSQL() string {
	return fmt.Sprintf(`
INSERT INTO archive_ingest_tasks (
    task_id, enterprise_id, source, %s, seq_start, seq_end, message_count,
    payload_json, status, attempt_count, next_retry_at, started_at, finished_at,
    last_error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, NULL, NULL, NULL, '', ?, ?)`, repository.cursorColumnSQL())
}

func (repository *Repository) updateExistingSQL() string {
	return fmt.Sprintf(`
UPDATE archive_ingest_tasks
SET %s = ?,
    seq_start = ?,
    seq_end = ?,
    message_count = ?,
    payload_json = ?,
    status = ?,
    next_retry_at = CASE WHEN ? = 'pending' THEN NULL ELSE next_retry_at END,
    last_error = CASE WHEN ? = 'pending' THEN '' ELSE last_error END,
    updated_at = ?
WHERE task_id = ?`, repository.cursorColumnSQL())
}

func (repository *Repository) claimTaskSQL() string {
	return `
UPDATE archive_ingest_tasks
SET status = 'running',
    attempt_count = ?,
    next_retry_at = NULL,
    started_at = ?,
    finished_at = NULL,
    last_error = '',
    updated_at = ?
WHERE task_id = ?`
}

func (repository *Repository) claimableScopeSelectSQL() string {
	return `
	SELECT ` + repository.recordColumnSQL("") + `
FROM archive_ingest_tasks
WHERE enterprise_id = ?
  AND source = ?
  AND (
    status = 'pending'
    OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?))
    OR (status = 'running' AND updated_at <= ?)
  )
ORDER BY seq_start ASC, created_at ASC
LIMIT 1`
}

func (repository *Repository) claimNextScopeUpdateSQL() string {
	return `
UPDATE archive_ingest_tasks
SET status = 'running',
    attempt_count = ?,
    next_retry_at = NULL,
    started_at = ?,
    finished_at = NULL,
    last_error = '',
    updated_at = ?
WHERE task_id = ?
  AND (
    status = 'pending'
    OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?))
    OR (status = 'running' AND updated_at <= ?)
  )`
}

func (repository *Repository) markSuccessSQL() string {
	return `
UPDATE archive_ingest_tasks
SET status = 'success',
    next_retry_at = NULL,
    finished_at = ?,
    last_error = '',
    updated_at = ?
WHERE task_id = ?`
}

func (repository *Repository) markFailedSQL() string {
	return `
UPDATE archive_ingest_tasks
SET status = 'failed',
    next_retry_at = ?,
    last_error = ?,
    updated_at = ?
WHERE task_id = ?`
}

func (repository *Repository) countPendingSQL() string {
	return `
SELECT COUNT(*) AS total
FROM archive_ingest_tasks
WHERE enterprise_id = ? AND source = ?
  AND (
    status = 'pending'
    OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?))
    OR (status = 'running' AND updated_at <= ?)
  )`
}

func (repository *Repository) listPendingScopesSQL() string {
	return `
SELECT enterprise_id, source
FROM archive_ingest_tasks
WHERE status = 'pending'
   OR (status = 'failed' AND (next_retry_at IS NULL OR next_retry_at <= ?))
   OR (status = 'running' AND updated_at <= ?)
GROUP BY enterprise_id, source
ORDER BY MIN(created_at) ASC
LIMIT ?`
}

func (repository *Repository) latestSeqSQL() string {
	return "SELECT MAX(seq_end) AS max_seq FROM archive_ingest_tasks WHERE enterprise_id = ? AND source = ?"
}

func (repository *Repository) pruneSelectSQL() string {
	return `
SELECT task_id
FROM archive_ingest_tasks
WHERE status = 'success'
  AND updated_at < ?
ORDER BY updated_at ASC, task_id ASC
LIMIT ?`
}

func (repository *Repository) cursorColumnSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return "`cursor`"
	}
	return `"cursor"`
}

func (repository *Repository) isClaimable(record Record, now time.Time) bool {
	switch record.Status {
	case StatusPending:
		return true
	case StatusFailed:
		return record.NextRetryAt == nil || !record.NextRetryAt.After(now)
	case StatusRunning:
		return !record.UpdatedAt.After(now.Add(-time.Duration(repository.leaseSeconds()) * time.Second))
	default:
		return false
	}
}

func (repository *Repository) computeRetryAt(attemptCount int) time.Time {
	attempt := attemptCount
	if attempt < 1 {
		attempt = 1
	}
	delay := float64(repository.retryBackoffBaseSeconds()) * math.Pow(2, float64(attempt-1))
	maxDelay := float64(repository.retryBackoffMaxSeconds())
	if delay > maxDelay {
		delay = maxDelay
	}
	return repository.now().Add(time.Duration(delay) * time.Second)
}

func (repository *Repository) retryBackoffBaseSeconds() int {
	if repository.RetryBackoffBaseSeconds > 0 {
		return repository.RetryBackoffBaseSeconds
	}
	return maxInt(1, envInt("ARCHIVE_INGEST_TASK_RETRY_BACKOFF_BASE_SEC", 5))
}

func (repository *Repository) retryBackoffMaxSeconds() int {
	if repository.RetryBackoffMaxSeconds > 0 {
		return repository.RetryBackoffMaxSeconds
	}
	return maxInt(5, envInt("ARCHIVE_INGEST_TASK_RETRY_BACKOFF_MAX_SEC", 300))
}

func (repository *Repository) leaseSeconds() int {
	if repository.LeaseSeconds > 0 {
		return repository.LeaseSeconds
	}
	return maxInt(30, envInt("ARCHIVE_INGEST_TASK_LEASE_SECONDS", 300))
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05-07:00")
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

type taskIDInput struct {
	EnterpriseID      string
	Source            string
	Cursor            string
	SeqStart          int64
	SeqEnd            int64
	MessageCount      int
	FirstArchiveMsgID string
	LastArchiveMsgID  string
}

func buildTaskID(input taskIDInput) string {
	seed := strings.Join([]string{
		input.EnterpriseID,
		input.Source,
		input.Cursor,
		strconv.FormatInt(input.SeqStart, 10),
		strconv.FormatInt(input.SeqEnd, 10),
		strconv.Itoa(input.MessageCount),
		input.FirstArchiveMsgID,
		input.LastArchiveMsgID,
	}, "|")
	sum := sha1.Sum([]byte(seed))
	return "ait-" + hex.EncodeToString(sum[:])
}

func normalizeMessages(messages []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if len(message) == 0 {
			continue
		}
		clone := make(map[string]any, len(message))
		for key, value := range message {
			clone[key] = value
		}
		normalized = append(normalized, clone)
	}
	return normalized
}

func normalizePayloadJSON(messages []map[string]any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(messages); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func messageSeqRange(messages []map[string]any) (int64, int64) {
	var start int64
	var end int64
	for index, message := range messages {
		seq := intValue(message["seq"])
		if seq < 0 {
			seq = 0
		}
		if index == 0 || seq < start {
			start = seq
		}
		if index == 0 || seq > end {
			end = seq
		}
	}
	return start, end
}

func messageArchiveMsgIDs(messages []map[string]any) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, strings.TrimSpace(textValue(message["archive_msgid"])))
	}
	return ids
}

func (repository *Repository) recordColumnSQL(prefix string) string {
	columns := []string{
		"task_id",
		"enterprise_id",
		"source",
		"cursor",
		"seq_start",
		"seq_end",
		"message_count",
		"payload_json",
		"status",
		"attempt_count",
		"next_retry_at",
		"started_at",
		"finished_at",
		"last_error",
		"created_at",
		"updated_at",
	}
	qualified := make([]string, 0, len(columns))
	for _, column := range columns {
		if column == "cursor" {
			qualified = append(qualified, prefix+repository.cursorColumnSQL())
			continue
		}
		qualified = append(qualified, prefix+column)
	}
	return strings.Join(qualified, ", ")
}

func scanRecordRows(rows RowsScanner) ([]Record, error) {
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func scanRecord(row RowScanner) (Record, error) {
	var taskID any
	var enterpriseID any
	var source any
	var cursor any
	var seqStart any
	var seqEnd any
	var messageCount any
	var payloadJSON any
	var status any
	var attemptCount any
	var nextRetryAt any
	var startedAt any
	var finishedAt any
	var lastError any
	var createdAt any
	var updatedAt any
	if err := row.Scan(
		&taskID,
		&enterpriseID,
		&source,
		&cursor,
		&seqStart,
		&seqEnd,
		&messageCount,
		&payloadJSON,
		&status,
		&attemptCount,
		&nextRetryAt,
		&startedAt,
		&finishedAt,
		&lastError,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Record{}, err
	}
	return Record{
		TaskID:       strings.TrimSpace(textValue(taskID)),
		EnterpriseID: normalizeEnterpriseID(textValue(enterpriseID)),
		Source:       normalizeSource(textValue(source)),
		Cursor:       strings.TrimSpace(textValue(cursor)),
		SeqStart:     intValue(seqStart),
		SeqEnd:       intValue(seqEnd),
		MessageCount: int(intValue(messageCount)),
		PayloadJSON:  textValue(payloadJSON),
		Status:       normalizeStatus(textValue(status)),
		AttemptCount: int(intValue(attemptCount)),
		NextRetryAt:  nullableDBTime(nextRetryAt),
		StartedAt:    nullableDBTime(startedAt),
		FinishedAt:   nullableDBTime(finishedAt),
		LastError:    strings.TrimSpace(textValue(lastError)),
		CreatedAt:    parseDBTime(createdAt),
		UpdatedAt:    parseDBTime(updatedAt),
	}, nil
}

func scanTaskIDs(rows RowsScanner) ([]string, error) {
	defer rows.Close()
	taskIDs := []string{}
	for rows.Next() {
		var taskID any
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		if key := strings.TrimSpace(textValue(taskID)); key != "" {
			taskIDs = append(taskIDs, key)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func nullableDBTime(value any) *time.Time {
	if value == nil {
		return nil
	}
	if typed, ok := value.(sql.NullTime); ok {
		if !typed.Valid {
			return nil
		}
		parsed := parseDBTime(typed.Time)
		return &parsed
	}
	parsed := parseDBTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func parseDBTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseDBTimeString(string(typed))
	case string:
		return parseDBTimeString(typed)
	default:
		return time.Time{}
	}
}

func parseDBTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func normalizeEnterpriseID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func normalizeSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "self_decrypt"
	}
	return value
}

func normalizeStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return StatusPending
	}
	return value
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func lastString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func truncate(value string, limit int) string {
	if limit < 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func placeholders(count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ", ")
}

func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case sql.NullString:
		if !typed.Valid {
			return ""
		}
		return typed.String
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		value, _ := typed.Int64()
		return value
	case []byte:
		return parseIntString(string(typed))
	case string:
		return parseIntString(typed)
	default:
		return 0
	}
}

func parseIntString(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive ingest task database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("archive ingest task database is not configured")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
