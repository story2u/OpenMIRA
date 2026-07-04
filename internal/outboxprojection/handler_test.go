package outboxprojection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"wework-go/internal/outbox"
	"wework-go/internal/projectionupdate"
	"wework-go/internal/readmodelcache"
)

func TestBuildMessageEventFromReceivedPayload(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageReceived,
		TenantID:  "tenant-fallback",
		Payload: map[string]any{
			"conversation_id":       " conv-1 ",
			"tenant_id":             "tenant-1",
			"device_id":             "device-1",
			"wework_user_id":        "wx-1",
			"sender_id":             "external-1",
			"sender_name":           "Alice",
			"sender_remark":         "VIP Alice",
			"sender_avatar":         "avatar-a",
			"sender_avatar_display": "avatar-display",
			"content":               "hello",
			"msg_type":              "",
			"direction":             "",
			"is_system_event":       "1",
			"timestamp":             "2026-06-30T10:00:00Z",
			"unread_count":          "7",
		},
	}}

	event, ok := BuildMessageEvent(record)
	if !ok {
		t.Fatal("BuildMessageEvent ok = false")
	}
	if event.ConversationID != "conv-1" || event.TenantID != "tenant-1" || event.DeviceID != "device-1" {
		t.Fatalf("event identity = %+v", event)
	}
	if event.ExternalUserID != "external-1" || event.SenderName != "Alice" || event.CustomerName != "Alice" {
		t.Fatalf("event sender fields = %+v", event)
	}
	if event.SenderAvatar != "avatar-display" || event.MsgType != "text" || event.Direction != "incoming" || !event.IsSystemEvent {
		t.Fatalf("event defaults = %+v", event)
	}
	if !event.Timestamp.Equal(time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp = %s", event.Timestamp)
	}
	if event.UnreadCount == nil || *event.UnreadCount != 7 {
		t.Fatalf("unread_count = %#v", event.UnreadCount)
	}
}

func TestBuildMessageEventSkipsMissingConversationID(t *testing.T) {
	_, ok := BuildMessageEvent(outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageReceived,
		Payload:   map[string]any{"content": "hello"},
	}})
	if ok {
		t.Fatal("missing conversation_id should skip projection event")
	}
}

func TestArchiveMessageDispatchSkipsDuplicatePayload(t *testing.T) {
	store := &recordingStore{}
	invalidator := &recordingInvalidator{}
	handler := Handler{Store: store, ReadModelInvalidator: invalidator}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventArchiveMessageIngested,
		Payload:   map[string]any{"message_created": false, "conversation_id": "conv-1"},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(store.messages) != 0 {
		t.Fatalf("messages = %#v", store.messages)
	}
	if len(invalidator.calls) != 0 {
		t.Fatalf("invalidator calls = %#v", invalidator.calls)
	}
}

func TestHandlerSkipsUnsupportedEventsWithoutStore(t *testing.T) {
	handler := Handler{}
	if err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{EventType: "archive.sync.requested"}}); err != nil {
		t.Fatalf("unsupported dispatch returned error: %v", err)
	}
	if err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventArchiveMessageIngested,
		Payload:   map[string]any{"message_created": false, "conversation_id": "conv-1"},
	}}); err != nil {
		t.Fatalf("duplicate archive dispatch returned error: %v", err)
	}
}

func TestBuildOutboundMessageEventDefaultsDirection(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationOutbound,
		TenantID:  "tenant-1",
		Payload: map[string]any{
			"message": map[string]any{
				"conversation_id": "conv-1",
				"device_id":       "device-1",
				"sender_id":       "agent-1",
				"content":         "reply",
				"timestamp":       "2026-06-30 18:00:00",
			},
		},
	}}
	event, ok := BuildOutboundMessageEvent(record)
	if !ok {
		t.Fatal("BuildOutboundMessageEvent ok = false")
	}
	if event.TenantID != "tenant-1" || event.Direction != "outgoing" || event.Content != "reply" {
		t.Fatalf("event = %+v", event)
	}
	if !event.Timestamp.Equal(time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("beijing timestamp not parsed as UTC instant: %s", event.Timestamp)
	}
}

func TestOutboundManualReplyClearsSensitiveHandoffProjection(t *testing.T) {
	store := &recordingStore{}
	handler := Handler{Store: store}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationOutbound,
		Payload: map[string]any{
			"message": map[string]any{
				"conversation_id": "conv-1",
				"content":         "reply",
				"message_origin":  "manual_reply",
			},
		},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(store.messages) != 1 || len(store.sensitiveClears) != 1 || store.sensitiveClears[0] != "conv-1" {
		t.Fatalf("messages=%#v sensitiveClears=%#v", store.messages, store.sensitiveClears)
	}
}

func TestOutboundNonManualDoesNotClearSensitiveHandoffProjection(t *testing.T) {
	store := &recordingStore{}
	handler := Handler{Store: store}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationOutbound,
		Payload: map[string]any{
			"message": map[string]any{
				"conversation_id": "conv-1",
				"content":         "reply",
				"message_origin":  "ai_reply",
			},
		},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(store.messages) != 1 || len(store.sensitiveClears) != 0 {
		t.Fatalf("messages=%#v sensitiveClears=%#v", store.messages, store.sensitiveClears)
	}
}

