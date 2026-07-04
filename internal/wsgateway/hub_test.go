package wsgateway

import (
	"context"
	"strings"
	"sync"
	"testing"

	"im-go/internal/realtime"
)

func TestHubPublishesToMatchingChannelAndTopics(t *testing.T) {
	hub := NewHub()
	allSender := &recordingSender{}
	messageSender := &recordingSender{}
	statusSender := &recordingSender{}
	hub.Register("conversations", nil, allSender)
	hub.Register("conversations", []string{"conversation.message"}, messageSender)
	hub.Register("conversations", []string{"task.status"}, statusSender)

	sent, err := hub.Publish(context.Background(), PublishEvent{
		Channel: "conversations",
		Event:   "conversation.message",
		Topic:   "conversation.message",
		Payload: map[string]any{"conversation_id": "conv-1"},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if sent != 2 {
		t.Fatalf("sent = %d, want 2", sent)
	}
	if allSender.messageCount() != 1 || messageSender.messageCount() != 1 || statusSender.messageCount() != 0 {
		t.Fatalf("senders all=%d message=%d status=%d", allSender.messageCount(), messageSender.messageCount(), statusSender.messageCount())
	}
	if !strings.Contains(string(allSender.messageAt(0)), `"consistency":"weak"`) {
		t.Fatalf("envelope = %s", allSender.messageAt(0))
	}
}

func TestHubUnregisterAndStats(t *testing.T) {
	hub := NewHub()
	sender := &recordingSender{}
	client := hub.Register("tasks", []string{"task.status"}, sender)
	stats, err := hub.ChannelStats(context.Background())
	if err != nil {
		t.Fatalf("ChannelStats returned error: %v", err)
	}
	if len(stats) != 1 || stats[0].Channel != "tasks" || stats[0].Connections != 1 {
		t.Fatalf("stats = %#v", stats)
	}
	hub.Unregister(client)
	stats, err = hub.ChannelStats(context.Background())
	if err != nil {
		t.Fatalf("ChannelStats returned error: %v", err)
	}
	if len(stats) != 0 || !sender.isClosed() {
		t.Fatalf("stats = %#v closed=%t", stats, sender.isClosed())
	}
}

func TestHubDeliversBrokerPayloadEnvelopeAndSkipsOwnOrigin(t *testing.T) {
	hub := NewHub()
	sender := &recordingSender{}
	hub.Register("conversations", []string{"conversation.message"}, sender)

	raw := []byte(`{"origin":"python-1","channel":"ignored","event":"ignored","topic":"ignored","payload":{"ignored":true},"envelope":{"channel":"conversations","event":"conversation.message","topic":"conversation.message","payload":{"conversation_id":"conv-1"},"consistency":"strong","cursor":7,"scope_key":"conversations:conversation.message"}}`)
	sent, err := hub.DeliverBrokerPayload(context.Background(), raw, "go-1")
	if err != nil {
		t.Fatalf("DeliverBrokerPayload returned error: %v", err)
	}
	if sent != 1 || sender.messageCount() != 1 {
		t.Fatalf("sent=%d messages=%d", sent, sender.messageCount())
	}
	message := string(sender.messageAt(0))
	for _, want := range []string{`"cursor":7`, `"consistency":"strong"`, `"scope_key":"conversations:conversation.message"`} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %s: %s", want, message)
		}
	}

	sent, err = hub.DeliverBrokerPayload(context.Background(), []byte(`{"origin":"go-1","channel":"conversations","event":"conversation.message","topic":"conversation.message","payload":{"conversation_id":"conv-2"}}`), "go-1")
	if err != nil {
		t.Fatalf("DeliverBrokerPayload own origin returned error: %v", err)
	}
	if sent != 0 || sender.messageCount() != 1 {
		t.Fatalf("own origin sent=%d messages=%d", sent, sender.messageCount())
	}
}

func TestHubAppendsStrongBrokerEventAfterLocalDelivery(t *testing.T) {
	hub := NewHub()
	eventLog := &recordingEventLog{}
	hub.EventLog = eventLog
	sender := &recordingSender{}
	hub.Register("conversations", []string{"conversation.message"}, sender)

	raw := []byte(`{"origin":"python-1","envelope":{"channel":"conversations","event":"conversation.message","topic":"conversation.message","payload":{"conversation_id":"conv-1"},"consistency":"strong","cursor":7,"scope_key":"conversations:conversation.message","scope_topic":"conversation.message"}}`)
	sent, err := hub.DeliverBrokerPayload(context.Background(), raw, "go-1")
	if err != nil {
		t.Fatalf("DeliverBrokerPayload returned error: %v", err)
	}
	if sent != 1 || len(eventLog.records) != 1 {
		t.Fatalf("sent=%d records=%#v", sent, eventLog.records)
	}
	record := eventLog.records[0]
	if record.ScopeKey != "conversations:conversation.message" || record.Cursor != 7 || record.Channel != "conversations" || record.Event != "conversation.message" || record.Topic != "conversation.message" || record.Payload["conversation_id"] != "conv-1" {
		t.Fatalf("record = %#v", record)
	}

	emptyHub := NewHub()
	emptyLog := &recordingEventLog{}
	emptyHub.EventLog = emptyLog
	sent, err = emptyHub.DeliverBrokerPayload(context.Background(), raw, "go-1")
	if err != nil {
		t.Fatalf("DeliverBrokerPayload without clients returned error: %v", err)
	}
	if sent != 0 || len(emptyLog.records) != 0 {
		t.Fatalf("empty sent=%d records=%#v", sent, emptyLog.records)
	}
}

func TestHubDeliversBrokerBatchWithoutEnvelope(t *testing.T) {
	hub := NewHub()
	allSender := &recordingSender{}
	taskSender := &recordingSender{}
	hub.Register("tasks", nil, allSender)
	hub.Register("tasks", []string{"task.status"}, taskSender)

	raw := []byte(`{"origin":"python-1","batch":[{"channel":"tasks","event":"task.status","topic":"task.status","payload":{"task_id":"task-1"}},{"channel":"tasks","event":"toast","topic":"toast","payload":{"message":"ok"}}]}`)
	sent, err := hub.DeliverBrokerPayload(context.Background(), raw, "go-1")
	if err != nil {
		t.Fatalf("DeliverBrokerPayload returned error: %v", err)
	}
	if sent != 3 {
		t.Fatalf("sent = %d, want 3", sent)
	}
	if allSender.messageCount() != 2 || taskSender.messageCount() != 1 {
		t.Fatalf("all=%d task=%d", allSender.messageCount(), taskSender.messageCount())
	}
	if !strings.Contains(string(allSender.messageAt(0)), `"consistency":"weak"`) {
		t.Fatalf("default envelope = %s", allSender.messageAt(0))
	}
}

type recordingSender struct {
	mu       sync.Mutex
	messages [][]byte
	closed   bool
}

func (sender *recordingSender) WriteText(ctx context.Context, message []byte) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.messages = append(sender.messages, append([]byte(nil), message...))
	return ctx.Err()
}

func (sender *recordingSender) Close() error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.closed = true
	return nil
}

func (sender *recordingSender) messageCount() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.messages)
}

func (sender *recordingSender) messageAt(index int) []byte {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]byte(nil), sender.messages[index]...)
}

func (sender *recordingSender) isClosed() bool {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.closed
}

type recordingEventLog struct {
	records []realtime.EventRecord
}

func (eventLog *recordingEventLog) AppendEvent(_ context.Context, record realtime.EventRecord) error {
	eventLog.records = append(eventLog.records, record)
	return nil
}
