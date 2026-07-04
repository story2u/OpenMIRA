package outboxnotify

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/outbox"
)

func TestNotifierPublishesWakePayload(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	notifier := New(client, " outbox:notify ")

	err := notifier.NotifyOutboxEnqueued(context.Background(), []outbox.Record{{EventEnvelope: outbox.EventEnvelope{EventID: "evt-1"}}})
	if err != nil {
		t.Fatalf("NotifyOutboxEnqueued returned error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0].channel != "outbox:notify" || client.calls[0].message != `{"wake":true}` {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestNotifierDefaultsChannelAndSkipsEmptyRecords(t *testing.T) {
	client := &recordingPublisher{result: redis.NewIntResult(1, nil)}
	notifier := New(client, "")
	if notifier.Channel != DefaultChannel {
		t.Fatalf("channel = %q", notifier.Channel)
	}
	if err := notifier.NotifyOutboxEnqueued(context.Background(), nil); err != nil {
		t.Fatalf("NotifyOutboxEnqueued returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestNotifierPropagatesPublishError(t *testing.T) {
	expected := errors.New("redis down")
	client := &recordingPublisher{result: redis.NewIntResult(0, expected)}
	err := New(client, "outbox:notify").NotifyOutboxEnqueued(context.Background(), []outbox.Record{{EventEnvelope: outbox.EventEnvelope{EventID: "evt-1"}}})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
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
