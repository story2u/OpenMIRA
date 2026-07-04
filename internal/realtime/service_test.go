package realtime

import (
	"context"
	"testing"
	"time"
)

func TestReplayEventsBuildsPythonShape(t *testing.T) {
	store := &fakeEventStore{events: []EventRecord{
		{ScopeKey: " conversations:conversation.message ", Cursor: 2, Channel: " conversations ", Event: " conversation.message ", Topic: " conversation.message ", Payload: map[string]any{"message_id": "m-2"}, CreatedAt: "2026-07-01T00:00:00Z"},
		{ScopeKey: "conversations:conversation.message", Cursor: 3, Channel: "conversations", Event: "conversation.message", Topic: "conversation.message", Payload: map[string]any{"message_id": "m-3"}},
		{ScopeKey: "conversations:conversation.message", Cursor: 4, Channel: "conversations", Event: "conversation.message", Topic: "conversation.message", Payload: map[string]any{"message_id": "m-4"}},
	}}
	service := Service{Events: store}

	payload, err := service.ReplayEvents(context.Background(), ReplayRequest{Scope: " conversations:conversation.message ", AfterCursor: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ReplayEvents returned error: %v", err)
	}
	if store.scope != "conversations:conversation.message" || store.after != 1 || store.limit != 3 {
		t.Fatalf("store request = %q/%d/%d", store.scope, store.after, store.limit)
	}
	if payload["has_more"] != true || payload["latest_cursor"] != int64(3) {
		t.Fatalf("payload = %#v", payload)
	}
	events := payload["events"].([]Payload)
	if len(events) != 2 || events[0]["cursor"] != int64(2) || events[0]["consistency"] != "strong" {
		t.Fatalf("events = %#v", events)
	}
}

func TestReplayEventsReturnsEmptyWithoutScopeOrStoreError(t *testing.T) {
	service := Service{}
	payload, err := service.ReplayEvents(context.Background(), ReplayRequest{AfterCursor: 12})
	if err != nil {
		t.Fatalf("ReplayEvents returned error: %v", err)
	}
	if payload["latest_cursor"] != int64(0) || payload["has_more"] != false {
		t.Fatalf("empty scope payload = %#v", payload)
	}

	service = Service{Events: &fakeEventStore{err: errStore}}
	payload, err = service.ReplayEvents(context.Background(), ReplayRequest{Scope: "scope-a", AfterCursor: 12, Limit: 100})
	if err != nil {
		t.Fatalf("ReplayEvents returned error: %v", err)
	}
	if payload["latest_cursor"] != int64(12) || len(payload["events"].([]Payload)) != 0 {
		t.Fatalf("store error payload = %#v", payload)
	}
}

func TestEventPayloadPreservesWorkbenchReplayEventShapes(t *testing.T) {
	tests := []struct {
		name          string
		record        EventRecord
		wantScope     string
		wantEvent     string
		wantTopic     string
		wantPayloadID string
	}{
		{
			name: "message created alias",
			record: EventRecord{
				ScopeKey:    "conversations:message_created",
				Cursor:      11,
				Channel:     "conversations",
				Event:       "message_created",
				Topic:       "message_created",
				Consistency: "strong",
				Payload:     map[string]any{"conversation_id": "conv-1", "message_created": true},
			},
			wantScope:     "conversations:message_created",
			wantEvent:     "message_created",
			wantTopic:     "message_created",
			wantPayloadID: "conv-1",
		},
		{
			name: "conversation assigned alias",
			record: EventRecord{
				ScopeKey: "conversations:conversation_assigned",
				Cursor:   12,
				Channel:  "conversations",
				Event:    "conversation_assigned",
				Topic:    "conversation_assigned",
				Payload:  map[string]any{"conversation_id": "conv-2", "assignee_id": "cs-1"},
			},
			wantScope:     "conversations:conversation_assigned",
			wantEvent:     "conversation_assigned",
			wantTopic:     "conversation_assigned",
			wantPayloadID: "conv-2",
		},
		{
			name: "conversation assignment topic",
			record: EventRecord{
				ScopeKey: "conversations:conversation.assignment",
				Cursor:   13,
				Channel:  "conversations",
				Event:    "conversation.transferred",
				Topic:    "conversation.assignment",
				Payload:  map[string]any{"conversation_id": "conv-3", "to_assignee_id": "cs-2"},
			},
			wantScope:     "conversations:conversation.assignment",
			wantEvent:     "conversation.transferred",
			wantTopic:     "conversation.assignment",
			wantPayloadID: "conv-3",
		},
		{
			name: "conversation media ready",
			record: EventRecord{
				ScopeKey: "conversations:conversation.media_ready",
				Cursor:   14,
				Channel:  "conversations",
				Event:    "conversation.media_ready",
				Topic:    "conversation.media_ready",
				Payload:  map[string]any{"conversation_id": "conv-4", "media_ready": true},
			},
			wantScope:     "conversations:conversation.media_ready",
			wantEvent:     "conversation.media_ready",
			wantTopic:     "conversation.media_ready",
			wantPayloadID: "conv-4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := eventPayload(test.record)
			if payload["scope_key"] != test.wantScope || payload["event"] != test.wantEvent || payload["topic"] != test.wantTopic || payload["consistency"] != "strong" {
				t.Fatalf("payload = %#v", payload)
			}
			body := payload["payload"].(map[string]any)
			if body["conversation_id"] != test.wantPayloadID {
				t.Fatalf("event body = %#v", body)
			}
		})
	}
}

func TestSnapshotWorkbenchReadsFixedScopes(t *testing.T) {
	now := time.Date(2026, 7, 1, 8, 0, 0, 123, time.UTC)
	store := &fakeEventStore{latest: map[string]int64{
		"conversations:conversation.message":    10,
		"conversations:conversation.assignment": 0,
		"chat:identity.updated":                 7,
	}}
	service := Service{Events: store, Now: func() time.Time { return now }}

	payload, err := service.SnapshotWorkbench(context.Background())
	if err != nil {
		t.Fatalf("SnapshotWorkbench returned error: %v", err)
	}
	cursors := payload["cursors"].(map[string]int64)
	if cursors["conversations:conversation.message"] != 10 || cursors["chat:identity.updated"] != 7 {
		t.Fatalf("cursors = %#v", cursors)
	}
	if _, ok := cursors["conversations:conversation.assignment"]; ok {
		t.Fatalf("zero cursor should be omitted: %#v", cursors)
	}
	if payload["resync_required"] != false || payload["timestamp"] != "2026-07-01T08:00:00.000000123Z" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.latestScopes) != 3 {
		t.Fatalf("latest scopes = %#v", store.latestScopes)
	}
}

var errStore = &storeError{}

type storeError struct{}

func (err *storeError) Error() string {
	return "store failed"
}

type fakeEventStore struct {
	events       []EventRecord
	err          error
	latest       map[string]int64
	scope        string
	after        int64
	limit        int
	latestScopes []string
}

func (store *fakeEventStore) ListAfterCursor(ctx context.Context, scopeKey string, afterCursor int64, limit int) ([]EventRecord, error) {
	store.scope = scopeKey
	store.after = afterCursor
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	if limit < len(store.events) {
		return store.events[:limit], nil
	}
	return store.events, nil
}

func (store *fakeEventStore) LatestCursor(ctx context.Context, scopeKey string) (int64, error) {
	store.latestScopes = append(store.latestScopes, scopeKey)
	if store.err != nil {
		return 0, store.err
	}
	return store.latest[scopeKey], nil
}
