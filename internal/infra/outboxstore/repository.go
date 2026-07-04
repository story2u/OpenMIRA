// Package outboxstore adapts the legacy outbox_events table for Go.
package outboxstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/outbox"
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

// OutboxTx is the transaction shape needed by durable claim operations.
type OutboxTx interface {
	Queryer
	Commit() error
	Rollback() error
}

// Transactioner starts outbox transactions.
type Transactioner interface {
	BeginOutboxTx(ctx context.Context) (OutboxTx, error)
}

// AfterEnqueueFunc is called after outbox rows are durably inserted.
type AfterEnqueueFunc func(ctx context.Context, records []outbox.Record) error

// Repository writes and updates outbox_events rows.
type Repository struct {
	DB           Queryer
	Tx           Transactioner
	Dialect      string
	Now          func() time.Time
	AfterEnqueue AfterEnqueueFunc
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	queryer := sqlQueryer{db: db}
	return &Repository{DB: queryer, Tx: queryer, Dialect: dialect}
}

// ClaimOptions controls one outbox relay claim pass.
type ClaimOptions struct {
	Limit                  int
	IncludeEventTypes      []string
	ExcludeEventTypes      []string
	ProcessingLeaseSeconds int
}

// Enqueue upserts one outbox event and returns the stored record shape.
func (repository *Repository) Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error) {
	if repository.DB == nil {
		return outbox.Record{}, fmt.Errorf("outbox database is not configured")
	}
	record := outbox.RecordFromEnvelope(event, repository.now())
	if err := repository.insertRecord(ctx, record); err != nil {
		return outbox.Record{}, err
	}
	repository.notifyAfterEnqueue(ctx, []outbox.Record{record})
	return record, nil
}

// EnqueueMany upserts multiple outbox events in order.
func (repository *Repository) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	if len(events) == 0 {
		return []outbox.Record{}, nil
	}
	if repository.DB == nil {
		return nil, fmt.Errorf("outbox database is not configured")
	}
	records := make([]outbox.Record, 0, len(events))
	now := repository.now()
	for _, event := range events {
		record := outbox.RecordFromEnvelope(event, now)
		if err := repository.insertRecord(ctx, record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	repository.notifyAfterEnqueue(ctx, records)
	return records, nil
}

// ExistsByTraceAndType mirrors Python exists_by_trace_and_type idempotency lookup.
func (repository *Repository) ExistsByTraceAndType(ctx context.Context, traceID string, eventType string, tenantID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("outbox database is not configured")
	}
	normalizedTraceID := strings.TrimSpace(traceID)
	normalizedEventType := strings.TrimSpace(eventType)
	normalizedTenantID := strings.TrimSpace(tenantID)
	if normalizedTraceID == "" || normalizedEventType == "" {
		return false, nil
	}
	clauses := []string{"trace_id = ?", "event_type = ?"}
	args := []any{normalizedTraceID, normalizedEventType}
	if normalizedTenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, normalizedTenantID)
	}
	query := "SELECT 1 AS found FROM outbox_events WHERE " + strings.Join(clauses, " AND ") + " LIMIT 1"
	var found int
	err := repository.DB.QueryRowContext(ctx, query, args...).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkPublished marks one pending or processing event as published.
func (repository *Repository) MarkPublished(ctx context.Context, eventID string) error {
	if repository.DB == nil {
		return fmt.Errorf("outbox database is not configured")
	}
	normalizedEventID := strings.TrimSpace(eventID)
	if normalizedEventID == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx,
		"UPDATE outbox_events SET status = 'published', published_at = ?, last_error = NULL WHERE event_id = ? AND status IN ('pending', 'processing')",
		repository.dbNowParam(),
		normalizedEventID,
	)
	return err
}

