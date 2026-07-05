// Package workbenchcsusers tests CS user summary reads for assignment panels.
package workbenchcsusers

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"im-go/internal/workbench"
)

func TestListCSUsersScansSummaryFields(t *testing.T) {
	lastSeenAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"cs-1", "消息端A", "cs", int64(1), int64(1), []byte("12"), "hash", lastSeenAt, "2026-06-28 09:00:00", "2026-06-29 10:01:00"},
		{[]byte("cs-2"), []byte("消息端B"), "supervisor", []byte("0"), []byte("0"), int64(5), "", nil, nil, nil},
		{"", "跳过", "cs", true, true, 1, "", "", "", ""},
	}}}}
	repository := &Repository{DB: db}

	users, err := repository.ListCSUsers(context.Background())
	if err != nil {
		t.Fatalf("ListCSUsers returned error: %v", err)
	}
	if !strings.Contains(db.queries[0], "FROM cs_users ORDER BY assignee_name ASC") {
		t.Fatalf("query = %q", db.queries[0])
	}
	if len(users) != 2 {
		t.Fatalf("users = %#v", users)
	}
	if users[0].AssigneeID != "cs-1" || users[0].AssigneeName != "消息端A" || !users[0].Enabled || !users[0].AIEnabled || users[0].MaxSessions != 12 || !users[0].HasPassword || users[0].LastSeenAt != "2026-06-29T10:00:00Z" {
		t.Fatalf("first user = %+v", users[0])
	}
	if users[1].AssigneeID != "cs-2" || users[1].Enabled || users[1].AIEnabled || users[1].MaxSessions != 5 || users[1].HasPassword || users[1].LastSeenAt != "" {
		t.Fatalf("second user = %+v", users[1])
	}
}

func TestGetCSUserReadsOneUser(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"cs-1", "消息端A", "cs", int64(1), int64(0), int64(3), "", nil, "2026-06-28 09:00:00", "2026-06-29 10:01:00"},
	}}}}
	repository := &Repository{DB: db}

	user, ok, err := repository.GetCSUser(context.Background(), " cs-1 ")
	if err != nil {
		t.Fatalf("GetCSUser returned error: %v", err)
	}
	if !ok || user.AssigneeID != "cs-1" || user.AssigneeName != "消息端A" {
		t.Fatalf("ok=%t user=%+v", ok, user)
	}
	if db.args[0][0] != "cs-1" {
		t.Fatalf("args = %#v", db.args)
	}
}

func TestUpsertCSUserWithPasswordHashesPassword(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"cs-1", "消息端A", "supervisor", int64(1), int64(1), int64(8), "hash", nil, "2026-06-28 09:00:00", "2026-06-29 10:01:00"},
	}}}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	user, err := repository.UpsertCSUser(context.Background(), workbench.CSUserCommand{AssigneeID: " cs-1 ", AssigneeName: " 消息端A ", Role: " supervisor ", Enabled: true, AIEnabled: true, MaxSessions: 8, Password: "secret1"})
	if err != nil {
		t.Fatalf("UpsertCSUser returned error: %v", err)
	}
	if user.AssigneeID != "cs-1" || !user.HasPassword {
		t.Fatalf("user = %+v", user)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "password_hash = VALUES(password_hash)") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execArgs[0][0] != "cs-1" || db.execArgs[0][1] != "消息端A" || db.execArgs[0][2] != "supervisor" || db.execArgs[0][3] != 1 || db.execArgs[0][4] != 1 || db.execArgs[0][5] != 8 {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
	if got := db.execArgs[0][6].(string); got != "5b11618c2e44027877d0cd0921ed166b9f176f50587fc91e7534dd2946db77d6" {
		t.Fatalf("password hash = %q", got)
	}
}

func TestUpsertCSUserWithoutPasswordPreservesPasswordHash(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"cs-1", "消息端A", "cs", int64(1), int64(0), int64(0), "old-hash", nil, "2026-06-28 09:00:00", "2026-06-29 10:01:00"},
	}}}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	if _, err := repository.UpsertCSUser(context.Background(), workbench.CSUserCommand{AssigneeID: "cs-1", AssigneeName: "消息端A", Role: "cs", Enabled: true}); err != nil {
		t.Fatalf("UpsertCSUser returned error: %v", err)
	}
	if len(db.execs) != 1 || strings.Contains(db.execs[0], "password_hash") {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestDeleteCSUserUsesRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteCSUser(context.Background(), " cs-1 ")
	if err != nil {
		t.Fatalf("DeleteCSUser returned error: %v", err)
	}
	if !deleted || db.execArgs[0][0] != "cs-1" {
		t.Fatalf("deleted=%t args=%#v", deleted, db.execArgs)
	}
}

type fakeDB struct {
	rowsQueue []*fakeRows
	queries   []string
	args      [][]any
	execs     []string
	execArgs  [][]any
	result    fakeResult
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, append([]any{}, args...))
	if len(db.rowsQueue) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rowsQueue[0]
	db.rowsQueue = db.rowsQueue[1:]
	return rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, query)
	db.execArgs = append(db.execArgs, append([]any{}, args...))
	return db.result, nil
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

type fakeResult struct {
	affected int64
}

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return result.affected, nil
}
