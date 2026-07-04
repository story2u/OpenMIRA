package errorevents

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wework-go/internal/clienterrors"
	"wework-go/internal/infra/sqldb"
)

func TestCaptureClientEventInsertsMySQLRecord(t *testing.T) {
	db := &fakeDB{}
	occurredAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 29, 10, 1, 0, 0, time.UTC)
	repository := &Repository{
		DB:          db,
		Dialect:     sqldb.DialectMySQL,
		Now:         func() time.Time { return createdAt },
		NextEventID: func() string { return "evt-1" },
	}

	err := repository.CaptureClientEvent(context.Background(), clienterrors.ErrorEvent{
		Level:          "ERROR",
		SourceType:     "client",
		EventCategory:  "js_error",
		EventCode:      "window.onerror",
		Module:         "client.runtime",
		Action:         "window.onerror",
		Detail:         "boom",
		TraceID:        "trace-1",
		TenantID:       "tenant-1",
		ConversationID: "conv-1",
		ScopeType:      "client",
		ScopeID:        "window.onerror",
		ErrorType:      "ClientRuntimeError",
		StackTrace:     "line1",
		Context:        map[string]any{"client_ip": "198.51.100.10"},
		OccurredAt:     occurredAt,
	})
	if err != nil {
		t.Fatalf("CaptureClientEvent returned error: %v", err)
	}
	if !strings.Contains(db.query, "INSERT INTO error_events") || !strings.Contains(db.query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected query:\n%s", db.query)
	}
	if len(db.args) != 21 {
		t.Fatalf("args len = %d, want 21", len(db.args))
	}
	if db.args[0] != "evt-1" || db.args[2] != "ERROR" || db.args[3] != "client" || db.args[6] != "client.runtime" || db.args[16] != "boom" {
		t.Fatalf("unexpected args: %#v", db.args)
	}
	if db.args[19] != "2026-06-29 18:00:00" || db.args[20] != "2026-06-29 18:01:00" {
		t.Fatalf("time args = %#v %#v", db.args[19], db.args[20])
	}
	var contextJSON map[string]any
	if err := json.Unmarshal([]byte(db.args[18].(string)), &contextJSON); err != nil {
		t.Fatalf("context json: %v", err)
	}
	if contextJSON["client_ip"] != "198.51.100.10" {
		t.Fatalf("context = %#v", contextJSON)
	}
}

func TestCaptureClientEventUsesPostgresUpsert(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{
		DB:          db,
		Dialect:     sqldb.DialectPostgres,
		Now:         func() time.Time { return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC) },
		NextEventID: func() string { return "evt-pg" },
	}
	err := repository.CaptureClientEvent(context.Background(), clienterrors.ErrorEvent{
		Level:      "WARN",
		Module:     "client.api",
		Action:     "fetch",
		Detail:     "gateway warning",
		OccurredAt: time.Date(2026, 6, 29, 9, 59, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CaptureClientEvent returned error: %v", err)
	}
	if !strings.Contains(db.query, "ON CONFLICT(event_id) DO UPDATE") {
		t.Fatalf("unexpected query:\n%s", db.query)
	}
	if _, ok := db.args[19].(time.Time); !ok {
		t.Fatalf("postgres occurred_at arg type = %T, want time.Time", db.args[19])
	}
}

type fakeDB struct {
	query string
	args  []any
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.query = query
	db.args = args
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (result fakeResult) RowsAffected() (int64, error) { return int64(result), nil }
