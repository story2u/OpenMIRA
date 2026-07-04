// Package messagestore reads legacy conversation message pages from SQL.
// It mirrors the Python message repository pagination contracts and hydrates
// bounded current-page archive/media/contact facts without changing paging SQL.
package messagestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/messages"
	"wework-go/internal/tasks"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the message repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Executor is the write shape used by revoke state updates.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// MediaURLBuilder signs or rewrites completed archive media object references.
type MediaURLBuilder interface {
	BuildAccessURL(taskID string, objectURL string) string
}

// Repository reads messages and message_revoke_states.
type Repository struct {
	DB              Queryer
	Dialect         string
	MediaURLBuilder MediaURLBuilder
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	repositoryDialect := ""
	if len(dialect) > 0 {
		repositoryDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: repositoryDialect}
}

// List returns a conversation message page using legacy pagination semantics.
func (repository *Repository) List(ctx context.Context, query messages.Query) (messages.Page, error) {
	if repository.DB == nil {
		return messages.Page{}, fmt.Errorf("conversation messages database is not configured")
	}
	query = query.Normalized()
	if query.ConversationID == "" {
		return messages.Page{}, fmt.Errorf("conversation_id is required")
	}
	scope, err := repository.lookupScope(ctx, query.ConversationID)
	if err != nil {
		return messages.Page{}, err
	}
	switch {
	case query.Before != nil:
		return repository.listBefore(ctx, scope, query)
	case query.After != nil:
		return repository.listAfter(ctx, scope, query)
	case query.Offset > 0:
		return repository.listPaged(ctx, scope, query)
	default:
		return repository.listLatest(ctx, scope, query)
	}
}

// GetMessageByTrace returns one legacy message row with revoke overlay by trace_id.
func (repository *Repository) GetMessageByTrace(ctx context.Context, traceID string) (messages.Record, bool, error) {
	if repository.DB == nil {
		return messages.Record{}, false, fmt.Errorf("conversation messages database is not configured")
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return messages.Record{}, false, fmt.Errorf("trace_id is required")
	}
	rows, err := repository.DB.QueryContext(ctx, repository.rebind(selectMessageByTraceSQL()), traceID)
	if err != nil {
		return messages.Record{}, false, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return messages.Record{}, false, err
	}
	if err := rows.Err(); err != nil {
		return messages.Record{}, false, err
	}
	if len(records) == 0 {
		return messages.Record{}, false, nil
	}
	records, err = repository.hydrateRecords(ctx, records[:1])
	if err != nil {
		return messages.Record{}, false, err
	}
	return records[0], true, nil
}

// UpdateMessageRevokeStatus upserts the side-table revoke state.
func (repository *Repository) UpdateMessageRevokeStatus(ctx context.Context, update tasks.MessageRevokeUpdate) error {
	if repository.DB == nil {
		return fmt.Errorf("conversation messages database is not configured")
	}
	executor, ok := repository.DB.(Executor)
	if !ok {
		return fmt.Errorf("conversation messages database does not support writes")
	}
	traceID := strings.TrimSpace(update.TraceID)
	taskID := strings.TrimSpace(update.TaskID)
	status := strings.ToLower(strings.TrimSpace(update.RevokeStatus))
	if traceID == "" {
		return nil
	}
	if status == "" {
		return nil
	}
	now := time.Now().In(beijingLocation)
	var revokedAt any
	if update.RevokedAt != nil && !update.RevokedAt.IsZero() {
		revokedAt = update.RevokedAt.In(beijingLocation)
	}
	_, err := executor.ExecContext(ctx, repository.rebind(updateMessageRevokeSQL(repository.Dialect)),
		traceID,
		taskID,
		status,
		strings.TrimSpace(update.RevokeError),
		revokedAt,
		now,
		now,
	)
	return err
}

func (repository *Repository) listLatest(ctx context.Context, scope lookupScope, query messages.Query) (messages.Page, error) {
	sqlText := selectPageSQL(scope.WhereClause, "", "DESC", "ASC")
	rows, err := repository.DB.QueryContext(ctx, sqlText, append(scope.Args, query.Limit+1)...)
	if err != nil {
		return messages.Page{}, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return messages.Page{}, err
	}
	if err := rows.Err(); err != nil {
		return messages.Page{}, err
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[len(records)-query.Limit:]
	}
	records, err = repository.hydrateRecords(ctx, records)
	if err != nil {
		return messages.Page{}, err
	}
	total := len(records)
	if hasMore {
		total++
	}
	return messages.Page{Records: records, Total: total, HasMore: hasMore}, nil
}

