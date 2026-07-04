package tasks

import (
	"context"
	"testing"
	"time"
)

// TestUpdateTerminalStatusSyncsDeliveryAfterTaskUpsert protects write order.
func TestUpdateTerminalStatusSyncsDeliveryAfterTaskUpsert(t *testing.T) {
	traceID := "trace-golden-0001"
	events := []string{}
	store := newRecordingStore(Record{
		TaskID:    "task-golden-0001",
		Source:    "cloud-web",
		Target:    Target{AgentID: "sdk:zimo", DeviceID: "zimo"},
		TaskType:  "send_text",
		Payload:   map[string]any{"text": "hello"},
		Status:    StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC),
		TraceID:   &traceID,
	}, &events)
	delivery := &recordingDelivery{events: &events}
	service := NewService(store)
	service.Delivery = delivery
	service.Now = func() time.Time { return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC) }

	record, err := service.UpdateTerminalStatus(context.Background(), "task-golden-0001", StatusUpdate{Status: StatusSuccess})
	if err != nil {
		t.Fatalf("UpdateTerminalStatus returned error: %v", err)
	}
	if record.Status != StatusSuccess {
		t.Fatalf("Status = %q, want success", record.Status)
	}
	wantEvents := []string{"get", "upsert:success", "delivery:success"}
	if len(events) != len(wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	for index := range wantEvents {
		if events[index] != wantEvents[index] {
			t.Fatalf("events = %#v, want %#v", events, wantEvents)
		}
	}
	if len(delivery.updates) != 1 || delivery.updates[0].SendStatus != "success" || delivery.updates[0].TraceID != traceID {
		t.Fatalf("delivery updates = %#v", delivery.updates)
	}
}

// TestUpdateStatusDoesNotSyncDelivery keeps HTTP status route behavior separate.
func TestUpdateStatusDoesNotSyncDelivery(t *testing.T) {
	store := newRecordingStore(Record{TaskID: "task-golden-0001", Status: StatusRunning}, nil)
	delivery := &recordingDelivery{}
	service := NewService(store)
	service.Delivery = delivery

	if _, err := service.UpdateStatus(context.Background(), "task-golden-0001", StatusUpdate{Status: StatusFailed}); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if len(delivery.updates) != 0 {
		t.Fatalf("delivery updates = %#v, want none", delivery.updates)
	}
}

// TestUpdateTerminalStatusStoresExecutionTimestamps preserves SDK dispatch timing.
func TestUpdateTerminalStatusStoresExecutionTimestamps(t *testing.T) {
	traceID := "trace-golden-0002"
	store := newRecordingStore(Record{
		TaskID:    "task-golden-0002",
		Status:    StatusRunning,
		TraceID:   &traceID,
		CreatedAt: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
	}, nil)
	service := NewService(store)
	service.Now = func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) }
	startedAt := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 30, 9, 10, 5, 0, time.UTC)

	record, err := service.UpdateTerminalStatus(context.Background(), "task-golden-0002", StatusUpdate{
		Status:          StatusSuccess,
		UpdatedAt:       &finishedAt,
		DispatchedAt:    &startedAt,
		ScriptStartedAt: &startedAt,
	})
	if err != nil {
		t.Fatalf("UpdateTerminalStatus returned error: %v", err)
	}
	if !record.UpdatedAt.Equal(finishedAt) || record.DispatchedAt == nil || !record.DispatchedAt.Equal(startedAt) {
		t.Fatalf("record timestamps = %#v", record)
	}
	if record.ScriptStartedAt == nil || !record.ScriptStartedAt.Equal(startedAt) {
		t.Fatalf("script_started_at = %#v", record.ScriptStartedAt)
	}
	if store.task.DispatchedAt == nil || !store.task.DispatchedAt.Equal(startedAt) || !store.task.UpdatedAt.Equal(finishedAt) {
		t.Fatalf("stored timestamps = %#v", store.task)
	}
}

// TestDeliveryUpdateFromTaskMapsTerminalStatuses mirrors Python status mapping.
func TestDeliveryUpdateFromTaskMapsTerminalStatuses(t *testing.T) {
	taskError := " phone offline "
	record := Record{TaskID: "task-golden-0001", Status: StatusTimeout, Error: &taskError}
	update, ok := DeliveryUpdateFromTask(record)
	if !ok || update.SendStatus != "failed" || update.SendError != "phone offline" {
		t.Fatalf("update=%#v ok=%t", update, ok)
	}
	if _, ok := DeliveryUpdateFromTask(Record{TaskID: "task-golden-0001", Status: StatusRunning}); ok {
		t.Fatal("running task produced delivery update")
	}
}

type recordingStore struct {
	task   Record
	events *[]string
}

func newRecordingStore(task Record, events *[]string) *recordingStore {
	return &recordingStore{task: task, events: events}
}

func (store *recordingStore) Upsert(_ context.Context, task Record) error {
	store.task = task
	store.appendEvent("upsert:" + string(task.Status))
	return nil
}

func (store *recordingStore) Get(_ context.Context, _ string) (Record, bool, error) {
	store.appendEvent("get")
	return store.task, true, nil
}

func (store *recordingStore) List(context.Context, Query) ([]Record, error) {
	return []Record{store.task}, nil
}

func (store *recordingStore) appendEvent(event string) {
	if store.events != nil {
		*store.events = append(*store.events, event)
	}
}

type recordingDelivery struct {
	updates []OutgoingDeliveryUpdate
	events  *[]string
}

func (delivery *recordingDelivery) UpdateOutgoingMessageDeliveryStatus(_ context.Context, update OutgoingDeliveryUpdate) error {
	delivery.updates = append(delivery.updates, update)
	if delivery.events != nil {
		*delivery.events = append(*delivery.events, "delivery:"+update.SendStatus)
	}
	return nil
}
