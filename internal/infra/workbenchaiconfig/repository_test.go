// Package workbenchaiconfig tests system_settings reads for AI config.
package workbenchaiconfig

import (
	"context"
	"database/sql"
	"testing"
)

// TestGetAIConfigValueReadsMySQLKey keeps system_settings key quoting stable.
func TestGetAIConfigValueReadsMySQLKey(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{{[]byte("deepseek-chat")}}}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	value, err := repository.GetAIConfigValue(context.Background(), " ai.model ")
	if err != nil {
		t.Fatalf("GetAIConfigValue returned error: %v", err)
	}
	if value != "deepseek-chat" {
		t.Fatalf("value = %q", value)
	}
	if db.query != "SELECT value FROM system_settings WHERE `key` = ?" || len(db.args) != 1 || db.args[0] != "ai.model" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

// TestGetAIConfigValueReadsPostgresKey keeps PostgreSQL key quoting stable.
func TestGetAIConfigValueReadsPostgresKey(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{}}
	repository := &Repository{DB: db, Dialect: "postgres"}

	value, err := repository.GetAIConfigValue(context.Background(), "ai.enabled")
	if err != nil {
		t.Fatalf("GetAIConfigValue returned error: %v", err)
	}
	if value != "" {
		t.Fatalf("value = %q, want empty", value)
	}
	if db.query != `SELECT value FROM system_settings WHERE "key" = ?` {
		t.Fatalf("query = %q", db.query)
	}
}

func TestSetAIConfigValueUpsertsSetting(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: "mysql"}

	if err := repository.SetAIConfigValue(context.Background(), " ai.model ", "deepseek-chat"); err != nil {
		t.Fatalf("SetAIConfigValue returned error: %v", err)
	}
	if db.execQuery == "" || db.execArgs[0] != "ai.model" || db.execArgs[1] != "deepseek-chat" {
		t.Fatalf("exec query=%q args=%#v", db.execQuery, db.execArgs)
	}
}

type fakeDB struct {
	rows      *fakeRows
	query     string
	args      []any
	execQuery string
	execArgs  []any
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
	db.execQuery = query
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
