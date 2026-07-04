// Package archivemediatask adapts the legacy archive_media_tasks table enqueue path.
package archivemediatask

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"

	StatusPending         = "pending"
	StatusRunning         = "running"
	StatusSuccess         = "success"
	StatusFailed          = "failed"
	StatusFailedRetryable = "failed_retryable"
	StatusFailedTerminal  = "failed_terminal"

	DefaultProcessingLeaseSeconds = 300
	MinProcessingLeaseSeconds     = 30
	MaxClaimLimit                 = 200
	DefaultListLimit              = 100
	MaxListLimit                  = 1000
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

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
}

// ArchiveMediaTx is the transaction shape needed by durable claim operations.
type ArchiveMediaTx interface {
	Queryer
	Commit() error
	Rollback() error
}

// Transactioner starts archive media task transactions.
type Transactioner interface {
	BeginArchiveMediaTx(ctx context.Context) (ArchiveMediaTx, error)
}

// AfterEnqueueFunc is called after archive media task rows are durably upserted.
type AfterEnqueueFunc func(ctx context.Context, results []EnqueueResult) error

// Record mirrors the enqueue-visible fields of Python ArchiveMediaTaskRecord.
type Record struct {
	TaskID          string
	EnterpriseID    string
	Source          string
	ArchiveMsgID    string
	SDKFileID       string
	TaskIdentity    string
	IndexBuf        string
	OutIndexBuf     string
	IsFinish        bool
	Status          string
	PayloadJSON     string
	LocalFilePath   string
	DownloadedBytes int64
	ObjectURL       string
	StorageBackend  string
	LastError       string
	RetryCount      int
	NextRetryAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EnqueueInput describes one media download task.
type EnqueueInput struct {
	EnterpriseID string
	Source       string
	ArchiveMsgID string
	SDKFileID    string
	PayloadJSON  string
}

type normalizedInput struct {
	EnqueueInput
	TaskIdentity string
}

// EnqueueResult carries the stable task id and creation flag.
type EnqueueResult struct {
	Created bool
	Record  Record
}

// ClaimOptions controls one archive media claim pass.
type ClaimOptions struct {
	EnterpriseID           string
	Source                 string
	Limit                  int
	ProcessingLeaseSeconds int
}

// UpdateInput describes a media task progress update.
type UpdateInput struct {
	TaskID          string
	Status          string
	IndexBuf        string
	OutIndexBuf     string
	IsFinish        bool
	PayloadJSON     string
	LocalFilePath   string
	DownloadedBytes int64
	ObjectURL       string
	StorageBackend  string
	LastError       string
	RetryCount      int
	NextRetryAt     *time.Time
}

// RequeueOptions controls failed_retryable reset.
type RequeueOptions struct {
	EnterpriseID string
	Source       string
	Limit        int
	ReadyBefore  time.Time
	MaxAttempts  *int
}

// ListOptions controls archive media task list queries.
type ListOptions struct {
	EnterpriseID string
	Source       string
	Status       string
	Limit        int
}

// Repository persists archive media task enqueue records.
type Repository struct {
	DB           Queryer
	Tx           Transactioner
	Dialect      string
	Now          func() time.Time
	NextTaskID   func() string
	AfterEnqueue AfterEnqueueFunc
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	queryer := sqlQueryer{db: db}
	return &Repository{DB: queryer, Tx: queryer, Dialect: dialect}
}

// Enqueue inserts or refreshes one media task.
func (repository *Repository) Enqueue(ctx context.Context, input EnqueueInput) (EnqueueResult, error) {
	results, err := repository.EnqueueMany(ctx, []EnqueueInput{input})
	if err != nil {
		return EnqueueResult{}, err
	}
	if len(results) == 0 {
		return EnqueueResult{}, fmt.Errorf("archive media enqueue failed")
	}
	return results[0], nil
}

// EnqueueMany inserts or refreshes media tasks with Python enqueue_many semantics.
func (repository *Repository) EnqueueMany(ctx context.Context, inputs []EnqueueInput) ([]EnqueueResult, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	items, err := normalizeInputs(inputs)
	if err != nil || len(items) == 0 {
		return []EnqueueResult{}, err
	}
	identities := make([]string, 0, len(items))
	for _, item := range items {
		identities = append(identities, item.TaskIdentity)
	}
	existing, err := repository.existingByIdentity(ctx, identities)
	if err != nil {
		return nil, err
	}
	now := repository.dbNowParam()
	updatedAt := repository.now()
	results := make([]EnqueueResult, 0, len(items))
	for _, item := range items {
		known := existing[item.TaskIdentity]
		taskID := strings.TrimSpace(known.TaskID)
		created := taskID == ""
		if taskID == "" {
			taskID = repository.nextTaskID()
		}
		createdAtParam := now
		createdAt := updatedAt
		if !known.CreatedAt.IsZero() {
			createdAt = known.CreatedAt
			createdAtParam = repository.dbTimeParam(known.CreatedAt)
		}
		if _, err := repository.DB.ExecContext(ctx, repository.upsertSQL(),
			taskID,
			item.EnterpriseID,
			item.Source,
			item.ArchiveMsgID,
			item.SDKFileID,
			item.TaskIdentity,
			item.PayloadJSON,
			createdAtParam,
			now,
		); err != nil {
			return nil, err
		}
		results = append(results, EnqueueResult{
			Created: created,
			Record: Record{
				TaskID:       taskID,
				EnterpriseID: item.EnterpriseID,
				Source:       item.Source,
				ArchiveMsgID: item.ArchiveMsgID,
				SDKFileID:    item.SDKFileID,
				TaskIdentity: item.TaskIdentity,
				Status:       StatusPending,
				PayloadJSON:  item.PayloadJSON,
				CreatedAt:    createdAt,
				UpdatedAt:    updatedAt,
			},
		})
	}
	repository.notifyAfterEnqueue(ctx, results)
	return results, nil
}

func (repository *Repository) notifyAfterEnqueue(ctx context.Context, results []EnqueueResult) {
	if repository == nil || repository.AfterEnqueue == nil || len(results) == 0 {
		return
	}
	_ = repository.AfterEnqueue(ctx, append([]EnqueueResult(nil), results...))
}

// ClaimPending leases pending tasks first, then stale running tasks.
func (repository *Repository) ClaimPending(ctx context.Context, options ClaimOptions) ([]Record, error) {
	if repository.Tx == nil {
		return nil, fmt.Errorf("archive media task transaction database is not configured")
	}
	ent := normalizeEnterpriseID(options.EnterpriseID)
	src := normalizeSource(options.Source)
	limit := normalizeLimit(options.Limit)
	leaseSeconds := normalizeLeaseSeconds(options.ProcessingLeaseSeconds)
	now := repository.now()
	staleBefore := now.Add(-time.Duration(leaseSeconds) * time.Second)
	tx, err := repository.Tx.BeginArchiveMediaTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var records []Record
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		records, err = repository.claimPendingPostgres(ctx, tx, ent, src, limit, now, staleBefore)
	} else {
		records, err = repository.claimPendingMySQL(ctx, tx, ent, src, limit, now, staleBefore)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return records, nil
}

// UpdateProgress stores one media task progress or terminal state.
func (repository *Repository) UpdateProgress(ctx context.Context, input UpdateInput) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, nil
	}
	_, err := repository.DB.ExecContext(ctx, `
UPDATE archive_media_tasks
SET status = ?,
    index_buf = ?,
    out_index_buf = ?,
    is_finish = ?,
    payload_json = ?,
    local_file_path = ?,
    downloaded_bytes = ?,
    object_url = ?,
    storage_backend = ?,
    last_error = ?,
    retry_count = ?,
    next_retry_at = ?,
    updated_at = ?
WHERE task_id = ?`,
		defaultText(input.Status, StatusPending),
		strings.TrimSpace(input.IndexBuf),
		strings.TrimSpace(input.OutIndexBuf),
		boolInt(input.IsFinish),
		input.PayloadJSON,
		strings.TrimSpace(input.LocalFilePath),
		maxInt64(0, input.DownloadedBytes),
		strings.TrimSpace(input.ObjectURL),
		defaultText(input.StorageBackend, "local"),
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

// ReleaseClaimed resets unfinished running tasks that this worker did not execute.
func (repository *Repository) ReleaseClaimed(ctx context.Context, taskIDs []string) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive media task database is not configured")
	}
	normalized := normalizeStrings(taskIDs)
	if len(normalized) == 0 {
		return 0, nil
	}
	args := []any{repository.dbNowParam()}
	for _, taskID := range normalized {
		args = append(args, taskID)
	}
	result, err := repository.DB.ExecContext(ctx, `
UPDATE archive_media_tasks
SET status = 'pending',
    last_error = '',
    updated_at = ?
WHERE status = 'running'
  AND is_finish = 0
  AND task_id IN (`+placeholders(len(normalized))+`)`, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return affected, nil
}