// MarkPublishedMany marks events in chunks, matching Python's MySQL pressure guard.
func (repository *Repository) MarkPublishedMany(ctx context.Context, eventIDs []string) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("outbox database is not configured")
	}
	normalizedIDs := outbox.NormalizeEventIDs(eventIDs)
	if len(normalizedIDs) == 0 {
		return 0, nil
	}
	chunkSize := len(normalizedIDs)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		chunkSize = outbox.MySQLMarkPublishedChunkSize
	}
	if chunkSize < 1 {
		chunkSize = 1
	}
	total := int64(0)
	for offset := 0; offset < len(normalizedIDs); offset += chunkSize {
		end := offset + chunkSize
		if end > len(normalizedIDs) {
			end = len(normalizedIDs)
		}
		chunk := normalizedIDs[offset:end]
		args := []any{repository.dbNowParam()}
		for _, eventID := range chunk {
			args = append(args, eventID)
		}
		result, err := repository.DB.ExecContext(ctx, `
UPDATE outbox_events
SET status = 'published',
    published_at = ?,
    last_error = NULL
WHERE event_id IN (`+placeholders(len(chunk))+`) AND status IN ('pending', 'processing')`, args...)
		if err != nil {
			return total, err
		}
		affected, err := result.RowsAffected()
		if err == nil {
			total += affected
		}
	}
	return total, nil
}

// MarkRetry records a publish failure and delays the next relay attempt.
func (repository *Repository) MarkRetry(ctx context.Context, eventID string, errText string, retryDelaySeconds float64) error {
	if repository.DB == nil {
		return fmt.Errorf("outbox database is not configured")
	}
	normalizedEventID := strings.TrimSpace(eventID)
	if normalizedEventID == "" {
		return nil
	}
	if retryDelaySeconds < 0 {
		retryDelaySeconds = 0
	}
	nextRunAt := repository.now().Add(time.Duration(retryDelaySeconds * float64(time.Second)))
	_, err := repository.DB.ExecContext(ctx, `
UPDATE outbox_events
SET attempt_count = attempt_count + 1,
    available_at = ?,
    last_error = ?,
    status = 'pending'
WHERE event_id = ?`,
		repository.dbTimeParam(nextRunAt),
		strings.TrimSpace(errText),
		normalizedEventID,
	)
	return err
}

// CountPending returns unfinished outbox rows.
func (repository *Repository) CountPending(ctx context.Context) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("outbox database is not configured")
	}
	var total int
	err := repository.DB.QueryRowContext(ctx, "SELECT COUNT(1) AS total FROM outbox_events WHERE status IN ('pending', 'processing')").Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// ClaimPending reads due events and leases them for relay processing.
func (repository *Repository) ClaimPending(ctx context.Context, options ClaimOptions) ([]outbox.Record, error) {
	if repository.Tx == nil {
		return nil, fmt.Errorf("outbox transaction database is not configured")
	}
	limit := options.Limit
	if limit < 1 {
		limit = 1
	}
	leaseSeconds := options.ProcessingLeaseSeconds
	if leaseSeconds <= 0 {
		leaseSeconds = outbox.DefaultProcessingLeaseSeconds
	}
	now := repository.now()
	nowParam := repository.dbTimeParam(now)
	claimUntilParam := repository.dbTimeParam(outbox.ClaimUntil(now, leaseSeconds))
	filterSQL, filterParams := outbox.EventTypeFilterSQL(options.IncludeEventTypes, options.ExcludeEventTypes, "?")
	tx, err := repository.Tx.BeginOutboxTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var records []outbox.Record
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		records, err = repository.claimPendingMySQL(ctx, tx, nowParam, claimUntilParam, limit, filterSQL, filterParams, options.IncludeEventTypes)
	} else {
		records, err = repository.claimPendingPostgres(ctx, tx, nowParam, claimUntilParam, limit, filterSQL, filterParams)
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

// MarkStalePendingPublishedBefore marks old pending/processing rows as published.
func (repository *Repository) MarkStalePendingPublishedBefore(ctx context.Context, cutoff time.Time, limit int, includeProcessing bool) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("outbox database is not configured")
	}
	if limit < 1 {
		limit = 1
	}
	statuses := []string{outbox.StatusPending}
	if includeProcessing {
		statuses = append(statuses, outbox.StatusProcessing)
	}
	statusArgs := make([]any, 0, len(statuses))
	for _, status := range statuses {
		statusArgs = append(statusArgs, status)
	}
	var args []any
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		args = []any{repository.dbNowParam()}
		args = append(args, statusArgs...)
		args = append(args, repository.dbTimeParam(cutoff), limit)
	} else {
		args = append([]any{}, statusArgs...)
		args = append(args, repository.dbTimeParam(cutoff), limit, repository.dbNowParam())
	}
	query := repository.markStaleSQL(len(statuses))
	result, err := repository.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return affected, nil
}

