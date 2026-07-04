// Package workbenchsensitivewords tests sensitive_words reads for admin candidates.
package workbenchsensitivewords

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestListSensitiveWordsReadsUpdatedOrder keeps DB column mapping stable.
func TestListSensitiveWordsReadsUpdatedOrder(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"sw-2", " 敏感词二 ", int64(1), nil, updatedAt},
		{[]byte("sw-1"), []byte("敏感词一"), []byte("0"), "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
		{"", "blank id", true, nil, nil},
	}}}}
	repository := &Repository{DB: db}

	words, err := repository.ListSensitiveWords(context.Background())
	if err != nil {
		t.Fatalf("ListSensitiveWords returned error: %v", err)
	}
	if len(words) != 2 {
		t.Fatalf("len(words) = %d; words=%+v", len(words), words)
	}
	if words[0].WordID != "sw-2" || words[0].Word != "敏感词二" || !words[0].Enabled || words[0].UpdatedAt != "2026-06-29T10:30:00Z" {
		t.Fatalf("first word = %+v", words[0])
	}
	if words[1].WordID != "sw-1" || words[1].Enabled || words[1].CreatedAt != "2026-06-28 09:00:00" {
		t.Fatalf("second word = %+v", words[1])
	}
	if db.queries[0] != "SELECT word_id, word, enabled, created_at, updated_at FROM sensitive_words ORDER BY updated_at DESC" || len(db.args[0]) != 0 {
		t.Fatalf("queries=%#v args=%#v", db.queries, db.args)
	}
}

// TestUpsertSensitiveWordPreservesExistingWord verifies Python's word dedupe.
func TestUpsertSensitiveWordPreservesExistingWord(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{
		{values: [][]any{{"sw-existing", "2026-06-28 09:00:00"}}},
		{values: [][]any{{"sw-existing", "敏感词", int64(1), "2026-06-28 09:00:00", "2026-06-29 09:00:00"}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	record, err := repository.UpsertSensitiveWord(context.Background(), workbench.SensitiveWordCommand{WordID: "sw-new", Word: " 敏感词 ", Enabled: true})
	if err != nil {
		t.Fatalf("UpsertSensitiveWord returned error: %v", err)
	}
	if record.WordID != "sw-existing" || record.Word != "敏感词" || !record.Enabled {
		t.Fatalf("record = %+v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execArgs[0][0] != "sw-existing" || db.execArgs[0][1] != "敏感词" || db.execArgs[0][2] != 1 || db.execArgs[0][3] != "2026-06-28 09:00:00" {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
}

// TestDeleteSensitiveWordUsesRowsAffected keeps delete success semantics stable.
func TestDeleteSensitiveWordUsesRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteSensitiveWord(context.Background(), " sw-1 ")
	if err != nil {
		t.Fatalf("DeleteSensitiveWord returned error: %v", err)
	}
	if !deleted || db.execArgs[0][0] != "sw-1" {
		t.Fatalf("deleted=%t args=%#v", deleted, db.execArgs)
	}
}

// TestReloadSensitiveWordCacheReadsEnabledWords verifies cache refresh evidence.
func TestReloadSensitiveWordCacheReadsEnabledWords(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{
		{"sw-1", "启用", int64(1), nil, nil},
		{"sw-2", "停用", int64(0), nil, nil},
	}}}}
	repository := &Repository{DB: db}

	if err := repository.ReloadSensitiveWordCache(context.Background()); err != nil {
		t.Fatalf("ReloadSensitiveWordCache returned error: %v", err)
	}
	if len(repository.cache) != 1 || repository.cache[0] != "启用" {
		t.Fatalf("cache = %#v", repository.cache)
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
