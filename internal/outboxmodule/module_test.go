package outboxmodule

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/infra/outboxstore"
	"wework-go/internal/outbox"
	"wework-go/internal/outboxarchivesync"
	"wework-go/internal/outboxdispatch"
	"wework-go/internal/outboxprojection"
	"wework-go/internal/outboxrelay"
	"wework-go/internal/projectionupdate"
)

func TestNewWiresStoreRelayAndHubDispatcher(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{
		EventEnvelope: outbox.EventEnvelope{
			EventID:   "evt-1",
			EventType: outboxdispatch.EventConversationAssignment,
			Payload:   map[string]any{"assignment": map[string]any{"conversation_id": "conv-1"}},
		},
	}}}
	hub := &recordingHub{}
	module, err := New(Options{
		Store:                  store,
		Hub:                    hub,
		RelayOptions:           outboxrelay.Options{BatchSize: 10, RetryBaseSec: 2},
		IncludeEventTypes:      []string{outboxdispatch.EventConversationAssignment},
		ProcessingLeaseSeconds: 90,
		RequireStore:           true,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 1 || len(hub.events) != 1 || hub.events[0].Event != "conversation.transferred" {
		t.Fatalf("processed=%d hub=%#v", processed, hub.events)
	}
	if store.claimOptions.Limit != 10 || store.claimOptions.ProcessingLeaseSeconds != 90 {
		t.Fatalf("claim options = %#v", store.claimOptions)
	}
	if len(store.published) != 1 || store.published[0] != "evt-1" {
		t.Fatalf("published = %#v", store.published)
	}
}

func TestNewAllowsInjectedDispatcher(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{EventEnvelope: outbox.EventEnvelope{EventID: "evt-1", EventType: "custom"}}}}
	called := false
	module, err := New(Options{
		Store: store,
		Dispatcher: func(context.Context, outbox.Record) error {
			called = true
			return nil
		},
		RelayOptions: outboxrelay.Options{BatchSize: 1, RetryBaseSec: 2},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 1 || !called {
		t.Fatalf("processed=%d called=%v", processed, called)
	}
}

func TestNewWiresProjectionAndRealtimeDispatchers(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{
		EventEnvelope: outbox.EventEnvelope{
			EventID:   "evt-1",
			EventType: outboxprojection.EventConversationMessageReceived,
			Payload:   map[string]any{"conversation_id": "conv-1", "content": "hello"},
		},
	}}}
	projection := &fakeProjectionStore{}
	hub := &recordingHub{}
	module, err := New(Options{
		Store:                  store,
		ProjectionStore:        projection,
		Hub:                    hub,
		RelayOptions:           outboxrelay.Options{BatchSize: 1, RetryBaseSec: 2},
		IncludeEventTypes:      []string{outboxprojection.EventConversationMessageReceived},
		ProcessingLeaseSeconds: 90,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 1 || len(projection.messages) != 1 || len(hub.events) != 1 {
		t.Fatalf("processed=%d projection=%#v hub=%#v", processed, projection.messages, hub.events)
	}
	if module.Projection == nil {
		t.Fatalf("projection handler not exposed")
	}
}

func TestNewWiresArchiveSyncDispatcher(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{
		EventEnvelope: outbox.EventEnvelope{
			EventID:   "evt-archive",
			EventType: outboxarchivesync.EventArchiveSyncRequested,
			TenantID:  "ent-1",
			Payload: map[string]any{
				"source":         "self_decrypt",
				"trigger_reason": "archive_primary_device_hint",
			},
		},
	}}}
	trigger := &fakeArchiveSyncTrigger{}
	module, err := New(Options{
		Store:              store,
		ArchiveSyncTrigger: trigger,
		RelayOptions:       outboxrelay.Options{BatchSize: 1, RetryBaseSec: 2},
		IncludeEventTypes:  outboxarchivesync.SupportedEventTypes(),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 1 || len(trigger.requests) != 1 || trigger.requests[0].EnterpriseID != "ent-1" {
		t.Fatalf("processed=%d requests=%#v", processed, trigger.requests)
	}
	if module.ArchiveSync == nil {
		t.Fatalf("archive sync handler not exposed")
	}
}

func TestNewIgnoresProjectionErrorsByDefault(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{
		EventEnvelope: outbox.EventEnvelope{
			EventID:   "evt-1",
			EventType: outboxprojection.EventConversationMessageReceived,
			Payload:   map[string]any{"conversation_id": "conv-1", "content": "hello"},
		},
	}}}
	projection := &fakeProjectionStore{err: errors.New("projection failed")}
	hub := &recordingHub{}
	module, err := New(Options{
		Store:           store,
		ProjectionStore: projection,
		Hub:             hub,
		RelayOptions:    outboxrelay.Options{BatchSize: 1, RetryBaseSec: 2},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 1 || len(hub.events) != 1 || len(store.published) != 1 || len(store.retries) != 0 {
		t.Fatalf("processed=%d hub=%#v published=%#v retries=%#v", processed, hub.events, store.published, store.retries)
	}
}

func TestNewCanBlockRelayOnProjectionErrors(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{{
		EventEnvelope: outbox.EventEnvelope{
			EventID:       "evt-1",
			EventType:     outboxprojection.EventConversationMessageReceived,
			Payload:       map[string]any{"conversation_id": "conv-1", "content": "hello"},
			PartitionKey:  "conv-1",
			AggregateID:   "conv-1",
			AggregateType: "conversation",
		},
	}}}
	projection := &fakeProjectionStore{err: errors.New("projection failed")}
	hub := &recordingHub{}
	module, err := New(Options{
		Store:                      store,
		ProjectionStore:            projection,
		Hub:                        hub,
		RelayOptions:               outboxrelay.Options{BatchSize: 1, RetryBaseSec: 2},
		ProjectionErrorsBlockRelay: true,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	processed, err := module.Relay.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 0 || len(hub.events) != 0 || len(store.published) != 0 || len(store.retries) != 1 {
		t.Fatalf("processed=%d hub=%#v published=%#v retries=%#v", processed, hub.events, store.published, store.retries)
	}
}

func TestNewRequiresStoreWhenRequested(t *testing.T) {
	_, err := New(Options{RequireStore: true})
	if !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRequiresProjectionStoreWhenRequested(t *testing.T) {
	_, err := New(Options{RequireProjectionStore: true})
	if !errors.Is(err, ErrProjectionStoreRequired) {
		t.Fatalf("err = %v", err)
	}
}

type fakeStore struct {
	claimed      []outbox.Record
	claimOptions outboxstore.ClaimOptions
	published    []string
	retries      []retryCall
}

type retryCall struct {
	eventID string
	errText string
	delay   float64
}

func (store *fakeStore) ClaimPending(ctx context.Context, options outboxstore.ClaimOptions) ([]outbox.Record, error) {
	store.claimOptions = options
	return store.claimed, nil
}

func (store *fakeStore) MarkPublished(ctx context.Context, eventID string) error {
	store.published = append(store.published, eventID)
	return nil
}

func (store *fakeStore) MarkPublishedMany(ctx context.Context, eventIDs []string) (int64, error) {
	store.published = append(store.published, eventIDs...)
	return int64(len(eventIDs)), nil
}

func (store *fakeStore) MarkRetry(ctx context.Context, eventID string, errText string, retryDelaySeconds float64) error {
	store.retries = append(store.retries, retryCall{eventID: eventID, errText: errText, delay: retryDelaySeconds})
	return nil
}

type recordingHub struct {
	events []outboxdispatch.RealtimeEvent
}

func (hub *recordingHub) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	hub.events = append(hub.events, outboxdispatch.RealtimeEvent{Channel: channel, Event: event, Topic: topic, Payload: payload})
	return nil
}

type fakeArchiveSyncTrigger struct {
	requests []outboxarchivesync.Request
}

func (trigger *fakeArchiveSyncTrigger) TriggerArchiveSync(ctx context.Context, request outboxarchivesync.Request) error {
	trigger.requests = append(trigger.requests, request)
	return nil
}

type fakeProjectionStore struct {
	messages    []projectionupdate.MessageEvent
	assignments []projectionupdate.Assignment
	err         error
}

func (store *fakeProjectionStore) UpsertMessageEvent(ctx context.Context, event projectionupdate.MessageEvent) error {
	if store.err != nil {
		return store.err
	}
	store.messages = append(store.messages, event)
	return nil
}

func (store *fakeProjectionStore) UpsertAssignment(ctx context.Context, assignment projectionupdate.Assignment) error {
	if store.err != nil {
		return store.err
	}
	store.assignments = append(store.assignments, assignment)
	return nil
}
