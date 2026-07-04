package realtimeeventlog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/realtime"
)

func TestRepositoryListsEventsAfterCursor(t *testing.T) {
	db := &fakeRealtimeDB{rows: []*fakeRealtimeRows{
		{},
		{values: [][]any{{
			" conversations:conversation.message ", []byte("2"), " conversations ", " conversation.message ",
			" conversation.message ", "", []byte(`{"message_id":"m-2"}`), []byte("2026-07-01 08:00:00"),
		}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	records, err := repository.ListAfterCursor(context.Background(), " conversations:conversation.message ", 1, 50)
	if err != nil {
		t.Fatalf("ListAfterCursor returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	record := records[0]
	if record.ScopeKey != "conversations:conversation.message" || record.Cursor != 2 || record.Consistency != "strong" || record.Payload["message_id"] != "m-2" {
		t.Fatalf("record = %#v", record)
	}
	if record.CreatedAt != "2026-07-01 08:00:00" {
		t.Fatalf("created_at = %#v", record.CreatedAt)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "`cursor` AS cursor_value") || !strings.Contains(db.queries[1], "ORDER BY `cursor` ASC") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][0] != "conversations:conversation.message" || db.args[1][1] != int64(1) || db.args[1][2] != 50 {
		t.Fatalf("args = %#v", db.args[1])
	}
}

func TestRepositoryReadsLatestCursor(t *testing.T) {
	db := &fakeRealtimeDB{rows: []*fakeRealtimeRows{
		{},
		{values: [][]any{{int64(42)}}},
	}}
	repository := &Repository{DB: db}

	cursor, err := repository.LatestCursor(context.Background(), "chat:identity.updated")
	if err != nil {
		t.Fatalf("LatestCursor returned error: %v", err)
	}
	if cursor != 42 {
		t.Fatalf("cursor = %d", cursor)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "MAX(cursor)") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][0] != "chat:identity.updated" {
		t.Fatalf("args = %#v", db.args[1])
	}
}

func TestRepositoryMissingTableReturnsEmpty(t *testing.T) {
	repository := &Repository{DB: &fakeRealtimeDB{errors: []error{errors.New("no such table")}}}
	records, err := repository.ListAfterCursor(context.Background(), "scope-a", 0, 100)
	if err != nil {
		t.Fatalf("ListAfterCursor returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	repository = &Repository{DB: &fakeRealtimeDB{errors: []error{errors.New("no such table")}}}
	cursor, err := repository.LatestCursor(context.Background(), "scope-a")
	if err != nil || cursor != 0 {
		t.Fatalf("cursor=%d err=%v", cursor, err)
	}
}

func TestRepositoryAppendsRealtimeEvent(t *testing.T) {
	db := &fakeRealtimeDB{}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
		},
	}

	err := repository.AppendEvent(context.Background(), realtime.EventRecord{
		ScopeKey:    " conversations:conversation.message ",
		Cursor:      42,
		Channel:     " conversations ",
		Event:       " conversation.message ",
		Topic:       " conversation.message ",
		Consistency: "",
		Payload:     map[string]any{"message_id": "m-42"},
	})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") || !strings.Contains(db.execs[0].query, "`cursor`") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "conversations:conversation.message" || args[1] != int64(42) || args[2] != "conversations" || args[5] != "strong" {
		t.Fatalf("args = %#v", args)
	}
	if args[6] != `{"message_id":"m-42"}` || args[7] != "2026-07-01 16:00:00" {
		t.Fatalf("args = %#v", args)
	}
}

func TestRepositoryAppendSkipsBlankScope(t *testing.T) {
	db := &fakeRealtimeDB{}
	repository := &Repository{DB: db}
	if err := repository.AppendEvent(context.Background(), realtime.EventRecord{Cursor: 1}); err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %#v", db.execs)
	}
}

type fakeRealtimeDB struct {
	rows    []*fakeRealtimeRows
	errors  []error
	queries []string
	args    [][]any
	execs   []fakeExec
}

func (db *fakeRealtimeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	index := len(db.queries) - 1
	if index < len(db.errors) && db.errors[index] != nil {
		return nil, db.errors[index]
	}
	if index < len(db.rows) {
		return db.rows[index], nil
	}
	return &fakeRealtimeRows{}, nil
}

func (db *fakeRealtimeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: args})
	return fakeResult(1), nil
}

type fakeRealtimeRows struct {
	values [][]any
	index  int
}

func (rows *fakeRealtimeRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRealtimeRows) Scan(dest ...any) error {
	values := rows.values[rows.index-1]
	for index := range dest {
		ptr := dest[index].(*any)
		*ptr = values[index]
	}
	return nil
}

func (rows *fakeRealtimeRows) Close() error {
	return nil
}

func (rows *fakeRealtimeRows) Err() error {
	return nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
