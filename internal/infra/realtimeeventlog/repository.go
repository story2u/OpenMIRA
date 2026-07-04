// Package realtimeeventlog reads the legacy realtime_event_log table.
package realtimeeventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/realtime"
)

const (
	// DialectMySQL is the production default backend.
	DialectMySQL = "mysql"
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

// Repository reads strong realtime event rows.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListAfterCursor returns events after a scope cursor, ordered ascending.
func (repository *Repository) ListAfterCursor(ctx context.Context, scopeKey string, afterCursor int64, limit int) ([]realtime.EventRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("realtime event log database is not configured")
	}
	if exists, err := repository.tableExists(ctx); err != nil || !exists {
		return []realtime.EventRecord{}, err
	}
	rows, err := repository.DB.QueryContext(ctx, fmt.Sprintf(`
SELECT
    scope_key,
    %s AS cursor_value,
    channel,
    event,
    topic,
    consistency,
    payload_json,
    created_at
FROM realtime_event_log
WHERE scope_key = ? AND %s > ?
ORDER BY %s ASC
LIMIT ?`, repository.cursorColumn(), repository.cursorColumn(), repository.cursorColumn()), strings.TrimSpace(scopeKey), afterCursor, normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]realtime.EventRecord, 0)
	for rows.Next() {
		var scope any
		var cursor any
		var channel any
		var event any
		var topic any
		var consistency any
		var payloadJSON any
		var createdAt any
		if err := rows.Scan(&scope, &cursor, &channel, &event, &topic, &consistency, &payloadJSON, &createdAt); err != nil {
			return nil, err
		}
		records = append(records, realtime.EventRecord{
			ScopeKey:    stringFromDB(scope),
			Cursor:      int64FromDB(cursor),
			Channel:     stringFromDB(channel),
			Event:       stringFromDB(event),
			Topic:       stringFromDB(topic),
			Consistency: defaultString(stringFromDB(consistency), "strong"),
			Payload:     jsonObjectFromDB(payloadJSON),
			CreatedAt:   nilIfBlank(createdAt),
		})
	}
	return records, rows.Err()
}

// AppendEvent upserts one strong realtime event log row.
func (repository *Repository) AppendEvent(ctx context.Context, record realtime.EventRecord) error {
	if repository.DB == nil {
		return fmt.Errorf("realtime event log database is not configured")
	}
	scopeKey := strings.TrimSpace(record.ScopeKey)
	if scopeKey == "" || record.Cursor <= 0 {
		return nil
	}
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = repository.DB.ExecContext(
		ctx,
		repository.appendSQL(),
		scopeKey,
		record.Cursor,
		strings.TrimSpace(record.Channel),
		strings.TrimSpace(record.Event),
		strings.TrimSpace(record.Topic),
		defaultString(record.Consistency, "strong"),
		string(payloadJSON),
		repository.dbNowParam(),
	)
	return err
}

// LatestCursor returns MAX(cursor) for one scope.
func (repository *Repository) LatestCursor(ctx context.Context, scopeKey string) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("realtime event log database is not configured")
	}
	if strings.TrimSpace(scopeKey) == "" {
		return 0, nil
	}
	if exists, err := repository.tableExists(ctx); err != nil || !exists {
		return 0, err
	}
	rows, err := repository.DB.QueryContext(ctx, fmt.Sprintf(`
SELECT MAX(%s) AS latest_cursor
FROM realtime_event_log
WHERE scope_key = ?`, repository.cursorColumn()), strings.TrimSpace(scopeKey))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var cursor any
	if err := rows.Scan(&cursor); err != nil {
		return 0, err
	}
	return int64FromDB(cursor), rows.Err()
}

func (repository *Repository) tableExists(ctx context.Context) (bool, error) {
	rows, err := repository.DB.QueryContext(ctx, "SELECT 1 FROM realtime_event_log WHERE 1 = 0")
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	return true, rows.Err()
}

func (repository *Repository) cursorColumn() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return "`cursor`"
	}
	return "cursor"
}

func (repository *Repository) appendSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return fmt.Sprintf(`
INSERT INTO realtime_event_log (
    scope_key, %s, channel, event, topic, consistency, payload_json, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    channel = VALUES(channel),
    event = VALUES(event),
    topic = VALUES(topic),
    consistency = VALUES(consistency),
    payload_json = VALUES(payload_json),
    created_at = VALUES(created_at)`, repository.cursorColumn())
	}
	return `
INSERT INTO realtime_event_log (
    scope_key, cursor, channel, event, topic, consistency, payload_json, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?)
ON CONFLICT(scope_key, cursor) DO UPDATE SET
    channel = EXCLUDED.channel,
    event = EXCLUDED.event,
    topic = EXCLUDED.topic,
    consistency = EXCLUDED.consistency,
    payload_json = EXCLUDED.payload_json,
    created_at = EXCLUDED.created_at`
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

func jsonObjectFromDB(value any) map[string]any {
	text := stringFromDB(value)
	if text == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func nilIfBlank(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		text := strings.TrimSpace(string(typed))
		if text == "" {
			return nil
		}
		return text
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return text
	default:
		return typed
	}
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func int64FromDB(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case []byte:
		return parseInt64Text(string(typed))
	case string:
		return parseInt64Text(typed)
	default:
		return parseInt64Text(fmt.Sprint(typed))
	}
}

func normalizeLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > realtime.MaxReplayLimit+1 {
		return realtime.MaxReplayLimit + 1
	}
	return limit
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseInt64Text(value string) int64 {
	var parsed int64
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("realtime event log database is not configured")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("realtime event log database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
