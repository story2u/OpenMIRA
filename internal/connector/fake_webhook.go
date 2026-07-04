package connector

import (
	"strings"
	"time"

	"im-go/internal/incomingqueue"
)

const KindConnectorInboundMessage = "connector.inbound_message"

// FakeWebhookConnector is the built-in connector used for local smoke tests.
type FakeWebhookConnector struct {
	ConnectorID string
	Channel     string
	Now         func() time.Time
}

// BuildIncomingQueuePayload converts a connector event into the ingest queue shape.
func (connector FakeWebhookConnector) BuildIncomingQueuePayload(event InboundEvent) map[string]any {
	now := connector.now()
	connectorID := firstNonBlank(event.ConnectorID, connector.ConnectorID, "internal-webhook")
	channel := firstNonBlank(event.Channel, connector.Channel, ChannelInternalWebhook)
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = now
	}
	timestamp := occurredAt.UTC().Format(time.RFC3339Nano)
	traceID := firstNonBlank(event.TraceID, event.EventID)
	if traceID == "" {
		traceID = "connector-" + strings.ReplaceAll(channel, ".", "-") + "-" + now.UTC().Format("20060102150405000000")
	}
	eventID := firstNonBlank(event.EventID, traceID)
	senderID := firstNonBlank(event.Sender.ExternalUserID, event.Sender.ContactID)
	senderName := firstNonBlank(event.Sender.DisplayName, senderID, "unknown")
	conversationType := firstNonBlank(event.Conversation.Type, "single")
	conversationName := firstNonBlank(event.Conversation.DisplayName, senderName)
	conversationKey := firstNonBlank(event.Conversation.ConversationKey, conversationKeyFor(channel, event.AccountID, senderID, event.Conversation.ExternalConversationID))
	messageType := normalizeMessageType(event.MessageType)
	metadata := cloneMap(event.Metadata)
	metadata["connector_id"] = connectorID
	metadata["channel"] = channel

	data := map[string]any{
		"tenant_id":                strings.TrimSpace(event.TenantID),
		"connector_id":             connectorID,
		"channel":                  channel,
		"channel_user_id":          strings.TrimSpace(event.ChannelUserID),
		"endpoint_id":              strings.TrimSpace(event.EndpointID),
		"device_id":                strings.TrimSpace(event.EndpointID),
		"sender_id":                senderID,
		"sender":                   senderName,
		"sender_name":              senderName,
		"sender_avatar":            strings.TrimSpace(event.Sender.AvatarURL),
		"sender_remark":            strings.TrimSpace(event.Sender.Remark),
		"content":                  event.Content,
		"msg_type":                 messageType,
		"conversation_name":        conversationName,
		"conversation_id":          strings.TrimSpace(event.Conversation.ConversationID),
		"conversation_key":         conversationKey,
		"account_id":               strings.TrimSpace(event.AccountID),
		"external_user_id":         senderID,
		"external_userid":          senderID,
		"room_id":                  firstNonBlank(event.Conversation.RoomID, event.Conversation.ExternalConversationID),
		"conversation_type":        conversationType,
		"message_id":               event.MessageID,
		"message_origin":           "connector:" + channel,
		"timestamp":                timestamp,
		"connector_event_id":       eventID,
		"idempotency_key":          strings.TrimSpace(event.IdempotencyKey),
		"external_conversation_id": strings.TrimSpace(event.Conversation.ExternalConversationID),
		"metadata":                 metadata,
	}
	if len(event.Media) > 0 {
		data["media"] = mediaPayload(event.Media)
	}
	return map[string]any{
		"event_type":   incomingqueue.EventTypeConnectorInbound,
		"kind":         KindConnectorInboundMessage,
		"event_id":     eventID,
		"trace_id":     traceID,
		"tenant_id":    strings.TrimSpace(event.TenantID),
		"connector_id": connectorID,
		"channel":      channel,
		"device_id":    strings.TrimSpace(event.EndpointID),
		"occurred_at":  timestamp,
		"data":         data,
	}
}

func (connector FakeWebhookConnector) now() time.Time {
	if connector.Now == nil {
		return time.Now().UTC()
	}
	return connector.Now().UTC()
}

func conversationKeyFor(channel string, accountID string, senderID string, externalConversationID string) string {
	parts := []string{strings.TrimSpace(channel)}
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		parts = append(parts, accountID)
	}
	if externalConversationID = strings.TrimSpace(externalConversationID); externalConversationID != "" {
		parts = append(parts, externalConversationID)
	}
	if senderID = strings.TrimSpace(senderID); senderID != "" {
		parts = append(parts, senderID)
	}
	return strings.Join(parts, ":")
}

func normalizeMessageType(value string) string {
	switch strings.TrimSpace(value) {
	case MessageTypeText, MessageTypeImage, MessageTypeVideo, MessageTypeVoice, MessageTypeFile, MessageTypeUnknown:
		return strings.TrimSpace(value)
	default:
		return MessageTypeText
	}
}

func mediaPayload(media []MediaAttachment) []map[string]any {
	payload := make([]map[string]any, 0, len(media))
	for _, item := range media {
		payload = append(payload, map[string]any{
			"attachment_id": strings.TrimSpace(item.AttachmentID),
			"type":          strings.TrimSpace(item.Type),
			"url":           strings.TrimSpace(item.URL),
			"object_key":    strings.TrimSpace(item.ObjectKey),
			"mime_type":     strings.TrimSpace(item.MIMEType),
			"bytes":         item.Bytes,
			"sha256":        strings.TrimSpace(item.SHA256),
			"metadata":      cloneMap(item.Metadata),
		})
	}
	return payload
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+2)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
