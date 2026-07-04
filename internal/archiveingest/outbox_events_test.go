package archiveingest

import (
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
)

func TestBuildArchiveMessageOutboxPayloadMirrorsPythonFields(t *testing.T) {
	firstMessageAt := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 6, 30, 10, 0, 0, 123, time.UTC)
	payload := BuildArchiveMessageOutboxPayload(
		ArchiveMessage{
			EnterpriseID:  "ent-1",
			ArchiveMsgID:  "am-1",
			TraceID:       "archive:am-1",
			Timestamp:     timestamp,
			IsSystemEvent: true,
		},
		incomingmodel.IncomingMessage{
			MessageID:        42,
			TraceID:          "archive:am-1",
			ArchiveMsgID:     "am-1",
			DeviceID:         "archive_user:ww-1",
			SenderID:         "wm_external_1",
			SenderName:       "Alice",
			SenderAvatar:     "avatar",
			SenderRemark:     "VIP",
			Content:          "hello",
			MsgType:          "text",
			Direction:        incomingmodel.DirectionIncoming,
			MessageOrigin:    "archive_history",
			ConversationName: "Alice chat",
		},
		incomingmodel.ConversationSnapshot{
			ConversationID:   "conv-1",
			ConversationKey:  "key-1",
			TenantID:         "ent-1",
			WeWorkUserID:     "ww-1",
			ExternalUserID:   "wm_external_1",
			ConversationType: "single",
			ConversationName: "Alice chat",
			FirstMessageAt:   &firstMessageAt,
			AIAutoReply:      true,
		},
		true,
	)

	if payload["conversation_id"] != "conv-1" || payload["conversation_key"] != "key-1" || payload["resolved_conversation_id"] != "key-1" {
		t.Fatalf("conversation payload = %#v", payload)
	}
	if payload["publish_event"] != DefaultArchivePublishEvent || payload["message_created"] != true || payload["canonical_source"] != DefaultArchiveCanonicalSource {
		t.Fatalf("defaults = %#v", payload)
	}
	if payload["message_id"] != int64(42) || payload["archive_msgid"] != "am-1" || payload["is_system_event"] != true || payload["ai_auto_reply"] != true {
		t.Fatalf("message fields = %#v", payload)
	}
	if payload["timestamp"] != "2026-06-30T10:00:00.000000123Z" || payload["first_message_at"] != "2026-06-30T09:00:00Z" {
		t.Fatalf("timestamps = %#v", payload)
	}
}

func TestBuildArchiveMessageEventsUseTenantScopedIDsAndConversationPartition(t *testing.T) {
	occurredAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"conversation_id": "conv-1",
		"device_id":       "device-1",
		"sender_id":       "wm_external_1",
	}

	received := BuildArchiveConversationMessageReceivedEvent(" ent-1 ", " archive:am-1 ", payload, occurredAt)
	if received.EventID != "ent-1:archive:am-1:conversation-message" || received.EventType != EventConversationMessageReceived {
		t.Fatalf("received event = %#v", received)
	}
	if received.AggregateID != "conv-1" || received.TenantID != "ent-1" || received.PartitionKey != "ent-1:conv-1" {
		t.Fatalf("received identity = %#v", received)
	}

	archive := BuildArchiveMessageIngestedEvent("ent-1", "archive:am-2", payload, occurredAt)
	if archive.EventID != "ent-1:archive:am-2:archive-message" || archive.EventType != EventArchiveMessageIngested {
		t.Fatalf("archive event = %#v", archive)
	}
}

func TestBuildArchiveMessagePartitionKeyFallsBackToDeviceAndSender(t *testing.T) {
	if got := BuildArchiveMessagePartitionKey("ent-1", "", "device-1", "sender-1"); got != "ent-1:device-1:sender-1" {
		t.Fatalf("partition key = %q", got)
	}
	if got := BuildArchiveMessagePartitionKey("", "", "", ""); got != "default:archive-message" {
		t.Fatalf("default partition key = %q", got)
	}
}
