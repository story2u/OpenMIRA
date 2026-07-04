package archiveingest

import (
	"fmt"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/outbox"
)

const (
	EventArchiveMessageIngested      = "archive.message.ingested"
	EventConversationMessageReceived = "conversation.message.received"

	DefaultArchivePublishEvent    = "conversation.archive_ingested"
	DefaultArchiveCanonicalSource = "archive_primary"
	DefaultArchiveIngestSource    = "archive_history"
)

// BuildArchiveMessageOutboxPayload builds the single-message archive payload shared by realtime and projection.
func BuildArchiveMessageOutboxPayload(message ArchiveMessage, stored incomingmodel.IncomingMessage, conversation incomingmodel.ConversationSnapshot, messageCreated bool) map[string]any {
	conversationID := firstTextValue(conversation.ConversationID, stored.ConversationID)
	conversationKey := firstTextValue(conversation.ConversationKey, stored.ConversationKey, conversationID)
	timestamp := message.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	payload := map[string]any{
		"conversation_id":          conversationID,
		"conversation_key":         conversationKey,
		"resolved_conversation_id": conversationKey,
		"tenant_id":                firstTextValue(message.EnterpriseID, conversation.TenantID, stored.TenantID),
		"trace_id":                 firstTextValue(message.TraceID, stored.TraceID),
		"archive_msgid":            firstTextValue(message.ArchiveMsgID, stored.ArchiveMsgID),
		"wework_user_id":           firstTextValue(conversation.WeWorkUserID, stored.WeWorkUserID),
		"external_userid":          firstTextValue(conversation.ExternalUserID, stored.ExternalUserID),
		"room_id":                  firstTextValue(conversation.RoomID, stored.RoomID),
		"conversation_type":        firstTextValue(conversation.ConversationType, stored.ConversationType),
		"sender_id":                stored.SenderID,
		"sender_name":              stored.SenderName,
		"sender_avatar":            stored.SenderAvatar,
		"sender_remark":            stored.SenderRemark,
		"conversation_name":        firstTextValue(conversation.ConversationName, stored.ConversationName),
		"content":                  stored.Content,
		"msg_type":                 defaultTextValue(stored.MsgType, DefaultMessageType),
		"direction":                defaultTextValue(stored.Direction, DefaultMessageDirection),
		"device_id":                stored.DeviceID,
		"timestamp":                timestamp.UTC().Format(time.RFC3339Nano),
		"created_at":               timestamp.UTC().Format(time.RFC3339Nano),
		"is_system_event":          message.IsSystemEvent,
		"publish_event":            DefaultArchivePublishEvent,
		"message_created":          messageCreated,
		"identity_needs_refresh":   false,
		"ingest_source":            DefaultArchiveIngestSource,
		"canonical_source":         DefaultArchiveCanonicalSource,
		"reconciled_from_archive":  false,
		"ai_auto_reply":            conversation.AIAutoReply,
		"message_origin":           defaultTextValue(stored.MessageOrigin, DefaultArchiveIngestSource),
	}
	if stored.MessageID > 0 {
		payload["message_id"] = stored.MessageID
	}
	if conversation.FirstMessageAt != nil {
		payload["first_message_at"] = conversation.FirstMessageAt.UTC().Format(time.RFC3339Nano)
	} else {
		payload["first_message_at"] = nil
	}
	return payload
}

// BuildArchiveConversationMessageReceivedEvent converts an incoming archive payload to the canonical message event.
func BuildArchiveConversationMessageReceivedEvent(enterpriseID string, traceID string, payload map[string]any, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	normalizedTraceID := strings.TrimSpace(traceID)
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	eventID := buildArchiveTenantScopedEventID(enterpriseID, normalizedTraceID, "conversation-message", "conversation-message", firstTextValue(conversationID, fmt.Sprint(occurredAt.UnixMilli())))
	return outbox.EventEnvelope{
		EventID:       eventID,
		EventType:     EventConversationMessageReceived,
		AggregateType: "conversation",
		AggregateID:   firstTextValue(conversationID, normalizedTraceID, "conversation-message"),
		TenantID:      defaultTextValue(enterpriseID, "default"),
		PartitionKey:  BuildArchiveMessagePartitionKey(enterpriseID, conversationID, textValue(payload["device_id"]), textValue(payload["sender_id"])),
		TraceID:       normalizedTraceID,
		Payload:       cloneMap(payload),
		OccurredAt:    occurredAt.UTC(),
		AvailableAt:   occurredAt.UTC(),
	}
}

// BuildArchiveMessageIngestedEvent converts a non-incoming archive payload to the archive-specific event.
func BuildArchiveMessageIngestedEvent(enterpriseID string, traceID string, payload map[string]any, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	normalizedTraceID := strings.TrimSpace(traceID)
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	eventID := buildArchiveTenantScopedEventID(enterpriseID, normalizedTraceID, "archive-message", "archive-message", firstTextValue(conversationID, fmt.Sprint(occurredAt.UnixMilli())))
	return outbox.EventEnvelope{
		EventID:       eventID,
		EventType:     EventArchiveMessageIngested,
		AggregateType: "conversation",
		AggregateID:   firstTextValue(conversationID, normalizedTraceID, "archive-message"),
		TenantID:      defaultTextValue(enterpriseID, "default"),
		PartitionKey:  BuildArchiveMessagePartitionKey(enterpriseID, conversationID, textValue(payload["device_id"]), textValue(payload["sender_id"])),
		TraceID:       normalizedTraceID,
		Payload:       cloneMap(payload),
		OccurredAt:    occurredAt.UTC(),
		AvailableAt:   occurredAt.UTC(),
	}
}

// BuildArchiveMessagePartitionKey keeps message order inside one tenant conversation.
func BuildArchiveMessagePartitionKey(enterpriseID string, conversationID string, deviceID string, senderID string) string {
	enterpriseKey := defaultTextValue(enterpriseID, "default")
	conversationKey := strings.TrimSpace(conversationID)
	if conversationKey != "" {
		return enterpriseKey + ":" + conversationKey
	}
	deviceKey := strings.TrimSpace(deviceID)
	senderKey := strings.TrimSpace(senderID)
	if deviceKey != "" || senderKey != "" {
		return enterpriseKey + ":" + deviceKey + ":" + senderKey
	}
	return enterpriseKey + ":archive-message"
}

func buildArchiveTenantScopedEventID(enterpriseID string, traceID string, suffix string, fallbackPrefix string, fallbackSeed string) string {
	enterpriseKey := defaultTextValue(enterpriseID, "default")
	traceKey := strings.TrimSpace(traceID)
	suffixKey := strings.TrimSpace(suffix)
	if traceKey != "" && suffixKey != "" {
		return enterpriseKey + ":" + traceKey + ":" + suffixKey
	}
	fallbackKey := defaultTextValue(fallbackSeed, fmt.Sprint(time.Now().UTC().UnixMilli()))
	prefixKey := defaultTextValue(fallbackPrefix, "archive-event")
	return prefixKey + ":" + enterpriseKey + ":" + fallbackKey
}
