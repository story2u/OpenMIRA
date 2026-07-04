// Package taskstore persists phase-six task records in the legacy tasks table.
// It mirrors the Python TaskRepository column contract while remaining behind
// an explicit adapter boundary for later route cutover.
package taskstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

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

// TaskStoreTx is the transaction shape needed by durable claim operations.
type TaskStoreTx interface {
	Queryer
	Commit() error
	Rollback() error
}

// Transactioner starts task store transactions.
type Transactioner interface {
	BeginTaskStoreTx(ctx context.Context) (TaskStoreTx, error)
}

// Repository implements tasks.Store over the legacy tasks table.
type Repository struct {
	DB      Queryer
	Tx      Transactioner
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	queryer := sqlQueryer{db: db}
	return &Repository{DB: queryer, Tx: queryer, Dialect: dialect}
}

// Upsert writes or replaces one task record using the legacy column layout.
func (repository *Repository) Upsert(ctx context.Context, task tasks.Record) error {
	if repository.DB == nil {
		return fmt.Errorf("task database is not configured")
	}
	payload, err := json.Marshal(task.Payload)
	if err != nil {
		return err
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		task.TaskID,
		task.Source,
		task.Target.AgentID,
		task.Target.DeviceID,
		task.TaskType,
		string(payload),
		string(task.Status),
		dbTime(task.CreatedAt),
		dbTime(task.UpdatedAt),
		stringPtrValue(task.TraceID),
		stringPtrValue(task.Error),
		task.RetryCount,
		timePtrValue(task.NextRetryAt),
		stringPtrValue(task.WeWorkUserID),
		stringPtrValue(task.EnterpriseID),
		timePtrValue(task.DispatchedAt),
		timePtrValue(task.ScriptStartedAt),
	)
	return err
}

// Get returns one task by id.
func (repository *Repository) Get(ctx context.Context, taskID string) (tasks.Record, bool, error) {
	if repository.DB == nil {
		return tasks.Record{}, false, fmt.Errorf("task database is not configured")
	}
	row := repository.DB.QueryRowContext(ctx, selectTaskSQL("task_id = ?", "LIMIT 1"), strings.TrimSpace(taskID))
	record, err := scanTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return tasks.Record{}, false, nil
		}
		return tasks.Record{}, false, err
	}
	return record, true, nil
}

