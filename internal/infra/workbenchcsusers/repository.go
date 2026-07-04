// Package workbenchcsusers reads customer-service users for assignment panels.
// It exposes only the management summary fields needed by the Go candidate
// route and leaves assignment counts to the assignment repository.
package workbenchcsusers

import (
	"context"
	"crypto/sha256"
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

// Queryer is the database/sql shape needed by the CS user repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes cs_users rows for management panel bootstrap.
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

// ListCSUsers returns enabled and disabled CS users in stable display order.
func (repository *Repository) ListCSUsers(ctx context.Context) ([]workbench.CSUserRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench cs user database is not configured")
	}
	query := "SELECT assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, password_hash, last_seen_at, created_at, updated_at FROM cs_users ORDER BY assignee_name ASC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanCSUsers(rows)
}

// GetCSUser returns one customer-service user by assignee id.
func (repository *Repository) GetCSUser(ctx context.Context, assigneeID string) (workbench.CSUserRecord, bool, error) {
	if repository.DB == nil {
		return workbench.CSUserRecord{}, false, fmt.Errorf("workbench cs user database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, password_hash, last_seen_at, created_at, updated_at FROM cs_users WHERE assignee_id = ?", strings.TrimSpace(assigneeID))
	if err != nil {
		return workbench.CSUserRecord{}, false, err
	}
	users, err := scanCSUsers(rows)
	if err != nil {
		return workbench.CSUserRecord{}, false, err
	}
	if len(users) == 0 {
		return workbench.CSUserRecord{}, false, nil
	}
	return users[0], true, nil
}

// UpsertCSUser creates or updates a customer-service user.
func (repository *Repository) UpsertCSUser(ctx context.Context, command workbench.CSUserCommand) (workbench.CSUserRecord, error) {
	if repository.DB == nil {
		return workbench.CSUserRecord{}, fmt.Errorf("workbench cs user database is not configured")
	}
	assigneeID := strings.TrimSpace(command.AssigneeID)
	assigneeName := strings.TrimSpace(command.AssigneeName)
	role := strings.TrimSpace(command.Role)
	if role == "" {
		role = "cs"
	}
	now := dbNow(repository.Dialect)
	password := strings.TrimSpace(command.Password)
	if password != "" {
		if _, err := repository.DB.ExecContext(ctx, repository.upsertWithPasswordSQL(), assigneeID, assigneeName, role, boolInt(command.Enabled), boolInt(command.AIEnabled), maxInt(0, command.MaxSessions), sha256Hex(password), now, now); err != nil {
			return workbench.CSUserRecord{}, err
		}
	} else {
		if _, err := repository.DB.ExecContext(ctx, repository.upsertWithoutPasswordSQL(), assigneeID, assigneeName, role, boolInt(command.Enabled), boolInt(command.AIEnabled), maxInt(0, command.MaxSessions), now, now); err != nil {
			return workbench.CSUserRecord{}, err
		}
	}
	user, ok, err := repository.GetCSUser(ctx, assigneeID)
	if err != nil {
		return workbench.CSUserRecord{}, err
	}
	if !ok {
		return workbench.CSUserRecord{}, fmt.Errorf("cs user was not found after upsert")
	}
	return user, nil
}

// DeleteCSUser removes one customer-service user by assignee id.
func (repository *Repository) DeleteCSUser(ctx context.Context, assigneeID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench cs user database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM cs_users WHERE assignee_id = ?", strings.TrimSpace(assigneeID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (repository *Repository) upsertWithPasswordSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO cs_users (assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(assignee_id) DO UPDATE SET
    assignee_name = EXCLUDED.assignee_name,
    role = EXCLUDED.role,
    enabled = EXCLUDED.enabled,
    ai_enabled = EXCLUDED.ai_enabled,
    max_sessions = EXCLUDED.max_sessions,
    password_hash = EXCLUDED.password_hash,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO cs_users (assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    assignee_name = VALUES(assignee_name),
    role = VALUES(role),
    enabled = VALUES(enabled),
    ai_enabled = VALUES(ai_enabled),
    max_sessions = VALUES(max_sessions),
    password_hash = VALUES(password_hash),
    updated_at = VALUES(updated_at)`
}

func (repository *Repository) upsertWithoutPasswordSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO cs_users (assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(assignee_id) DO UPDATE SET
    assignee_name = EXCLUDED.assignee_name,
    role = EXCLUDED.role,
    enabled = EXCLUDED.enabled,
    ai_enabled = EXCLUDED.ai_enabled,
    max_sessions = EXCLUDED.max_sessions,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO cs_users (assignee_id, assignee_name, role, enabled, ai_enabled, max_sessions, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    assignee_name = VALUES(assignee_name),
    role = VALUES(role),
    enabled = VALUES(enabled),
    ai_enabled = VALUES(ai_enabled),
    max_sessions = VALUES(max_sessions),
    updated_at = VALUES(updated_at)`
}

func scanCSUsers(rows RowsScanner) ([]workbench.CSUserRecord, error) {
	defer rows.Close()
	records := make([]workbench.CSUserRecord, 0)
	for rows.Next() {
		var assigneeID any
		var assigneeName any
		var role any
		var enabled any
		var aiEnabled any
		var maxSessions any
		var passwordHash any
		var lastSeenAt any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&assigneeID, &assigneeName, &role, &enabled, &aiEnabled, &maxSessions, &passwordHash, &lastSeenAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(assigneeID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.CSUserRecord{
			AssigneeID:   normalizedID,
			AssigneeName: stringFromDB(assigneeName),
			Role:         stringFromDB(role),
			Enabled:      boolFromDB(enabled),
			AIEnabled:    boolFromDB(aiEnabled),
			MaxSessions:  intFromDB(maxSessions),
			HasPassword:  stringFromDB(passwordHash) != "",
			LastSeenAt:   stringFromDB(lastSeenAt),
			CreatedAt:    stringFromDB(createdAt),
			UpdatedAt:    stringFromDB(updatedAt),
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

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
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

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(typed)), "%d", &parsed)
		return parsed
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(typed)), "%d", &parsed)
		return parsed
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
