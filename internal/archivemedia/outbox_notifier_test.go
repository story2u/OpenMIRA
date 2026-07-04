package archivemedia

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestOutboxNotifierEnqueuesMediaReadyEvent(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	messageTime := now.Add(-time.Hour)
	sink := &fakeMediaOutbox{}

	err := (OutboxNotifier{
		Outbox: sink,
		Now:    func() time.Time { return now },
	}).NotifyArchiveMediaReady(context.Background(), MediaReadyEvent{
		EnterpriseID:   " ent-1 ",
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		ArchiveMsgID:   "am-1",
		DeviceID:       "dev-1",
		SenderID:       "sender-1",
		SenderName:     "Alice",
		MsgType:        "image",
		Direction:      "incoming",
		MediaTaskID:    "amt-1",
		ObjectURL:      "https://objects/ent-1/am-1.bin",
		Timestamp:      messageTime,
	})
	if err != nil {
		t.Fatalf("NotifyArchiveMediaReady returned error: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("events = %#v", sink.events)
	}
	event := sink.events[0]
	if event.EventID != "ent-1:trace-1:media-ready" || event.EventType != EventConversationMediaReady {
		t.Fatalf("event identity = %#v", event)
	}
	if event.AggregateType != "conversation" || event.AggregateID != "conv-1" || event.PartitionKey != "ent-1:conv-1" {
		t.Fatalf("event routing = %#v", event)
	}
	if event.TenantID != "ent-1" || event.TraceID != "trace-1" || !event.OccurredAt.Equal(now) || !event.AvailableAt.Equal(now) {
		t.Fatalf("event metadata = %#v", event)
	}
	if event.Payload["tenant_id"] != "ent-1" || event.Payload["conversation_id"] != "conv-1" || event.Payload["media_ready"] != true || event.Payload["media_status"] != "success" {
		t.Fatalf("payload defaults = %#v", event.Payload)
	}
	if event.Payload["timestamp"] != messageTime.Format(time.RFC3339Nano) || event.Payload["created_at"] != messageTime.Format(time.RFC3339Nano) {
		t.Fatalf("payload timestamps = %#v", event.Payload)
	}
	if event.Payload["object_url"] != "https://objects/ent-1/am-1.bin" || event.Payload["publish_event"] != EventConversationMediaReady {
		t.Fatalf("payload media fields = %#v", event.Payload)
	}
}

func TestOutboxNotifierRequiresOutbox(t *testing.T) {
	err := (OutboxNotifier{}).NotifyArchiveMediaReady(context.Background(), MediaReadyEvent{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOutboxNotifierPropagatesEnqueueError(t *testing.T) {
	expected := errors.New("outbox down")
	err := (OutboxNotifier{Outbox: &fakeMediaOutbox{err: expected}}).NotifyArchiveMediaReady(context.Background(), MediaReadyEvent{
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-1",
		ObjectURL:    "https://objects/ent-1/am-1.bin",
	})
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}

type fakeMediaOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (sink *fakeMediaOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append(sink.events, events...)
	if sink.err != nil {
		return nil, sink.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.RecordFromEnvelope(event, event.OccurredAt))
	}
	return records, nil
}
