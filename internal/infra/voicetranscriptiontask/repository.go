// Package voicetranscriptiontask adapts voice_transcription_tasks enqueue.
package voicetranscriptiontask

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/voicetranscription"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by worker operations.
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

// VoiceTranscriptionTx is the transaction shape needed by durable claim operations.
type VoiceTranscriptionTx interface {
	Queryer
	Commit() error
	Rollback() error
}

// Transactioner starts voice transcription task transactions.
type Transactioner interface {
	BeginVoiceTranscriptionTx(ctx context.Context) (VoiceTranscriptionTx, error)
}

// AfterEnqueueFunc is called after a voice transcription task is durably upserted.
type AfterEnqueueFunc func(ctx context.Context, result EnqueueResult) error

// EnqueueResult carries the stable task id and creation flag.
type EnqueueResult struct {
	Created        bool
	TaskID         string
	EnterpriseID   string
	ConversationID string
	ArchiveMsgID   string
	MediaTaskID    string
	ObjectURL      string
}

// Repository persists voice transcription enqueue requests.
type Repository struct {
	DB           Queryer
	Tx           Transactioner
	Dialect      string
	Now          func() time.Time
	NewTaskID    func() string
	AfterEnqueue AfterEnqueueFunc
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	if db == nil {
		return &Repository{Dialect: dialect}
	}
	queryer := sqlQueryer{db: db}
	return &Repository{DB: queryer, Tx: queryer, Dialect: dialect}
}

// EnqueueVoiceTranscription upserts one pending task while preserving successful results.
func (repository *Repository) EnqueueVoiceTranscription(ctx context.Context, input archivemedia.VoiceTranscriptionInput) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("voice transcription task database is not configured")
	}
	normalized, err := normalizeInput(input)
	if err != nil {
		return false, err
	}
	now := repository.now()
	identity := BuildTaskIdentity(normalized.EnterpriseID, normalized.ArchiveMsgID)
	taskID, createdAt, existing, err := repository.findExisting(ctx, identity)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(taskID) == "" {
		taskID = repository.newTaskID()
	}
	if createdAt.IsZero() {
		createdAt = now
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		taskID,
		normalized.EnterpriseID,
		normalized.ConversationID,
		normalized.ArchiveMsgID,
		normalized.MediaTaskID,
		normalized.ObjectURL,
		identity,
		createdAt.UTC(),
		now.UTC(),
	)
	if err != nil {
		return false, err
	}
	repository.notifyAfterEnqueue(ctx, EnqueueResult{
		Created:        !existing,
		TaskID:         taskID,
		EnterpriseID:   normalized.EnterpriseID,
		ConversationID: normalized.ConversationID,
		ArchiveMsgID:   normalized.ArchiveMsgID,
		MediaTaskID:    normalized.MediaTaskID,
		ObjectURL:      normalized.ObjectURL,
	})
	return !existing, nil
}

func (repository *Repository) notifyAfterEnqueue(ctx context.Context, result EnqueueResult) {
	if repository == nil || repository.AfterEnqueue == nil || strings.TrimSpace(result.TaskID) == "" {
		return
	}
	_ = repository.AfterEnqueue(ctx, result)
}