// List returns filtered tasks ordered by created_at descending.
func (repository *Repository) List(ctx context.Context, query tasks.Query) ([]tasks.Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("task database is not configured")
	}
	where, args := repository.where(query)
	sqlText := selectTaskSQL(where, "ORDER BY created_at DESC")
	if query.Limit != nil {
		sqlText += " LIMIT ?"
		limit := *query.Limit
		if limit < 0 {
			limit = 0
		}
		args = append(args, limit)
	}
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]tasks.Record, 0)
	for rows.Next() {
		record, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO tasks (
    task_id, source, target_agent_id, target_device_id, task_type, payload_json,
    status, created_at, updated_at, trace_id, error, retry_count, next_retry_at,
    wework_user_id, enterprise_id, dispatched_at, script_started_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
    source=excluded.source,
    target_agent_id=excluded.target_agent_id,
    target_device_id=excluded.target_device_id,
    task_type=excluded.task_type,
    payload_json=excluded.payload_json,
    status=excluded.status,
    created_at=excluded.created_at,
    updated_at=excluded.updated_at,
    trace_id=excluded.trace_id,
    error=excluded.error,
    retry_count=excluded.retry_count,
    next_retry_at=excluded.next_retry_at,
    wework_user_id=excluded.wework_user_id,
    enterprise_id=excluded.enterprise_id,
    dispatched_at=excluded.dispatched_at,
    script_started_at=excluded.script_started_at`
	}
	return `
INSERT INTO tasks (
    task_id, source, target_agent_id, target_device_id, task_type, payload_json,
    status, created_at, updated_at, trace_id, error, retry_count, next_retry_at,
    wework_user_id, enterprise_id, dispatched_at, script_started_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    source=VALUES(source),
    target_agent_id=VALUES(target_agent_id),
    target_device_id=VALUES(target_device_id),
    task_type=VALUES(task_type),
    payload_json=VALUES(payload_json),
    status=VALUES(status),
    created_at=VALUES(created_at),
    updated_at=VALUES(updated_at),
    trace_id=VALUES(trace_id),
    error=VALUES(error),
    retry_count=VALUES(retry_count),
    next_retry_at=VALUES(next_retry_at),
    wework_user_id=VALUES(wework_user_id),
    enterprise_id=VALUES(enterprise_id),
    dispatched_at=VALUES(dispatched_at),
    script_started_at=VALUES(script_started_at)`
}

func (repository *Repository) where(query tasks.Query) (string, []any) {
	clauses := []string{"1 = 1"}
	args := []any{}
	if query.Status != nil {
		clauses = append(clauses, "status = ?")
		args = append(args, string(*query.Status))
	}
	if strings.TrimSpace(query.AgentID) != "" {
		clauses = append(clauses, "target_agent_id = ?")
		args = append(args, strings.TrimSpace(query.AgentID))
	}
	if strings.TrimSpace(query.DeviceID) != "" {
		clauses = append(clauses, "target_device_id = ?")
		args = append(args, strings.TrimSpace(query.DeviceID))
	}
	if strings.TrimSpace(query.TaskType) != "" {
		clauses = append(clauses, "task_type = ?")
		args = append(args, strings.TrimSpace(query.TaskType))
	}
	return strings.Join(clauses, " AND "), args
}

func selectTaskSQL(where string, suffix string) string {
	return `
SELECT task_id, source, target_agent_id, target_device_id, task_type, payload_json,
       status, created_at, updated_at, trace_id, error, retry_count, next_retry_at,
       wework_user_id, enterprise_id, dispatched_at, script_started_at
FROM tasks
WHERE ` + where + " " + suffix
}

func scanTask(scanner RowScanner) (tasks.Record, error) {
	var taskID any
	var source any
	var agentID any
	var deviceID any
	var taskType any
	var payloadJSON any
	var status any
	var createdAt any
	var updatedAt any
	var traceID any
	var taskError any
	var retryCount any
	var nextRetryAt any
	var weworkUserID any
	var enterpriseID any
	var dispatchedAt any
	var scriptStartedAt any
	if err := scanner.Scan(
		&taskID,
		&source,
		&agentID,
		&deviceID,
		&taskType,
		&payloadJSON,
		&status,
		&createdAt,
		&updatedAt,
		&traceID,
		&taskError,
		&retryCount,
		&nextRetryAt,
		&weworkUserID,
		&enterpriseID,
		&dispatchedAt,
		&scriptStartedAt,
	); err != nil {
		return tasks.Record{}, err
	}
	payload, err := payloadFromDB(payloadJSON)
	if err != nil {
		return tasks.Record{}, err
	}
	return tasks.Record{
		TaskID:          stringFromDB(taskID),
		Source:          stringFromDB(source),
		Target:          tasks.Target{AgentID: stringFromDB(agentID), DeviceID: stringFromDB(deviceID)},
		TaskType:        stringFromDB(taskType),
		Payload:         payload,
		Status:          tasks.Status(stringFromDB(status)),
		CreatedAt:       timeFromDB(createdAt),
		UpdatedAt:       timeFromDB(updatedAt),
		TraceID:         stringPtrFromDB(traceID),
		Error:           stringPtrFromDB(taskError),
		RetryCount:      intFromDB(retryCount),
		NextRetryAt:     timePtrFromDB(nextRetryAt),
		WeWorkUserID:    stringPtrFromDB(weworkUserID),
		EnterpriseID:    stringPtrFromDB(enterpriseID),
		DispatchedAt:    timePtrFromDB(dispatchedAt),
		ScriptStartedAt: timePtrFromDB(scriptStartedAt),
	}, nil
}

func payloadFromDB(value any) (map[string]any, error) {
	raw := strings.TrimSpace(stringFromDB(value))
	if raw == "" {
		return map[string]any{}, nil
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func dbTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func timePtrValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return dbTime(*value)
}

func stringPtrValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringFromDB(value any) string {
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

func stringPtrFromDB(value any) *string {
	text := strings.TrimSpace(stringFromDB(value))
	if text == "" {
		return nil
	}
	return &text
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case []byte:
		parsed, _ := strconv.Atoi(strings.TrimSpace(string(typed)))
		return parsed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseTimeText(string(typed))
	case string:
		return parseTimeText(typed)
	default:
		return time.Time{}
	}
}

func timePtrFromDB(value any) *time.Time {
	parsed := timeFromDB(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func parseTimeText(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
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

func (queryer sqlQueryer) BeginTaskStoreTx(ctx context.Context) (TaskStoreTx, error) {
	tx, err := queryer.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlTxQueryer{tx: tx}, nil
}

type sqlTxQueryer struct {
	tx *sql.Tx
}

func (queryer sqlTxQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.tx.ExecContext(ctx, query, args...)
}

func (queryer sqlTxQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	return queryer.tx.QueryContext(ctx, query, args...)
}

func (queryer sqlTxQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.tx.QueryRowContext(ctx, query, args...)
}

func (queryer sqlTxQueryer) Commit() error {
	return queryer.tx.Commit()
}

func (queryer sqlTxQueryer) Rollback() error {
	return queryer.tx.Rollback()
}
