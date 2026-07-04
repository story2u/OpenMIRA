// Package archivesynccursor tests archive_sync_cursors SQL compatibility.
package archivesynccursor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGetCursorReadsMySQLQuotedCursor(t *testing.T) {
	updatedAt := time.Date(2026, 4, 27, 14, 56, 4, 0, beijingLocation)
	db := &fakeDB{rows: []fakeRow{{values: []any{"sdk", "512195", updatedAt}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	record, err := repository.GetCursor(context.Background(), " sdk ", " ent-a ")
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if record == nil || record.Source != "sdk" || record.Cursor != "512195" {
		t.Fatalf("record = %#v", record)
	}
	if !record.UpdatedAt.Equal(updatedAt.UTC()) {
		t.Fatalf("updated_at = %s, want %s", record.UpdatedAt, updatedAt.UTC())
	}
	if !strings.Contains(db.queries[0], "SELECT source, `cursor` AS `cursor`, updated_at") || db.queryArgs[0][0] != "ent-a" || db.queryArgs[0][1] != "sdk" {
		t.Fatalf("query=%q args=%#v", db.queries[0], db.queryArgs[0])
	}
}

func TestGetCursorHandlesBlankAndMissing(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: sql.ErrNoRows}}}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	record, err := repository.GetCursor(context.Background(), " ", "ent-a")
	if err != nil {
		t.Fatalf("GetCursor blank returned error: %v", err)
	}
	if record != nil || len(db.queries) != 0 {
		t.Fatalf("blank record=%#v queries=%#v", record, db.queries)
	}
	record, err = repository.GetCursor(context.Background(), "sdk", "")
	if err != nil {
		t.Fatalf("GetCursor missing returned error: %v", err)
	}
	if record != nil {
		t.Fatalf("record = %#v, want nil", record)
	}
	if !strings.Contains(db.queries[0], `SELECT source, "cursor" AS "cursor", updated_at`) || db.queryArgs[0][0] != "default" {
		t.Fatalf("query=%q args=%#v", db.queries[0], db.queryArgs[0])
	}
}

func TestUpsertCursorUsesMySQLUpsertAndBeijingTime(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRow{{err: sql.ErrNoRows}}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	record, err := repository.UpsertCursor(context.Background(), " sdk ", " cursor-1 ", " ent-a ")
	if err != nil {
		t.Fatalf("UpsertCursor returned error: %v", err)
	}
	if record.Source != "sdk" || record.Cursor != "cursor-1" || !record.UpdatedAt.Equal(now) {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON DUPLICATE KEY UPDATE") || !strings.Contains(db.execs[0], "VALUES(`cursor`)") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if got := db.execArgs[0][3]; got != "2026-06-30 18:00:00" {
		t.Fatalf("updated_at arg = %#v", got)
	}
}

func TestUpsertCursorUsesPostgresConflictSQL(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: sql.ErrNoRows}}}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time {
		return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	}}

	_, err := repository.UpsertCursor(context.Background(), "sdk", "cursor-1", "ent-a")
	if err != nil {
		t.Fatalf("UpsertCursor returned error: %v", err)
	}
	if !strings.Contains(db.execs[0], `ON CONFLICT(enterprise_id, source) DO UPDATE`) || !strings.Contains(db.execs[0], `"cursor" = excluded."cursor"`) {
		t.Fatalf("exec = %q", db.execs[0])
	}
	if got := db.execArgs[0][3]; got != "2026-06-30T18:00:00+08:00" {
		t.Fatalf("updated_at arg = %#v", got)
	}
}

func TestUpsertCursorKeepsNewerExistingSequence(t *testing.T) {
	updatedAt := time.Date(2026, 4, 27, 14, 56, 4, 0, beijingLocation)
	db := &fakeDB{rows: []fakeRow{{values: []any{"sdk", "512195", updatedAt}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	record, err := repository.UpsertCursor(context.Background(), "sdk", "488745", "ent-a")
	if err != nil {
		t.Fatalf("UpsertCursor returned error: %v", err)
	}
	if record.Cursor != "512195" || !record.UpdatedAt.Equal(updatedAt.UTC()) {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %#v, want no upsert", db.execs)
	}
}

func TestUpsertCursorAllowsNonNumericReplacement(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{values: []any{"sdk", "cursor-old", "2026-04-27 14:56:04"}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time {
		return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	}}

	record, err := repository.UpsertCursor(context.Background(), "sdk", "cursor-new", "ent-a")
	if err != nil {
		t.Fatalf("UpsertCursor returned error: %v", err)
	}
	if record.Cursor != "cursor-new" || len(db.execs) != 1 {
		t.Fatalf("record=%#v execs=%#v", record, db.execs)
	}
}

func TestUpsertCursorValidatesInputAndReturnsStoreErrors(t *testing.T) {
	repository := &Repository{DB: &fakeDB{}}
	if _, err := repository.UpsertCursor(context.Background(), " ", "cursor", "ent-a"); err == nil || !strings.Contains(err.Error(), "source is required") {
		t.Fatalf("source error = %v", err)
	}
	if _, err := repository.UpsertCursor(context.Background(), "sdk", " ", "ent-a"); err == nil || !strings.Contains(err.Error(), "cursor is required") {
		t.Fatalf("cursor error = %v", err)
	}

	repository = &Repository{DB: &fakeDB{rows: []fakeRow{{err: errors.New("db down")}}}}
	if _, err := repository.UpsertCursor(context.Background(), "sdk", "cursor", "ent-a"); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("store error = %v", err)
	}
}

func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil, DialectMySQL)
	_, err := repository.GetCursor(context.Background(), "sdk", "ent-a")
	if err == nil || !strings.Contains(err.Error(), "sql db is nil") {
		t.Fatalf("error = %v, want nil sql db error", err)
	}
}

type fakeDB struct {
	rows      []fakeRow
	queries   []string
	queryArgs [][]any
	execs     []string
	execArgs  [][]any
	execErr   error
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.queries = append(db.queries, query)
	db.queryArgs = append(db.queryArgs, args)
	if len(db.rows) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, query)
	db.execArgs = append(db.execArgs, args)
	if db.execErr != nil {
		return nil, db.execErr
	}
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
	for index, value := range row.values {
		switch target := dest[index].(type) {
		case *string:
			*target = strings.TrimSpace(textValue(value))
		case *any:
			*target = value
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

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
