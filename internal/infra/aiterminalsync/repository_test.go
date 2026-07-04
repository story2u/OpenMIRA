package aiterminalsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wework-go/internal/senddispatcher"
)

// TestRepositorySyncAITerminalStateUpdatesLatestAttempt mirrors Python update_attempt terminal writes.
func TestRepositorySyncAITerminalStateUpdatesLatestAttempt(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 30, 9, 0, 5, 500*1000*1000, time.UTC)
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"attempt-1", "task-old", "trace-old", "2026-06-30 16:59:59"}},
		{err: sql.ErrNoRows},
	}}
	repository := &Repository{DB: db, Dialect: "mysql", Now: func() time.Time { return now }}

	err := repository.SyncAITerminalState(context.Background(), senddispatcher.AITerminalSyncUpdate{
		TaskID:          "task-1",
		TraceID:         "trace-1",
		AttemptStatus:   "sent",
		RuntimeStatus:   "sent",
		RuntimePhase:    "message_send_finished",
		FinishedAt:      &finishedAt,
		ProviderError:   "",
		UserFacingError: "",
	})
	if err != nil {
		t.Fatalf("SyncAITerminalState returned error: %v", err)
	}
	if !strings.Contains(db.queries[0], "FROM ai_reply_attempts") || !strings.Contains(db.queries[0], "task_id = ? OR outgoing_trace_id = ?") {
		t.Fatalf("query = %s", db.queries[0])
	}
	if len(db.queryArgsList[0]) != 2 || db.queryArgsList[0][0] != "task-1" || db.queryArgsList[0][1] != "trace-1" {
		t.Fatalf("query args = %#v", db.queryArgsList[0])
	}
	if !strings.Contains(db.execQuery, "UPDATE ai_reply_attempts") {
		t.Fatalf("exec query = %s", db.execQuery)
	}
	if db.execArgs[0] != "sent" || db.execArgs[1] != "" || db.execArgs[4] != "trace-1" || db.execArgs[5] != "task-1" {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if db.execArgs[6] != "2026-06-30 17:00:05" || db.execArgs[8] != "2026-06-30 18:00:00" || db.execArgs[9] != "attempt-1" {
		t.Fatalf("time args = %#v", db.execArgs)
	}
	duration, ok := db.execArgs[7].(float64)
	if !ok || duration != 6500 {
		t.Fatalf("total_duration_ms = %#v", db.execArgs[7])
	}
}

// TestRepositorySyncAITerminalStateKeepsArchiveConfirmationOpen mirrors commit-unknown wait semantics.
func TestRepositorySyncAITerminalStateKeepsArchiveConfirmationOpen(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"attempt-archive", "task-archive", "trace-archive", "2026-06-30T08:59:59Z"}},
		{err: sql.ErrNoRows},
	}}
	repository := &Repository{
		DB:      db,
		Dialect: "postgres",
		Now:     func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) },
	}

	err := repository.SyncAITerminalState(context.Background(), senddispatcher.AITerminalSyncUpdate{
		TaskID:                     "task-archive",
		TraceID:                    "trace-archive",
		AttemptStatus:              "archive_confirming",
		FailureType:                "commit_unknown_wait_archive",
		ProviderError:              "wait_chat_compose_ready timeout context=album_send",
		UserFacingError:            "wait_chat_compose_ready timeout context=album_send",
		ArchiveConfirmationPending: true,
	})
	if err != nil {
		t.Fatalf("SyncAITerminalState returned error: %v", err)
	}
	if db.execArgs[0] != "archive_confirming" || db.execArgs[1] != "commit_unknown_wait_archive" {
		t.Fatalf("status args = %#v", db.execArgs)
	}
	if db.execArgs[6] != nil || db.execArgs[7] != nil {
		t.Fatalf("archive wait should keep finished_at/duration nil: %#v", db.execArgs)
	}
	if db.execArgs[8] != "2026-06-30T18:00:00+08:00" {
		t.Fatalf("updated_at = %#v", db.execArgs[8])
	}
}