// RequeueRetryable resets due failed_retryable tasks to pending.
func (repository *Repository) RequeueRetryable(ctx context.Context, options RequeueOptions) (int, error) {
	if repository.Tx == nil {
		return 0, fmt.Errorf("archive media task transaction database is not configured")
	}
	ent := normalizeEnterpriseID(options.EnterpriseID)
	src := normalizeSource(options.Source)
	limit := normalizeLimit(options.Limit)
	readyBefore := options.ReadyBefore
	if readyBefore.IsZero() {
		readyBefore = repository.now()
	}
	tx, err := repository.Tx.BeginArchiveMediaTx(ctx)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	taskIDs, err := repository.selectRetryableTaskIDs(ctx, tx, ent, src, readyBefore, options.MaxAttempts, limit)
	if err != nil {
		return 0, err
	}
	if len(taskIDs) > 0 {
		args := []any{repository.dbNowParam()}
		for _, taskID := range taskIDs {
			args = append(args, taskID)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE archive_media_tasks
SET status = 'pending',
    next_retry_at = NULL,
    updated_at = ?
WHERE task_id IN (`+placeholders(len(taskIDs))+`)`, args...); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return len(taskIDs), nil
}

// GetTask loads one media task by task_id.
func (repository *Repository) GetTask(ctx context.Context, taskID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	rows, err := repository.DB.QueryContext(ctx, selectRecordSQL()+" WHERE task_id = ? LIMIT 1", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return &records[0], nil
}

// ListTasks returns archive media tasks with optional scope and status filters.
func (repository *Repository) ListTasks(ctx context.Context, options ListOptions) ([]Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	query := selectRecordSQL() + " WHERE 1=1"
	args := []any{}
	if enterpriseID := strings.TrimSpace(options.EnterpriseID); enterpriseID != "" {
		query += " AND enterprise_id = ?"
		args = append(args, enterpriseID)
	}
	if source := strings.TrimSpace(options.Source); source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}
	if status := strings.TrimSpace(options.Status); status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, normalizeListLimit(options.Limit))
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// ListByArchiveMsgIDs returns media tasks for archive messages, newest first.
func (repository *Repository) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	ids := normalizeStrings(archiveMsgIDs)
	if len(ids) == 0 {
		return []Record{}, nil
	}
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	query := selectRecordSQL() + " WHERE archive_msgid IN (" + placeholders(len(ids)) + ")"
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
	return scanRecords(rows)
}

// ListFinishedBefore returns completed media tasks older than cutoff.
func (repository *Repository) ListFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) ([]Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	limit := normalizePruneBatchSize(batchSize)
	rows, err := repository.DB.QueryContext(ctx, selectRecordSQL()+" WHERE updated_at < ? AND is_finish = 1 ORDER BY updated_at ASC LIMIT ?", repository.dbTimeParam(cutoff), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

// DeleteTasks deletes media tasks by task_id.
func (repository *Repository) DeleteTasks(ctx context.Context, taskIDs []string) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive media task database is not configured")
	}
	ids := normalizeStrings(taskIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := repository.DB.ExecContext(ctx, "DELETE FROM archive_media_tasks WHERE task_id IN ("+placeholders(len(ids))+")", args...); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// PruneBefore deletes completed media tasks older than cutoff.
func (repository *Repository) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	records, err := repository.ListFinishedBefore(ctx, cutoff, batchSize)
	if err != nil {
		return 0, err
	}
	taskIDs := make([]string, 0, len(records))
	for _, record := range records {
		taskIDs = append(taskIDs, record.TaskID)
	}
	return repository.DeleteTasks(ctx, taskIDs)
}

// ClaimTask marks a single unfinished task running when it is claimable.
func (repository *Repository) ClaimTask(ctx context.Context, taskID string, processingLeaseSeconds int) (*Record, error) {
	if repository.Tx == nil {
		return nil, fmt.Errorf("archive media task transaction database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	leaseSeconds := normalizeLeaseSeconds(processingLeaseSeconds)
	now := repository.now()
	staleBefore := now.Add(-time.Duration(leaseSeconds) * time.Second)
	tx, err := repository.Tx.BeginArchiveMediaTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	query := selectRecordSQL() + " WHERE task_id = ? LIMIT 1"
	if strings.EqualFold(repository.Dialect, DialectMySQL) || strings.EqualFold(repository.Dialect, DialectPostgres) {
		query += " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, query, key)
	if err != nil {
		return nil, err
	}
	records, err := scanRecords(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		committed = true
		return nil, nil
	}
	task := records[0]
	if task.IsFinish || !claimTaskStatusAllowed(task, staleBefore) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		committed = true
		return &task, nil
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE archive_media_tasks
SET status = 'running',
    last_error = '',
    next_retry_at = NULL,
    updated_at = ?
WHERE task_id = ? AND is_finish = 0`, repository.dbTimeParam(now), key); err != nil {
		return nil, err
	}
	updated, err := repository.loadRecordsByTaskIDs(ctx, tx, []string{key})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	if len(updated) == 0 {
		return nil, nil
	}
	return &updated[0], nil
}

func normalizeInputs(inputs []EnqueueInput) ([]normalizedInput, error) {
	byIdentity := map[string]normalizedInput{}
	order := []string{}
	for _, input := range inputs {
		item := normalizedInput{EnqueueInput: EnqueueInput{
			EnterpriseID: normalizeEnterpriseID(input.EnterpriseID),
			Source:       normalizeSource(input.Source),
			ArchiveMsgID: strings.TrimSpace(input.ArchiveMsgID),
			SDKFileID:    strings.TrimSpace(input.SDKFileID),
			PayloadJSON:  strings.TrimSpace(input.PayloadJSON),
		}}
		if item.ArchiveMsgID == "" || item.SDKFileID == "" {
			return nil, fmt.Errorf("archive_msgid and sdk_file_id are required")
		}
		item.TaskIdentity = BuildTaskIdentity(item.EnterpriseID, item.Source, item.ArchiveMsgID, item.SDKFileID)
		if _, ok := byIdentity[item.TaskIdentity]; !ok {
			order = append(order, item.TaskIdentity)
		}
		byIdentity[item.TaskIdentity] = item
	}
	output := make([]normalizedInput, 0, len(order))
	for _, identity := range order {
		output = append(output, byIdentity[identity])
	}
	return output, nil
}

func (repository *Repository) existingByIdentity(ctx context.Context, identities []string) (map[string]Record, error) {
	if len(identities) == 0 {
		return map[string]Record{}, nil
	}
	args := make([]any, 0, len(identities))
	for _, identity := range identities {
		args = append(args, identity)
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT task_id, created_at, task_identity FROM archive_media_tasks WHERE task_identity IN ("+placeholders(len(identities))+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	output := map[string]Record{}
	for rows.Next() {
		var taskID any
		var createdAt any
		var taskIdentity any
		if err := rows.Scan(&taskID, &createdAt, &taskIdentity); err != nil {
			return nil, err
		}
		identity := strings.TrimSpace(textValue(taskIdentity))
		if identity == "" {
			continue
		}
		output[identity] = Record{
			TaskID:       strings.TrimSpace(textValue(taskID)),
			TaskIdentity: identity,
			CreatedAt:    parseDBTime(createdAt),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return output, nil
}

func (repository *Repository) claimPendingMySQL(ctx context.Context, tx ArchiveMediaTx, enterpriseID string, source string, limit int, now time.Time, staleBefore time.Time) ([]Record, error) {
	taskIDs, err := repository.selectClaimableTaskIDs(ctx, tx, `
SELECT task_id
FROM archive_media_tasks FORCE INDEX (idx_archive_media_claim_scope)
WHERE enterprise_id = ? AND source = ? AND is_finish = 0 AND status = 'pending'
ORDER BY created_at DESC, updated_at DESC
LIMIT ?
FOR UPDATE SKIP LOCKED`, enterpriseID, source, limit)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) < limit {
		staleIDs, err := repository.selectClaimableTaskIDs(ctx, tx, `
SELECT task_id
FROM archive_media_tasks FORCE INDEX (idx_archive_media_claim_scope)
WHERE enterprise_id = ? AND source = ? AND is_finish = 0
  AND status = 'running' AND updated_at <= ?
ORDER BY created_at DESC, updated_at DESC
LIMIT ?
FOR UPDATE SKIP LOCKED`, enterpriseID, source, repository.dbTimeParam(staleBefore), limit-len(taskIDs))
		if err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, staleIDs...)
	}
	if len(taskIDs) == 0 {
		return []Record{}, nil
	}
	if err := repository.markTaskIDsRunning(ctx, tx, taskIDs, now); err != nil {
		return nil, err
	}
	return repository.loadRecordsByTaskIDs(ctx, tx, taskIDs)
}

func (repository *Repository) claimPendingPostgres(ctx context.Context, tx ArchiveMediaTx, enterpriseID string, source string, limit int, now time.Time, staleBefore time.Time) ([]Record, error) {
	rows, err := tx.QueryContext(ctx, `
WITH claimable AS (
    SELECT task_id
    FROM archive_media_tasks
    WHERE enterprise_id = ? AND source = ? AND is_finish = 0
      AND (status = 'pending' OR (status = 'running' AND updated_at <= ?))
    ORDER BY
      CASE WHEN status = 'pending' THEN 0 ELSE 1 END ASC,
      status ASC,
      created_at DESC,
      updated_at DESC
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
UPDATE archive_media_tasks AS target
SET status = 'running',
    last_error = '',
    next_retry_at = NULL,
    updated_at = ?
FROM claimable
WHERE target.task_id = claimable.task_id
RETURNING `+recordColumnsSQL("target"),
		enterpriseID,
		source,
		repository.dbTimeParam(staleBefore),
		limit,
		repository.dbTimeParam(now),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (repository *Repository) selectClaimableTaskIDs(ctx context.Context, queryer Queryer, query string, args ...any) ([]string, error) {
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

func (repository *Repository) markTaskIDsRunning(ctx context.Context, queryer Queryer, taskIDs []string, now time.Time) error {
	if len(taskIDs) == 0 {
		return nil
	}
	args := []any{repository.dbTimeParam(now)}
	for _, taskID := range taskIDs {
		args = append(args, taskID)
	}
	_, err := queryer.ExecContext(ctx, `
UPDATE archive_media_tasks
SET status = 'running',
    last_error = '',
    next_retry_at = NULL,
    updated_at = ?
WHERE task_id IN (`+placeholders(len(taskIDs))+`)`, args...)
	return err
}

func (repository *Repository) loadRecordsByTaskIDs(ctx context.Context, queryer Queryer, taskIDs []string) ([]Record, error) {
	if len(taskIDs) == 0 {
		return []Record{}, nil
	}
	args := make([]any, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		args = append(args, taskID)
	}
	rows, err := queryer.QueryContext(ctx, selectRecordSQL()+" WHERE task_id IN ("+placeholders(len(taskIDs))+") ORDER BY created_at DESC, updated_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (repository *Repository) selectRetryableTaskIDs(ctx context.Context, queryer Queryer, enterpriseID string, source string, readyBefore time.Time, maxAttempts *int, limit int) ([]string, error) {
	args := []any{enterpriseID, source, repository.dbTimeParam(readyBefore)}
	query := `
SELECT task_id
FROM archive_media_tasks
WHERE enterprise_id = ? AND source = ? AND is_finish = 0
  AND status = 'failed_retryable'
  AND (next_retry_at IS NULL OR next_retry_at <= ?)`
	if maxAttempts != nil {
		query += " AND retry_count < ?"
		args = append(args, maxInt(0, *maxAttempts))
	}
	query += " ORDER BY updated_at ASC LIMIT ?"
	args = append(args, limit)
	if strings.EqualFold(repository.Dialect, DialectMySQL) || strings.EqualFold(repository.Dialect, DialectPostgres) {
		query += " FOR UPDATE SKIP LOCKED"
	}
	return repository.selectClaimableTaskIDs(ctx, queryer, query, args...)
}

func selectRecordSQL() string {
	return "SELECT " + recordColumnsSQL("") + " FROM archive_media_tasks"
}

func recordColumnsSQL(prefix string) string {
	columns := []string{
		"task_id",
		"enterprise_id",
		"source",
		"archive_msgid",
		"sdk_file_id",
		"task_identity",
		"index_buf",
		"out_index_buf",
		"is_finish",
		"status",
		"payload_json",
		"local_file_path",
		"downloaded_bytes",
		"object_url",
		"storage_backend",
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

func scanRecords(rows RowsScanner) ([]Record, error) {
	records := []Record{}
	for rows.Next() {
		values := make([]any, 20)
		dest := make([]any, len(values))
		for index := range values {
			dest[index] = &values[index]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		records = append(records, Record{
			TaskID:          strings.TrimSpace(textValue(values[0])),
			EnterpriseID:    strings.TrimSpace(textValue(values[1])),
			Source:          strings.TrimSpace(textValue(values[2])),
			ArchiveMsgID:    strings.TrimSpace(textValue(values[3])),
			SDKFileID:       strings.TrimSpace(textValue(values[4])),
			TaskIdentity:    strings.TrimSpace(textValue(values[5])),
			IndexBuf:        strings.TrimSpace(textValue(values[6])),
			OutIndexBuf:     strings.TrimSpace(textValue(values[7])),
			IsFinish:        boolValue(values[8]),
			Status:          strings.TrimSpace(textValue(values[9])),
			PayloadJSON:     strings.TrimSpace(textValue(values[10])),
			LocalFilePath:   strings.TrimSpace(textValue(values[11])),
			DownloadedBytes: int64Value(values[12]),
			ObjectURL:       strings.TrimSpace(textValue(values[13])),
			StorageBackend:  strings.TrimSpace(textValue(values[14])),
			LastError:       strings.TrimSpace(textValue(values[15])),
			RetryCount:      intValue(values[16]),
			NextRetryAt:     nullableDBTime(values[17]),
			CreatedAt:       parseDBTime(values[18]),
			UpdatedAt:       parseDBTime(values[19]),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func claimTaskStatusAllowed(task Record, staleBefore time.Time) bool {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	switch status {
	case StatusPending, StatusFailed, StatusFailedRetryable:
		return true
	case StatusRunning:
		return task.UpdatedAt.IsZero() || !task.UpdatedAt.After(staleBefore)
	default:
		return false
	}
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO archive_media_tasks (
    task_id, enterprise_id, source, archive_msgid, sdk_file_id, task_identity,
    index_buf, out_index_buf, is_finish, status, payload_json,
    local_file_path, downloaded_bytes, object_url, storage_backend, last_error,
    retry_count, next_retry_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, '', '', 0, 'pending', ?, '', 0, '', 'local', '', 0, NULL, ?, ?)
ON DUPLICATE KEY UPDATE
    status=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.status
        ELSE 'pending'
    END,
    retry_count=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.retry_count
        ELSE 0
    END,
    next_retry_at=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.next_retry_at
        ELSE NULL
    END,
    payload_json=VALUES(payload_json),
    updated_at=VALUES(updated_at)`
	}
	return `
INSERT INTO archive_media_tasks (
    task_id, enterprise_id, source, archive_msgid, sdk_file_id, task_identity,
    index_buf, out_index_buf, is_finish, status, payload_json,
    local_file_path, downloaded_bytes, object_url, storage_backend, last_error,
    retry_count, next_retry_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, '', '', 0, 'pending', ?, '', 0, '', 'local', '', 0, NULL, ?, ?)
ON CONFLICT(task_identity) DO UPDATE SET
    status=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.status
        ELSE 'pending'
    END,
    retry_count=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.retry_count
        ELSE 0
    END,
    next_retry_at=CASE
        WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.next_retry_at
        ELSE NULL
    END,
    payload_json=excluded.payload_json,
    updated_at=excluded.updated_at`
}

// BuildTaskIdentity mirrors Python _build_task_identity.
func BuildTaskIdentity(enterpriseID string, source string, archiveMsgID string, sdkFileID string) string {
	raw := strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.TrimSpace(source),
		strings.TrimSpace(archiveMsgID),
		strings.TrimSpace(sdkFileID),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func (repository *Repository) nextTaskID() string {
	if repository.NextTaskID != nil {
		if value := strings.TrimSpace(repository.NextTaskID()); value != "" {
			return value
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("amt-%d", repository.now().UnixNano())
	}
	return "amt-" + hex.EncodeToString(random)
}

func normalizeLimit(value int) int {
	if value < 1 {
		return 1
	}
	if value > MaxClaimLimit {
		return MaxClaimLimit
	}
	return value
}

func normalizeListLimit(value int) int {
	if value <= 0 {
		return DefaultListLimit
	}
	if value > MaxListLimit {
		return MaxListLimit
	}
	return value
}

func normalizePruneBatchSize(value int) int {
	if value <= 0 {
		return 5000
	}
	return value
}

func normalizeLeaseSeconds(value int) int {
	if value <= 0 {
		value = DefaultProcessingLeaseSeconds
	}
	if value < MinProcessingLeaseSeconds {
		return MinProcessingLeaseSeconds
	}
	return value
}

func normalizeStrings(values []string) []string {
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

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ", ")
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

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []byte:
		return boolValue(string(typed))
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intValue(value any) int {
	return int(int64Value(value))
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case []byte:
		return int64Value(string(typed))
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0
		}
		var output int64
		_, _ = fmt.Sscan(text, &output)
		return output
	default:
		return 0
	}
}

func nullableDBTime(value any) *time.Time {
	parsed := parseDBTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func defaultText(value string, fallback string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return strings.TrimSpace(fallback)
	}
	return text
}

func maxInt(minimum int, value int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func maxInt64(minimum int64, value int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
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

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) BeginArchiveMediaTx(ctx context.Context) (ArchiveMediaTx, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("archive media task database is not configured")
	}
	tx, err := queryer.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlArchiveMediaTx{tx: tx}, nil
}

type sqlArchiveMediaTx struct {
	tx *sql.Tx
}

func (tx sqlArchiveMediaTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, query, args...)
}

func (tx sqlArchiveMediaTx) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	return tx.tx.QueryContext(ctx, query, args...)
}

func (tx sqlArchiveMediaTx) Commit() error {
	return tx.tx.Commit()
}

func (tx sqlArchiveMediaTx) Rollback() error {
	return tx.tx.Rollback()
}