func (repository *Repository) listPaged(ctx context.Context, scope lookupScope, query messages.Query) (messages.Page, error) {
	total, err := repository.count(ctx, scope)
	if err != nil {
		return messages.Page{}, err
	}
	sqlText := selectPageSQL(scope.WhereClause, "", "DESC", "ASC") + " OFFSET ?"
	rows, err := repository.DB.QueryContext(ctx, sqlText, append(scope.Args, query.Limit, query.Offset)...)
	if err != nil {
		return messages.Page{}, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return messages.Page{}, err
	}
	if err := rows.Err(); err != nil {
		return messages.Page{}, err
	}
	records, err = repository.hydrateRecords(ctx, records)
	if err != nil {
		return messages.Page{}, err
	}
	return messages.Page{Records: records, Total: total, HasMore: query.Offset+query.Limit < total}, nil
}

func (repository *Repository) listAfter(ctx context.Context, scope lookupScope, query messages.Query) (messages.Page, error) {
	total, err := repository.count(ctx, scope)
	if err != nil {
		return messages.Page{}, err
	}
	cursorWhere, cursorArgs := afterCursorClause(query.After)
	sqlText := selectPageSQL(scope.WhereClause, cursorWhere, "ASC", "ASC")
	args := append(append(scope.Args, cursorArgs...), query.Limit+1)
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return messages.Page{}, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return messages.Page{}, err
	}
	if err := rows.Err(); err != nil {
		return messages.Page{}, err
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	records, err = repository.hydrateRecords(ctx, records)
	if err != nil {
		return messages.Page{}, err
	}
	return messages.Page{Records: records, Total: total, HasMore: hasMore}, nil
}

func (repository *Repository) listBefore(ctx context.Context, scope lookupScope, query messages.Query) (messages.Page, error) {
	total, err := repository.count(ctx, scope)
	if err != nil {
		return messages.Page{}, err
	}
	cursorWhere, cursorArgs := beforeCursorClause(query.Before)
	sqlText := selectPageSQL(scope.WhereClause, cursorWhere, "DESC", "ASC")
	args := append(append(scope.Args, cursorArgs...), query.Limit+1)
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return messages.Page{}, err
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		return messages.Page{}, err
	}
	if err := rows.Err(); err != nil {
		return messages.Page{}, err
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[1:]
	}
	records, err = repository.hydrateRecords(ctx, records)
	if err != nil {
		return messages.Page{}, err
	}
	return messages.Page{Records: records, Total: total, HasMore: hasMore}, nil
}

func (repository *Repository) count(ctx context.Context, scope lookupScope) (int, error) {
	var total any
	err := repository.DB.QueryRowContext(ctx, "SELECT COUNT(*) AS cnt FROM messages WHERE "+scope.WhereClause, scope.Args...).Scan(&total)
	if err != nil {
		return 0, err
	}
	return intFromDB(total), nil
}

type lookupScope struct {
	WhereClause string
	Args        []any
}

func (repository *Repository) lookupScope(ctx context.Context, conversationID string) (lookupScope, error) {
	normalized := strings.TrimSpace(conversationID)
	if normalized == "" {
		return lookupScope{}, fmt.Errorf("conversation_id is required")
	}
	args := []any{normalized, normalized}
	sqlText := "SELECT conversation_id, conversation_key, conversation_pk FROM conversations WHERE conversation_id = ? OR conversation_key = ?"
	if numericPK, ok := parseInt64(normalized); ok {
		sqlText += " OR conversation_pk = ?"
		args = append(args, numericPK)
	}
	sqlText += " LIMIT 1"
	var rowConversationID any
	var rowConversationKey any
	var rowConversationPK any
	err := repository.DB.QueryRowContext(ctx, sqlText, args...).Scan(&rowConversationID, &rowConversationKey, &rowConversationPK)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return lookupScope{}, err
	}
	if err == nil {
		resolvedID := strings.TrimSpace(stringFromDB(rowConversationID))
		resolvedKey := strings.TrimSpace(stringFromDB(rowConversationKey))
		resolvedPK, hasPK := int64FromDB(rowConversationPK)
		switch {
		case hasPK && resolvedKey != "" && resolvedKey != resolvedID:
			return lookupScope{WhereClause: "(conversation_id = ? OR conversation_key = ? OR conversation_pk = ?)", Args: []any{resolvedID, resolvedKey, resolvedPK}}, nil
		case hasPK:
			return lookupScope{WhereClause: "(conversation_id = ? OR conversation_pk = ?)", Args: []any{resolvedID, resolvedPK}}, nil
		case resolvedKey != "" && resolvedKey != resolvedID:
			return lookupScope{WhereClause: "(conversation_id = ? OR conversation_key = ?)", Args: []any{resolvedID, resolvedKey}}, nil
		case resolvedID != "":
			return lookupScope{WhereClause: "conversation_id = ?", Args: []any{resolvedID}}, nil
		}
	}
	if numericPK, ok := parseInt64(normalized); ok {
		return lookupScope{WhereClause: "(conversation_id = ? OR conversation_key = ? OR conversation_pk = ?)", Args: []any{normalized, normalized, numericPK}}, nil
	}
	return lookupScope{WhereClause: "(conversation_id = ? OR conversation_key = ?)", Args: []any{normalized, normalized}}, nil
}