// PrunePublishedBefore deletes already published outbox rows older than cutoff.
func (repository *Repository) PrunePublishedBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("outbox database is not configured")
	}
	if limit < 1 {
		limit = 1
	}
	result, err := repository.DB.ExecContext(ctx, repository.pruneSQL(), repository.dbTimeParam(cutoff), limit)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return affected, nil
}

func (repository *Repository) claimPendingMySQL(ctx context.Context, tx OutboxTx, nowParam any, claimUntilParam any, limit int, filterSQL string, filterParams []string, includeEventTypes []string) ([]outbox.Record, error) {
	forceIndex := "FORCE INDEX (idx_outbox_events_pending)"
	includeValues, _ := outbox.NormalizeEventTypeFilters(includeEventTypes, nil)
	if len(includeValues) > 0 {
		forceIndex = "FORCE INDEX (idx_outbox_events_type_pending)"
	}
	selected := []outbox.Record{}
	for _, status := range []string{outbox.StatusPending, outbox.StatusProcessing} {
		remainingLimit := limit - len(selected)
		if remainingLimit <= 0 {
			break
		}
		args := []any{status, nowParam}
		for _, param := range filterParams {
			args = append(args, param)
		}
		args = append(args, remainingLimit)
		rows, err := tx.QueryContext(ctx, `
SELECT `+recordColumnSQL("")+`
FROM outbox_events `+forceIndex+`
WHERE status = ? AND available_at <= ?`+filterSQL+`
ORDER BY available_at ASC, created_at ASC, event_id ASC
LIMIT ?
FOR UPDATE SKIP LOCKED`, args...)
		if err != nil {
			return nil, err
		}
		records, err := scanRecordRows(rows)
		if err != nil {
			return nil, err
		}
		selected = append(selected, records...)
	}
	eventIDs := recordEventIDs(selected)
	if len(eventIDs) == 0 {
		return selected, nil
	}
	updateArgs := []any{claimUntilParam}
	for _, eventID := range eventIDs {
		updateArgs = append(updateArgs, eventID)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE outbox_events
SET status = 'processing',
    available_at = ?
WHERE event_id IN (`+placeholders(len(eventIDs))+`) AND status IN ('pending', 'processing')`, updateArgs...); err != nil {
		return nil, err
	}
	selectArgs := make([]any, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		selectArgs = append(selectArgs, eventID)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT `+recordColumnSQL("")+`
FROM outbox_events
WHERE event_id IN (`+placeholders(len(eventIDs))+`)
ORDER BY created_at ASC`, selectArgs...)
	if err != nil {
		return nil, err
	}
	return scanRecordRows(rows)
}

func (repository *Repository) claimPendingPostgres(ctx context.Context, tx OutboxTx, nowParam any, claimUntilParam any, limit int, filterSQL string, filterParams []string) ([]outbox.Record, error) {
	args := []any{nowParam}
	for _, param := range filterParams {
		args = append(args, param)
	}
	args = append(args, limit, claimUntilParam)
	rows, err := tx.QueryContext(ctx, `
WITH claimable AS (
    SELECT event_id
    FROM outbox_events
    WHERE status IN ('pending', 'processing') AND available_at <= ?`+filterSQL+`
    ORDER BY created_at ASC
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events AS target
SET status = 'processing',
    available_at = ?
FROM claimable
WHERE target.event_id = claimable.event_id
RETURNING `+recordColumnSQL("target."), args...)
	if err != nil {
		return nil, err
	}
	return scanRecordRows(rows)
}

