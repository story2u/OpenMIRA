// Package archivecallbackreceipt adapts archive_callback_receipts for Go.
package archivecallbackreceipt

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archivecallback"
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

// Repository writes archive callback receipt state.
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

// RecordCallback mirrors Python record_callback.
func (repository *Repository) RecordCallback(ctx context.Context, input archivecallback.ReceiptInput) (bool, archivecallback.Receipt, error) {
	if repository.DB == nil {
		return false, archivecallback.Receipt{}, fmt.Errorf("archive callback receipt database is not configured")
	}
	key := strings.TrimSpace(input.CallbackEventKey)
	if key == "" {
		return false, archivecallback.Receipt{}, fmt.Errorf("callback_event_key is required")
	}
	existing, err := repository.GetByEventKey(ctx, key)
	if err != nil {
		return false, archivecallback.Receipt{}, err
	}
	now := repository.dbNowParam()
	if existing == nil {
		_, err = repository.DB.ExecContext(ctx, insertReceiptSQL,
			repository.newID(),
			defaultText(input.EnterpriseID, archivecallback.DefaultEnterpriseID),
			defaultText(input.Source, archivecallback.DefaultSource),
			defaultText(input.EventName, "unknown"),
			key,
			strings.TrimSpace(input.MsgSignature),
			strings.TrimSpace(input.Timestamp),
			strings.TrimSpace(input.Nonce),
			strings.TrimSpace(input.EncryptHash),
			input.PlainPayload,
			defaultText(input.Status, "received"),
			now,
			now,
		)
		if err != nil {
			return false, archivecallback.Receipt{}, err
		}
		receipt, err := repository.GetByEventKey(ctx, key)
		if err != nil {
			return false, archivecallback.Receipt{}, err
		}
		if receipt == nil {
			return false, archivecallback.Receipt{}, fmt.Errorf("archive callback receipt upsert failed")
		}
		return true, *receipt, nil
	}
	duplicateSQL := ""
	if input.IncrementDuplicate {
		duplicateSQL = "duplicate_count = duplicate_count + 1,"
	}
	_, err = repository.DB.ExecContext(ctx, updateReceiptSQL(duplicateSQL),
		now,
		defaultText(input.EventName, "unknown"),
		strings.TrimSpace(input.MsgSignature),
		strings.TrimSpace(input.Timestamp),
		strings.TrimSpace(input.Nonce),
		strings.TrimSpace(input.EncryptHash),
		input.PlainPayload,
		key,
	)
	if err != nil {
		return false, archivecallback.Receipt{}, err
	}
	receipt, err := repository.GetByEventKey(ctx, key)
	if err != nil {
		return false, archivecallback.Receipt{}, err
	}
	if receipt == nil {
		return false, archivecallback.Receipt{}, fmt.Errorf("archive callback receipt upsert failed")
	}
	return false, *receipt, nil
}

// GetByEventKey returns one receipt by callback_event_key.
func (repository *Repository) GetByEventKey(ctx context.Context, callbackEventKey string) (*archivecallback.Receipt, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive callback receipt database is not configured")
	}
	key := strings.TrimSpace(callbackEventKey)
	if key == "" {
		return nil, nil
	}
	receipt, err := scanReceipt(repository.DB.QueryRowContext(ctx, selectReceiptSQL, key))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// CountRecent returns the callback receipt count for monitor pagination.
func (repository *Repository) CountRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive callback receipt database is not configured")
	}
	whereSQL, args := recentFilters(filter)
	var total any
	if err := repository.DB.QueryRowContext(ctx, "SELECT COUNT(1) AS total FROM archive_callback_receipts "+whereSQL, args...).Scan(&total); err != nil {
		return 0, err
	}
	return intFromDB(total), nil
}

