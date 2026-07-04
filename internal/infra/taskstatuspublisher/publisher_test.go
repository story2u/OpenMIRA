package taskstatuspublisher

import (
	"context"
	"strings"
	"testing"

	"wework-go/internal/senddispatcher"
)

// TestPublisherPublishesTaskStatusEnvelope protects the ws_hub.publish adapter shape.
func TestPublisherPublishesTaskStatusEnvelope(t *testing.T) {
	hub := &recordingHub{}
	publisher := New(hub)
	payload := map[string]any{"task_id": "task-1", "status": "success"}

	err := publisher.PublishTaskStatus(context.Background(), senddispatcher.TaskStatusEvent{
		Channel: " tasks ",
		Event:   " task.status ",
		Topic:   " task.status ",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("PublishTaskStatus returned error: %v", err)
	}
	if len(hub.calls) != 1 {
		t.Fatalf("calls = %#v", hub.calls)
	}
	call := hub.calls[0]
	if call.channel != "tasks" || call.event != "task.status" || call.topic != "task.status" {
		t.Fatalf("call = %#v", call)
	}
	if call.payload["task_id"] != "task-1" || call.payload["status"] != "success" {
		t.Fatalf("payload = %#v", call.payload)
	}
	call.payload["status"] = "mutated"
	if payload["status"] != "success" {
		t.Fatalf("source payload was mutated: %#v", payload)
	}
}

// TestPublisherAppliesLegacyDefaults keeps direct adapter calls safe.
func TestPublisherAppliesLegacyDefaults(t *testing.T) {
	hub := &recordingHub{}
	if err := New(hub).PublishTaskStatus(context.Background(), senddispatcher.TaskStatusEvent{}); err != nil {
		t.Fatalf("PublishTaskStatus returned error: %v", err)
	}
	call := hub.calls[0]
	if call.channel != "tasks" || call.event != "task.status" || call.topic != "task.status" {
		t.Fatalf("call = %#v", call)
	}
	if call.payload == nil || len(call.payload) != 0 {
		t.Fatalf("payload = %#v", call.payload)
	}
}

// TestPublisherRequiresHub fails closed when no realtime hub is configured.
func TestPublisherRequiresHub(t *testing.T) {
	err := New(nil).PublishTaskStatus(context.Background(), senddispatcher.TaskStatusEvent{})
	if err == nil || !strings.Contains(err.Error(), "hub is not configured") {
		t.Fatalf("error = %v", err)
	}
}

type recordingHub struct {
	calls []publishCall
}

type publishCall struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (hub *recordingHub) Publish(_ context.Context, channel string, event string, topic string, payload map[string]any) error {
	hub.calls = append(hub.calls, publishCall{
		channel: channel,
		event:   event,
		topic:   topic,
		payload: payload,
	})
	return nil
}
