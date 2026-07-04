package friendaddedevents

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/friendadded"
)

func TestAddFriendEventInsertsWhenTraceIsNew(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	repository := &Repository{DB: db}
	event := friendadded.Event{
		TraceID:    "trace-1",
		DeviceID:   "dev-1",
		FriendName: "Qiu",
		FriendID:   "ext-1",
		Source:     "manual",
		Timestamp:  time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		CreatedAt:  time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC),
	}

	inserted, err := repository.AddFriendEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("AddFriendEvent returned error: %v", err)
	}
	if !inserted {
		t.Fatal("inserted = false, want true")
	}
	if !strings.Contains(db.query, "SELECT trace_id FROM friend_added_events") || db.queryArgs[0] != "trace-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.queryArgs)
	}
	if !strings.Contains(db.execQuery, "INSERT INTO friend_added_events") || len(db.execArgs) != 7 {
		t.Fatalf("exec=%q args=%#v", db.execQuery, db.execArgs)
	}
	if db.execArgs[0] != "trace-1" || db.execArgs[1] != "dev-1" || db.execArgs[2] != "Qiu" || db.execArgs[3] != "ext-1" || db.execArgs[4] != "manual" {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
}

func TestAddFriendEventSkipsDuplicateTrace(t *testing.T) {
	db := &fakeDB{row: fakeRow{value: "trace-1"}}
	repository := &Repository{DB: db}

	inserted, err := repository.AddFriendEvent(context.Background(), friendadded.Event{TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("AddFriendEvent returned error: %v", err)
	}
	if inserted {
		t.Fatal("inserted = true, want false")
	}
	if db.execQuery != "" {
		t.Fatalf("unexpected exec=%q", db.execQuery)
	}
}

func TestAddFriendEventPropagatesDatabaseErrors(t *testing.T) {
	expected := errors.New("select down")
	repository := &Repository{DB: &fakeDB{row: fakeRow{err: expected}}}

	_, err := repository.AddFriendEvent(context.Background(), friendadded.Event{TraceID: "trace-1"})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
}

func TestTouchConversationFirstMessageAtUpsertsMySQLConversation(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:                 db,
		Dialect:            DialectMySQL,
		Now:                func() time.Time { return time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC) },
		NextConversationPK: func() int64 { return 99 },
	}

	err := repository.TouchConversationFirstMessageAt(context.Background(), friendadded.ConversationTouch{
		DeviceID:       " dev-1 ",
		FriendID:       " wm-1 ",
		FriendName:     " Qiu ",
		FirstMessageAt: time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
		TenantID:       " tenant-1 ",
		AccountID:      " account-1 ",
		WeWorkUserID:   " wxuser ",
	})
	if err != nil {
		t.Fatalf("TouchConversationFirstMessageAt returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "INSERT INTO conversations") || !strings.Contains(db.execQuery, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("exec query = %q", db.execQuery)
	}
	if !strings.Contains(db.execQuery, "first_message_at=COALESCE(first_message_at, VALUES(first_message_at))") {
		t.Fatalf("exec query missing first_message_at COALESCE: %q", db.execQuery)
	}
	if len(db.execArgs) != 26 {
		t.Fatalf("len(execArgs) = %d, want 26: %#v", len(db.execArgs), db.execArgs)
	}
	if db.execArgs[0] != int64(99) || db.execArgs[1] != "ww:wxuser:wm-1" || db.execArgs[2] != "ww:wxuser:wm-1" {
		t.Fatalf("identity args = %#v", db.execArgs[:3])
	}
	if db.execArgs[3] != "tenant-1" || db.execArgs[4] != "account-1" || db.execArgs[5] != "wxuser" || db.execArgs[6] != "wm-1" {
		t.Fatalf("scope args = %#v", db.execArgs[3:7])
	}
	if db.execArgs[9] != "dev-1" || db.execArgs[10] != "wm-1" || db.execArgs[11] != "Qiu" || db.execArgs[14] != "Qiu" {
		t.Fatalf("participant args = %#v", db.execArgs[9:15])
	}
	if db.execArgs[15] != "2026-03-08 09:12:34" || db.execArgs[18] != "2026-03-08 09:12:34" || db.execArgs[25] != "2026-03-08 10:00:00" {
		t.Fatalf("time args = first=%#v last=%#v updated=%#v", db.execArgs[15], db.execArgs[18], db.execArgs[25])
	}
}

func TestTouchConversationFirstMessageAtUsesPostgresConflictAndPendingIdentity(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:                 db,
		Dialect:            DialectPostgres,
		Now:                func() time.Time { return time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC) },
		NextConversationPK: func() int64 { return 100 },
	}

	err := repository.TouchConversationFirstMessageAt(context.Background(), friendadded.ConversationTouch{
		DeviceID:       "device-a",
		FriendName:     "Qiu",
		FirstMessageAt: time.Date(2026, 3, 8, 1, 12, 34, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("TouchConversationFirstMessageAt returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "ON CONFLICT(conversation_id) DO UPDATE SET") {
		t.Fatalf("exec query = %q", db.execQuery)
	}
	if db.execArgs[1] != "pending:qiu:qiu" || db.execArgs[6] != "Qiu" {
		t.Fatalf("identity args = %#v", db.execArgs[:7])
	}
	if db.execArgs[15] != "2026-03-08T09:12:34+08:00" || db.execArgs[25] != "2026-03-08T10:00:00+08:00" {
		t.Fatalf("time args = first=%#v updated=%#v", db.execArgs[15], db.execArgs[25])
	}
}

type fakeDB struct {
	row       fakeRow
	query     string
	queryArgs []any
	execQuery string
	execArgs  []any
	execErr   error
}

func (db *fakeDB) QueryRowContext(_ context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	return db.row
}

func (db *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	if db.execErr != nil {
		return nil, db.execErr
	}
	return fakeResult(1), nil
}

type fakeRow struct {
	value string
	err   error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) > 0 {
		switch target := dest[0].(type) {
		case *string:
			*target = row.value
		}
	}
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, driver.ErrSkip
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
