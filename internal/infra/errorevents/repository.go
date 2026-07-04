// Package errorevents writes legacy error_events records.
package errorevents

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/clienterrors"
	"wework-go/internal/infra/sqldb"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Execer is the database/sql shape needed by Repository.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository inserts browser-derived events into the legacy error_events table.
type Repository struct {
	DB          Execer
	Dialect     string
	Now         func() time.Time
	NextEventID func() string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: strings.TrimSpace(dialect)}
}

// CaptureClientEvent persists one high-severity frontend log as error_events.
func (repository *Repository) CaptureClientEvent(ctx context.Context, event clienterrors.ErrorEvent) error {
	if repository == nil || repository.DB == nil {
		return fmt.Errorf("error events database is not configured")
	}
	contextJSON, err := json.Marshal(normalizeContext(event.Context))
	if err != nil {
		return err
	}
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = repository.now()
	}
	createdAt := repository.now()
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(),
		repository.eventID(),
		cleanText(event.TraceID),
		defaultText(strings.ToUpper(cleanText(event.Level)), "ERROR"),
		defaultText(event.SourceType, "client"),
		defaultText(event.EventCategory, "client_apm"),
		defaultText(event.EventCode, event.Action),
		defaultText(event.Module, "client.web"),
		cleanText(event.Action),
		cleanText(event.DeviceID),
		cleanText(event.TenantID),
		cleanText(event.ConversationID),
		cleanText(event.TaskID),
		cleanText(event.WeWorkUserID),
		cleanText(event.ScopeType),
		cleanText(event.ScopeID),
		cleanText(event.ErrorType),
		defaultText(truncateText(event.Detail, 4096), defaultText(event.ErrorType, "event")),
		nullableText(truncateText(event.StackTrace, 4096)),
		string(contextJSON),
		repository.dbTimeParam(occurredAt),
		repository.dbTimeParam(createdAt),
	)
	return err
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), sqldb.DialectPostgres) {
		return postgresUpsertSQL
	}
	return mysqlUpsertSQL
}

func (repository *Repository) eventID() string {
	if repository.NextEventID != nil {
		return cleanText(repository.NextEventID())
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("evt-%d", repository.now().UnixNano())
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now()
	}
	return time.Now()
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	if value.IsZero() {
		value = repository.now()
	}
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), sqldb.DialectPostgres) {
		return beijing
	}
	return beijing.Format("2006-01-02 15:04:05")
}

func normalizeContext(context map[string]any) map[string]any {
	if context == nil {
		return map[string]any{}
	}
	return context
}

func cleanText(value string) string {
	return strings.TrimSpace(value)
}

func defaultText(value string, fallback string) string {
	value = cleanText(value)
	if value == "" {
		return cleanText(fallback)
	}
	return value
}

func nullableText(value string) any {
	value = cleanText(value)
	if value == "" {
		return nil
	}
	return value
}

func truncateText(value string, limit int) string {
	value = cleanText(value)
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}

const mysqlUpsertSQL = `
INSERT INTO error_events (
    event_id, trace_id, level, source_type, event_category, event_code, module, action,
    device_id, tenant_id, conversation_id, task_id, wework_user_id, scope_type, scope_id,
    error_type, error_message, stack_trace, context_json, occurred_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    trace_id = VALUES(trace_id),
    level = VALUES(level),
    source_type = VALUES(source_type),
    event_category = VALUES(event_category),
    event_code = VALUES(event_code),
    module = VALUES(module),
    action = VALUES(action),
    device_id = VALUES(device_id),
    tenant_id = VALUES(tenant_id),
    conversation_id = VALUES(conversation_id),
    task_id = VALUES(task_id),
    wework_user_id = VALUES(wework_user_id),
    scope_type = VALUES(scope_type),
    scope_id = VALUES(scope_id),
    error_type = VALUES(error_type),
    error_message = VALUES(error_message),
    stack_trace = VALUES(stack_trace),
    context_json = VALUES(context_json),
    occurred_at = VALUES(occurred_at),
    created_at = VALUES(created_at)`

const postgresUpsertSQL = `
INSERT INTO error_events (
    event_id, trace_id, level, source_type, event_category, event_code, module, action,
    device_id, tenant_id, conversation_id, task_id, wework_user_id, scope_type, scope_id,
    error_type, error_message, stack_trace, context_json, occurred_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO UPDATE SET
    trace_id = EXCLUDED.trace_id,
    level = EXCLUDED.level,
    source_type = EXCLUDED.source_type,
    event_category = EXCLUDED.event_category,
    event_code = EXCLUDED.event_code,
    module = EXCLUDED.module,
    action = EXCLUDED.action,
    device_id = EXCLUDED.device_id,
    tenant_id = EXCLUDED.tenant_id,
    conversation_id = EXCLUDED.conversation_id,
    task_id = EXCLUDED.task_id,
    wework_user_id = EXCLUDED.wework_user_id,
    scope_type = EXCLUDED.scope_type,
    scope_id = EXCLUDED.scope_id,
    error_type = EXCLUDED.error_type,
    error_message = EXCLUDED.error_message,
    stack_trace = EXCLUDED.stack_trace,
    context_json = EXCLUDED.context_json,
    occurred_at = EXCLUDED.occurred_at,
    created_at = EXCLUDED.created_at`