// ListRecent returns callback receipts ordered by updated_at descending.
func (repository *Repository) ListRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) ([]archivecallback.Receipt, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive callback receipt database is not configured")
	}
	whereSQL, args := recentFilters(filter)
	limit := clampInt(filter.Limit, 1, 500, 50)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := repository.DB.QueryContext(ctx, selectRecentReceiptsSQL(whereSQL), args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	receipts := make([]archivecallback.Receipt, 0, limit)
	for rows.Next() {
		receipt, err := scanReceipt(rows)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

// ListPendingCompensation returns received/dispatched receipts for timeout compensation.
func (repository *Repository) ListPendingCompensation(ctx context.Context, limit int) ([]archivecallback.PendingCompensationReceipt, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive callback receipt database is not configured")
	}
	normalizedLimit := clampInt(limit, 1, 500, 50)
	rows, err := repository.DB.QueryContext(ctx, selectPendingCompensationSQL, normalizedLimit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	items := make([]archivecallback.PendingCompensationReceipt, 0, normalizedLimit)
	for rows.Next() {
		var (
			receiptID        any
			enterpriseID     any
			source           any
			callbackEventKey any
			status           any
		)
		if err := rows.Scan(&receiptID, &enterpriseID, &source, &callbackEventKey, &status); err != nil {
			return nil, err
		}
		items = append(items, archivecallback.PendingCompensationReceipt{
			ReceiptID:        stringFromDB(receiptID),
			EnterpriseID:     stringFromDB(enterpriseID),
			Source:           stringFromDB(source),
			CallbackEventKey: stringFromDB(callbackEventKey),
			Status:           defaultText(stringFromDB(status), "received"),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// PruneBefore deletes old terminal callback receipts while preserving pending rows.
func (repository *Repository) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive callback receipt database is not configured")
	}
	normalizedBatch := clampInt(batchSize, 1, 5000, 5000)
	rows, err := repository.DB.QueryContext(ctx, selectPruneReceiptIDsSQL, repository.dbTimeParam(cutoff), normalizedBatch)
	if err != nil {
		return 0, err
	}
	receiptIDs := make([]string, 0, normalizedBatch)
	for rows.Next() {
		var receiptID any
		if err := rows.Scan(&receiptID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if value := strings.TrimSpace(stringFromDB(receiptID)); value != "" {
			receiptIDs = append(receiptIDs, value)
		}
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if len(receiptIDs) == 0 {
		return 0, nil
	}
	_, err = repository.DB.ExecContext(ctx, "DELETE FROM archive_callback_receipts WHERE receipt_id IN ("+placeholders(len(receiptIDs))+")", stringsToAny(receiptIDs)...)
	if err != nil {
		return 0, err
	}
	return len(receiptIDs), nil
}

// MarkTriggerRequested mirrors Python mark_trigger_requested.
func (repository *Repository) MarkTriggerRequested(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	return repository.mark(ctx, markTriggerRequestedSQL, callbackEventKey, defaultText(status, "dispatched"), strings.TrimSpace(lastError))
}

// MarkProcessed mirrors Python mark_processed.
func (repository *Repository) MarkProcessed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	return repository.mark(ctx, markProcessedSQL, callbackEventKey, defaultText(status, "processed"), strings.TrimSpace(lastError))
}

// MarkFailed mirrors Python mark_failed.
func (repository *Repository) MarkFailed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	return repository.mark(ctx, markFailedSQL, callbackEventKey, defaultText(status, "failed"), strings.TrimSpace(lastError))
}

func (repository *Repository) mark(ctx context.Context, query string, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive callback receipt database is not configured")
	}
	key := strings.TrimSpace(callbackEventKey)
	if key == "" {
		return nil, nil
	}
	now := repository.dbNowParam()
	var err error
	if query == markTriggerRequestedSQL {
		_, err = repository.DB.ExecContext(ctx, query, status, now, nullableText(lastError), now, key)
	} else if query == markProcessedSQL {
		_, err = repository.DB.ExecContext(ctx, query, status, now, nullableText(lastError), now, key)
	} else {
		_, err = repository.DB.ExecContext(ctx, query, status, nullableText(lastError), now, key)
	}
	if err != nil {
		return nil, err
	}
	return repository.GetByEventKey(ctx, key)
}

const selectReceiptSQL = "SELECT receipt_id, enterprise_id, source, event_name, callback_event_key, msg_signature, timestamp, nonce, encrypt_hash, plain_payload, status, duplicate_count, trigger_requested_at, processed_at, last_error, created_at, updated_at FROM archive_callback_receipts WHERE callback_event_key = ?"

func selectRecentReceiptsSQL(whereSQL string) string {
	return "SELECT receipt_id, enterprise_id, source, event_name, callback_event_key, msg_signature, timestamp, nonce, encrypt_hash, plain_payload, status, duplicate_count, trigger_requested_at, processed_at, last_error, created_at, updated_at FROM archive_callback_receipts " + whereSQL + " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
}

const selectPendingCompensationSQL = `
SELECT receipt_id, enterprise_id, source, callback_event_key, status
FROM archive_callback_receipts
WHERE status IN ('received', 'dispatched')
ORDER BY updated_at ASC
LIMIT ?`

const selectPruneReceiptIDsSQL = `
SELECT receipt_id
FROM archive_callback_receipts
WHERE status IN ('processed', 'failed')
  AND updated_at < ?
ORDER BY updated_at ASC, receipt_id ASC
LIMIT ?`

func recentFilters(filter archivecallback.ReceiptListFilter) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if enterpriseID := strings.TrimSpace(filter.EnterpriseID); enterpriseID != "" {
		clauses = append(clauses, "enterprise_id = ?")
		args = append(args, enterpriseID)
	}
	if eventName := strings.TrimSpace(filter.EventName); eventName != "" {
		clauses = append(clauses, "event_name = ?")
		args = append(args, eventName)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

const insertReceiptSQL = `
INSERT INTO archive_callback_receipts (
    receipt_id, enterprise_id, source, event_name, callback_event_key,
    msg_signature, timestamp, nonce, encrypt_hash, plain_payload,
    status, duplicate_count, trigger_requested_at, processed_at, last_error, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, NULL, NULL, ?, ?)`

func updateReceiptSQL(duplicateSQL string) string {
	return `
UPDATE archive_callback_receipts
SET ` + duplicateSQL + `
    updated_at = ?,
    event_name = ?,
    msg_signature = ?,
    timestamp = ?,
    nonce = ?,
    encrypt_hash = ?,
    plain_payload = CASE WHEN plain_payload = '' THEN ? ELSE plain_payload END
WHERE callback_event_key = ?`
}

const markTriggerRequestedSQL = `
UPDATE archive_callback_receipts
SET status = ?,
    trigger_requested_at = ?,
    last_error = ?,
    updated_at = ?
WHERE callback_event_key = ?`

const markProcessedSQL = `
UPDATE archive_callback_receipts
SET status = ?,
    processed_at = ?,
    last_error = ?,
    updated_at = ?
WHERE callback_event_key = ?`

const markFailedSQL = `
UPDATE archive_callback_receipts
SET status = ?,
    last_error = ?,
    updated_at = ?
WHERE callback_event_key = ?`

func scanReceipt(row RowScanner) (archivecallback.Receipt, error) {
	var (
		receiptID          any
		enterpriseID       any
		source             any
		eventName          any
		callbackEventKey   any
		msgSignature       any
		timestamp          any
		nonce              any
		encryptHash        any
		plainPayload       any
		status             any
		duplicateCount     any
		triggerRequestedAt any
		processedAt        any
		lastError          any
		createdAt          any
		updatedAt          any
	)
	if err := row.Scan(
		&receiptID,
		&enterpriseID,
		&source,
		&eventName,
		&callbackEventKey,
		&msgSignature,
		&timestamp,
		&nonce,
		&encryptHash,
		&plainPayload,
		&status,
		&duplicateCount,
		&triggerRequestedAt,
		&processedAt,
		&lastError,
		&createdAt,
		&updatedAt,
	); err != nil {
		return archivecallback.Receipt{}, err
	}
	return archivecallback.Receipt{
		ReceiptID:          stringFromDB(receiptID),
		EnterpriseID:       stringFromDB(enterpriseID),
		Source:             stringFromDB(source),
		EventName:          stringFromDB(eventName),
		CallbackEventKey:   stringFromDB(callbackEventKey),
		MsgSignature:       stringFromDB(msgSignature),
		Timestamp:          stringFromDB(timestamp),
		Nonce:              stringFromDB(nonce),
		EncryptHash:        stringFromDB(encryptHash),
		PlainPayload:       stringFromDB(plainPayload),
		Status:             defaultText(stringFromDB(status), "received"),
		DuplicateCount:     intFromDB(duplicateCount),
		TriggerRequestedAt: timePtrFromDB(triggerRequestedAt),
		ProcessedAt:        timePtrFromDB(processedAt),
		LastError:          stringFromDB(lastError),
		CreatedAt:          timeFromDB(createdAt),
		UpdatedAt:          timeFromDB(updatedAt),
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
		return fmt.Sprintf("acr-%d", repository.now().UnixNano())
	}
	return "acr-" + hex.EncodeToString(bytes[:])
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
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

func timePtrFromDB(value any) *time.Time {
	if isBlank(value) {
		return nil
	}
	parsed := timeFromDB(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
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