func selectMessageByTraceSQL() string {
	return `
SELECT m.*, COALESCE(revoke.revoke_status, '') AS revoke_status, COALESCE(revoke.revoke_task_id, '') AS revoke_task_id, COALESCE(revoke.revoke_error, '') AS revoke_error, revoke.revoked_at AS revoked_at
FROM messages m
LEFT JOIN message_revoke_states revoke ON revoke.trace_id = m.trace_id
WHERE m.trace_id = ?
LIMIT 1`
}

func selectPageSQL(scopeWhere string, cursorWhere string, innerOrder string, outerOrder string) string {
	whereSQL := scopeWhere
	if strings.TrimSpace(cursorWhere) != "" {
		whereSQL = "(" + whereSQL + ") AND " + cursorWhere
	}
	return `
SELECT recent_messages.*, COALESCE(revoke.revoke_status, '') AS revoke_status, COALESCE(revoke.revoke_task_id, '') AS revoke_task_id, COALESCE(revoke.revoke_error, '') AS revoke_error, revoke.revoked_at AS revoked_at
FROM (
    SELECT *
    FROM messages
    WHERE ` + whereSQL + `
    ORDER BY timestamp ` + innerOrder + `, COALESCE(message_id, 0) ` + innerOrder + `, trace_id ` + innerOrder + `
    LIMIT ?
) recent_messages
LEFT JOIN message_revoke_states revoke ON revoke.trace_id = recent_messages.trace_id
ORDER BY recent_messages.timestamp ` + outerOrder + `, COALESCE(recent_messages.message_id, 0) ` + outerOrder + `, recent_messages.trace_id ` + outerOrder
}

func updateMessageRevokeSQL(dialect string) string {
	if strings.EqualFold(strings.TrimSpace(dialect), DialectPostgres) {
		return `
INSERT INTO message_revoke_states (
    trace_id, revoke_task_id, revoke_status, revoke_error, revoked_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (trace_id) DO UPDATE SET
    revoke_task_id = CASE WHEN excluded.revoke_task_id != '' THEN excluded.revoke_task_id ELSE message_revoke_states.revoke_task_id END,
    revoke_status = excluded.revoke_status,
    revoke_error = excluded.revoke_error,
    revoked_at = CASE WHEN excluded.revoked_at IS NOT NULL THEN excluded.revoked_at ELSE message_revoke_states.revoked_at END,
    updated_at = excluded.updated_at`
	}
	return `
INSERT INTO message_revoke_states (
    trace_id, revoke_task_id, revoke_status, revoke_error, revoked_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    revoke_task_id = CASE WHEN VALUES(revoke_task_id) != '' THEN VALUES(revoke_task_id) ELSE revoke_task_id END,
    revoke_status = VALUES(revoke_status),
    revoke_error = VALUES(revoke_error),
    revoked_at = CASE WHEN VALUES(revoked_at) IS NOT NULL THEN VALUES(revoked_at) ELSE revoked_at END,
    updated_at = VALUES(updated_at)`
}

