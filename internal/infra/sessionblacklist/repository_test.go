// Package sessionblacklist tests SQL issued against the legacy blacklist table.
// Test fakes implement database/sql-compatible interfaces so no concrete
// database driver is needed in the phase-two harness.
package sessionblacklist

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestContainsPrunesExpiredRowsAndFindsJTI verifies the read path SQL contract.
func TestContainsPrunesExpiredRowsAndFindsJTI(t *testing.T) {
	db := &fakeDB{row: fakeRow{value: "jwt-test"}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now: func() time.Time {
			return time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
		},
	}

	contains, err := repository.Contains(context.Background(), " jwt-test ")
	if err != nil {
		t.Fatalf("Contains returned error: %v", err)
	}
	if !contains {
		t.Fatal("contains = false, want true")
	}
	if db.exec != "DELETE FROM session_blacklist WHERE expires_at <= ?" {
		t.Fatalf("unexpected prune query %q", db.exec)
	}
	if got := strings.TrimSpace(db.execArgs[0].(string)); got != "2026-06-28 09:02:03" {
		t.Fatalf("prune time = %q, want Beijing DATETIME", got)
	}
	if db.query != "SELECT jti FROM session_blacklist WHERE jti = ?" || db.queryArgs[0] != "jwt-test" {
		t.Fatalf("unexpected query %q args=%v", db.query, db.queryArgs)
	}
}

// TestAddUpsertsJTIWithBeijingTimes verifies the blacklist write SQL contract.
func TestAddUpsertsJTIWithBeijingTimes(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now: func() time.Time {
			return time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
		},
	}

	err := repository.Add(context.Background(), " jwt-new ", time.Date(2026, 6, 29, 2, 3, 4, 0, time.UTC))
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if !strings.Contains(db.exec, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected upsert SQL: %s", db.exec)
	}
	if db.execArgs[0] != "jwt-new" || db.execArgs[1] != "2026-06-29 10:03:04" || db.execArgs[2] != "2026-06-28 09:02:03" {
		t.Fatalf("unexpected upsert args: %v", db.execArgs)
	}
}

// TestAddUsesPostgresConflictUpsert keeps historical deployments compatible.
func TestAddUsesPostgresConflictUpsert(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	err := repository.Add(context.Background(), "jwt-new", time.Unix(1000, 0).UTC())
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if !strings.Contains(db.exec, "ON CONFLICT(jti) DO UPDATE") {
		t.Fatalf("unexpected postgres upsert SQL: %s", db.exec)
	}
}

// TestContainsHandlesMissingJTIAsFalse keeps absent blacklist rows non-fatal.
func TestContainsHandlesMissingJTIAsFalse(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	contains, err := repository.Contains(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Contains returned error: %v", err)
	}
	if contains {
		t.Fatal("contains = true, want false")
	}
}

// TestAddReturnsStoreErrors keeps refresh/logout fail-closed on DB issues.
func TestAddReturnsStoreErrors(t *testing.T) {
	repository := &Repository{DB: &fakeDB{execErr: errors.New("db down")}}

	err := repository.Add(context.Background(), "jwt-test", time.Unix(1000, 0).UTC())
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("Add error = %v, want db down", err)
	}
}

// TestContainsReturnsStoreErrors makes verifier callers fail closed on DB issues.
func TestContainsReturnsStoreErrors(t *testing.T) {
	repository := &Repository{DB: &fakeDB{execErr: errors.New("db down")}}

	_, err := repository.Contains(context.Background(), "jwt-test")
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("Contains error = %v, want db down", err)
	}
}

// TestMaybePruneExpiredThrottlesDeletes avoids writing on every token check.
func TestMaybePruneExpiredThrottlesDeletes(t *testing.T) {
	now := time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
	db := &fakeDB{row: fakeRow{value: "jwt-test"}}
	repository := &Repository{
		DB:            db,
		PruneInterval: time.Hour,
		Now: func() time.Time {
			return now
		},
	}

	_, _ = repository.Contains(context.Background(), "jwt-test")
	_, _ = repository.Contains(context.Background(), "jwt-test")

	if db.execCount != 1 {
		t.Fatalf("prune exec count = %d, want 1", db.execCount)
	}
}

// TestNewSQLRepositoryWrapsNilDB keeps nil *sql.DB failures explicit.
func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil, DialectPostgres)
	_, err := repository.Contains(context.Background(), "jwt-test")
	if err == nil {
		t.Fatal("Contains error = nil, want nil sql db error")
	}
}

type fakeDB struct {
	row       fakeRow
	query     string
	queryArgs []any
	exec      string
	execArgs  []any
	execErr   error
	execCount int
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	return db.row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.exec = query
	db.execArgs = args
	db.execCount++
	if db.execErr != nil {
		return nil, db.execErr
	}
	return fakeResult(1), nil
}

type fakeRow struct {
	value any
	err   error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	target := dest[0].(*string)
	*target = row.value.(string)
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
