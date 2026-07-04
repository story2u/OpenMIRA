package sessionprofile

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestGetProfileLoadsAIEnabled(t *testing.T) {
	db := &fakeDB{row: fakeRow{value: int64(1)}}
	repository := &Repository{DB: db}

	profile, ok, err := repository.GetProfile(context.Background(), "cs-001")
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if !ok || !profile.AIEnabled {
		t.Fatalf("profile = %+v, ok=%t; want ai enabled", profile, ok)
	}
	if db.query != "SELECT ai_enabled FROM cs_users WHERE assignee_id = ?" || db.queryArgs[0] != "cs-001" {
		t.Fatalf("unexpected query %q args=%v", db.query, db.queryArgs)
	}
}

func TestGetProfileHandlesMissingUser(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	_, ok, err := repository.GetProfile(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false for missing user")
	}
}

func TestGetUserLoadsLoginFields(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{"客服一", "supervisor", int64(1), []byte("0"), "hash"}}}
	repository := &Repository{DB: db}

	user, ok, err := repository.GetUser(context.Background(), " cs-001 ")
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if !ok || user.AssigneeID != "cs-001" || user.AssigneeName != "客服一" || user.Role != "supervisor" || !user.Enabled || user.AIEnabled || user.PasswordHash != "hash" {
		t.Fatalf("user = %+v ok=%t", user, ok)
	}
	if db.query != "SELECT assignee_name, role, enabled, ai_enabled, password_hash FROM cs_users WHERE assignee_id = ?" || db.queryArgs[0] != "cs-001" {
		t.Fatalf("unexpected query %q args=%v", db.query, db.queryArgs)
	}
}

func TestGetUserHandlesMissingUser(t *testing.T) {
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	_, ok, err := repository.GetUser(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false for missing user")
	}
}

func TestUpdateLastSeenUsesBeijingDatabaseTime(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now: func() time.Time {
			return time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC)
		},
	}

	if err := repository.UpdateLastSeen(context.Background(), "cs-001"); err != nil {
		t.Fatalf("UpdateLastSeen returned error: %v", err)
	}
	if db.exec != "UPDATE cs_users SET last_seen_at=?, updated_at=? WHERE assignee_id=?" {
		t.Fatalf("unexpected exec query %q", db.exec)
	}
	got := strings.TrimSpace(db.execArgs[0].(string))
	if got != "2026-06-28 09:02:03" || db.execArgs[1] != got || db.execArgs[2] != "cs-001" {
		t.Fatalf("unexpected exec args: %#v", db.execArgs)
	}
}

func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil, DialectPostgres)
	_, _, err := repository.GetProfile(context.Background(), "cs-001")
	if err == nil {
		t.Fatal("GetProfile error = nil, want nil sql db error")
	}
}

type fakeDB struct {
	row       fakeRow
	query     string
	queryArgs []any
	exec      string
	execArgs  []any
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	return db.row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.exec = query
	db.execArgs = args
	return fakeResult(1), nil
}

type fakeRow struct {
	value  any
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(row.values) > 0 {
		for index := range dest {
			target := dest[index].(*any)
			if index < len(row.values) {
				*target = row.values[index]
			}
		}
		return nil
	}
	target := dest[0].(*any)
	*target = row.value
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
