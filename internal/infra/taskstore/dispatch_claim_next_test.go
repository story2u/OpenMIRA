package taskstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestClaimNextSDKDispatchTaskSelectsAndClaimsInTransaction freezes the claim sequence.
func TestClaimNextSDKDispatchTaskSelectsAndClaimsInTransaction(t *testing.T) {
	tx := &fakeTaskStoreTx{
		rows: []RowScanner{
			fakeRow{values: taskRow("task-golden-0001", "accepted")},
			fakeRow{values: taskRow("task-golden-0001", "running")},
		},
	}
	source := &fakeTransactioner{tx: tx}
	repository := Repository{Tx: source}
	now := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)

	record, ok, err := repository.ClaimNextSDKDispatchTask(context.Background(), SDKDispatchClaimQuery{
		TaskTypes:           []string{"send_text"},
		ForUpdateSkipLocked: true,
	}, "worker-1", now)
	if err != nil || !ok {
		t.Fatalf("ClaimNextSDKDispatchTask returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if source.beginCount != 1 || !tx.committed || tx.rolledBack {
		t.Fatalf("transaction state begin=%d committed=%t rolledBack=%t", source.beginCount, tx.committed, tx.rolledBack)
	}
	if record.TaskID != "task-golden-0001" || record.Status != tasks.StatusRunning {
		t.Fatalf("unexpected record: %#v", record)
	}
	if len(tx.queryLog) != 2 {
		t.Fatalf("query log = %#v", tx.queryLog)
	}
	for _, fragment := range []string{"status = ?", "target_agent_id LIKE ?", "LIMIT 1 FOR UPDATE SKIP LOCKED"} {
		if !strings.Contains(tx.queryLog[0], fragment) {
			t.Fatalf("claim select missing %q: %s", fragment, tx.queryLog[0])
		}
	}
	if tx.queryArgsLog[0][0] != "accepted" || tx.queryArgsLog[0][1] != "sdk:%" || tx.queryArgsLog[0][2] != "send_text" {
		t.Fatalf("unexpected select args: %#v", tx.queryArgsLog[0])
	}
	if !strings.Contains(tx.execQuery, "WHERE task_id = ? AND status = ?") {
		t.Fatalf("claim update missing status guard: %s", tx.execQuery)
	}
	if tx.execArgs[0] != "running" || tx.execArgs[2] != "claimed by sdk dispatcher worker_id=worker-1" || tx.execArgs[4] != "accepted" {
		t.Fatalf("unexpected update args: %#v", tx.execArgs)
	}
	if tx.queryArgsLog[1][0] != "task-golden-0001" {
		t.Fatalf("unexpected readback args: %#v", tx.queryArgsLog[1])
	}
}

// TestClaimNextSDKDispatchTaskCommitsEmptySelection mirrors Python no-row behavior.
func TestClaimNextSDKDispatchTaskCommitsEmptySelection(t *testing.T) {
	tx := &fakeTaskStoreTx{rows: []RowScanner{fakeRow{err: sql.ErrNoRows}}}
	source := &fakeTransactioner{tx: tx}
	repository := Repository{Tx: source}

	record, ok, err := repository.ClaimNextSDKDispatchTask(context.Background(), SDKDispatchClaimQuery{TaskTypes: []string{"send_text"}}, "worker-1", time.Now())
	if err != nil || ok || record.TaskID != "" {
		t.Fatalf("ClaimNextSDKDispatchTask returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if source.beginCount != 1 || !tx.committed || tx.rolledBack {
		t.Fatalf("transaction state begin=%d committed=%t rolledBack=%t", source.beginCount, tx.committed, tx.rolledBack)
	}
	if tx.execQuery != "" {
		t.Fatalf("empty selection updated database: %s", tx.execQuery)
	}
}

// TestClaimNextSDKDispatchTaskRejectsInvalidQueryBeforeTransaction aligns ValueError fallback.
func TestClaimNextSDKDispatchTaskRejectsInvalidQueryBeforeTransaction(t *testing.T) {
	source := &fakeTransactioner{tx: &fakeTaskStoreTx{}}
	repository := Repository{Tx: source}

	record, ok, err := repository.ClaimNextSDKDispatchTask(context.Background(), SDKDispatchClaimQuery{}, "worker-1", time.Now())
	if err != nil || ok || record.TaskID != "" {
		t.Fatalf("ClaimNextSDKDispatchTask returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if source.beginCount != 0 {
		t.Fatalf("invalid query started transaction %d times", source.beginCount)
	}
}

type fakeTransactioner struct {
	tx         *fakeTaskStoreTx
	beginCount int
}

func (source *fakeTransactioner) BeginTaskStoreTx(context.Context) (TaskStoreTx, error) {
	source.beginCount++
	return source.tx, nil
}

type fakeTaskStoreTx struct {
	fakeDB
	rows             []RowScanner
	resultRows       [][]any
	queryLog         []string
	queryArgsLog     [][]any
	queryContextLog  []string
	queryContextArgs [][]any
	execCount        int
	committed        bool
	rolledBack       bool
}

func (tx *fakeTaskStoreTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx.execCount++
	return tx.fakeDB.ExecContext(ctx, query, args...)
}

func (tx *fakeTaskStoreTx) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	tx.query = query
	tx.queryArgs = args
	tx.queryContextLog = append(tx.queryContextLog, query)
	tx.queryContextArgs = append(tx.queryContextArgs, args)
	return &fakeRows{rows: tx.resultRows}, nil
}

func (tx *fakeTaskStoreTx) QueryRowContext(_ context.Context, query string, args ...any) RowScanner {
	tx.query = query
	tx.queryArgs = args
	tx.queryLog = append(tx.queryLog, query)
	tx.queryArgsLog = append(tx.queryArgsLog, args)
	if len(tx.rows) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := tx.rows[0]
	tx.rows = tx.rows[1:]
	return row
}

func (tx *fakeTaskStoreTx) Commit() error {
	tx.committed = true
	return nil
}

func (tx *fakeTaskStoreTx) Rollback() error {
	tx.rolledBack = true
	return nil
}
