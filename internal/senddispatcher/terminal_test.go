package senddispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestBuildTaskStatusPayloadMirrorsPythonEnvelope protects task.status realtime fields.
func TestBuildTaskStatusPayloadMirrorsPythonEnvelope(t *testing.T) {
	traceID := " trace-terminal-1 "
	taskError := " phone offline "
	dispatchedAt := time.Date(2026, 6, 30, 9, 10, 0, 0, time.UTC)
	scriptStartedAt := time.Date(2026, 6, 30, 9, 10, 1, 450000000, time.UTC)
	record := tasks.Record{
		TaskID:          "task-terminal-1",
		Target:          tasks.Target{DeviceID: " zimo "},
		TaskType:        " send_text ",
		Status:          tasks.StatusFailed,
		Error:           &taskError,
		TraceID:         &traceID,
		CreatedAt:       time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 6, 30, 9, 11, 0, 0, time.UTC),
		DispatchedAt:    &dispatchedAt,
		ScriptStartedAt: &scriptStartedAt,
		Payload: map[string]any{
			"conversation_id": " conversation-1 ",
			"session_id":      " session-1 ",
			"sender_id":       " sender-1 ",
			"entity":          " customer ",
			"receiver":        " Qiu ",
			"aliases":         " A ",
			"target_trace_id": " target-trace-1 ",
		},
	}

	event := BuildTaskStatusEvent(record, map[string]any{"source": "sdk_executor", "success": false})
	payload := event.Payload

	if event.Channel != "tasks" || event.Event != "task.status" || event.Topic != "task.status" {
		t.Fatalf("event = %#v", event)
	}
	if payload["task_id"] != "task-terminal-1" || payload["trace_id"] != "trace-terminal-1" || payload["status"] != "failed" {
		t.Fatalf("core payload = %#v", payload)
	}
	if payload["error"] != "phone offline" || payload["device_id"] != "zimo" || payload["command_type"] != "send_text" {
		t.Fatalf("error/device payload = %#v", payload)
	}
	if payload["conversation_id"] != "conversation-1" || payload["session_id"] != "session-1" || payload["receiver"] != "Qiu" {
		t.Fatalf("message payload = %#v", payload)
	}
	if payload["created_at"] != "2026-06-30T09:00:00+00:00" || payload["updated_at"] != "2026-06-30T09:11:00+00:00" {
		t.Fatalf("time payload = %#v", payload)
	}
	if payload["dispatched_at"] != "2026-06-30T09:10:00+00:00" || payload["script_started_at"] != "2026-06-30T09:10:01.450000+00:00" {
		t.Fatalf("execution time payload = %#v", payload)
	}
	resultPayload, ok := payload["result_payload"].(map[string]any)
	if !ok || resultPayload["source"] != "sdk_executor" || resultPayload["success"] != false {
		t.Fatalf("result payload = %#v", payload["result_payload"])
	}
}

// TestSyncSDKTerminalStateBestEffortOrder mirrors Python delivery, publish, then AI order.
func TestSyncSDKTerminalStateBestEffortOrder(t *testing.T) {
	traceID := "trace-terminal-2"
	events := []string{}
	delivery := &recordingTerminalDelivery{events: &events}
	publisher := &recordingTaskStatusPublisher{order: &events}
	ai := &recordingAITerminalSyncer{order: &events}
	record := terminalRecord("task-terminal-2", tasks.StatusSuccess, nil, &traceID)

	result := SyncSDKTerminalState(context.Background(), record, TerminalStateSyncOptions{
		Delivery:      delivery,
		Status:        publisher,
		AI:            ai,
		ResultPayload: map[string]any{"source": "sdk_executor"},
	})

	if !result.DeliverySynced || !result.StatusPublished || !result.AISynced || result.DeliveryError != nil || result.StatusError != nil || result.AIError != nil {
		t.Fatalf("result = %#v", result)
	}
	if len(events) != 3 || events[0] != "delivery:success" || events[1] != "publish:task.status" || events[2] != "ai:sent" {
		t.Fatalf("events = %#v", events)
	}
	if len(delivery.updates) != 1 || delivery.updates[0].TraceID != traceID || delivery.updates[0].SendStatus != "success" {
		t.Fatalf("delivery updates = %#v", delivery.updates)
	}
	if len(publisher.events) != 1 || publisher.events[0].Payload["result_payload"] == nil {
		t.Fatalf("published events = %#v", publisher.events)
	}
	if len(ai.updates) != 1 || ai.updates[0].AttemptStatus != "sent" || ai.updates[0].TraceID != traceID {
		t.Fatalf("ai updates = %#v", ai.updates)
	}
}

// TestSyncSDKTerminalStateContinuesAfterDeliveryError keeps side effects best-effort.
func TestSyncSDKTerminalStateContinuesAfterDeliveryError(t *testing.T) {
	taskError := "sdk failed"
	delivery := &recordingTerminalDelivery{err: errors.New("delivery down")}
	publisher := &recordingTaskStatusPublisher{}
	record := terminalRecord("task-terminal-3", tasks.StatusFailed, &taskError, nil)

	result := SyncSDKTerminalState(context.Background(), record, TerminalStateSyncOptions{
		Delivery: delivery,
		Status:   publisher,
	})

	if result.DeliveryError == nil || !result.StatusPublished || result.StatusError != nil {
		t.Fatalf("result = %#v", result)
	}
	if len(publisher.events) != 1 || publisher.events[0].Payload["error"] != "sdk failed" {
		t.Fatalf("published events = %#v", publisher.events)
	}
}

func terminalRecord(taskID string, status tasks.Status, errorText *string, traceID *string) tasks.Record {
	return tasks.Record{
		TaskID:    taskID,
		Target:    tasks.Target{DeviceID: "zimo"},
		TaskType:  "send_text",
		Status:    status,
		Error:     errorText,
		TraceID:   traceID,
		CreatedAt: time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 30, 9, 11, 0, 0, time.UTC),
		Payload: map[string]any{
			"conversation_id": "conversation-1",
			"sender_id":       "sender-1",
			"receiver":        "Qiu",
		},
	}
}

type recordingTerminalDelivery struct {
	updates []tasks.OutgoingDeliveryUpdate
	events  *[]string
	err     error
}

func (delivery *recordingTerminalDelivery) UpdateOutgoingMessageDeliveryStatus(_ context.Context, update tasks.OutgoingDeliveryUpdate) error {
	delivery.updates = append(delivery.updates, update)
	if delivery.events != nil {
		*delivery.events = append(*delivery.events, "delivery:"+update.SendStatus)
	}
	return delivery.err
}

type recordingTaskStatusPublisher struct {
	events []TaskStatusEvent
	order  *[]string
	err    error
}

func (publisher *recordingTaskStatusPublisher) PublishTaskStatus(_ context.Context, event TaskStatusEvent) error {
	publisher.events = append(publisher.events, event)
	if publisher.order != nil {
		*publisher.order = append(*publisher.order, "publish:"+event.Topic)
	}
	return publisher.err
}

type recordingAITerminalSyncer struct {
	updates []AITerminalSyncUpdate
	order   *[]string
	err     error
}

func (syncer *recordingAITerminalSyncer) SyncAITerminalState(_ context.Context, update AITerminalSyncUpdate) error {
	syncer.updates = append(syncer.updates, update)
	if syncer.order != nil {
		*syncer.order = append(*syncer.order, "ai:"+update.AttemptStatus)
	}
	return syncer.err
}