// TestRepositorySyncAITerminalStateUpdatesConversationRuntime mirrors guarded sop_runtime_state updates.
func TestRepositorySyncAITerminalStateUpdatesConversationRuntime(t *testing.T) {
	hub := &recordingHub{}
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"attempt-runtime", "task-runtime", "trace-runtime", "2026-06-30T08:59:59Z"}},
		{values: []any{"conv-from-message", "sender-from-message"}},
		{values: []any{"conv-from-message", `{"last_mode":"coze_auto_reply","ai_reply_task_id":"task-runtime","ai_reply_message_trace_id":"trace-runtime","ai_reply_processing_started_at":"2026-06-30T08:59:00Z","ai_reply_force_manual":true,"ai_reply_job_id":"job-1","ai_reply_preview":"hello"}`}},
	}}
	repository := &Repository{
		DB:      db,
		Dialect: "mysql",
		Now:     func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) },
		Hub:     hub,
	}

	err := repository.SyncAITerminalState(context.Background(), senddispatcher.AITerminalSyncUpdate{
		TaskID:                     "task-runtime",
		TraceID:                    "trace-runtime",
		ConversationID:             "conv-from-payload",
		DeviceID:                   "device-1",
		SenderID:                   "sender-from-payload",
		SendStatus:                 "failed",
		SendError:                  "wait_chat_compose_ready timeout context=album_send",
		AttemptStatus:              "archive_confirming",
		FailureType:                "commit_unknown_wait_archive",
		RuntimeStatus:              "sending",
		RuntimePhase:               "archive_confirmation_pending",
		RuntimeError:               "wait_chat_compose_ready timeout context=album_send",
		KeepProcessingStartedAt:    true,
		ArchiveConfirmationPending: true,
	})
	if err != nil {
		t.Fatalf("SyncAITerminalState returned error: %v", err)
	}
	if len(db.execs) != 2 || !strings.Contains(db.execs[1].query, "UPDATE conversations") {
		t.Fatalf("execs = %#v", db.execs)
	}
	runtimeState := map[string]any{}
	if err := json.Unmarshal([]byte(db.execs[1].args[0].(string)), &runtimeState); err != nil {
		t.Fatalf("runtime JSON failed: %v", err)
	}
	if runtimeState["ai_reply_status"] != "sending" || runtimeState["ai_reply_phase"] != "archive_confirmation_pending" {
		t.Fatalf("runtime state = %#v", runtimeState)
	}
	if runtimeState["ai_reply_processing_started_at"] != "2026-06-30T08:59:00Z" || runtimeState["ai_reply_force_manual"] != false {
		t.Fatalf("runtime guarded fields = %#v", runtimeState)
	}
	if runtimeState["ai_reply_error"] != "wait_chat_compose_ready timeout context=album_send" {
		t.Fatalf("runtime error = %#v", runtimeState)
	}
	if db.execs[1].args[1] != "2026-06-30 18:00:00" || db.execs[1].args[2] != "conv-from-message" {
		t.Fatalf("conversation update args = %#v", db.execs[1].args)
	}
	if len(hub.events) != 1 {
		t.Fatalf("hub events = %#v", hub.events)
	}
	event := hub.events[0]
	if event.channel != "conversations" || event.event != "conversation.ai_reply_status" || event.topic != "conversation.ai_reply_status" {
		t.Fatalf("hub event = %#v", event)
	}
	if event.payload["conversation_id"] != "conv-from-message" || event.payload["sender_id"] != "sender-from-message" || event.payload["device_id"] != "device-1" {
		t.Fatalf("hub payload identity = %#v", event.payload)
	}
	if event.payload["source"] != "coze-auto-reply" || event.payload["ai_trace_id"] != "job-1" || event.payload["reply_preview"] != "hello" {
		t.Fatalf("hub payload runtime = %#v", event.payload)
	}
}

// TestRepositorySyncAITerminalStateFallsBackToExistingIdentifiers protects partial terminal updates.
func TestRepositorySyncAITerminalStateFallsBackToExistingIdentifiers(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"attempt-1", "task-old", "trace-old", "2026-06-30 16:59:59"}},
		{err: sql.ErrNoRows},
	}}
	repository := &Repository{DB: db, Dialect: "mysql", Now: func() time.Time {
		return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	}}

	err := repository.SyncAITerminalState(context.Background(), senddispatcher.AITerminalSyncUpdate{
		TraceID:       "trace-1",
		AttemptStatus: "send_failed",
		FailureType:   "task_failed",
		ProviderError: "phone offline",
	})
	if err != nil {
		t.Fatalf("SyncAITerminalState returned error: %v", err)
	}
	if db.execArgs[4] != "trace-1" || db.execArgs[5] != "task-old" {
		t.Fatalf("identifier fallback args = %#v", db.execArgs)
	}
	if db.execArgs[6] != "2026-06-30 18:00:00" {
		t.Fatalf("finished_at fallback = %#v", db.execArgs[6])
	}
}

// TestRepositorySyncAITerminalStateNoopsWithoutAttempt matches Python best-effort missing attempt behavior.
func TestRepositorySyncAITerminalStateNoopsWithoutAttempt(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	err := repository.SyncAITerminalState(context.Background(), senddispatcher.AITerminalSyncUpdate{
		TaskID:        "task-missing",
		AttemptStatus: "sent",
	})
	if err != nil {
		t.Fatalf("SyncAITerminalState returned error: %v", err)
	}
	if db.execQuery != "" {
		t.Fatalf("unexpected update query: %s", db.execQuery)
	}
}

type fakeDB struct {
	query         string
	queryArgs     []any
	queries       []string
	queryArgsList [][]any
	execQuery     string
	execArgs      []any
	row           fakeRow
	rows          []fakeRow
	execs         []execCall
}

func (db *fakeDB) QueryRowContext(_ context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	db.queries = append(db.queries, query)
	db.queryArgsList = append(db.queryArgsList, args)
	if len(db.rows) > 0 {
		row := db.rows[0]
		db.rows = db.rows[1:]
		return row
	}
	return db.row
}

func (db *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	db.execs = append(db.execs, execCall{query: query, args: args})
	return fakeResult(1), nil
}

type execCall struct {
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
	if len(row.values) < len(dest) {
		return sql.ErrNoRows
	}
	for index := range dest {
		*(dest[index].(*any)) = row.values[index]
	}
	return nil
}

type recordingHub struct {
	events []hubEvent
}

func (hub *recordingHub) Publish(_ context.Context, channel string, event string, topic string, payload map[string]any) error {
	hub.events = append(hub.events, hubEvent{channel: channel, event: event, topic: topic, payload: payload})
	return nil
}

type hubEvent struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return int64(result), nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