// RequeueRetryable moves due failed_retryable tasks back to pending.
func (repository *Repository) RequeueRetryable(ctx context.Context, options voicetranscription.RequeueOptions) (int, error) {
	if repository.Tx == nil {
		return 0, fmt.Errorf("voice transcription task transactioner is not configured")
	}
	ent := defaultText(options.EnterpriseID, "default")
	limit := claimLimit(options.Limit)
	readyBefore := options.ReadyBefore
	if readyBefore.IsZero() {
		readyBefore = repository.now()
	}
	tx, err := repository.Tx.BeginVoiceTranscriptionTx(ctx)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	taskIDs, err := repository.selectRetryableTaskIDs(ctx, tx, ent, limit, readyBefore, options.MaxAttempts)
	if err != nil {
		return 0, err
	}
	if len(taskIDs) == 0 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		committed = true
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE voice_transcription_tasks
SET status = 'pending',
    next_retry_at = NULL,
    updated_at = ?
WHERE task_id IN (`+placeholders(len(taskIDs))+`)`, append([]any{repository.dbNowParam()}, stringsToAny(taskIDs)...)...); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return len(taskIDs), nil
}

// ClaimPending claims pending or stale running tasks for a worker pass.
func (repository *Repository) ClaimPending(ctx context.Context, options voicetranscription.ClaimOptions) ([]voicetranscription.Task, error) {
	if repository.Tx == nil {
		return nil, fmt.Errorf("voice transcription task transactioner is not configured")
	}
	ent := defaultText(options.EnterpriseID, "default")
	limit := claimLimit(options.Limit)
	leaseSeconds := options.ProcessingLeaseSeconds
	if leaseSeconds < 30 {
		leaseSeconds = voicetranscription.DefaultProcessingLeaseSeconds
	}
	now := repository.now()
	staleBefore := now.Add(-time.Duration(leaseSeconds) * time.Second)
	tx, err := repository.Tx.BeginVoiceTranscriptionTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var tasks []voicetranscription.Task
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		tasks, err = repository.claimPendingPostgres(ctx, tx, ent, limit, now, staleBefore)
	} else {
		tasks, err = repository.claimPendingMySQL(ctx, tx, ent, limit, now, staleBefore)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return tasks, nil
}

// UpdateTask updates a task state and returns the refreshed row.
func (repository *Repository) UpdateTask(ctx context.Context, input voicetranscription.UpdateInput) (*voicetranscription.Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("voice transcription task database is not configured")
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("voice transcription task_id is required")
	}
	_, err := repository.DB.ExecContext(ctx, `
UPDATE voice_transcription_tasks
SET status = ?,
    input_url = ?,
    transcript_text = ?,
    coze_execute_id = ?,
    coze_logid = ?,
    raw_response_json = ?,
    last_error = ?,
    retry_count = ?,
    next_retry_at = ?,
    updated_at = ?
WHERE task_id = ?`,
		defaultText(input.Status, voicetranscription.StatusPending),
		strings.TrimSpace(input.InputURL),
		input.TranscriptText,
		strings.TrimSpace(input.CozeExecuteID),
		strings.TrimSpace(input.CozeLogID),
		input.RawResponseJSON,
		strings.TrimSpace(input.LastError),
		maxInt(0, input.RetryCount),
		repository.dbNullableTimeParam(input.NextRetryAt),
		repository.dbNowParam(),
		taskID,
	)
	if err != nil {
		return nil, err
	}
	return repository.GetTask(ctx, taskID)
}

// GetTask returns one voice transcription task by task_id.
func (repository *Repository) GetTask(ctx context.Context, taskID string) (*voicetranscription.Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("voice transcription task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	task, err := scanTask(repository.DB.QueryRowContext(ctx, "SELECT "+recordColumnsSQL("")+" FROM voice_transcription_tasks WHERE task_id = ? LIMIT 1", key))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ListByArchiveMsgIDs returns transcription tasks for archive messages, newest first.
func (repository *Repository) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]voicetranscription.Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("voice transcription task database is not configured")
	}
	ids := normalizeArchiveMsgIDs(archiveMsgIDs)
	if len(ids) == 0 {
		return []voicetranscription.Task{}, nil
	}
	args := stringsToAny(ids)
	query := "SELECT " + recordColumnsSQL("") + " FROM voice_transcription_tasks WHERE archive_msgid IN (" + placeholders(len(ids)) + ")"
	if ent := strings.TrimSpace(enterpriseID); ent != "" {
		query += " AND enterprise_id = ?"
		args = append(args, ent)
	}
	query += " ORDER BY updated_at DESC, created_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// BuildTaskIdentity mirrors Python _build_task_identity(ent, archive_msgid).
func BuildTaskIdentity(enterpriseID string, archiveMsgID string) string {
	raw := defaultText(enterpriseID, "default") + "|" + strings.TrimSpace(archiveMsgID)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type normalizedInput struct {
	EnterpriseID   string
	ConversationID string
	ArchiveMsgID   string
	MediaTaskID    string
	ObjectURL      string
}

func normalizeInput(input archivemedia.VoiceTranscriptionInput) (normalizedInput, error) {
	normalized := normalizedInput{
		EnterpriseID:   defaultText(input.EnterpriseID, "default"),
		ConversationID: strings.TrimSpace(input.ConversationID),
		ArchiveMsgID:   strings.TrimSpace(input.ArchiveMsgID),
		MediaTaskID:    strings.TrimSpace(input.MediaTaskID),
		ObjectURL:      strings.TrimSpace(input.ObjectURL),
	}
	if normalized.ConversationID == "" || normalized.ArchiveMsgID == "" || normalized.MediaTaskID == "" || normalized.ObjectURL == "" {
		return normalizedInput{}, fmt.Errorf("conversation_id, archive_msgid, media_task_id and object_url are required")
	}
	return normalized, nil
}

func normalizeArchiveMsgIDs(values []string) []string {
	seen := map[string]struct{}{}
	output := []string{}
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		output = append(output, text)
	}
	return output
}

func (repository *Repository) findExisting(ctx context.Context, identity string) (string, time.Time, bool, error) {
	var taskID sql.NullString
	var createdAt sql.NullTime
	err := repository.DB.QueryRowContext(ctx, `
SELECT task_id, created_at
FROM voice_transcription_tasks
WHERE task_identity = ?
LIMIT 1`, identity).Scan(&taskID, &createdAt)
	if err == sql.ErrNoRows {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	return strings.TrimSpace(taskID.String), createdAt.Time, true, nil
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return postgresUpsertSQL
	}
	return mysqlUpsertSQL
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func (repository *Repository) newTaskID() string {
	if repository.NewTaskID != nil {
		if taskID := strings.TrimSpace(repository.NewTaskID()); taskID != "" {
			return taskID
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "vtt-" + hex.EncodeToString([]byte(fmt.Sprint(repository.now().UnixNano())))
	}
	return "vtt-" + hex.EncodeToString(random[:])
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return strings.TrimSpace(fallback)
}

func (repository *Repository) selectRetryableTaskIDs(ctx context.Context, queryer Queryer, enterpriseID string, limit int, readyBefore time.Time, maxAttempts *int) ([]string, error) {
	args := []any{enterpriseID, repository.dbTimeParam(readyBefore)}
	retryFilter := ""
	if maxAttempts != nil {
		retryFilter = " AND retry_count < ?"
		args = append(args, maxInt(0, *maxAttempts))
	}
	query := `
SELECT task_id
FROM voice_transcription_tasks
WHERE enterprise_id = ?
  AND status = 'failed_retryable'
  AND (next_retry_at IS NULL OR next_retry_at <= ?)` + retryFilter + `
ORDER BY updated_at ASC
LIMIT ?`
	args = append(args, limit)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		query += " FOR UPDATE SKIP LOCKED"
	} else {
		query += " FOR UPDATE SKIP LOCKED"
	}
	return selectTaskIDs(ctx, queryer, query, args...)
}

func (repository *Repository) claimPendingMySQL(ctx context.Context, tx VoiceTranscriptionTx, enterpriseID string, limit int, now time.Time, staleBefore time.Time) ([]voicetranscription.Task, error) {
	taskIDs, err := selectTaskIDs(ctx, tx, `
SELECT task_id
FROM voice_transcription_tasks
WHERE enterprise_id = ? AND status = 'pending'
ORDER BY created_at DESC, updated_at DESC
LIMIT ?
FOR UPDATE SKIP LOCKED`, enterpriseID, limit)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) < limit {
		staleIDs, err := selectTaskIDs(ctx, tx, `
SELECT task_id
FROM voice_transcription_tasks
WHERE enterprise_id = ? AND status = 'running' AND updated_at <= ?
ORDER BY created_at DESC, updated_at DESC
LIMIT ?
FOR UPDATE SKIP LOCKED`, enterpriseID, repository.dbTimeParam(staleBefore), limit-len(taskIDs))
		if err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, staleIDs...)
	}
	if len(taskIDs) == 0 {
		return []voicetranscription.Task{}, nil
	}
	if err := repository.markTaskIDsRunning(ctx, tx, taskIDs, now); err != nil {
		return nil, err
	}
	return repository.loadTasksByTaskIDs(ctx, tx, taskIDs)
}

func (repository *Repository) claimPendingPostgres(ctx context.Context, tx VoiceTranscriptionTx, enterpriseID string, limit int, now time.Time, staleBefore time.Time) ([]voicetranscription.Task, error) {
	rows, err := tx.QueryContext(ctx, `
WITH claimable AS (
    SELECT task_id
    FROM voice_transcription_tasks
    WHERE enterprise_id = ?
      AND (status = 'pending' OR (status = 'running' AND updated_at <= ?))
    ORDER BY
      CASE WHEN status = 'pending' THEN 0 ELSE 1 END ASC,
      created_at DESC,
      updated_at DESC
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
UPDATE voice_transcription_tasks AS target
SET status = 'running',
    last_error = '',
    next_retry_at = NULL,
    updated_at = ?
FROM claimable
WHERE target.task_id = claimable.task_id
RETURNING `+recordColumnsSQL("target"),
		enterpriseID,
		repository.dbTimeParam(staleBefore),
		limit,
		repository.dbTimeParam(now),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (repository *Repository) markTaskIDsRunning(ctx context.Context, queryer Queryer, taskIDs []string, now time.Time) error {
	if len(taskIDs) == 0 {
		return nil
	}
	_, err := queryer.ExecContext(ctx, `
UPDATE voice_transcription_tasks
SET status = 'running',
    last_error = '',
    next_retry_at = NULL,
    updated_at = ?
WHERE task_id IN (`+placeholders(len(taskIDs))+`)`, append([]any{repository.dbTimeParam(now)}, stringsToAny(taskIDs)...)...)
	return err
}

func (repository *Repository) loadTasksByTaskIDs(ctx context.Context, queryer Queryer, taskIDs []string) ([]voicetranscription.Task, error) {
	if len(taskIDs) == 0 {
		return []voicetranscription.Task{}, nil
	}
	rows, err := queryer.QueryContext(ctx, "SELECT "+recordColumnsSQL("")+" FROM voice_transcription_tasks WHERE task_id IN ("+placeholders(len(taskIDs))+") ORDER BY created_at DESC, updated_at DESC", stringsToAny(taskIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func selectTaskIDs(ctx context.Context, queryer Queryer, query string, args ...any) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taskIDs := []string{}
	for rows.Next() {
		var taskID any
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		if text := strings.TrimSpace(textValue(taskID)); text != "" {
			taskIDs = append(taskIDs, text)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func scanTasks(rows RowsScanner) ([]voicetranscription.Task, error) {
	tasks := []voicetranscription.Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (voicetranscription.Task, error) {
	var taskID any
	var enterpriseID any
	var conversationID any
	var archiveMsgID any
	var mediaTaskID any
	var objectURL any
	var inputURL any
	var status any
	var transcriptText any
	var cozeExecuteID any
	var cozeLogID any
	var rawResponseJSON any
	var lastError any
	var retryCount any
	var nextRetryAt any
	var createdAt any
	var updatedAt any
	if err := row.Scan(
		&taskID,
		&enterpriseID,
		&conversationID,
		&archiveMsgID,
		&mediaTaskID,
		&objectURL,
		&inputURL,
		&status,
		&transcriptText,
		&cozeExecuteID,
		&cozeLogID,
		&rawResponseJSON,
		&lastError,
		&retryCount,
		&nextRetryAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return voicetranscription.Task{}, err
	}
	nextRetry := parseDBTime(nextRetryAt)
	var nextRetryPtr *time.Time
	if !nextRetry.IsZero() {
		nextRetryPtr = &nextRetry
	}
	return voicetranscription.Task{
		TaskID:          strings.TrimSpace(textValue(taskID)),
		EnterpriseID:    strings.TrimSpace(textValue(enterpriseID)),
		ConversationID:  strings.TrimSpace(textValue(conversationID)),
		ArchiveMsgID:    strings.TrimSpace(textValue(archiveMsgID)),
		MediaTaskID:     strings.TrimSpace(textValue(mediaTaskID)),
		ObjectURL:       strings.TrimSpace(textValue(objectURL)),
		InputURL:        strings.TrimSpace(textValue(inputURL)),
		Status:          strings.TrimSpace(textValue(status)),
		TranscriptText:  textValue(transcriptText),
		CozeExecuteID:   strings.TrimSpace(textValue(cozeExecuteID)),
		CozeLogID:       strings.TrimSpace(textValue(cozeLogID)),
		RawResponseJSON: textValue(rawResponseJSON),
		LastError:       strings.TrimSpace(textValue(lastError)),
		RetryCount:      intValue(retryCount),
		NextRetryAt:     nextRetryPtr,
		CreatedAt:       parseDBTime(createdAt),
		UpdatedAt:       parseDBTime(updatedAt),
	}, nil
}

func recordColumnsSQL(prefix string) string {
	columns := []string{
		"task_id",
		"enterprise_id",
		"conversation_id",
		"archive_msgid",
		"media_task_id",
		"object_url",
		"input_url",
		"status",
		"transcript_text",
		"coze_execute_id",
		"coze_logid",
		"raw_response_json",
		"last_error",
		"retry_count",
		"next_retry_at",
		"created_at",
		"updated_at",
	}
	if strings.TrimSpace(prefix) == "" {
		return strings.Join(columns, ", ")
	}
	prefixed := make([]string, 0, len(columns))
	for _, column := range columns {
		prefixed = append(prefixed, strings.TrimSpace(prefix)+"."+column)
	}
	return strings.Join(prefixed, ", ")
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ", ")
}

func stringsToAny(values []string) []any {
	output := make([]any, 0, len(values))
	for _, value := range values {
		output = append(output, value)
	}
	return output
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbNullableTimeParam(value *time.Time) any {
	if value == nil {
		return nil
	}
	return repository.dbTimeParam(*value)
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05-07:00")
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

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func claimLimit(value int) int {
	if value < 1 {
		return voicetranscription.DefaultClaimLimit
	}
	if value > voicetranscription.DefaultClaimLimit {
		return voicetranscription.DefaultClaimLimit
	}
	return value
}

func maxInt(minimum int, value int) int {
	if value < minimum {
		return minimum
	}
	return value
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) BeginVoiceTranscriptionTx(ctx context.Context) (VoiceTranscriptionTx, error) {
	tx, err := queryer.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlVoiceTranscriptionTx{tx: tx}, nil
}

type sqlVoiceTranscriptionTx struct {
	tx *sql.Tx
}

func (tx sqlVoiceTranscriptionTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, query, args...)
}

func (tx sqlVoiceTranscriptionTx) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	return tx.tx.QueryContext(ctx, query, args...)
}

func (tx sqlVoiceTranscriptionTx) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return tx.tx.QueryRowContext(ctx, query, args...)
}

func (tx sqlVoiceTranscriptionTx) Commit() error {
	return tx.tx.Commit()
}

func (tx sqlVoiceTranscriptionTx) Rollback() error {
	return tx.tx.Rollback()
}

const mysqlUpsertSQL = `
INSERT INTO voice_transcription_tasks (
    task_id, enterprise_id, conversation_id, archive_msgid, media_task_id,
    object_url, input_url, task_identity, status, transcript_text,
    coze_execute_id, coze_logid, raw_response_json, last_error,
    retry_count, next_retry_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, '', ?, 'pending', '', '', '', '', '', 0, NULL, ?, ?)
ON DUPLICATE KEY UPDATE
    conversation_id = VALUES(conversation_id),
    media_task_id = VALUES(media_task_id),
    object_url = VALUES(object_url),
    input_url = '',
    status = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.status
        ELSE 'pending'
    END,
    transcript_text = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.transcript_text
        ELSE ''
    END,
    coze_execute_id = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.coze_execute_id
        ELSE ''
    END,
    coze_logid = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.coze_logid
        ELSE ''
    END,
    raw_response_json = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.raw_response_json
        ELSE ''
    END,
    last_error = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.last_error
        ELSE ''
    END,
    retry_count = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.retry_count
        ELSE 0
    END,
    next_retry_at = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.next_retry_at
        ELSE NULL
    END,
    updated_at = VALUES(updated_at)`

const postgresUpsertSQL = `
INSERT INTO voice_transcription_tasks (
    task_id, enterprise_id, conversation_id, archive_msgid, media_task_id,
    object_url, input_url, task_identity, status, transcript_text,
    coze_execute_id, coze_logid, raw_response_json, last_error,
    retry_count, next_retry_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, '', ?, 'pending', '', '', '', '', '', 0, NULL, ?, ?)
ON CONFLICT(task_identity) DO UPDATE SET
    conversation_id = excluded.conversation_id,
    media_task_id = excluded.media_task_id,
    object_url = excluded.object_url,
    input_url = '',
    status = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.status
        ELSE 'pending'
    END,
    transcript_text = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.transcript_text
        ELSE ''
    END,
    coze_execute_id = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.coze_execute_id
        ELSE ''
    END,
    coze_logid = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.coze_logid
        ELSE ''
    END,
    raw_response_json = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.raw_response_json
        ELSE ''
    END,
    last_error = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.last_error
        ELSE ''
    END,
    retry_count = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.retry_count
        ELSE 0
    END,
    next_retry_at = CASE
        WHEN voice_transcription_tasks.status = 'success' THEN voice_transcription_tasks.next_retry_at
        ELSE NULL
    END,
    updated_at = excluded.updated_at`
