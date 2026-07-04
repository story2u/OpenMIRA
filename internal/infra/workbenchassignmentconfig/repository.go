// Package workbenchassignmentconfig reads and writes allocation config in
// system_settings for guarded Go assignment config candidates.
package workbenchassignmentconfig

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the settings repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads assignment config JSON values from system_settings.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// GetAssignmentConfigValue returns a raw JSON config string or an empty value.
func (repository *Repository) GetAssignmentConfigValue(ctx context.Context, key string) (string, error) {
	if repository.DB == nil {
		return "", fmt.Errorf("workbench assignment config database is not configured")
	}
	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return "", nil
	}
	query := fmt.Sprintf("SELECT value FROM system_settings WHERE %s = ?", repository.keyColumn())
	rows, err := repository.DB.QueryContext(ctx, query, normalizedKey)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var value any
		if err := rows.Scan(&value); err != nil {
			return "", err
		}
		return stringFromDB(value), rows.Err()
	}
	return "", rows.Err()
}

// SetAssignmentConfigValue upserts one system_settings JSON value.
func (repository *Repository) SetAssignmentConfigValue(ctx context.Context, key string, value string) error {
	if repository.DB == nil {
		return fmt.Errorf("workbench assignment config database is not configured")
	}
	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx, repository.upsertSQL(), normalizedKey, value, dbNow(repository.Dialect))
	return err
}

func (repository *Repository) keyColumn() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `"key"`
	}
	return "`key`"
}

func (repository *Repository) upsertSQL() string {
	keyColumn := repository.keyColumn()
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return fmt.Sprintf(`
INSERT INTO system_settings (%s, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(%s) DO UPDATE SET
    value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at`, keyColumn, keyColumn)
	}
	return fmt.Sprintf(`
INSERT INTO system_settings (%s, value, updated_at)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE
    value = VALUES(value),
    updated_at = VALUES(updated_at)`, keyColumn)
}

func dbNow(dialect string) any {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
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
