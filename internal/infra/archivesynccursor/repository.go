// Package archivesynccursor adapts the legacy archive_sync_cursors table.
package archivesynccursor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
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

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Record mirrors Python ArchiveSyncCursorRecord.
type Record struct {
	Source    string
	Cursor    string
	UpdatedAt time.Time
}

// Repository reads and advances archive sync cursors.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// GetCursor returns one cursor by enterprise_id and source.
func (repository *Repository) GetCursor(ctx context.Context, source string, enterpriseID string) (*Record, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("archive sync cursor database is not configured")
	}
	key := strings.TrimSpace(source)
	if key == "" {
		return nil, nil
	}
	record, found, err := repository.getCursor(ctx, key, enterpriseID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &record, nil
}

// UpsertCursor advances a cursor without allowing numeric sequence rollback.
func (repository *Repository) UpsertCursor(ctx context.Context, source string, cursor string, enterpriseID string) (Record, error) {
	if repository.DB == nil {
		return Record{}, fmt.Errorf("archive sync cursor database is not configured")
	}
	key := strings.TrimSpace(source)
	value := strings.TrimSpace(cursor)
	ent := normalizeEnterpriseID(enterpriseID)
	if key == "" {
		return Record{}, fmt.Errorf("source is required")
	}
	if value == "" {
		return Record{}, fmt.Errorf("cursor is required")
	}
	existing, found, err := repository.getCursor(ctx, key, ent)
	if err != nil {
		return Record{}, err
	}
	effectiveValue := value
	if found {
		effectiveValue = preferMonotonicCursor(existing.Cursor, value)
		if effectiveValue == existing.Cursor {
			return existing, nil
		}
	}
	now := repository.now()
	_, err = repository.DB.ExecContext(ctx, repository.upsertSQL(), ent, key, effectiveValue, repository.dbTimeParam(now))
	if err != nil {
		return Record{}, err
	}
	return Record{Source: key, Cursor: effectiveValue, UpdatedAt: now}, nil
}

func (repository *Repository) getCursor(ctx context.Context, source string, enterpriseID string) (Record, bool, error) {
	ent := normalizeEnterpriseID(enterpriseID)
	var rowSource string
	var rowCursor string
	var updatedAt any
	err := repository.DB.QueryRowContext(ctx, repository.selectSQL(), ent, source).Scan(&rowSource, &rowCursor, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	return Record{
		Source:    strings.TrimSpace(rowSource),
		Cursor:    strings.TrimSpace(rowCursor),
		UpdatedAt: parseDBTime(updatedAt),
	}, true, nil
}

func (repository *Repository) selectSQL() string {
	cursorColumn := repository.cursorColumnSQL()
	return fmt.Sprintf("SELECT source, %s AS %s, updated_at FROM archive_sync_cursors WHERE enterprise_id = ? AND source = ?", cursorColumn, cursorColumn)
}

func (repository *Repository) upsertSQL() string {
	cursorColumn := repository.cursorColumnSQL()
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return fmt.Sprintf("INSERT INTO archive_sync_cursors (enterprise_id, source, %s, updated_at) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE %s = VALUES(%s), updated_at = VALUES(updated_at)", cursorColumn, cursorColumn, cursorColumn)
	}
	return fmt.Sprintf("INSERT INTO archive_sync_cursors (enterprise_id, source, %s, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(enterprise_id, source) DO UPDATE SET %s = excluded.%s, updated_at = excluded.updated_at", cursorColumn, cursorColumn, cursorColumn)
}

func (repository *Repository) cursorColumnSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return "`cursor`"
	}
	return `"cursor"`
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

func normalizeEnterpriseID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func preferMonotonicCursor(existing string, candidate string) string {
	existingText := strings.TrimSpace(existing)
	candidateText := strings.TrimSpace(candidate)
	if existingText == "" || candidateText == "" {
		if candidateText != "" {
			return candidateText
		}
		return existingText
	}
	existingSeq, existingErr := strconv.ParseInt(existingText, 10, 64)
	candidateSeq, candidateErr := strconv.ParseInt(candidateText, 10, 64)
	if existingErr != nil || candidateErr != nil {
		return candidateText
	}
	if candidateSeq < existingSeq {
		return existingText
	}
	return candidateText
}

func parseDBTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseDBTimeString(string(typed))
	case string:
		return parseDBTimeString(typed)
	default:
		return time.Time{}
	}
}

func parseDBTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
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

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is nil")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
