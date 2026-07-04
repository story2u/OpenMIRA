package archiveingestnotify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/infra/archiveingesttask"
)

func TestNotifierPublishesArchiveIngestEnqueuedPayload(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	notifier := New(client, " archive_ingest:notify ")

	err := notifier.NotifyArchiveIngestEnqueued(context.Background(), archiveingesttask.Record{
		TaskID:       " ait-1 ",
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Cursor:       " cursor-1 ",
	})
	if err != nil {
		t.Fatalf("NotifyArchiveIngestEnqueued returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != "archive_ingest:notify" {
		t.Fatalf("calls = %#v", client.calls)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != "ent-1" || payload["source"] != "self_decrypt" || payload["task_id"] != "ait-1" || payload["cursor"] != "cursor-1" || payload["reason"] != DefaultReason {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNotifierDefaultsPayloadAndSkipsEmptyTask(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	err := New(client, "").NotifyArchiveIngestEnqueued(context.Background(), archiveingesttask.Record{TaskID: "ait-1"})
	if err != nil {
		t.Fatalf("NotifyArchiveIngestEnqueued returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != DefaultChannel {
		t.Fatalf("calls = %#v", client.calls)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != DefaultEnterpriseID || payload["source"] != DefaultSource || payload["task_id"] != "ait-1" || payload["reason"] != DefaultReason {
		t.Fatalf("payload = %#v", payload)
	}

	err = New(client, "").NotifyArchiveIngestEnqueued(context.Background(), archiveingesttask.Record{})
	if err != nil {
		t.Fatalf("empty NotifyArchiveIngestEnqueued returned error: %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("empty task should not publish: %#v", client.calls)
	}
}

func TestNotifierPropagatesPublishError(t *testing.T) {
	expected := errors.New("redis down")
	client := &recordingPublisher{result: redis.NewIntResult(0, expected)}
	err := New(client, "archive_ingest:notify").NotifyArchiveIngestEnqueued(context.Background(), archiveingesttask.Record{
		TaskID: "ait-1",
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}

func TestNotifierPublishesDirectSignal(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	err := New(client, "archive_ingest:notify").Publish(context.Background(), Signal{
		EnterpriseID: " ent-1 ",
		Source:       " official ",
		TaskID:       " ait-1 ",
		Cursor:       " cursor-1 ",
		Reason:       " manual ",
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	payload := decodePayload(t, client.calls[0].message)
	if payload["enterprise_id"] != "ent-1" || payload["source"] != "official" || payload["task_id"] != "ait-1" || payload["cursor"] != "cursor-1" || payload["reason"] != "manual" {
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