func (repository *Repository) rebind(query string) string {
	if !strings.EqualFold(strings.TrimSpace(repository.Dialect), DialectPostgres) {
		return query
	}
	var builder strings.Builder
	index := 1
	for _, char := range query {
		if char == '?' {
			builder.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func afterCursorClause(cursor *messages.Cursor) (string, []any) {
	if cursor == nil {
		return "", nil
	}
	timestamp := cursorTimestampParam(cursor)
	if cursor.MessageID == nil {
		return "(timestamp > ? OR (timestamp = ? AND trace_id > ?))", []any{timestamp, timestamp, cursor.TraceID}
	}
	return `(timestamp > ? OR (timestamp = ? AND (COALESCE(message_id, 0) > ? OR (COALESCE(message_id, 0) = ? AND trace_id > ?))))`,
		[]any{timestamp, timestamp, *cursor.MessageID, *cursor.MessageID, cursor.TraceID}
}

func beforeCursorClause(cursor *messages.Cursor) (string, []any) {
	if cursor == nil {
		return "", nil
	}
	timestamp := cursorTimestampParam(cursor)
	if cursor.MessageID == nil {
		return "(timestamp < ? OR (timestamp = ? AND trace_id < ?))", []any{timestamp, timestamp, cursor.TraceID}
	}
	return `(timestamp < ? OR (timestamp = ? AND (COALESCE(message_id, 0) < ? OR (COALESCE(message_id, 0) = ? AND trace_id < ?))))`,
		[]any{timestamp, timestamp, *cursor.MessageID, *cursor.MessageID, cursor.TraceID}
}

func cursorTimestampParam(cursor *messages.Cursor) time.Time {
	return cursor.Timestamp.In(beijingLocation)
}

func scanRecords(rows RowsScanner) ([]messages.Record, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	records := make([]messages.Record, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for index := range values {
			dest[index] = &values[index]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for index, column := range columns {
			row[column] = values[index]
		}
		records = append(records, recordFromRow(row))
	}
	return records, nil
}

func recordFromRow(row map[string]any) messages.Record {
	return messages.Record{
		MessageID:                   int64PtrFromDB(row["message_id"]),
		TraceID:                     stringFromDB(row["trace_id"]),
		ArchiveMsgID:                stringFromDB(row["archive_msgid"]),
		TenantID:                    stringFromDB(row["tenant_id"]),
		ConversationID:              stringFromDB(row["conversation_id"]),
		DeviceID:                    stringFromDB(row["device_id"]),
		SenderID:                    stringFromDB(row["sender_id"]),
		SenderName:                  stringFromDB(row["sender_name"]),
		SenderAvatar:                stringFromDB(row["sender_avatar"]),
		SenderRemark:                stringFromDB(row["sender_remark"]),
		Content:                     stringFromDB(row["content"]),
		MsgType:                     stringFromDB(row["msg_type"]),
		Direction:                   stringFromDB(row["direction"]),
		MessageOrigin:               stringFromDB(row["message_origin"]),
		TaskID:                      stringFromDB(row["task_id"]),
		SendStatus:                  stringFromDB(row["send_status"]),
		SendError:                   stringFromDB(row["send_error"]),
		RevokeStatus:                stringFromDB(row["revoke_status"]),
		RevokeTaskID:                stringFromDB(row["revoke_task_id"]),
		RevokeError:                 stringFromDB(row["revoke_error"]),
		RevokedAt:                   timePtrFromDB(row["revoked_at"]),
		Timestamp:                   timeFromDB(row["timestamp"]),
		CreatedAt:                   timeFromDB(row["created_at"]),
		ArchiveSeq:                  int64PtrFromDB(row["archive_seq"]),
		ArchiveMsgtime:              int64PtrFromDB(firstNonNil(row["archive_msgtime_ms"], row["archive_msgtime"])),
		ArchiveTypeRaw:              stringFromDB(row["archive_msg_type_raw"]),
		MediaURL:                    stringFromDB(row["media_url"]),
		MediaReady:                  boolFromDB(row["media_ready"]),
		MediaStatus:                 stringFromDB(row["media_status"]),
		MediaTaskID:                 stringFromDB(row["media_task_id"]),
		FileName:                    stringFromDB(row["file_name"]),
		MediaFingerprint:            stringFromDB(row["media_fingerprint"]),
		MediaSizeBytes:              int64ValueFromDB(row["media_size_bytes"]),
		VoiceDurationSec:            intFromDB(row["voice_duration_sec"]),
		VoiceText:                   stringFromDB(row["voice_text"]),
		VoiceTranscriptionStatus:    stringFromDB(row["voice_transcription_status"]),
		VoiceTranscriptionError:     stringFromDB(row["voice_transcription_error"]),
		VoiceTranscriptionExecuteID: stringFromDB(row["voice_transcription_execute_id"]),
	}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is not configured")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
