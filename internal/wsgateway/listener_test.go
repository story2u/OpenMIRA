package wsgateway

import (
	"context"
	"testing"
	"time"
)

func TestListenerConsumesBrokerFeed(t *testing.T) {
	hub := NewHub()
	sender := &recordingSender{}
	hub.Register("tasks", []string{"task.status"}, sender)
	feed := newFakeBrokerFeed()
	cleanup := (Listener{Hub: hub, Feed: feed}).Start(context.Background())

	feed.messages <- []byte(`{"origin":"python-1","channel":"tasks","event":"task.status","topic":"task.status","payload":{"task_id":"task-1"}}`)
	waitFor(t, func() bool { return len(sender.messages) == 1 })

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	if !feed.closed {
		t.Fatal("feed was not closed")
	}
}

func TestListenerCleanupStopsWithoutFeedClose(t *testing.T) {
	hub := NewHub()
	feed := newFakeBrokerFeed()
	cleanup := (Listener{Hub: hub, Feed: feed}).Start(context.Background())
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
}

type fakeBrokerFeed struct {
	messages chan []byte
	closed   bool
}

func newFakeBrokerFeed() *fakeBrokerFeed {
	return &fakeBrokerFeed{messages: make(chan []byte, 1)}
}

func (feed *fakeBrokerFeed) Messages() <-chan []byte {
	return feed.messages
}

func (feed *fakeBrokerFeed) Close() error {
	if !feed.closed {
		close(feed.messages)
		feed.closed = true
	}
	return nil
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}
