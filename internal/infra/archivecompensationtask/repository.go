// Package archivecompensationtask adapts archive_compensation_tasks for Go.
package archivecompensationtask

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
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Task is the Go shape of archive_compensation_tasks.
type Task struct {
	TaskID       string
	EnterpriseID string
	Source       string
	ReasonType   string
	ReasonKey    string
	TaskIdentity string
	SeqStart     int
	SeqEnd       int
	CursorHint   string
	Status       string
	AttemptCount int
	AvailableAt  time.Time
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// EnqueueInput mirrors Python archive compensation enqueue kwargs.
type EnqueueInput struct {
	EnterpriseID string
	Source       string
	ReasonType   string
	ReasonKey    string
	SeqStart     int
	SeqEnd       int
	CursorHint   string
	AvailableAt  time.Time
}

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql cursor shape needed by list queries.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Repository writes and reads archive compensation task state.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
	NewID   func() string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// BuildTaskIdentity returns the stable dedupe key used by Python.
func BuildTaskIdentity(enterpriseID string, source string, reasonType string, reasonKey string) string {
	payload := strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.TrimSpace(source),
		strings.TrimSpace(reasonType),
		strings.TrimSpace(reasonKey),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// Enqueue inserts or refreshes a compensation task by task_identity.
func (repository *Repository) Enqueue(ctx context.Context, input EnqueueInput) (Task, error) {
	if repository.DB == nil {
		return Task{}, fmt.Errorf("archive compensation task database is not configured")
	}
	ent := defaultText(input.EnterpriseID, "default")
	source := defaultText(input.Source, "self_decrypt")
	reasonType := defaultText(input.ReasonType, "unknown")
	reasonKey := strings.TrimSpace(input.ReasonKey)
	if reasonKey == "" {
		return Task{}, fmt.Errorf("reason_key is required")
	}
	now := repository.dbNowParam()
	availableAt := input.AvailableAt
	if availableAt.IsZero() {
		availableAt = repository.now()
	}
	identity := BuildTaskIdentity(ent, source, reasonType, reasonKey)
	taskID := repository.newID()
	createdAt := now
	var existingTaskID any
	var existingAttemptCount any
	var existingCreatedAt any
	err := repository.DB.QueryRowContext(ctx, selectTaskIdentitySQL, identity).Scan(&existingTaskID, &existingAttemptCount, &existingCreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return Task{}, err
	}
	if err == nil {
		if existingID := strings.TrimSpace(stringFromDB(existingTaskID)); existingID != "" {
			taskID = existingID
		}
		if !isBlank(existingCreatedAt) {
			createdAt = existingCreatedAt
		}
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		taskID,
		ent,
		source,
		reasonType,
		reasonKey,
		identity,
		nonNegative(input.SeqStart),
		nonNegative(input.SeqEnd),
		strings.TrimSpace(input.CursorHint),
		repository.dbTimeParam(availableAt),
		createdAt,
		now,
	)
	if err != nil {
		return Task{}, err
	}
	task, err := repository.Get(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	if task == nil {
		return Task{}, fmt.Errorf("archive compensation task enqueue failed")
	}
	return *task, nil
}

// PullPending returns pending/running compensation tasks ready for execution.
func (repository *Repository) PullPending(ctx context.Context, limit int) ([]Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive compensation task database is not configured")
	}
	normalizedLimit := clampInt(limit, 1, 200, 20)
	rows, err := repository.DB.QueryContext(ctx, selectPendingSQL, repository.dbNowParam(), normalizedLimit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	tasks := make([]Task, 0, normalizedLimit)
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

// MarkRunning marks a task running.
func (repository *Repository) MarkRunning(ctx context.Context, taskID string) (*Task, error) {
	return repository.updateStatus(ctx, taskID, "running")
}

// MarkCompleted marks a task completed.
func (repository *Repository) MarkCompleted(ctx context.Context, taskID string) (*Task, error) {
	return repository.updateStatus(ctx, taskID, "completed")
}

// MarkRetry requeues a task with an incremented attempt count.
func (repository *Repository) MarkRetry(ctx context.Context, taskID string, lastError string, delay time.Duration) (*Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive compensation task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	if delay < 0 {
		delay = 0
	}
	now := repository.now()
	_, err := repository.DB.ExecContext(ctx, markRetrySQL,
		repository.dbTimeParam(now.Add(delay)),
		strings.TrimSpace(lastError),
		repository.dbTimeParam(now),
		key,
	)
	if err != nil {
		return nil, err
	}
	return repository.Get(ctx, key)
}

// Get returns a task by task_id.
func (repository *Repository) Get(ctx context.Context, taskID string) (*Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive compensation task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	task, err := scanTask(repository.DB.QueryRowContext(ctx, selectTaskSQL, key))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// PruneBefore deletes old completed compensation tasks.
func (repository *Repository) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive compensation task database is not configured")
	}
	normalizedBatch := clampInt(batchSize, 1, 5000, 5000)
	rows, err := repository.DB.QueryContext(ctx, selectPruneTaskIDsSQL, repository.dbTimeParam(cutoff), normalizedBatch)
	if err != nil {
		return 0, err
	}
	taskIDs := make([]string, 0, normalizedBatch)
	for rows.Next() {
		var taskID any
		if err := rows.Scan(&taskID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if value := strings.TrimSpace(stringFromDB(taskID)); value != "" {
			taskIDs = append(taskIDs, value)
		}
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if len(taskIDs) == 0 {
		return 0, nil
	}
	_, err = repository.DB.ExecContext(ctx, "DELETE FROM archive_compensation_tasks WHERE task_id IN ("+placeholders(len(taskIDs))+")", stringsToAny(taskIDs)...)
	if err != nil {
		return 0, err
	}
	return len(taskIDs), nil
}

func (repository *Repository) updateStatus(ctx context.Context, taskID string, status string) (*Task, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive compensation task database is not configured")
	}
	key := strings.TrimSpace(taskID)
	if key == "" {
		return nil, nil
	}
	_, err := repository.DB.ExecContext(ctx, updateStatusSQL, defaultText(status, "pending"), repository.dbNowParam(), key)
	if err != nil {
		return nil, err
	}
	return repository.Get(ctx, key)
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return enqueueMySQLSQL
	}
	return enqueuePostgresSQL
}

const taskColumnsSQL = "task_id, enterprise_id, source, reason_type, reason_key, task_identity, seq_start, seq_end, cursor_hint, status, attempt_count, available_at, last_error, created_at, updated_at"

const selectTaskIdentitySQL = "SELECT task_id, attempt_count, created_at FROM archive_compensation_tasks WHERE task_identity = ?"

const selectTaskSQL = "SELECT " + taskColumnsSQL + " FROM archive_compensation_tasks WHERE task_id = ?"

const selectPendingSQL = `
SELECT ` + taskColumnsSQL + `
FROM archive_compensation_tasks
WHERE status IN ('pending', 'running') AND available_at <= ?
ORDER BY updated_at ASC
LIMIT ?`

const selectPruneTaskIDsSQL = `
SELECT task_id
FROM archive_compensation_tasks
WHERE status = 'completed'
  AND updated_at < ?
ORDER BY updated_at ASC, task_id ASC
LIMIT ?`

const enqueueMySQLSQL = `
INSERT INTO archive_compensation_tasks (
    task_id, enterprise_id, source, reason_type, reason_key, task_identity,
    seq_start, seq_end, cursor_hint, status, attempt_count,
    available_at, last_error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, '', ?, ?)
ON DUPLICATE KEY UPDATE
    seq_start = VALUES(seq_start),
    seq_end = VALUES(seq_end),
    cursor_hint = VALUES(cursor_hint),
    status = CASE
        WHEN archive_compensation_tasks.status = 'completed' THEN archive_compensation_tasks.status
        ELSE 'pending'
    END,
    available_at = VALUES(available_at),
    updated_at = VALUES(updated_at)`

const enqueuePostgresSQL = `
INSERT INTO archive_compensation_tasks (
    task_id, enterprise_id, source, reason_type, reason_key, task_identity,
    seq_start, seq_end, cursor_hint, status, attempt_count,
    available_at, last_error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, '', ?, ?)
ON CONFLICT(task_identity) DO UPDATE SET
    seq_start = excluded.seq_start,
    seq_end = excluded.seq_end,
    cursor_hint = excluded.cursor_hint,
    status = CASE
        WHEN archive_compensation_tasks.status = 'completed' THEN archive_compensation_tasks.status
        ELSE 'pending'
    END,
    available_at = excluded.available_at,
    updated_at = excluded.updated_at`

const updateStatusSQL = "UPDATE archive_compensation_tasks SET status = ?, updated_at = ? WHERE task_id = ?"

const markRetrySQL = `
UPDATE archive_compensation_tasks
SET status = 'pending',
    attempt_count = attempt_count + 1,
    available_at = ?,
    last_error = ?,
    updated_at = ?
WHERE task_id = ?`

func scanTask(row RowScanner) (Task, error) {
	var (
		taskID       any
		enterpriseID any
		source       any
		reasonType   any
		reasonKey    any
		taskIdentity any
		seqStart     any
		seqEnd       any
		cursorHint   any
		status       any
		attemptCount any
		availableAt  any
		lastError    any
		createdAt    any
		updatedAt    any
	)
	if err := row.Scan(
		&taskID,
		&enterpriseID,
		&source,
		&reasonType,
		&reasonKey,
		&taskIdentity,
		&seqStart,
		&seqEnd,
		&cursorHint,
		&status,
		&attemptCount,
		&availableAt,
		&lastError,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Task{}, err
	}
	return Task{
		TaskID:       stringFromDB(taskID),
		EnterpriseID: stringFromDB(enterpriseID),
		Source:       stringFromDB(source),
		ReasonType:   stringFromDB(reasonType),
		ReasonKey:    stringFromDB(reasonKey),
		TaskIdentity: stringFromDB(taskIdentity),
		SeqStart:     intFromDB(seqStart),
		SeqEnd:       intFromDB(seqEnd),
		CursorHint:   stringFromDB(cursorHint),
		Status:       defaultText(stringFromDB(status), "pending"),
		AttemptCount: intFromDB(attemptCount),
		AvailableAt:  timeFromDB(availableAt),
		LastError:    stringFromDB(lastError),
		CreatedAt:    timeFromDB(createdAt),
		UpdatedAt:    timeFromDB(updatedAt),
	}, nil
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	if value.IsZero() {
		value = repository.now()
	}
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return value.UTC().Format(time.RFC3339)
	}
	return value.In(beijingLocation).Format("2006-01-02 15:04:05")
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func (repository *Repository) newID() string {
	if repository.NewID != nil {
		if value := strings.TrimSpace(repository.NewID()); value != "" {
			return value
		}
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("act-%d", repository.now().UnixNano())
	}
	return "act-" + hex.EncodeToString(bytes[:])
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampInt(value int, minimum int, maximum int, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case time.Time:
		return typed.Format(time.RFC3339)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		_, _ = fmt.Sscan(string(typed), &parsed)
		return parsed
	case string:
		var parsed int
		_, _ = fmt.Sscan(typed, &parsed)
		return parsed
	default:
		var parsed int
		_, _ = fmt.Sscan(fmt.Sprint(typed), &parsed)
		return parsed
	}
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseDBTime(string(typed))
	case string:
		return parseDBTime(typed)
	default:
		return parseDBTime(fmt.Sprint(typed))
	}
}

func parseDBTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, beijingLocation); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func isBlank(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []byte:
		return strings.TrimSpace(string(typed)) == ""
	default:
		return false
	}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is nil")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
