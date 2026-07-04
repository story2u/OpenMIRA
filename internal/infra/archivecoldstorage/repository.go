// Package archivecoldstorage adapts encrypted_messages and archive_metadata.
package archivecoldstorage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
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
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// EncryptedMessage mirrors the cold-archive visible encrypted_messages row.
type EncryptedMessage struct {
	MessageID           int64
	TraceID             string
	TenantID            string
	ConversationID      string
	DeviceID            string
	SenderID            string
	MsgType             string
	Direction           string
	EncryptedContent    []byte
	EncryptedKey        []byte
	Nonce               []byte
	AuthTag             []byte
	KeyVersion          int
	EncryptionAlgorithm string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ListEncryptedMessagesOptions mirrors Python list_encrypted_messages kwargs.
type ListEncryptedMessagesOptions struct {
	TenantID   string
	StartDate  time.Time
	EndDate    time.Time
	KeyVersion int
	Limit      int
	Offset     int
}

// ArchiveMetadataInput mirrors Python upsert_archive_metadata kwargs.
type ArchiveMetadataInput struct {
	PartitionName string
	TenantID      string
	RowCount      int
	SizeBytes     int64
	StoragePath   string
	ArchivedAt    time.Time
}

// Repository reads encrypted messages and writes cold archive metadata.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListEncryptedMessages returns encrypted messages ordered for deterministic export.
func (repository *Repository) ListEncryptedMessages(ctx context.Context, options ListEncryptedMessagesOptions) ([]EncryptedMessage, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive cold storage database is not configured")
	}
	clauses := []string{"1=1"}
	args := []any{}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, tenantID)
	}
	if !options.StartDate.IsZero() {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, repository.dbTimeParam(options.StartDate))
	}
	if !options.EndDate.IsZero() {
		clauses = append(clauses, "created_at < ?")
		args = append(args, repository.dbTimeParam(options.EndDate))
	}
	if options.KeyVersion > 0 {
		clauses = append(clauses, "key_version = ?")
		args = append(args, options.KeyVersion)
	}
	args = append(args, positiveOrDefault(options.Limit, 1000), nonNegative(options.Offset))
	query := "SELECT " + encryptedMessageColumnsSQL + " FROM encrypted_messages WHERE " + strings.Join(clauses, " AND ") + " ORDER BY created_at ASC, message_id ASC LIMIT ? OFFSET ?"
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	records := []EncryptedMessage{}
	for rows.Next() {
		record, err := scanEncryptedMessage(rows)
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

// ListArchiveTenants returns tenants that have encrypted messages before endDate.
func (repository *Repository) ListArchiveTenants(ctx context.Context, endDate time.Time, limit int) ([]string, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive cold storage database is not configured")
	}
	clauses := []string{"tenant_id <> ''"}
	args := []any{}
	if !endDate.IsZero() {
		clauses = append(clauses, "created_at < ?")
		args = append(args, repository.dbTimeParam(endDate))
	}
	args = append(args, positiveOrDefault(limit, 1000))
	query := "SELECT DISTINCT tenant_id FROM encrypted_messages WHERE " + strings.Join(clauses, " AND ") + " ORDER BY tenant_id ASC LIMIT ?"
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	tenantIDs := []string{}
	for rows.Next() {
		var tenantID any
		if err := rows.Scan(&tenantID); err != nil {
			return nil, err
		}
		if value := strings.TrimSpace(stringFromDB(tenantID)); value != "" {
			tenantIDs = append(tenantIDs, value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tenantIDs, nil
}

// DeleteEncryptedMessages deletes exported encrypted message rows by message_id.
func (repository *Repository) DeleteEncryptedMessages(ctx context.Context, messageIDs []int64) (int, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("archive cold storage database is not configured")
	}
	ids := normalizeMessageIDs(messageIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	var (
		result sql.Result
		err    error
	)
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		result, err = repository.DB.ExecContext(ctx, "DELETE FROM encrypted_messages WHERE message_id = ANY(?)", ids)
	} else {
		result, err = repository.DB.ExecContext(ctx, "DELETE FROM encrypted_messages WHERE message_id IN ("+placeholders(len(ids))+")", int64sToAny(ids)...)
	}
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// UpsertArchiveMetadata records one cold archive export summary.
func (repository *Repository) UpsertArchiveMetadata(ctx context.Context, input ArchiveMetadataInput) error {
	if repository.DB == nil {
		return fmt.Errorf("archive cold storage database is not configured")
	}
	archivedAt := input.ArchivedAt
	if archivedAt.IsZero() {
		archivedAt = repository.now()
	}
	_, err := repository.DB.ExecContext(ctx, repository.upsertArchiveMetadataSQL(),
		strings.TrimSpace(input.PartitionName),
		strings.TrimSpace(input.TenantID),
		nonNegative(input.RowCount),
		nonNegative64(input.SizeBytes),
		strings.TrimSpace(input.StoragePath),
		repository.dbTimeParam(archivedAt),
		repository.dbNowParam(),
	)
	return err
}

func (repository *Repository) upsertArchiveMetadataSQL() string {
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return upsertArchiveMetadataPostgresSQL
	}
	return upsertArchiveMetadataMySQLSQL
}

const encryptedMessageColumnsSQL = "message_id, trace_id, tenant_id, conversation_id, device_id, sender_id, msg_type, direction, encrypted_content, encrypted_key, nonce, auth_tag, key_version, encryption_algorithm, created_at, updated_at"

const upsertArchiveMetadataMySQLSQL = `
INSERT INTO archive_metadata (
    partition_name, tenant_id, row_count, size_bytes,
    storage_path, archived_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    row_count = VALUES(row_count),
    size_bytes = VALUES(size_bytes),
    storage_path = VALUES(storage_path),
    archived_at = VALUES(archived_at)`

const upsertArchiveMetadataPostgresSQL = `
INSERT INTO archive_metadata (
    partition_name, tenant_id, row_count, size_bytes,
    storage_path, archived_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(partition_name, tenant_id) DO UPDATE SET
    row_count = excluded.row_count,
    size_bytes = excluded.size_bytes,
    storage_path = excluded.storage_path,
    archived_at = excluded.archived_at`

func scanEncryptedMessage(row RowScanner) (EncryptedMessage, error) {
	var (
		messageID           any
		traceID             any
		tenantID            any
		conversationID      any
		deviceID            any
		senderID            any
		msgType             any
		direction           any
		encryptedContent    any
		encryptedKey        any
		nonce               any
		authTag             any
		keyVersion          any
		encryptionAlgorithm any
		createdAt           any
		updatedAt           any
	)
	if err := row.Scan(
		&messageID,
		&traceID,
		&tenantID,
		&conversationID,
		&deviceID,
		&senderID,
		&msgType,
		&direction,
		&encryptedContent,
		&encryptedKey,
		&nonce,
		&authTag,
		&keyVersion,
		&encryptionAlgorithm,
		&createdAt,
		&updatedAt,
	); err != nil {
		return EncryptedMessage{}, err
	}
	return EncryptedMessage{
		MessageID:           int64FromDB(messageID),
		TraceID:             stringFromDB(traceID),
		TenantID:            stringFromDB(tenantID),
		ConversationID:      stringFromDB(conversationID),
		DeviceID:            stringFromDB(deviceID),
		SenderID:            stringFromDB(senderID),
		MsgType:             defaultText(stringFromDB(msgType), "text"),
		Direction:           defaultText(stringFromDB(direction), "incoming"),
		EncryptedContent:    bytesFromDB(encryptedContent),
		EncryptedKey:        bytesFromDB(encryptedKey),
		Nonce:               bytesFromDB(nonce),
		AuthTag:             bytesFromDB(authTag),
		KeyVersion:          intFromDB(keyVersion),
		EncryptionAlgorithm: defaultText(stringFromDB(encryptionAlgorithm), "AES-256-GCM"),
		CreatedAt:           timeFromDB(createdAt),
		UpdatedAt:           timeFromDB(updatedAt),
	}, nil
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	if value.IsZero() {
		value = repository.now()
	}
	beijingValue := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return beijingValue.Format(time.RFC3339)
	}
	return beijingValue.Format("2006-01-02 15:04:05")
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeMessageIDs(values []int64) []int64 {
	seen := map[int64]struct{}{}
	ids := []int64{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}
	sort.Slice(ids, func(i int, j int) bool { return ids[i] < ids[j] })
	return ids
}

func positiveOrDefault(value int, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < 1 {
		return 1
	}
	return value
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegative64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func int64sToAny(values []int64) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
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

func bytesFromDB(value any) []byte {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return append([]byte(nil), typed...)
	case string:
		return []byte(typed)
	default:
		return []byte(fmt.Sprint(typed))
	}
}

func intFromDB(value any) int {
	return int(int64FromDB(value))
}

func int64FromDB(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int64:
		return typed
	case []byte:
		var parsed int64
		_, _ = fmt.Sscan(string(typed), &parsed)
		return parsed
	case string:
		var parsed int64
		_, _ = fmt.Sscan(typed, &parsed)
		return parsed
	default:
		var parsed int64
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

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}
