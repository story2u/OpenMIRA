package senddispatcher

import (
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestBuildAITerminalSyncUpdateSuccess mirrors sent attempt/runtime decisions.
func TestBuildAITerminalSyncUpdateSuccess(t *testing.T) {
	traceID := "trace-success"
	record := tasks.Record{
		TaskID:    "task-success",
		Target:    tasks.Target{DeviceID: "device-1"},
		Status:    tasks.StatusSuccess,
		TraceID:   &traceID,
		UpdatedAt: time.Date(2026, 6, 30, 9, 0, 3, 0, time.UTC),
		Payload:   map[string]any{"conversation_id": "conv-1", "sender_id": "sender-1"},
	}

	update, ok := BuildAITerminalSyncUpdate(record, map[string]any{"success": true})
	if !ok {
		t.Fatal("expected AI terminal update")
	}
	if update.AttemptStatus != "sent" || update.RuntimeStatus != "sent" || update.RuntimePhase != "message_send_finished" {
		t.Fatalf("update = %#v", update)
	}
	if update.FinishedAt == nil || !update.FinishedAt.Equal(record.UpdatedAt) {
		t.Fatalf("finished_at = %#v", update.FinishedAt)
	}
	if update.DeviceID != "device-1" || update.SenderID != "sender-1" || update.ConversationID != "conv-1" {
		t.Fatalf("identity fields = %#v", update)
	}
}

// TestBuildAITerminalSyncUpdateFailed mirrors ordinary send_failed state.
func TestBuildAITerminalSyncUpdateFailed(t *testing.T) {
	errorText := "phone offline"
	record := tasks.Record{
		TaskID:    "task-failed",
		Status:    tasks.StatusFailed,
		Error:     &errorText,
		UpdatedAt: time.Date(2026, 6, 30, 9, 0, 3, 0, time.UTC),
		Payload:   map[string]any{"session_id": "conv-1"},
	}

	update, ok := BuildAITerminalSyncUpdate(record, map[string]any{"success": false})
	if !ok {
		t.Fatal("expected AI terminal update")
	}
	if update.AttemptStatus != "send_failed" || update.FailureType != "task_failed" || update.RuntimeStatus != "failed" || update.RuntimePhase != "message_send_failed" {
		t.Fatalf("update = %#v", update)
	}
	if update.ProviderError != "phone offline" || update.UserFacingError != "phone offline" || update.RuntimeError != "phone offline" {
		t.Fatalf("update errors = %#v", update)
	}
}

// TestBuildAITerminalSyncUpdateCommitUnknownWaitsArchive mirrors archive_confirming behavior.
func TestBuildAITerminalSyncUpdateCommitUnknownWaitsArchive(t *testing.T) {
	errorText := "wait_chat_compose_ready timeout context=album_send"
	record := tasks.Record{
		TaskID:    "task-ai-commit-unknown",
		Status:    tasks.StatusFailed,
		Error:     &errorText,
		UpdatedAt: time.Date(2026, 4, 20, 12, 0, 3, 0, time.UTC),
		Payload:   map[string]any{"conversation_id": "conv-archive"},
	}

	update, ok := BuildAITerminalSyncUpdate(record, map[string]any{
		"success":                 false,
		"sdk_failure_commit_risk": "unknown_after_commit_attempt",
	})
	if !ok {
		t.Fatal("expected AI terminal update")
	}
	if update.AttemptStatus != "archive_confirming" || update.FailureType != "commit_unknown_wait_archive" {
		t.Fatalf("update = %#v", update)
	}
	if update.FinishedAt != nil {
		t.Fatalf("finished_at should remain nil while waiting archive, got %#v", update.FinishedAt)
	}
	if update.RuntimeStatus != "sending" || update.RuntimePhase != "archive_confirmation_pending" || !update.KeepProcessingStartedAt || !update.ArchiveConfirmationPending {
		t.Fatalf("runtime update = %#v", update)
	}
}
