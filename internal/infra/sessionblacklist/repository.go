// Package sessionblacklist adapts the legacy session_blacklist table for Go.
// It is used by JWT verification and candidate refresh/logout flows while the
// table name, columns, and upsert semantics stay compatible with Python.
package sessionblacklist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"wework-go/internal/auth"
)

const (
	// DialectMySQL stores DATETIME values in Beijing local wall time.
	DialectMySQL = "mysql"
	// DialectPostgres stores timestamp parameters with explicit +08:00 offset.
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// Queryer is the database/sql shape needed by the blacklist repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads revoked JWT ids from session_blacklist.
type Repository struct {
	DB            Queryer
	Dialect       string
	Now           func() time.Time
	PruneInterval time.Duration

	mu        sync.Mutex
	lastPrune time.Time
}

var _ auth.Blacklist = (*Repository)(nil)
var _ auth.Revoker = (*Repository)(nil)

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// Contains reports whether jti has been revoked and prunes expired rows first.
func (repository *Repository) Contains(ctx context.Context, jti string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("session blacklist database is not configured")
	}
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return false, nil
	}
	if _, err := repository.MaybePruneExpired(ctx); err != nil {
		return false, err
	}
	var storedJTI string
	err := repository.DB.QueryRowContext(
		ctx,
		"SELECT jti FROM session_blacklist WHERE jti = ?",
		jti,
	).Scan(&storedJTI)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Add records jti as revoked until expiresAt using the legacy upsert contract.
func (repository *Repository) Add(ctx context.Context, jti string, expiresAt time.Time) error {
	if repository.DB == nil {
		return fmt.Errorf("session blacklist database is not configured")
	}
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(
		ctx,
		repository.upsertSQL(),
		jti,
		repository.dbTimeParam(expiresAt),
		repository.dbNowParam(),
	)
	return err
}

// MaybePruneExpired deletes expired rows no more often than the configured interval.
func (repository *Repository) MaybePruneExpired(ctx context.Context) (int64, error) {
	now := repository.clock()
	interval := repository.PruneInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if !repository.lastPrune.IsZero() && now.Sub(repository.lastPrune) < interval {
		return 0, nil
	}
	rowsAffected, err := repository.PruneExpired(ctx)
	if err != nil {
		return 0, err
	}
	repository.lastPrune = repository.clock()
	return rowsAffected, nil
}

// PruneExpired removes blacklist rows whose token expiry is no longer active.
func (repository *Repository) PruneExpired(ctx context.Context) (int64, error) {
	if repository.DB == nil {
		return 0, fmt.Errorf("session blacklist database is not configured")
	}
	result, err := repository.DB.ExecContext(
		ctx,
		"DELETE FROM session_blacklist WHERE expires_at <= ?",
		repository.dbNowParam(),
	)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return rowsAffected, nil
}

func (repository *Repository) dbNowParam() string {
	return repository.dbTimeParam(repository.clock())
}

func (repository *Repository) dbTimeParam(value time.Time) string {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05+08:00")
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `INSERT INTO session_blacklist (jti, expires_at, revoked_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE expires_at=VALUES(expires_at), revoked_at=VALUES(revoked_at)`
	}
	return `INSERT INTO session_blacklist (jti, expires_at, revoked_at) VALUES (?, ?, ?) ON CONFLICT(jti) DO UPDATE SET expires_at=excluded.expires_at, revoked_at=excluded.revoked_at`
}

func (repository *Repository) clock() time.Time {
	if repository.Now == nil {
		return time.Now()
	}
	return repository.Now()
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
