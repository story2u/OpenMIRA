package incomingwrite

import (
	"testing"
	"time"
)

func TestBuildIncomingEventsCreatesRealtimeEnvelope(t *testing.T) {
	timestamp := time.Date(2026, 6, 30, 10, 0, 0, 123, time.UTC)
	result := BuildIncomingEvents(
		IncomingMessage{
			TraceID:      "trace-1",
			MessageID:    "msg-1",
			TenantID:     "tenant-1",
			DeviceID:     "device-1",
			SenderID:     "external-1",
			Content:      "hello",
			MsgType:      "text",
			Timestamp:    timestamp,
			WeWorkUserID: "ww-1",
		},
		ConversationSnapshot{
			ConversationID:   "conv-1",
			ConversationKey:  "key-1",
			AccountID:        "account-1",
			WeWorkUserID:     "conv-ww",
			ExternalUserID:   "external-1",
			RoomID:           "room-1",
			ConversationType: "single",
			SenderName:       "Alice",
			SenderAvatar:     "avatar",
			SenderRemark:     "VIP",
			ConversationName: "Alice chat",
		},
		BuildOptions{IsNew: true, IngestSource: "", CanonicalSource: "", ReconciledFromArchive: true},
	)

	if result.AutoReplyQueued || result.RealtimeEvent != "conversation.incoming" || len(result.Events) != 1 {
		t.Fatalf("result = %#v", result)
	}
	event := result.Events[0]
	if event.EventID != "trace-1:realtime" || event.EventType != EventConversationMessage || event.PartitionKey != "device-1:external-1" {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["conversation_id"] != "conv-1" || event.Payload["conversation_key"] != "key-1" || event.Payload["wework_user_id"] != "conv-ww" {
		t.Fatalf("payload identity = %#v", event.Payload)
	}
	if event.Payload["publish_event"] != "conversation.incoming" || event.Payload["ingest_source"] != DefaultIngestSource || event.Payload["canonical_source"] != DefaultCanonicalSource || event.Payload["reconciled_from_archive"] != true {
		t.Fatalf("payload defaults = %#v", event.Payload)
	}
}

func TestBuildIncomingEventsQueuesAutoReply(t *testing.T) {
	timestamp := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	firstMessageAt := timestamp.Add(-time.Hour)
	result := BuildIncomingEvents(
		IncomingMessage{TraceID: "trace-1", DeviceID: "device-1", SenderID: "external-1", Content: "hello", Timestamp: timestamp},
		ConversationSnapshot{ConversationID: "conv-1", AccountID: "account-1", SenderName: "Alice", ConversationName: "Alice chat", FirstMessageAt: firstMessageAt},
		BuildOptions{IsNew: true, EffectiveAIAutoReply: true, AutoReplyWhen: "new_only", TenantID: "tenant-override"},
	)

	if !result.AutoReplyQueued || len(result.Events) != 2 {
		t.Fatalf("result = %#v", result)
	}
	autoReply := result.Events[1]
	if autoReply.EventID != "trace-1:auto-reply" || autoReply.EventType != EventConversationAutoReply || autoReply.AvailableAt != timestamp.Add(time.Millisecond) {
		t.Fatalf("autoReply = %#v", autoReply)
	}
	if autoReply.Payload["tenant_id"] != "tenant-override" || autoReply.Payload["first_message_at"] != firstMessageAt.Format(time.RFC3339Nano) {
		t.Fatalf("autoReply payload = %#v", autoReply.Payload)
	}
}

func TestBuildIncomingEventsSkipsAutoReplyForExistingNewOnly(t *testing.T) {
	result := BuildIncomingEvents(
		IncomingMessage{TraceID: "trace-1", Timestamp: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)},
		ConversationSnapshot{ConversationID: "conv-1", AccountID: "account-1"},
		BuildOptions{IsNew: false, EffectiveAIAutoReply: true, AutoReplyWhen: "new_only"},
	)
	if result.AutoReplyQueued || len(result.Events) != 1 || result.RealtimeEvent != "conversation.message" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildArchiveSyncSignalMirrorsPythonDefaults(t *testing.T) {
	occurredAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	event := BuildArchiveSyncSignal(ArchiveSyncSignal{
		EnterpriseID: " ent-1 ",
		TraceID:      "trace-1",
		OccurredAt:   occurredAt,
	})
	if event.EventID != "archive-sync:ent-1:self_decrypt:trace-1" || event.EventType != EventArchiveSyncRequested {
		t.Fatalf("event = %#v", event)
	}
	if event.AggregateID != "ent-1:self_decrypt" || event.PartitionKey != "ent-1:self_decrypt" || event.AvailableAt != occurredAt.Add(time.Second) {
		t.Fatalf("event timing = %#v", event)
	}
	if event.Payload["device_id"] != "unknown_device" || event.Payload["sender_id"] != "unknown_sender" || event.Payload["trigger_reason"] != DefaultArchiveSyncTriggerReason {
		t.Fatalf("payload = %#v", event.Payload)
	}
}
