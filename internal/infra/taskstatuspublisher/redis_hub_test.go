package taskstatuspublisher

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/realtime"
)

// TestRedisHubPublishesLegacyBrokerPayload protects the Python ws broker envelope.
func TestRedisHubPublishesLegacyBrokerPayload(t *testing.T) {
	client := &recordingRedisPublisher{result: redis.NewIntResult(1, nil)}
	hub := NewRedisHub(client, RedisHubOptions{Topic: " cloud_ws_events ", Origin: " go-test "})
	payload := map[string]any{"task_id": "task-1", "receiver": "阿文"}

	err := hub.Publish(context.Background(), " tasks ", " task.status ", " task.status ", payload)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != "cloud_ws_events" {
		t.Fatalf("calls = %#v", client.calls)
	}
	raw, ok := client.calls[0].message.(string)
	if !ok {
		t.Fatalf("message type = %T", client.calls[0].message)
	}
	if !strings.Contains(raw, "阿文") {
		t.Fatalf("message escaped non-ascii text: %s", raw)
	}
	var message map[string]any
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	if message["origin"] != "go-test" || message["channel"] != "tasks" || message["event"] != "task.status" || message["topic"] != "task.status" {
		t.Fatalf("message = %#v", message)
	}
	envelope, ok := message["envelope"].(map[string]any)
	if !ok || envelope["consistency"] != "weak" || envelope["channel"] != "tasks" || envelope["event"] != "task.status" {
		t.Fatalf("envelope = %#v", message["envelope"])
	}
	envelopePayload, ok := envelope["payload"].(map[string]any)
	if !ok || envelopePayload["task_id"] != "task-1" || envelopePayload["receiver"] != "阿文" {
		t.Fatalf("envelope payload = %#v", envelope["payload"])
	}
	envelopePayload["task_id"] = "mutated"
	if payload["task_id"] != "task-1" {
		t.Fatalf("source payload was mutated: %#v", payload)
	}
}

// TestRedisHubDefaultsTopicAndOrigin keeps runtime construction lightweight.
func TestRedisHubDefaultsTopicAndOrigin(t *testing.T) {
	client := &recordingRedisPublisher{result: redis.NewIntResult(1, nil)}
	hub := NewRedisHub(client, RedisHubOptions{})
	if hub.Topic != defaultRedisTopic || !strings.HasPrefix(hub.Origin, "go-") {
		t.Fatalf("hub = %#v", hub)
	}
	if err := hub.Publish(context.Background(), "tasks", "task.status", "task.status", nil); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if client.calls[0].channel != defaultRedisTopic {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestRedisHubPublishesStrongEnvelopeAndAppendsEventLog(t *testing.T) {
	client := &recordingRedisPublisher{result: redis.NewIntResult(1, nil)}
	allocator := &recordingCursorAllocator{cursors: []int64{42}}
	eventLog := &recordingEventLog{}
	hub := NewRedisHub(client, RedisHubOptions{
		Topic:           "cloud_ws_events",
		Origin:          "go-test",
		CursorAllocator: allocator,
		EventLog:        eventLog,
	})

	err := hub.Publish(context.Background(), "conversations", "conversation.message", "conversation.message", map[string]any{"message_id": "m-42"})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if allocator.scopeKey != "conversations:conversation.message" || allocator.count != 1 {
		t.Fatalf("allocator = %#v", allocator)
	}
	raw := client.calls[0].message.(string)
	var message map[string]any
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	envelope := message["envelope"].(map[string]any)
	if envelope["consistency"] != "strong" || envelope["cursor"].(float64) != 42 || envelope["scope_key"] != "conversations:conversation.message" || envelope["scope_topic"] != "conversation.message" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if len(eventLog.records) != 1 {
		t.Fatalf("event log records = %#v", eventLog.records)
	}
	record := eventLog.records[0]
	if record.ScopeKey != "conversations:conversation.message" || record.Cursor != 42 || record.Channel != "conversations" || record.Event != "conversation.message" || record.Topic != "conversation.message" || record.Payload["message_id"] != "m-42" {
		t.Fatalf("record = %#v", record)
	}
}

func TestRedisHubKeepsStrongEventWeakWithoutEventLog(t *testing.T) {
	client := &recordingRedisPublisher{result: redis.NewIntResult(1, nil)}
	allocator := &recordingCursorAllocator{cursors: []int64{42}}
	hub := NewRedisHub(client, RedisHubOptions{CursorAllocator: allocator})

	if err := hub.Publish(context.Background(), "conversations", "conversation.message", "conversation.message", nil); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	raw := client.calls[0].message.(string)
	var message map[string]any
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	envelope := message["envelope"].(map[string]any)
	if envelope["consistency"] != "weak" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if allocator.count != 0 {
		t.Fatalf("allocator called without event log: %#v", allocator)
	}
}

// TestRedisHubRequiresClientAndEventIdentity fails closed before losing events.
func TestRedisHubRequiresClientAndEventIdentity(t *testing.T) {
	if err := NewRedisHub(nil, RedisHubOptions{}).Publish(context.Background(), "tasks", "task.status", "", nil); err == nil {
		t.Fatal("missing client returned nil error")
	}
	client := &recordingRedisPublisher{result: redis.NewIntResult(1, nil)}
	err := NewRedisHub(client, RedisHubOptions{}).Publish(context.Background(), "", "task.status", "", nil)
	if err == nil || !strings.Contains(err.Error(), "channel and event") {
		t.Fatalf("error = %v", err)
	}
}

type recordingRedisPublisher struct {
	calls  []redisPublishCall
	result *redis.IntCmd
}

type redisPublishCall struct {
	channel string
	message any
}

func (publisher *recordingRedisPublisher) Publish(_ context.Context, channel string, message any) *redis.IntCmd {
	publisher.calls = append(publisher.calls, redisPublishCall{channel: channel, message: message})
	if publisher.result != nil {
		return publisher.result
	}
	return redis.NewIntResult(0, nil)
}

type recordingCursorAllocator struct {
	cursors  []int64
	scopeKey string
	count    int
}

func (allocator *recordingCursorAllocator) Allocate(_ context.Context, scopeKey string, count int) ([]int64, error) {
	allocator.scopeKey = scopeKey
	allocator.count = count
	return allocator.cursors, nil
}

type recordingEventLog struct {
	records []realtime.EventRecord
}

func (eventLog *recordingEventLog) AppendEvent(_ context.Context, record realtime.EventRecord) error {
	eventLog.records = append(eventLog.records, record)
	return nil
}
