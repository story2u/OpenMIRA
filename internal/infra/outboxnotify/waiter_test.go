package outboxnotify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestWaiterSubscribesAndReturnsOnMessage(t *testing.T) {
	pubsub := &fakePubSub{message: &redis.Message{Channel: "outbox:notify", Payload: `{"wake":true}`}}
	client := &fakeSubscriber{pubsub: pubsub}
	waiter := NewWaiter(client, " outbox:notify ")

	err := waiter.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if len(client.channels) != 1 || client.channels[0] != "outbox:notify" || pubsub.receiveCalls != 1 {
		t.Fatalf("client=%#v pubsub=%#v", client, pubsub)
	}
}

func TestMultiWaiterSubscribesDistinctChannels(t *testing.T) {
	pubsub := &fakePubSub{message: &redis.Message{Channel: "archive_sync:notify", Payload: `{"wake":true}`}}
	client := &fakeSubscriber{pubsub: pubsub}
	waiter := NewMultiWaiter(client, []string{" outbox:notify ", "archive_sync:notify", "outbox:notify", ""})

	err := waiter.Wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if len(client.channels) != 2 || client.channels[0] != "outbox:notify" || client.channels[1] != "archive_sync:notify" {
		t.Fatalf("channels = %#v", client.channels)
	}
}

func TestWaiterTimeoutReturnsNil(t *testing.T) {
	pubsub := &fakePubSub{err: context.DeadlineExceeded}
	waiter := NewWaiter(&fakeSubscriber{pubsub: pubsub}, "")

	err := waiter.Wait(context.Background(), time.Millisecond)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if pubsub.closed {
		t.Fatalf("timeout should not close reusable pubsub")
	}
}

func TestWaiterRedisErrorResetsSubscription(t *testing.T) {
	pubsub := &fakePubSub{err: errors.New("redis down")}
	waiter := NewWaiter(&fakeSubscriber{pubsub: pubsub}, "outbox:notify")

	err := waiter.Wait(context.Background(), 0)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !pubsub.closed {
		t.Fatalf("redis error should close pubsub")
	}
}

func TestWaiterCloseClosesSubscription(t *testing.T) {
	pubsub := &fakePubSub{message: &redis.Message{Payload: "wake"}}
	waiter := NewWaiter(&fakeSubscriber{pubsub: pubsub}, "outbox:notify")
	if err := waiter.Wait(context.Background(), time.Second); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if err := waiter.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !pubsub.closed {
		t.Fatalf("pubsub was not closed")
	}
}

type fakeSubscriber struct {
	pubsub   PubSub
	channels []string
}

func (client *fakeSubscriber) Subscribe(_ context.Context, channels ...string) PubSub {
	client.channels = append([]string(nil), channels...)
	return client.pubsub
}

type fakePubSub struct {
	message      *redis.Message
	err          error
	receiveCalls int
	closed       bool
}

func (pubsub *fakePubSub) ReceiveMessage(context.Context) (*redis.Message, error) {
	pubsub.receiveCalls++
	if pubsub.err != nil {
		return nil, pubsub.err
	}
	return pubsub.message, nil
}

func (pubsub *fakePubSub) Close() error {
	pubsub.closed = true
	return nil
}
