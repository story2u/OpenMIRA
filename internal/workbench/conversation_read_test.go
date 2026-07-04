package workbench

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceMarkConversationReadSkipsPendingConversation(t *testing.T) {
	service := Service{ConversationReadStore: &fakeConversationReadStore{}}

	payload, err := service.MarkConversationRead(context.Background(), NewConversationReadRequest(" pending:conv-1 ", auth.Session{Role: "cs"}))
	if err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}
	if payload["success"] != true || payload["pending"] != true || payload["conversation"] != nil {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestServiceMarkConversationReadReturnsAlreadyReadWithoutSideEffects(t *testing.T) {
	store := &fakeConversationReadStore{records: map[string]ConversationReadRecord{
		"conv-1": {ConversationID: "conv-1", ConversationKey: "key-1", TenantID: "tenant-a", AccountID: "acc-1", UnreadCount: 0},
	}}
	events := &fakeScriptEventPublisher{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{ConversationReadStore: store, ConversationReadEvents: events, ReadModelInvalidator: invalidator}

	payload, err := service.MarkConversationRead(context.Background(), NewConversationReadRequest(" conv-1 ", auth.Session{Role: "cs"}))
	if err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}
	conversation := payload["conversation"].(Payload)
	if payload["already_read"] != true || conversation["conversation_id"] != "conv-1" || conversation["unread_count"] != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if store.markedID != "" {
		t.Fatalf("markedID = %q, want no write", store.markedID)
	}
	if len(events.events) != 0 {
		t.Fatalf("events = %+v", events.events)
	}
	if len(invalidator.namespaces) != 0 {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceMarkConversationReadClearsUnreadAndPublishes(t *testing.T) {
	store := &fakeConversationReadStore{records: map[string]ConversationReadRecord{
		"conv-1": {ConversationID: "conv-1", TenantID: "tenant-a", AccountID: "acc-1", DeviceID: "device-1", WeWorkUserID: "wx-1", ExternalUserID: "ext-1", AssigneeID: "cs-1", ConversationName: "客户一", UnreadCount: 3, LastMessageAt: "2026-07-01T09:00:00Z"},
	}}
	events := &fakeScriptEventPublisher{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{ConversationReadStore: store, ConversationReadEvents: events, ReadModelInvalidator: invalidator}

	payload, err := service.MarkConversationRead(context.Background(), NewConversationReadRequest("conv-1", auth.Session{Role: "cs"}))
	if err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}
	if store.markedID != "conv-1" {
		t.Fatalf("markedID = %q", store.markedID)
	}
	conversation := payload["conversation"].(Payload)
	if payload["already_read"] != false || conversation["unread_count"] != 0 || conversation["conversation_name"] != "客户一" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation_unread_changed" || events.events[0].topic != "conversation.message" {
		t.Fatalf("events = %+v", events.events)
	}
	if events.events[0].payload["conversation_id"] != "conv-1" || events.events[0].payload["unread_count"] != 0 {
		t.Fatalf("event payload = %#v", events.events[0].payload)
	}
	wantNamespaces := allReadModelNamespacesForTest()
	if !reflect.DeepEqual(invalidator.namespaces, wantNamespaces) {
		t.Fatalf("invalidated namespaces = %+v, want %+v", invalidator.namespaces, wantNamespaces)
	}
}

func TestServiceMarkConversationReadMapsMissingConversation(t *testing.T) {
	service := Service{ConversationReadStore: &fakeConversationReadStore{records: map[string]ConversationReadRecord{}}}

	_, err := service.MarkConversationRead(context.Background(), NewConversationReadRequest("missing", auth.Session{Role: "cs"}))
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("err = %v, want conversation not found", err)
	}
}

type fakeConversationReadStore struct {
	records  map[string]ConversationReadRecord
	markedID string
}

func (store *fakeConversationReadStore) GetConversationRead(ctx context.Context, conversationID string) (ConversationReadRecord, bool, error) {
	record, ok := store.records[conversationID]
	return record, ok, nil
}

func (store *fakeConversationReadStore) MarkConversationRead(ctx context.Context, conversationID string) (ConversationReadRecord, bool, error) {
	store.markedID = conversationID
	record, ok := store.records[conversationID]
	if !ok {
		return ConversationReadRecord{}, false, nil
	}
	record.UnreadCount = 0
	store.records[conversationID] = record
	return record, true, nil
}

type fakeReadModelInvalidator struct {
	namespaces []string
}

func (invalidator *fakeReadModelInvalidator) InvalidateNamespaces(ctx context.Context, namespaces ...string) error {
	invalidator.namespaces = append([]string{}, namespaces...)
	return nil
}

func allReadModelNamespacesForTest() []string {
	return []string{
		ReadModelConversationListNamespace,
		ReadModelPanelSnapshotNamespace,
		ReadModelAccountStatsNamespace,
		ReadModelCSWorkbenchSearchNamespace,
	}
}
