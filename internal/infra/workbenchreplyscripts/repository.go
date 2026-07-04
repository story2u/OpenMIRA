// Package workbenchreplyscripts reads and writes admin-managed quick reply
// scripts for the Go admin candidate.
package workbenchreplyscripts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the reply script repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes reply_scripts rows for admin candidates.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := ""
	if len(dialect) > 0 {
		resolvedDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// ListReplyScripts returns scripts ordered by newest update first.
func (repository *Repository) ListReplyScripts(ctx context.Context) ([]workbench.ReplyScriptRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench reply script database is not configured")
	}
	query := "SELECT script_id, title, content, category, enabled, target_audience, created_at, updated_at FROM reply_scripts ORDER BY updated_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanScripts(rows)
}

// UpsertReplyScript creates or updates one quick-reply script by script_id.
func (repository *Repository) UpsertReplyScript(ctx context.Context, command workbench.ReplyScriptCommand) (workbench.ReplyScriptRecord, error) {
	if repository.DB == nil {
		return workbench.ReplyScriptRecord{}, fmt.Errorf("workbench reply script database is not configured")
	}
	title := strings.TrimSpace(command.Title)
	if title == "" {
		return workbench.ReplyScriptRecord{}, workbench.ErrReplyScriptTitleRequired
	}
	content := strings.TrimSpace(command.Content)
	if content == "" {
		return workbench.ReplyScriptRecord{}, workbench.ErrReplyScriptContentRequired
	}
	scriptID := strings.TrimSpace(command.ScriptID)
	if scriptID == "" {
		scriptID = "script-" + randomHex(16)
	}
	category := strings.TrimSpace(command.Category)
	if category == "" {
		category = "default"
	}
	now := dbNow(repository.Dialect)
	if _, err := repository.DB.ExecContext(ctx, repository.upsertSQL(), scriptID, title, content, category, boolInt(command.Enabled), strings.TrimSpace(command.TargetAudience), now, now); err != nil {
		return workbench.ReplyScriptRecord{}, err
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT script_id, title, content, category, enabled, target_audience, created_at, updated_at FROM reply_scripts WHERE script_id = ?", scriptID)
	if err != nil {
		return workbench.ReplyScriptRecord{}, err
	}
	records, err := scanScripts(rows)
	if err != nil {
		return workbench.ReplyScriptRecord{}, err
	}
	if len(records) == 0 {
		return workbench.ReplyScriptRecord{}, fmt.Errorf("reply script was not found after upsert")
	}
	return records[0], nil
}

// DeleteReplyScript removes one script by id and reports whether it existed.
func (repository *Repository) DeleteReplyScript(ctx context.Context, scriptID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench reply script database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM reply_scripts WHERE script_id = ?", strings.TrimSpace(scriptID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO reply_scripts (script_id, title, content, category, enabled, target_audience, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(script_id) DO UPDATE SET
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    category = EXCLUDED.category,
    enabled = EXCLUDED.enabled,
    target_audience = EXCLUDED.target_audience,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO reply_scripts (script_id, title, content, category, enabled, target_audience, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    title = VALUES(title),
    content = VALUES(content),
    category = VALUES(category),
    enabled = VALUES(enabled),
    target_audience = VALUES(target_audience),
    updated_at = VALUES(updated_at)`
}

func scanScripts(rows RowsScanner) ([]workbench.ReplyScriptRecord, error) {
	defer rows.Close()
	records := make([]workbench.ReplyScriptRecord, 0)
	for rows.Next() {
		var scriptID any
		var title any
		var content any
		var category any
		var enabled any
		var targetAudience any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&scriptID, &title, &content, &category, &enabled, &targetAudience, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(scriptID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.ReplyScriptRecord{
			ScriptID:       normalizedID,
			Title:          stringFromDB(title),
			Content:        stringFromDB(content),
			Category:       stringFromDB(category),
			Enabled:        boolFromDB(enabled),
			TargetAudience: stringFromDB(targetAudience),
			CreatedAt:      timeFromDB(createdAt),
			UpdatedAt:      timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func dbNow(dialect string) any {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
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

func timeFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	default:
		return false
	}
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
