package archiveeventnotify

import (
	"context"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestNotifyEnqueuesArchiveSyncRequestedEvent(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeOutboxStore{}
	service := Service{
		Outbox:       store,
		Now:          func() time.Time { return now },
		NewTriggerID: func(time.Time) string { return "archive-trigger-test" },
	}

	result, err := service.Notify(context.Background(), Request{
		EnterpriseID: " ent-1 ",
		Source:       " official ",
		Cursor:       " 42 ",
		Limit:        500,
		Event:        " message.new ",
		Vendor:       " bridge-a ",
		Payload:      map[string]any{"msgid": "m-1"},
	})
	if err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if !result.Accepted || result.Running || result.TriggerID != "archive-trigger-test" || result.EnterpriseID != "ent-1" || result.Event != "message.new" || result.Vendor != "bridge-a" {
		t.Fatalf("result = %#v", result)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %#v", store.events)
	}
	event := store.events[0]
	if event.EventID != "archive-event-notify:archive-trigger-test" || event.EventType != EventArchiveSyncRequested || event.TraceID != "archive-trigger-test" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.AggregateType != "archive_sync" || event.AggregateID != "ent-1:official" || event.TenantID != "ent-1" || event.PartitionKey != "ent-1:official" {
		t.Fatalf("event aggregate = %#v", event)
	}
	if event.Payload["enterprise_id"] != "ent-1" || event.Payload["source"] != "official" || event.Payload["cursor"] != "42" || event.Payload["limit"] != 500 || event.Payload["trigger_reason"] != DefaultTriggerReason {
		t.Fatalf("payload = %#v", event.Payload)
	}
	if payload, ok := event.Payload["payload"].(map[string]any); !ok || payload["msgid"] != "m-1" {
		t.Fatalf("nested payload = %#v", event.Payload["payload"])
	}
	if !event.OccurredAt.Equal(now) || !event.AvailableAt.Equal(now) {
		t.Fatalf("event time = %#v", event)
	}
}

func TestBuildOutboxEventAppliesPythonDefaults(t *testing.T) {
	event := BuildOutboxEvent(OutboxInput{TriggerID: "archive-trigger-defaults"})
	if event.TenantID != DefaultEnterpriseID || event.Payload["source"] != DefaultSource || event.Payload["limit"] != DefaultLimit || event.Payload["event"] != DefaultEvent {
		t.Fatalf("event = %#v payload=%#v", event, event.Payload)
	}
	if event.Payload["cursor"] != nil {
		t.Fatalf("cursor = %#v, want nil", event.Payload["cursor"])
	}
}

func TestNotifyRequiresOutboxStore(t *testing.T) {
	_, err := (Service{}).Notify(context.Background(), Request{})
	if err != ErrOutboxStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrOutboxStoreUnavailable)
	}
}

type fakeOutboxStore struct {
	events []outbox.EventEnvelope
}

func (store *fakeOutboxStore) Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error) {
	store.events = append(store.events, event)
	return outbox.RecordFromEnvelope(event, time.Now().UTC()), nil
}
