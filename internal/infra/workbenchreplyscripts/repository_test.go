// Package workbenchreplyscripts tests reply_scripts reads for admin candidates.
package workbenchreplyscripts

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestListReplyScriptsReadsUpdatedOrder keeps DB column mapping stable.
func TestListReplyScriptsReadsUpdatedOrder(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"script-2", " 欢迎语二 ", " 内容二 ", " default ", int64(1), "all", nil, updatedAt},
		{[]byte("script-1"), []byte("欢迎语一"), []byte("内容一"), []byte("sales"), []byte("0"), []byte("cs-001"), "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
		{"", "blank id", "blank", "default", true, "", nil, nil},
	}}}}
	repository := &Repository{DB: db}

	scripts, err := repository.ListReplyScripts(context.Background())
	if err != nil {
		t.Fatalf("ListReplyScripts returned error: %v", err)
	}
	if len(scripts) != 2 {
		t.Fatalf("len(scripts) = %d; scripts=%+v", len(scripts), scripts)
	}
	if scripts[0].ScriptID != "script-2" || scripts[0].Title != "欢迎语二" || !scripts[0].Enabled || scripts[0].UpdatedAt != "2026-06-29T10:30:00Z" {
		t.Fatalf("first script = %+v", scripts[0])
	}
	if scripts[1].ScriptID != "script-1" || scripts[1].Enabled || scripts[1].TargetAudience != "cs-001" {
		t.Fatalf("second script = %+v", scripts[1])
	}
	if db.queries[0] != "SELECT script_id, title, content, category, enabled, target_audience, created_at, updated_at FROM reply_scripts ORDER BY updated_at DESC" || len(db.args[0]) != 0 {
		t.Fatalf("queries=%#v args=%#v", db.queries, db.args)
	}
}

// TestUpsertReplyScriptWritesAndReadsBack verifies Python-compatible upsert.
func TestUpsertReplyScriptWritesAndReadsBack(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"script-1", "欢迎语", "您好", "default", int64(1), "__NONE__", "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	record, err := repository.UpsertReplyScript(context.Background(), workbench.ReplyScriptCommand{
		ScriptID:       " script-1 ",
		Title:          " 欢迎语 ",
		Content:        " 您好 ",
		Category:       "",
		Enabled:        true,
		TargetAudience: " __NONE__ ",
	})
	if err != nil {
		t.Fatalf("UpsertReplyScript returned error: %v", err)
	}
	if record.ScriptID != "script-1" || record.Title != "欢迎语" || record.Category != "default" || !record.Enabled {
		t.Fatalf("record = %+v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execArgs[0][0] != "script-1" || db.execArgs[0][1] != "欢迎语" || db.execArgs[0][2] != "您好" || db.execArgs[0][3] != "default" || db.execArgs[0][4] != 1 || db.execArgs[0][5] != "__NONE__" {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
	if db.queries[0] != "SELECT script_id, title, content, category, enabled, target_audience, created_at, updated_at FROM reply_scripts WHERE script_id = ?" || db.args[0][0] != "script-1" {
		t.Fatalf("queries=%#v args=%#v", db.queries, db.args)
	}
}

// TestDeleteReplyScriptUsesRowsAffected keeps delete success semantics stable.
func TestDeleteReplyScriptUsesRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteReplyScript(context.Background(), " script-1 ")
	if err != nil {
		t.Fatalf("DeleteReplyScript returned error: %v", err)
	}
	if !deleted || db.execArgs[0][0] != "script-1" {
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