func (repository *Repository) insertRecord(ctx context.Context, record outbox.Record) error {
	payloadJSON, err := outbox.PayloadJSON(record.Payload)
	if err != nil {
		return err
	}
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		record.EventID,
		record.EventType,
		record.AggregateType,
		record.AggregateID,
		record.TenantID,
		record.PartitionKey,
		record.TraceID,
		payloadJSON,
		record.Status,
		record.AttemptCount,
		repository.dbTimeParam(record.AvailableAt),
		repository.dbTimeParam(record.CreatedAt),
		repository.dbNullableTimeParam(record.PublishedAt),
		nullableText(record.LastError),
	)
	return err
}

func (repository *Repository) notifyAfterEnqueue(ctx context.Context, records []outbox.Record) {
	if repository == nil || repository.AfterEnqueue == nil || len(records) == 0 {
		return
	}
	_ = repository.AfterEnqueue(ctx, append([]outbox.Record(nil), records...))
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO outbox_events (
    event_id, event_type, aggregate_type, aggregate_id, tenant_id, partition_key, trace_id,
    payload_json, status, attempt_count, available_at, created_at, published_at, last_error
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    event_type = VALUES(event_type),
    aggregate_type = VALUES(aggregate_type),
    aggregate_id = VALUES(aggregate_id),
    tenant_id = VALUES(tenant_id),
    partition_key = VALUES(partition_key),
    trace_id = VALUES(trace_id),
    payload_json = VALUES(payload_json),
    status = VALUES(status),
    attempt_count = VALUES(attempt_count),
    available_at = VALUES(available_at),
    created_at = VALUES(created_at),
    published_at = VALUES(published_at),
    last_error = VALUES(last_error)`
	}
	return `
INSERT INTO outbox_events (
    event_id, event_type, aggregate_type, aggregate_id, tenant_id, partition_key, trace_id,
    payload_json, status, attempt_count, available_at, created_at, published_at, last_error
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO UPDATE SET
    event_type = EXCLUDED.event_type,
    aggregate_type = EXCLUDED.aggregate_type,
    aggregate_id = EXCLUDED.aggregate_id,
    tenant_id = EXCLUDED.tenant_id,
    partition_key = EXCLUDED.partition_key,
    trace_id = EXCLUDED.trace_id,
    payload_json = EXCLUDED.payload_json,
    status = EXCLUDED.status,
    attempt_count = EXCLUDED.attempt_count,
    available_at = EXCLUDED.available_at,
    created_at = EXCLUDED.created_at,
    published_at = EXCLUDED.published_at,
    last_error = EXCLUDED.last_error`
}

func (repository *Repository) markStaleSQL(statusCount int) string {
	statusPlaceholders := placeholders(statusCount)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
UPDATE outbox_events
SET status = 'published',
    published_at = ?,
    last_error = COALESCE(NULLIF(last_error, ''), 'marked stale by maintenance')
WHERE event_id IN (
    SELECT event_id
    FROM (
        SELECT event_id
        FROM outbox_events
        WHERE status IN (` + statusPlaceholders + `) AND created_at < ?
        ORDER BY created_at ASC
        LIMIT ?
    ) AS stale_batch
)`
	}
	return `
WITH stale_batch AS (
    SELECT ctid
    FROM outbox_events
    WHERE status IN (` + statusPlaceholders + `)
      AND created_at < ?
    ORDER BY created_at ASC
    LIMIT ?
)
UPDATE outbox_events AS target
SET status = 'published',
    published_at = ?,
    last_error = COALESCE(NULLIF(target.last_error, ''), 'marked stale by maintenance')
FROM stale_batch
WHERE target.ctid = stale_batch.ctid`
}

func (repository *Repository) pruneSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
DELETE FROM outbox_events
WHERE event_id IN (
    SELECT event_id
    FROM (
        SELECT event_id
        FROM outbox_events FORCE INDEX (idx_outbox_events_published_prune)
        WHERE status = 'published' AND published_at IS NOT NULL AND published_at < ?
        ORDER BY published_at ASC, event_id ASC
        LIMIT ?
    ) AS prune_batch
)`
	}
	return `
DELETE FROM outbox_events
WHERE ctid IN (
    SELECT ctid
    FROM outbox_events
    WHERE status = 'published' AND published_at IS NOT NULL AND published_at < ?
    ORDER BY published_at ASC
    LIMIT ?
)`
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

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func placeholders(count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ", ")
}

func recordColumnSQL(prefix string) string {
	columns := outbox.Columns()
	qualified := make([]string, 0, len(columns))
	for _, column := range columns {
		qualified = append(qualified, prefix+column)
	}
	return strings.Join(qualified, ", ")
}

func scanRecordRows(rows RowsScanner) ([]outbox.Record, error) {
	defer rows.Close()
	records := []outbox.Record{}
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

func scanRecord(row RowScanner) (outbox.Record, error) {
	var eventID string
	var eventType string
	var aggregateType string
	var aggregateID string
	var tenantID string
	var partitionKey string
	var traceID string
	var payloadRaw any
	var status string
	var attemptCount int
	var availableAtRaw any
	var createdAtRaw any
	var publishedAtRaw any
	var lastError sql.NullString
	if err := row.Scan(
		&eventID,
		&eventType,
		&aggregateType,
		&aggregateID,
		&tenantID,
		&partitionKey,
		&traceID,
		&payloadRaw,
		&status,
		&attemptCount,
		&availableAtRaw,
		&createdAtRaw,
		&publishedAtRaw,
		&lastError,
	); err != nil {
		return outbox.Record{}, err
	}
	createdAt := parseDBTime(createdAtRaw)
	publishedAt := nullableDBTime(publishedAtRaw)
	normalizedStatus := strings.TrimSpace(status)
	if normalizedStatus == "" {
		normalizedStatus = outbox.StatusPending
	}
	lastErrorText := ""
	if lastError.Valid {
		lastErrorText = strings.TrimSpace(lastError.String)
	}
	return outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventID:       strings.TrimSpace(eventID),
			EventType:     strings.TrimSpace(eventType),
			AggregateType: strings.TrimSpace(aggregateType),
			AggregateID:   strings.TrimSpace(aggregateID),
			TenantID:      strings.TrimSpace(tenantID),
			PartitionKey:  strings.TrimSpace(partitionKey),
			TraceID:       strings.TrimSpace(traceID),
			Payload:       parsePayload(payloadRaw),
			OccurredAt:    createdAt,
			AvailableAt:   parseDBTime(availableAtRaw),
		},
		Status:       normalizedStatus,
		AttemptCount: attemptCount,
		CreatedAt:    createdAt,
		PublishedAt:  publishedAt,
		LastError:    lastErrorText,
	}, nil
}

func recordEventIDs(records []outbox.Record) []string {
	eventIDs := make([]string, 0, len(records))
	for _, record := range records {
		if eventID := strings.TrimSpace(record.EventID); eventID != "" {
			eventIDs = append(eventIDs, eventID)
		}
	}
	return eventIDs
}

func parsePayload(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return typed
	case []byte:
		return parsePayloadJSON(string(typed))
	case string:
		return parsePayloadJSON(typed)
	default:
		return map[string]any{}
	}
}

func parsePayloadJSON(value string) map[string]any {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &payload); err != nil {
		return map[string]any{}
	}
	return payload
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

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("outbox database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("outbox database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("outbox database is not configured")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) BeginOutboxTx(ctx context.Context) (OutboxTx, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("outbox database is not configured")
	}
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

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
