// Package workbenchassignmentconfig tests system_settings reads for allocation config.
package workbenchassignmentconfig

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestGetAssignmentConfigValueReadsMySQLKey(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{{[]byte(`[{"rule_id":"rule-1"}]`)}}}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	value, err := repository.GetAssignmentConfigValue(context.Background(), " assignment.config.rules ")
	if err != nil {
		t.Fatalf("GetAssignmentConfigValue returned error: %v", err)
	}
	if value != `[{"rule_id":"rule-1"}]` {
		t.Fatalf("value = %q", value)
	}
	if db.query != "SELECT value FROM system_settings WHERE `key` = ?" || len(db.args) != 1 || db.args[0] != "assignment.config.rules" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestGetAssignmentConfigValueReadsPostgresKey(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{}}
	repository := &Repository{DB: db, Dialect: "postgres"}

	value, err := repository.GetAssignmentConfigValue(context.Background(), "assignment.config.pools")
	if err != nil {
		t.Fatalf("GetAssignmentConfigValue returned error: %v", err)
	}
	if value != "" {
		t.Fatalf("value = %q, want empty", value)
	}
	if db.query != `SELECT value FROM system_settings WHERE "key" = ?` {
		t.Fatalf("query = %q", db.query)
	}
}

func TestSetAssignmentConfigValueUpsertsMySQLKey(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: "mysql"}

	err := repository.SetAssignmentConfigValue(context.Background(), " assignment.config.rules ", `[{"rule_id":"rule-1"}]`)
	if err != nil {
		t.Fatalf("SetAssignmentConfigValue returned error: %v", err)
	}
	if !strings.Contains(db.exec, "INSERT INTO system_settings (`key`, value, updated_at)") || !strings.Contains(db.exec, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("exec query = %q", db.exec)
	}
	if len(db.execArgs) != 3 || db.execArgs[0] != "assignment.config.rules" || db.execArgs[1] != `[{"rule_id":"rule-1"}]` {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
}

func TestSetAssignmentConfigValueUpsertsPostgresKey(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: "postgres"}

	err := repository.SetAssignmentConfigValue(context.Background(), "assignment.pool_state.pool-1", `{}`)
	if err != nil {
		t.Fatalf("SetAssignmentConfigValue returned error: %v", err)
	}
	if !strings.Contains(db.exec, `INSERT INTO system_settings ("key", value, updated_at)`) || !strings.Contains(db.exec, `ON CONFLICT("key") DO UPDATE`) {
		t.Fatalf("exec query = %q", db.exec)
	}
	if len(db.execArgs) != 3 || db.execArgs[0] != "assignment.pool_state.pool-1" || db.execArgs[1] != `{}` {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
}

type fakeDB struct {
	rows     *fakeRows
	query    string
	args     []any
	exec     string
	execArgs []any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.exec = query
	db.execArgs = args
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}
