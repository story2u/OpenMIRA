// Package sessionprofile adapts cs_users rows for session /me responses.
// It deliberately uses small database/sql-compatible interfaces so phase two
// can test SQL behavior without adding a concrete driver dependency yet.
package sessionprofile

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/session"
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

// Queryer is the database/sql shape needed by the session profile repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads cs_users profile fields and writes last_seen_at.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

var _ session.ProfileResolver = (*Repository)(nil)
var _ session.UserResolver = (*Repository)(nil)
var _ session.LastSeenUpdater = (*Repository)(nil)

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// GetProfile loads only the fields needed by /api/v1/session/me.
func (repository *Repository) GetProfile(ctx context.Context, assigneeID string) (session.Profile, bool, error) {
	if repository.DB == nil {
		return session.Profile{}, false, fmt.Errorf("session profile database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return session.Profile{}, false, nil
	}
	var rawAIEnabled any
	err := repository.DB.QueryRowContext(
		ctx,
		"SELECT ai_enabled FROM cs_users WHERE assignee_id = ?",
		assigneeID,
	).Scan(&rawAIEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return session.Profile{}, false, nil
		}
		return session.Profile{}, false, err
	}
	return session.Profile{AIEnabled: boolFromDB(rawAIEnabled)}, true, nil
}

// GetUser loads the cs_users subset needed by passwordless login.
func (repository *Repository) GetUser(ctx context.Context, assigneeID string) (session.User, bool, error) {
	if repository.DB == nil {
		return session.User{}, false, fmt.Errorf("session profile database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return session.User{}, false, nil
	}
	var rawAssigneeName any
	var rawRole any
	var rawEnabled any
	var rawAIEnabled any
	var rawPasswordHash any
	err := repository.DB.QueryRowContext(
		ctx,
		"SELECT assignee_name, role, enabled, ai_enabled, password_hash FROM cs_users WHERE assignee_id = ?",
		assigneeID,
	).Scan(&rawAssigneeName, &rawRole, &rawEnabled, &rawAIEnabled, &rawPasswordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return session.User{}, false, nil
		}
		return session.User{}, false, err
	}
	return session.User{
		AssigneeID:   assigneeID,
		AssigneeName: stringFromDB(rawAssigneeName),
		Role:         stringFromDB(rawRole),
		Enabled:      boolFromDB(rawEnabled),
		AIEnabled:    boolFromDB(rawAIEnabled),
		PasswordHash: stringFromDB(rawPasswordHash),
	}, true, nil
}

// UpdateLastSeen mirrors the legacy repository update for /me activity.
func (repository *Repository) UpdateLastSeen(ctx context.Context, assigneeID string) error {
	if repository.DB == nil {
		return fmt.Errorf("session profile database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return nil
	}
	now := repository.dbNowParam()
	_, err := repository.DB.ExecContext(
		ctx,
		"UPDATE cs_users SET last_seen_at=?, updated_at=? WHERE assignee_id=?",
		now,
		now,
		assigneeID,
	)
	return err
}

func (repository *Repository) dbNowParam() string {
	now := time.Now().UTC()
	if repository.Now != nil {
		now = repository.Now().UTC()
	}
	beijing := now.In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05+08:00")
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int64:
		return typed != 0
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	default:
		return false
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
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
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
