package outboxdispatch

import (
	"context"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestBuildRealtimeEventForConversationMessage(t *testing.T) {
	record := outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventID:   "evt-1",
			EventType: EventConversationMessageReceived,
			TenantID:  "tenant-fallback",
			TraceID:   "trace-fallback",
			Payload: map[string]any{
				"conversation_id": "conv-1",
				"sender_id":       "external-1",
				"sender_name":     "Alice",
				"sender_remark":   "VIP Alice",
				"content":         "hello",
				"message_id":      "42",
				"timestamp":       "2026-06-30T10:00:00Z",
				"publish_event":   "conversation.incoming",
			},
		},
	}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != "conversation.incoming" || event.Topic != "conversation.message" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "tenant-fallback" || event.Payload["trace_id"] != "trace-fallback" {
		t.Fatalf("payload identity = %#v", event.Payload)
	}
	if event.Payload["message_id"] != 42 || event.Payload["msg_type"] != "text" || event.Payload["direction"] != "incoming" {
		t.Fatalf("payload defaults = %#v", event.Payload)
	}
	if event.Payload["sender_name"] != "VIP Alice" || event.Payload["send_target_name"] != "VIP Alice" || event.Payload["external_userid"] != "external-1" {
		t.Fatalf("payload sender = %#v", event.Payload)
	}
}

func TestArchiveMessageRequiresCreatedFlag(t *testing.T) {
	skipped := outbox.Record{EventEnvelope: outbox.EventEnvelope{EventType: EventArchiveMessageIngested, Payload: map[string]any{"message_created": false}}}
	if _, ok := BuildRealtimeEvent(skipped); ok {
		t.Fatal("archive duplicate should not publish")
	}
	record := outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventType: EventArchiveMessageIngested,
			Payload: map[string]any{
				"message_created": true,
				"conversation_id": "conv-1",
				"content":         "archive",
			},
		},
		CreatedAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}
	event, ok := BuildRealtimeEvent(record)
	if !ok || event.Event != "conversation.archive_ingested" || event.Payload["timestamp"] != "2026-06-30T10:00:00Z" {
		t.Fatalf("event=%#v ok=%v", event, ok)
	}
}

func TestOutboundRecordedPublishesMessagePayload(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationOutbound,
		TenantID:  "tenant-1",
		Payload: map[string]any{
			"publish_event": "conversation.custom_replied",
			"message": map[string]any{
				"conversation_id": "conv-1",
				"content":         "sent",
			},
		},
	}}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Event != "conversation.custom_replied" || event.Topic != "conversation.message" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "tenant-1" || event.Payload["content"] != "sent" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestMessageRevokePublishesMessagePayload(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationMessageRevoke,
		TenantID:  "tenant-1",
		Payload: map[string]any{
			"message": map[string]any{
				"conversation_id": "conv-1",
				"trace_id":        "trace-1",
				"revoke_status":   "pending",
			},
		},
	}}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != "conversation.message.revoke" || event.Topic != "conversation.message" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "tenant-1" || event.Payload["revoke_status"] != "pending" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestAssignmentChangedPublishesAssignmentPayload(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationAssignment,
		TenantID:  "tenant-1",
		Payload: map[string]any{
			"assignment": map[string]any{"conversation_id": "conv-1", "to_assignee_id": "agent-1"},
		},
	}}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Event != "conversation.transferred" || event.Topic != "conversation.assignment" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "tenant-1" || event.Payload["to_assignee_id"] != "agent-1" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestMediaReadyPublishesConversationRefreshPayload(t *testing.T) {
	record := outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventType: EventConversationMediaReady,
			TenantID:  "tenant-1",
			TraceID:   "trace-1",
			Payload: map[string]any{
				"conversation_id": "conv-1",
				"archive_msgid":   "am-1",
				"media_task_id":   "amt-1",
				"object_url":      "http://object-storage:9102/objects/ent-1/file.png",
			},
		},
		CreatedAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != "conversation.media_ready" || event.Topic != "conversation.media_ready" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["conversation_id"] != "conv-1" || event.Payload["tenant_id"] != "tenant-1" || event.Payload["media_ready"] != true || event.Payload["media_status"] != "success" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestVoiceTranscriptionReadyPublishesConversationRefreshPayload(t *testing.T) {
	record := outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventType: EventConversationVoiceReady,
			TenantID:  "tenant-1",
			TraceID:   "trace-1",
			Payload: map[string]any{
				"conversation_id":                "conv-1",
				"archive_msgid":                  "am-1",
				"media_task_id":                  "amt-1",
				"voice_transcription_status":     "success",
				"voice_transcription_execute_id": "exec-1",
				"voice_text":                     "你好",
			},
		},
		CreatedAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != "conversation.voice_transcription_ready" || event.Topic != "conversation.voice_transcription_ready" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["conversation_id"] != "conv-1" || event.Payload["tenant_id"] != "tenant-1" || event.Payload["voice_text"] != "你好" || event.Payload["voice_transcription_status"] != "success" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestCustomerRelationChangedPublishesLegacyTopic(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventCustomerRelationChanged,
		TenantID:  "ent-1",
		Payload: map[string]any{
			"conversation_id":                 "ww:user-1:ext-1",
			"change_type":                     "del_follow_user",
			"customer_deleted_current_member": true,
		},
	}}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != EventCustomerRelationChanged || event.Topic != "customer.relation" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "ent-1" || event.Payload["conversation_id"] != "ww:user-1:ext-1" || event.Payload["customer_deleted_current_member"] != true {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestContactProfileUpdatedPublishesLegacyTopic(t *testing.T) {
	record := outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventContactProfileUpdated,
		TenantID:  "ent-1",
		Payload: map[string]any{
			"conversation_id": "ww:user-1:wmexternal123",
			"sender_id":       "WMExternal123",
			"customer_name":   "Display",
		},
	}}

	event, ok := BuildRealtimeEvent(record)
	if !ok {
		t.Fatal("BuildRealtimeEvent ok = false")
	}
	if event.Channel != "conversations" || event.Event != "contact_profile_updated" || event.Topic != "contact.profile_updated" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["tenant_id"] != "ent-1" || event.Payload["sender_id"] != "WMExternal123" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestDispatcherPublishesThroughHub(t *testing.T) {
	hub := &recordingHub{}
	dispatcher := Dispatcher{Hub: hub}
	err := dispatcher.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventConversationAssignment,
		Payload:   map[string]any{"assignment": map[string]any{"conversation_id": "conv-1"}},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(hub.events) != 1 || hub.events[0].Event != "conversation.transferred" {
		t.Fatalf("hub events = %#v", hub.events)
	}
}

type recordingHub struct {
	events []RealtimeEvent
}

func (hub *recordingHub) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	hub.events = append(hub.events, RealtimeEvent{Channel: channel, Event: event, Topic: topic, Payload: payload})
	return nil
}
