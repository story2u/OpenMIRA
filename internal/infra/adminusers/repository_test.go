package adminusers

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"im-go/internal/auth"
)

func TestEnsureDefaultAdminSeedsRootWithoutOverwrite(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:      db,
		Dialect: DialectPostgres,
		Now: func() time.Time {
			return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
		},
	}

	if err := repository.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	if err := repository.EnsureDefaultAdmin(context.Background()); err != nil {
		t.Fatalf("EnsureDefaultAdmin returned error: %v", err)
	}

	if len(db.execs) != 2 || !strings.Contains(db.execs[0].query, "CREATE TABLE IF NOT EXISTS admin_users") {
		t.Fatalf("execs = %+v", db.execs)
	}
	insert := db.execs[1]
	if !strings.Contains(insert.query, "ON CONFLICT(username) DO NOTHING") {
		t.Fatalf("insert query = %s", insert.query)
	}
	if insert.args[0] != DefaultUsername || !auth.VerifyPasswordHash(insert.args[1].(string), DefaultPassword) || insert.args[2] != true {
		t.Fatalf("insert args = %#v", insert.args)
	}
}

func TestGetAdminUserReadsPasswordChangeRequired(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{"root", "hash", true}}}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	user, ok, err := repository.GetAdminUser(context.Background(), " root ")
	if err != nil {
		t.Fatalf("GetAdminUser returned error: %v", err)
	}
	if !ok || user.Username != "root" || user.PasswordHash != "hash" || !user.PasswordChangeRequired {
		t.Fatalf("user = %+v ok=%t", user, ok)
	}
	if !strings.Contains(db.query, "WHERE username = $1") || db.queryArgs[0] != "root" {
		t.Fatalf("query = %q args=%#v", db.query, db.queryArgs)
	}
}

func TestUpdateAdminPasswordUsesDialectSQL(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	if err := repository.UpdateAdminPassword(context.Background(), "root", "hash-new", false); err != nil {
		t.Fatalf("UpdateAdminPassword returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "password_change_required = ?") {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != "hash-new" || db.execs[0].args[1] != 0 || db.execs[0].args[3] != "root" {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

type fakeDB struct {
	execs     []execCall
	query     string
	queryArgs []any
	row       fakeRow
}

type execCall struct {
	query string
	args  []any
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = append([]any(nil), args...)
	return db.row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, execCall{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index := range dest {
		if index >= len(row.values) {
			break
		}
		switch target := dest[index].(type) {
		case *any:
			*target = row.values[index]
		}
	}
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
