// Package adminusers stores management account credentials for the Go backend.
package adminusers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"im-go/internal/auth"
	"im-go/internal/session"
)

const (
	// DefaultUsername is the built-in bootstrap administrator account.
	DefaultUsername = "root"
	// DefaultPassword must be changed after the first successful login.
	DefaultPassword = "1234567890"

	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// Queryer is the database/sql shape needed by the admin user repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes admin_users rows.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

var _ session.AdminUserStore = (*Repository)(nil)

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// EnsureSchema creates the admin credential table when it is missing.
func (repository *Repository) EnsureSchema(ctx context.Context) error {
	if repository.DB == nil {
		return fmt.Errorf("admin user database is not configured")
	}
	_, err := repository.DB.ExecContext(ctx, repository.createTableSQL())
	return err
}

// EnsureDefaultAdmin inserts the bootstrap root account without overwriting changes.
func (repository *Repository) EnsureDefaultAdmin(ctx context.Context) error {
	if repository.DB == nil {
		return fmt.Errorf("admin user database is not configured")
	}
	passwordHash, err := auth.HashPassword(DefaultPassword)
	if err != nil {
		return err
	}
	now := repository.dbNowParam()
	_, err = repository.DB.ExecContext(ctx, repository.insertDefaultSQL(), DefaultUsername, passwordHash, repository.dbBoolParam(true), now, now)
	return err
}

// GetAdminUser loads one admin user by username.
func (repository *Repository) GetAdminUser(ctx context.Context, username string) (session.AdminUser, bool, error) {
	if repository.DB == nil {
		return session.AdminUser{}, false, fmt.Errorf("admin user database is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return session.AdminUser{}, false, nil
	}
	var storedUsername any
	var passwordHash any
	var passwordChangeRequired any
	err := repository.DB.QueryRowContext(ctx, repository.selectByUsernameSQL(), username).Scan(&storedUsername, &passwordHash, &passwordChangeRequired)
	if err != nil {
		if err == sql.ErrNoRows {
			return session.AdminUser{}, false, nil
		}
		return session.AdminUser{}, false, err
	}
	return session.AdminUser{
		Username:               stringFromDB(storedUsername),
		PasswordHash:           stringFromDB(passwordHash),
		PasswordChangeRequired: boolFromDB(passwordChangeRequired),
	}, true, nil
}

// UpdateAdminPassword replaces the stored password hash and change-required flag.
func (repository *Repository) UpdateAdminPassword(ctx context.Context, username string, passwordHash string, passwordChangeRequired bool) error {
	if repository.DB == nil {
		return fmt.Errorf("admin user database is not configured")
	}
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" || passwordHash == "" {
		return fmt.Errorf("admin username and password hash are required")
	}
	_, err := repository.DB.ExecContext(ctx, repository.updatePasswordSQL(), passwordHash, repository.dbBoolParam(passwordChangeRequired), repository.dbNowParam(), username)
	return err
}

// RecordAdminLogin records the most recent successful admin login.
func (repository *Repository) RecordAdminLogin(ctx context.Context, username string) error {
	if repository.DB == nil {
		return fmt.Errorf("admin user database is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	now := repository.dbNowParam()
	_, err := repository.DB.ExecContext(ctx, repository.recordLoginSQL(), now, now, username)
	return err
}

func (repository *Repository) createTableSQL() string {
	if repository.isPostgres() {
		return `CREATE TABLE IF NOT EXISTS admin_users (
    username TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    password_change_required BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMPTZ NULL
)`
	}
	return `CREATE TABLE IF NOT EXISTS admin_users (
    username VARCHAR(191) PRIMARY KEY,
    password_hash VARCHAR(255) NOT NULL,
    password_change_required TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME NULL
)`
}

func (repository *Repository) insertDefaultSQL() string {
	if repository.isPostgres() {
		return `INSERT INTO admin_users (username, password_hash, password_change_required, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT(username) DO NOTHING`
	}
	return `INSERT IGNORE INTO admin_users (username, password_hash, password_change_required, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
}

func (repository *Repository) selectByUsernameSQL() string {
	if repository.isPostgres() {
		return `SELECT username, password_hash, password_change_required FROM admin_users WHERE username = $1`
	}
	return `SELECT username, password_hash, password_change_required FROM admin_users WHERE username = ?`
}

func (repository *Repository) updatePasswordSQL() string {
	if repository.isPostgres() {
		return `UPDATE admin_users SET password_hash = $1, password_change_required = $2, updated_at = $3 WHERE username = $4`
	}
	return `UPDATE admin_users SET password_hash = ?, password_change_required = ?, updated_at = ? WHERE username = ?`
}

func (repository *Repository) recordLoginSQL() string {
	if repository.isPostgres() {
		return `UPDATE admin_users SET last_login_at = $1, updated_at = $2 WHERE username = $3`
	}
	return `UPDATE admin_users SET last_login_at = ?, updated_at = ? WHERE username = ?`
}

func (repository *Repository) dbNowParam() any {
	now := time.Now()
	if repository.Now != nil {
		now = repository.Now()
	}
	if repository.isPostgres() {
		return now.UTC()
	}
	return now.In(beijingLocation).Format("2006-01-02 15:04:05")
}

func (repository *Repository) dbBoolParam(value bool) any {
	if repository.isPostgres() {
		return value
	}
	if value {
		return 1
	}
	return 0
}

func (repository *Repository) isPostgres() bool {
	return strings.EqualFold(strings.TrimSpace(repository.Dialect), DialectPostgres)
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

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is nil")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
