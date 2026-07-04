package archivecompensationtask

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestBuildTaskIdentityIsStable(t *testing.T) {
	first := BuildTaskIdentity(" ent-1 ", " self_decrypt ", " callback_timeout ", " cb-1 ")
	second := BuildTaskIdentity("ent-1", "self_decrypt", "callback_timeout", "cb-1")

	if first != second || len(first) != 64 {
		t.Fatalf("identity first=%q second=%q", first, second)
	}
}

func TestEnqueueInsertsNewTaskWithMySQLUpsert(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	available := time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC)
	identity := BuildTaskIdentity("ent-1", "self_decrypt", "callback_timeout", "cb-1")
	db := &fakeDB{rows: []fakeRow{
		{err: sql.ErrNoRows},
		taskRow("act-1", "ent-1", "self_decrypt", "callback_timeout", "cb-1", identity, 0, 3, "cursor-1", "pending", 0, "2026-06-30 18:05:00", "", "2026-06-30 18:00:00"),
	}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
		NewID:   func() string { return "act-1" },
	}

	task, err := repository.Enqueue(context.Background(), EnqueueInput{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		ReasonType:   " callback_timeout ",
		ReasonKey:    " cb-1 ",
		SeqStart:     -1,
		SeqEnd:       3,
		CursorHint:   " cursor-1 ",
		AvailableAt:  available,
	})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if task.TaskID != "act-1" || task.TaskIdentity != identity || task.SeqEnd != 3 || task.CursorHint != "cursor-1" {
		t.Fatalf("task = %#v", task)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "act-1" || args[1] != "ent-1" || args[5] != identity || args[6] != 0 || args[7] != 3 || args[8] != "cursor-1" || args[9] != "2026-06-30 18:05:00" {
		t.Fatalf("enqueue args = %#v", args)
	}
}

func TestEnqueuePreservesExistingTaskID(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	identity := BuildTaskIdentity("ent-1", "self_decrypt", "raw_message_gap", "gap-1")
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"act-existing", int64(2), "2026-06-29T10:00:00Z"}},
		taskRow("act-existing", "ent-1", "self_decrypt", "raw_message_gap", "gap-1", identity, 1, 9, "cursor", "pending", 2, "2026-06-30T10:00:00Z", "", "2026-06-29T10:00:00Z"),
	}}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time { return now }}

	task, err := repository.Enqueue(context.Background(), EnqueueInput{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		ReasonType:   "raw_message_gap",
		ReasonKey:    "gap-1",
		SeqStart:     1,
		SeqEnd:       9,
		CursorHint:   "cursor",
	})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if task.TaskID != "act-existing" {
		t.Fatalf("task = %#v", task)
	}
	if !strings.Contains(db.execs[0].query, "ON CONFLICT(task_identity) DO UPDATE") || db.execs[0].args[0] != "act-existing" || db.execs[0].args[10] != "2026-06-29T10:00:00Z" {
		t.Fatalf("exec = %#v", db.execs[0])
	}
}

func TestEnqueueRequiresReasonKey(t *testing.T) {
	_, err := (&Repository{DB: &fakeDB{}}).Enqueue(context.Background(), EnqueueInput{ReasonType: "callback_timeout"})
	if err == nil || !strings.Contains(err.Error(), "reason_key is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestPullPendingReturnsReadyTasks(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		taskRow("act-1", "ent-1", "self_decrypt", "callback_timeout", "cb-1", "identity", 0, 0, "", "pending", 0, "2026-06-30 18:00:00", "", "2026-06-30 18:00:00"),
	}}}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 1, 0, 0, time.UTC) }}

	tasks, err := repository.PullPending(context.Background(), 20)
	if err != nil {
		t.Fatalf("PullPending returned error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "act-1" || tasks[0].Status != "pending" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "status IN ('pending', 'running')") || db.queries[0].args[1] != 20 {
		t.Fatalf("query = %#v", db.queries)
	}
}

func TestMarkRetryRequeuesTask(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		taskRow("act-1", "ent-1", "self_decrypt", "callback_timeout", "cb-1", "identity", 0, 0, "", "pending", 1, "2026-06-30 18:02:00", "boom", "2026-06-30 18:00:00"),
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) }}

	task, err := repository.MarkRetry(context.Background(), " act-1 ", " boom ", 2*time.Minute)
	if err != nil {
		t.Fatalf("MarkRetry returned error: %v", err)
	}
	if task == nil || task.AttemptCount != 1 || task.LastError != "boom" {
		t.Fatalf("task = %#v", task)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "attempt_count = attempt_count + 1") || db.execs[0].args[0] != "2026-06-30 18:02:00" || db.execs[0].args[3] != "act-1" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

func TestPruneBeforeDeletesCompletedTasks(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		{values: []any{"act-completed"}},
	}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE status = 'completed'") || db.queries[0].args[0] != "2026-06-30 18:00:00" {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "DELETE FROM archive_compensation_tasks WHERE task_id IN (?)") || db.execs[0].args[0] != "act-completed" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

func taskRow(taskID string, enterpriseID string, source string, reasonType string, reasonKey string, identity string, seqStart int, seqEnd int, cursorHint string, status string, attempts int, availableAt string, lastError string, createdAt string) fakeRow {
	return fakeRow{values: []any{
		taskID,
		enterpriseID,
		source,
		reasonType,
		reasonKey,
		identity,
		int64(seqStart),
		int64(seqEnd),
		cursorHint,
		status,
		int64(attempts),
		availableAt,
		lastError,
		createdAt,
		createdAt,
	}}
}

type fakeDB struct {
	rows        []fakeRow
	rowIndex    int
	rowsets     []fakeRows
	rowsetIndex int
	execs       []fakeExec
	queries     []fakeQuery
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.queries = append(db.queries, fakeQuery{query: query, args: append([]any(nil), args...)})
	if db.rowIndex >= len(db.rows) {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rows[db.rowIndex]
	db.rowIndex++
	return row
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, fakeQuery{query: query, args: append([]any(nil), args...)})
	if db.rowsetIndex >= len(db.rowsets) {
		return &fakeRows{}, nil
	}
	rowset := &db.rowsets[db.rowsetIndex]
	db.rowsetIndex++
	return rowset, nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeQuery struct {
	query string
	args  []any
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
		target := dest[index].(*any)
		*target = value
	}
	return nil
}

type fakeRows struct {
	rows   []fakeRow
	index  int
	closed bool
	err    error
}

func (rows *fakeRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return sql.ErrNoRows
	}
	return rows.rows[rows.index-1].Scan(dest...)
}

func (rows *fakeRows) Close() error {
	rows.closed = true
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