func TestBuildAssignmentUsesToAssigneeFields(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationAssignment,
		TenantID:  "tenant-1",
		Payload: map[string]any{
			"assignment": map[string]any{
				"conversation_id":  "conv-1",
				"to_assignee_id":   "cs-1",
				"to_assignee_name": "Agent One",
			},
		},
	}, CreatedAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)}
	assignment, ok := BuildAssignment(record)
	if !ok {
		t.Fatal("BuildAssignment ok = false")
	}
	if assignment.ConversationID != "conv-1" || assignment.TenantID != "tenant-1" || assignment.AssigneeID != "cs-1" || assignment.AssigneeName != "Agent One" {
		t.Fatalf("assignment = %+v", assignment)
	}
	if !assignment.UpdatedAt.Equal(record.CreatedAt) {
		t.Fatalf("updated_at = %s", assignment.UpdatedAt)
	}
}

func TestBuildContactProfileUpdate(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventContactProfileUpdated,
		TenantID:  "ent-fallback",
		Payload: map[string]any{
			"tenant_id":                    "ent-1",
			"sender_id":                    "WMExternal123",
			"wework_user_id":               "user-1",
			"customer_name":                "Display",
			"sender_remark":                "Remark",
			"sender_name":                  "Nick",
			"sender_avatar":                "avatar.png",
			"identity_profile_verified_at": "2026-07-02T18:30:00+08:00",
		},
	}}
	update, ok := BuildContactProfileUpdate(record)
	if !ok {
		t.Fatal("BuildContactProfileUpdate ok = false")
	}
	if update.EnterpriseID != "ent-1" || update.SenderID != "WMExternal123" || update.WeWorkUserID != "user-1" {
		t.Fatalf("identity = %+v", update)
	}
	if update.DisplayName != "Display" || update.RemarkName != "Remark" || update.Nickname != "Nick" || update.AvatarURL != "avatar.png" {
		t.Fatalf("display = %+v", update)
	}
	if !update.UpdatedAt.Equal(time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("updated_at = %s", update.UpdatedAt)
	}
}

func TestHandlerDispatchesSupportedProjectionEvents(t *testing.T) {
	store := &recordingStore{}
	invalidator := &recordingInvalidator{}
	handler := Handler{Store: store, ReadModelInvalidator: invalidator}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageReceived,
		Payload:   map[string]any{"conversation_id": "conv-1", "content": "hello"},
	}})
	if err != nil {
		t.Fatalf("message Dispatch returned error: %v", err)
	}
	err = handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationAssignment,
		Payload:   map[string]any{"assignment": map[string]any{"conversation_id": "conv-1", "to_assignee_id": "cs-1"}},
	}})
	if err != nil {
		t.Fatalf("assignment Dispatch returned error: %v", err)
	}
	err = handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventContactProfileUpdated,
		Payload: map[string]any{
			"enterprise_id":  "ent-1",
			"sender_id":      "wm-1",
			"wework_user_id": "user-1",
			"customer_name":  "Display",
		},
	}})
	if err != nil {
		t.Fatalf("profile Dispatch returned error: %v", err)
	}
	if len(store.messages) != 1 || len(store.assignments) != 1 || len(store.identityUpdates) != 1 {
		t.Fatalf("store messages=%#v assignments=%#v identities=%#v", store.messages, store.assignments, store.identityUpdates)
	}
	if len(invalidator.calls) != 3 {
		t.Fatalf("invalidator calls = %#v", invalidator.calls)
	}
	for _, call := range invalidator.calls {
		if !reflect.DeepEqual(call, readmodelcache.AllNamespaces()) {
			t.Fatalf("invalidated namespaces = %+v", call)
		}
	}
}

func TestHandlerSwallowsReadModelInvalidationErrors(t *testing.T) {
	store := &recordingStore{}
	handler := Handler{Store: store, ReadModelInvalidator: &recordingInvalidator{err: errors.New("redis down")}}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageReceived,
		Payload:   map[string]any{"conversation_id": "conv-1"},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(store.messages) != 1 {
		t.Fatalf("messages = %#v", store.messages)
	}
}

func TestHandlerPropagatesStoreErrors(t *testing.T) {
	expected := errors.New("projection failed")
	invalidator := &recordingInvalidator{}
	handler := Handler{Store: &recordingStore{err: expected}, ReadModelInvalidator: invalidator}
	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageReceived,
		Payload:   map[string]any{"conversation_id": "conv-1"},
	}})
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
	if len(invalidator.calls) != 0 {
		t.Fatalf("invalidator calls = %#v", invalidator.calls)
	}
}

type recordingStore struct {
	messages        []projectionupdate.MessageEvent
	assignments     []projectionupdate.Assignment
	identityUpdates []projectionupdate.IdentityUpdate
	sensitiveClears []string
	err             error
}

type recordingInvalidator struct {
	calls [][]string
	err   error
}

func (invalidator *recordingInvalidator) InvalidateNamespaces(ctx context.Context, namespaces ...string) error {
	invalidator.calls = append(invalidator.calls, append([]string{}, namespaces...))
	return invalidator.err
}

func (store *recordingStore) UpsertMessageEvent(ctx context.Context, event projectionupdate.MessageEvent) error {
	if store.err != nil {
		return store.err
	}
	store.messages = append(store.messages, event)
	return nil
}

func (store *recordingStore) UpsertAssignment(ctx context.Context, assignment projectionupdate.Assignment) error {
	if store.err != nil {
		return store.err
	}
	store.assignments = append(store.assignments, assignment)
	return nil
}

func (store *recordingStore) UpdateIdentity(ctx context.Context, update projectionupdate.IdentityUpdate) error {
	if store.err != nil {
		return store.err
	}
	store.identityUpdates = append(store.identityUpdates, update)
	return nil
}

func (store *recordingStore) ClearSensitiveHandoff(ctx context.Context, conversationID string) error {
	if store.err != nil {
		return store.err
	}
	store.sensitiveClears = append(store.sensitiveClears, conversationID)
	return nil
}
