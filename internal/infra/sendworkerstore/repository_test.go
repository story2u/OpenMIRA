package sendworkerstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// TestUpsertWorkerHeartbeatMirrorsPythonSQLAndArgs protects heartbeat writes.
func TestUpsertWorkerHeartbeatMirrorsPythonSQLAndArgs(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: "mysql"}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	err := repository.UpsertWorkerHeartbeat(context.Background(), Heartbeat{
		WorkerID:         " worker-1 ",
		WorkerPool:       " pool-a ",
		Hostname:         " host-a ",
		VisibleDeviceIDs: []string{"zimo", "ada", "zimo", ""},
		OwnedDeviceIDs:   []string{"zimo", "ada", "zimo"},
		LeaseTTLSeconds:  0.5,
		Now:              now,
		Metadata:         map[string]any{"runtime": "sdk_dispatcher"},
	})
	if err != nil {
		t.Fatalf("UpsertWorkerHeartbeat returned error: %v", err)
	}
	if len(db.calls) != 4 {
		t.Fatalf("exec calls = %#v", db.calls)
	}
	worker := db.calls[0]
	if !strings.Contains(worker.query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("worker SQL = %s", worker.query)
	}
	if worker.args[0] != "worker-1" || worker.args[1] != "send-dispatcher" || worker.args[2] != "pool-a" || worker.args[3] != "host-a" {
		t.Fatalf("worker args = %#v", worker.args)
	}
	if worker.args[4] != 2 || worker.args[5] != 2 || worker.args[8] != `{"runtime":"sdk_dispatcher"}` {
		t.Fatalf("worker count/meta args = %#v", worker.args)
	}
	if !worker.args[6].(time.Time).Equal(now) || !worker.args[7].(time.Time).Equal(now.Add(time.Second)) {
		t.Fatalf("worker time args = %#v", worker.args[6:8])
	}
	if db.calls[1].query != "DELETE FROM send_worker_devices WHERE worker_id = ?" || db.calls[1].args[0] != "worker-1" {
		t.Fatalf("delete call = %#v", db.calls[1])
	}
	if !strings.Contains(db.calls[2].query, "send_worker_devices") || db.calls[2].args[1] != "zimo" || db.calls[3].args[1] != "ada" {
		t.Fatalf("device calls = %#v", db.calls[2:])
	}
}

// TestUpsertWorkerHeartbeatSQLiteSQLAndValidation protects alternate backend SQL.
func TestUpsertWorkerHeartbeatSQLiteSQLAndValidation(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: "sqlite"}
	err := repository.UpsertWorkerHeartbeat(context.Background(), Heartbeat{
		WorkerID:        "worker-1",
		WorkerRole:      "edge-agent",
		LeaseTTLSeconds: 30,
		Now:             time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertWorkerHeartbeat returned error: %v", err)
	}
	if len(db.calls) != 2 || !strings.Contains(db.calls[0].query, "ON CONFLICT(worker_id) DO UPDATE") {
		t.Fatalf("sqlite calls = %#v", db.calls)
	}
	if err := repository.UpsertWorkerHeartbeat(context.Background(), Heartbeat{}); err == nil {
		t.Fatal("missing worker_id returned nil error")
	}
	if err := (&Repository{}).UpsertWorkerHeartbeat(context.Background(), Heartbeat{WorkerID: "worker-1"}); err == nil {
		t.Fatal("missing database returned nil error")
	}
}

type execCall struct {
	query string
	args  []any
}

type fakeDB struct {
	calls []execCall
}

func (db *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.calls = append(db.calls, execCall{query: strings.TrimSpace(query), args: append([]any(nil), args...)})
	return fakeResult{}, nil
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (fakeResult) RowsAffected() (int64, error) {
	return 1, nil
}
