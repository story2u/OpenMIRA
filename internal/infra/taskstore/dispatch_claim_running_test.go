package taskstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestClaimSDKDispatchRowMarksRunningWithStatusGuard protects the durable claim transition.
func TestClaimSDKDispatchRowMarksRunningWithStatusGuard(t *testing.T) {
	db := &fakeDB{row: taskRow("task-golden-0001", "running")}
	repository := Repository{DB: db}
	now := time.Date(2026, 6, 29, 13, 14, 15, 0, time.FixedZone("CST", 8*60*60))

	record, ok, err := repository.ClaimSDKDispatchRow(context.Background(), " task-golden-0001 ", " worker-1 ", now)
	if err != nil || !ok {
		t.Fatalf("ClaimSDKDispatchRow returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if record.TaskID != "task-golden-0001" || record.Status != tasks.StatusRunning {
		t.Fatalf("unexpected claimed record: %#v", record)
	}
	for _, fragment := range []string{
		"UPDATE tasks",
		"SET status = ?, updated_at = ?, error = ?",
		"WHERE task_id = ? AND status = ?",
	} {
		if !strings.Contains(db.execQuery, fragment) {
			t.Fatalf("exec query missing %q: %s", fragment, db.execQuery)
		}
	}
	if len(db.execArgs) != 5 {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if db.execArgs[0] != "running" || db.execArgs[2] != "claimed by sdk dispatcher worker_id=worker-1" || db.execArgs[3] != "task-golden-0001" || db.execArgs[4] != "accepted" {
		t.Fatalf("unexpected exec args: %#v", db.execArgs)
	}
	updatedAt, ok := db.execArgs[1].(time.Time)
	if !ok || !updatedAt.Equal(now.UTC()) {
		t.Fatalf("updated_at arg = %#v, want UTC %s", db.execArgs[1], now.UTC())
	}
	if len(db.queryArgs) != 1 || db.queryArgs[0] != "task-golden-0001" {
		t.Fatalf("unexpected readback args: %#v", db.queryArgs)
	}
}

// TestClaimSDKDispatchRowUsesUnknownWorker mirrors Python fallback detail text.
func TestClaimSDKDispatchRowUsesUnknownWorker(t *testing.T) {
	db := &fakeDB{row: taskRow("task-golden-0001", "running")}
	repository := Repository{DB: db}

	_, ok, err := repository.ClaimSDKDispatchRow(context.Background(), "task-golden-0001", " ", time.Date(2026, 6, 29, 13, 14, 15, 0, time.UTC))
	if err != nil || !ok {
		t.Fatalf("ClaimSDKDispatchRow ok=%t err=%v", ok, err)
	}
	if db.execArgs[2] != "claimed by sdk dispatcher worker_id=unknown" {
		t.Fatalf("detail = %#v", db.execArgs[2])
	}
}

// TestClaimSDKDispatchRowIgnoresBlankTaskID avoids ambiguous updates.
func TestClaimSDKDispatchRowIgnoresBlankTaskID(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db}

	record, ok, err := repository.ClaimSDKDispatchRow(context.Background(), " ", "worker-1", time.Now())
	if err != nil || ok || record.TaskID != "" {
		t.Fatalf("ClaimSDKDispatchRow returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if db.execQuery != "" || db.query != "" {
		t.Fatalf("blank task id touched database: exec=%q query=%q", db.execQuery, db.query)
	}
}
