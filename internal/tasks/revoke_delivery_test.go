package tasks

import (
	"context"
	"testing"
	"time"
)

func TestRevokeUpdateFromTaskMapsTerminalStatus(t *testing.T) {
	updatedAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	record := Record{
		TaskID:    "task-revoke-1",
		TaskType:  "revoke_text_message",
		Payload:   map[string]any{"target_trace_id": "trace-1"},
		Status:    StatusSuccess,
		UpdatedAt: updatedAt,
	}

	update, ok := RevokeUpdateFromTask(record)
	if !ok || update.TraceID != "trace-1" || update.TaskID != "task-revoke-1" || update.RevokeStatus != "success" || update.RevokedAt == nil || !update.RevokedAt.Equal(updatedAt) {
		t.Fatalf("update = %+v ok=%t", update, ok)
	}

	detail := "sdk failed"
	record.Status = StatusTimeout
	record.Error = &detail
	update, ok = RevokeUpdateFromTask(record)
	if !ok || update.RevokeStatus != "failed" || update.RevokeError != "sdk failed" || update.RevokedAt != nil {
		t.Fatalf("failed update = %+v ok=%t", update, ok)
	}

	record.TaskType = "send_text"
	if update, ok := RevokeUpdateFromTask(record); ok {
		t.Fatalf("non-revoke task produced update: %+v", update)
	}
}

func TestUpdateTerminalStatusSyncsMessageRevoke(t *testing.T) {
	store := NewMemoryStore()
	revoke := &fakeRevokeUpdater{}
	service := NewService(store)
	service.Revoke = revoke
	service.Now = func() time.Time {
		return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	}
	_, err := service.Create(context.Background(), CreateRequest{
		TaskID:    "task-revoke-1",
		Source:    "cloud-web",
		Target:    Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType:  "revoke_text_message",
		Payload:   map[string]any{"target_trace_id": "trace-1", "username": "客户一", "receiver": "客户一", "target_content": "hello", "target_msg_type": "text", "occurrence_from_bottom": 1},
		CreatedAt: time.Date(2026, 7, 1, 9, 59, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	_, err = service.UpdateTerminalStatus(context.Background(), "task-revoke-1", StatusUpdate{Status: StatusSuccess})
	if err != nil {
		t.Fatalf("UpdateTerminalStatus returned error: %v", err)
	}
	if len(revoke.updates) != 1 || revoke.updates[0].TraceID != "trace-1" || revoke.updates[0].RevokeStatus != "success" || revoke.updates[0].RevokedAt == nil {
		t.Fatalf("unexpected revoke updates: %+v", revoke.updates)
	}
}

type fakeRevokeUpdater struct {
	updates []MessageRevokeUpdate
}

func (updater *fakeRevokeUpdater) UpdateMessageRevokeStatus(_ context.Context, update MessageRevokeUpdate) error {
	updater.updates = append(updater.updates, update)
	return nil
}
