package archivesyncnotify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/outbox"
)

func TestNotifierPublishesArchiveSyncRecordPayload(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	notifier := New(client, " archive_sync:notify ")
	record := outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventType: EventArchiveSyncRequested,
			TenantID:  " tenant-fallback ",
			Payload: map[string]any{
				"enterprise_id":  " ent-1 ",
				"source":         " self_decrypt ",
				"cursor":         " 42 ",
				"trigger_reason": " device_message_received ",
			},
		},
	}

	err := notifier.NotifyArchiveSyncRequested(context.Background(), []outbox.Record{record})
	if err != nil {
		t.Fatalf("NotifyArchiveSyncRequested returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != "archive_sync:notify" {
		t.Fatalf("calls = %#v", client.calls)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != "ent-1" || payload["source"] != "self_decrypt" || payload["cursor"] != "42" || payload["reason"] != "device_message_received" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNotifierSkipsNonArchiveSyncRecords(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	err := New(client, "").NotifyArchiveSyncRequested(context.Background(), []outbox.Record{
		{EventEnvelope: outbox.EventEnvelope{EventType: "conversation.message.received"}},
	})
	if err != nil {
		t.Fatalf("NotifyArchiveSyncRequested returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestNotifierDefaultsPayload(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	err := New(client, "").NotifyArchiveSyncRequested(context.Background(), []outbox.Record{
		{EventEnvelope: outbox.EventEnvelope{EventType: EventArchiveSyncRequested, TenantID: " ent-2 "}},
	})
	if err != nil {
		t.Fatalf("NotifyArchiveSyncRequested returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != DefaultChannel {
		t.Fatalf("calls = %#v", client.calls)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != "ent-2" || payload["source"] != DefaultSource || payload["cursor"] != nil || payload["reason"] != DefaultReason {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNotifierPropagatesPublishError(t *testing.T) {
	expected := errors.New("redis down")
	client := &recordingPublisher{result: redis.NewIntResult(0, expected)}
	err := New(client, "archive_sync:notify").NotifyArchiveSyncRequested(context.Background(), []outbox.Record{
		{EventEnvelope: outbox.EventEnvelope{EventType: EventArchiveSyncRequested}},
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}

func TestNotifierPublishesDirectSignal(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	err := New(client, "archive_sync:notify").Publish(context.Background(), Signal{
		EnterpriseID: " ent-1 ",
		Source:       " official ",
		Cursor:       " 100 ",
		Reason:       " archive_callback_http ",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != "ent-1" || payload["source"] != "official" || payload["cursor"] != "100" || payload["reason"] != "archive_callback_http" {
		t.Fatalf("payload = %#v", payload)
	}
}

type recordingPublisher struct {
	calls  []publishCall
	result *redis.IntCmd
}

type publishCall struct {
	channel string
	message any
}

func (publisher *recordingPublisher) Publish(_ context.Context, channel string, message any) *redis.IntCmd {
	publisher.calls = append(publisher.calls, publishCall{channel: channel, message: message})
	if publisher.result != nil {
		return publisher.result
	}
	return redis.NewIntResult(0, nil)
}

func decodePayload(t *testing.T, raw any) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw.(string)), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	return payload
}
