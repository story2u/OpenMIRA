// Package outboxdispatch maps outbox records to realtime side effects.
package outboxdispatch

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/outbox"
)

const (
	EventArchiveMessageIngested      = "archive.message.ingested"
	EventConversationMessageReceived = "conversation.message.received"
	EventConversationOutbound        = "conversation.message.outbound_recorded"
	EventConversationMessageRevoke   = "conversation.message.revoke"
	EventConversationAssignment      = "conversation.assignment.changed"
	EventConversationMediaReady      = "conversation.media_ready"
	EventConversationVoiceReady      = "conversation.voice_transcription_ready"
	EventCustomerRelationChanged     = "customer.relation.changed"
	EventContactProfileUpdated       = "contact.profile.updated"
)

var supportedRealtimeEventTypes = []string{
	EventConversationMessageReceived,
	EventArchiveMessageIngested,
	EventConversationOutbound,
	EventConversationMessageRevoke,
	EventConversationAssignment,
	EventConversationMediaReady,
	EventConversationVoiceReady,
	EventCustomerRelationChanged,
	EventContactProfileUpdated,
}

// SupportedRealtimeEventTypes returns event types currently safe for the Go dispatcher.
func SupportedRealtimeEventTypes() []string {
	return append([]string(nil), supportedRealtimeEventTypes...)
}

// Hub is the minimal realtime publish shape used by the dispatcher.
type Hub interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// RealtimeEvent describes one ws_hub.publish call.
type RealtimeEvent struct {
	Channel string
	Event   string
	Topic   string
	Payload map[string]any
}

// Dispatcher publishes supported outbox records through a realtime hub.
type Dispatcher struct {
	Hub Hub
}

// Dispatch handles one outbox record.
func (dispatcher Dispatcher) Dispatch(ctx context.Context, record outbox.Record) error {
	event, ok := BuildRealtimeEvent(record)
	if !ok {
		return nil
	}
	if dispatcher.Hub == nil {
		return fmt.Errorf("outbox dispatcher realtime hub is not configured")
	}
	return dispatcher.Hub.Publish(ctx, event.Channel, event.Event, event.Topic, event.Payload)
}

// BuildRealtimeEvent maps supported outbox records to Python-compatible realtime envelopes.
func BuildRealtimeEvent(record outbox.Record) (RealtimeEvent, bool) {
	payload := cloneMap(record.Payload)
	switch strings.TrimSpace(record.EventType) {
	case EventConversationMessageReceived:
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.message"),
			Topic:   "conversation.message",
			Payload: conversationMessagePayload(record, payload),
		}, true
	case EventArchiveMessageIngested:
		if !truthy(payload["message_created"]) {
			return RealtimeEvent{}, false
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.archive_ingested"),
			Topic:   "conversation.message",
			Payload: conversationMessagePayload(record, payload),
		}, true
	case EventConversationOutbound:
		tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
		messagePayload := cloneMap(mapValue(payload["message"]))
		if _, ok := messagePayload["tenant_id"]; !ok {
			messagePayload["tenant_id"] = strings.TrimSpace(tenantID)
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.replied"),
			Topic:   "conversation.message",
			Payload: messagePayload,
		}, true
	case EventConversationMessageRevoke:
		tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
		messagePayload := cloneMap(mapValue(payload["message"]))
		if _, ok := messagePayload["tenant_id"]; !ok {
			messagePayload["tenant_id"] = strings.TrimSpace(tenantID)
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.message.revoke"),
			Topic:   "conversation.message",
			Payload: messagePayload,
		}, true
	case EventConversationAssignment:
		tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
		assignmentPayload := cloneMap(mapValue(payload["assignment"]))
		if _, ok := assignmentPayload["tenant_id"]; !ok {
			assignmentPayload["tenant_id"] = strings.TrimSpace(tenantID)
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.transferred"),
			Topic:   "conversation.assignment",
			Payload: assignmentPayload,
		}, true
	case EventConversationMediaReady:
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.media_ready"),
			Topic:   "conversation.media_ready",
			Payload: mediaReadyPayload(record, payload),
		}, true
	case EventConversationVoiceReady:
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "conversation.voice_transcription_ready"),
			Topic:   "conversation.voice_transcription_ready",
			Payload: voiceTranscriptionReadyPayload(record, payload),
		}, true
	case EventCustomerRelationChanged:
		tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
		if _, ok := payload["tenant_id"]; !ok {
			payload["tenant_id"] = strings.TrimSpace(tenantID)
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), EventCustomerRelationChanged),
			Topic:   "customer.relation",
			Payload: payload,
		}, true
	case EventContactProfileUpdated:
		tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
		if _, ok := payload["tenant_id"]; !ok {
			payload["tenant_id"] = strings.TrimSpace(tenantID)
		}
		return RealtimeEvent{
			Channel: "conversations",
			Event:   defaultText(textValue(payload["publish_event"]), "contact_profile_updated"),
			Topic:   "contact.profile_updated",
			Payload: payload,
		}, true
	default:
		return RealtimeEvent{}, false
	}
}

func conversationMessagePayload(record outbox.Record, payload map[string]any) map[string]any {
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	conversationKey := defaultText(firstText(payload["conversation_key"], payload["resolved_conversation_id"], conversationID), conversationID)
	resolvedConversationID := defaultText(firstText(payload["resolved_conversation_id"], payload["conversation_key"], conversationID), conversationID)
	tenantID := defaultText(textValue(payload["tenant_id"]), record.TenantID)
	timestamp := defaultText(textValue(payload["timestamp"]), fallbackTimestamp(record))
	senderName := strings.TrimSpace(textValue(payload["sender_name"]))
	senderRemark := strings.TrimSpace(textValue(payload["sender_remark"]))
	displayName := firstText(payload["sender_display_name"], senderRemark, senderName)
	senderAvatar := firstText(payload["sender_avatar_display"], payload["sender_avatar"])
	return map[string]any{
		"conversation_id":          conversationID,
		"conversation_key":         conversationKey,
		"resolved_conversation_id": resolvedConversationID,
		"tenant_id":                strings.TrimSpace(tenantID),
		"message_id":               messageIDValue(payload["message_id"]),
		"trace_id":                 defaultText(textValue(payload["trace_id"]), record.TraceID),
		"archive_msgid":            strings.TrimSpace(textValue(payload["archive_msgid"])),
		"wework_user_id":           strings.TrimSpace(textValue(payload["wework_user_id"])),
		"external_userid":          firstText(payload["external_userid"], payload["sender_id"]),
		"room_id":                  strings.TrimSpace(textValue(payload["room_id"])),
		"conversation_type":        strings.TrimSpace(textValue(payload["conversation_type"])),
		"sender_id":                strings.TrimSpace(textValue(payload["sender_id"])),
		"sender_name":              displayName,
		"send_target_name":         firstText(displayName, senderName),
		"sender_avatar":            senderAvatar,
		"sender_remark":            senderRemark,
		"customer_name":            displayName,
		"identity_status":          strings.TrimSpace(textValue(payload["identity_status"])),
		"needs_refresh":            truthy(firstAny(payload["needs_identity_refresh"], payload["needs_refresh"])),
		"conversation_name":        strings.TrimSpace(textValue(payload["conversation_name"])),
		"content":                  textValue(payload["content"]),
		"msg_type":                 defaultText(textValue(payload["msg_type"]), "text"),
		"direction":                defaultText(textValue(payload["direction"]), "incoming"),
		"message_origin":           strings.TrimSpace(textValue(payload["message_origin"])),
		"task_id":                  strings.TrimSpace(textValue(payload["task_id"])),
		"send_status":              strings.TrimSpace(textValue(payload["send_status"])),
		"send_error":               strings.TrimSpace(textValue(payload["send_error"])),
		"is_system_event":          truthy(payload["is_system_event"]),
		"device_id":                strings.TrimSpace(textValue(payload["device_id"])),
		"timestamp":                timestamp,
		"created_at":               defaultText(textValue(payload["created_at"]), timestamp),
		"assignee_id":              strings.TrimSpace(textValue(payload["assignee_id"])),
		"assignee_name":            strings.TrimSpace(textValue(payload["assignee_name"])),
	}
}

func mediaReadyPayload(record outbox.Record, payload map[string]any) map[string]any {
	return map[string]any{
		"conversation_id": strings.TrimSpace(textValue(payload["conversation_id"])),
		"trace_id":        defaultText(textValue(payload["trace_id"]), record.TraceID),
		"archive_msgid":   strings.TrimSpace(textValue(payload["archive_msgid"])),
		"tenant_id":       defaultText(textValue(payload["tenant_id"]), record.TenantID),
		"device_id":       strings.TrimSpace(textValue(payload["device_id"])),
		"sender_id":       strings.TrimSpace(textValue(payload["sender_id"])),
		"sender_name":     strings.TrimSpace(textValue(payload["sender_name"])),
		"msg_type":        strings.TrimSpace(textValue(payload["msg_type"])),
		"direction":       strings.TrimSpace(textValue(payload["direction"])),
		"timestamp":       defaultText(textValue(payload["timestamp"]), fallbackTimestamp(record)),
		"created_at":      defaultText(textValue(payload["created_at"]), fallbackTimestamp(record)),
		"media_status":    defaultText(textValue(payload["media_status"]), "success"),
		"media_ready":     true,
		"media_task_id":   strings.TrimSpace(textValue(payload["media_task_id"])),
		"object_url":      strings.TrimSpace(textValue(payload["object_url"])),
	}
}

func voiceTranscriptionReadyPayload(record outbox.Record, payload map[string]any) map[string]any {
	return map[string]any{
		"conversation_id":                strings.TrimSpace(textValue(payload["conversation_id"])),
		"trace_id":                       defaultText(textValue(payload["trace_id"]), record.TraceID),
		"archive_msgid":                  strings.TrimSpace(textValue(payload["archive_msgid"])),
		"tenant_id":                      defaultText(textValue(payload["tenant_id"]), record.TenantID),
		"device_id":                      strings.TrimSpace(textValue(payload["device_id"])),
		"sender_id":                      strings.TrimSpace(textValue(payload["sender_id"])),
		"sender_name":                    strings.TrimSpace(textValue(payload["sender_name"])),
		"msg_type":                       strings.TrimSpace(textValue(payload["msg_type"])),
		"direction":                      strings.TrimSpace(textValue(payload["direction"])),
		"timestamp":                      defaultText(textValue(payload["timestamp"]), fallbackTimestamp(record)),
		"created_at":                     defaultText(textValue(payload["created_at"]), fallbackTimestamp(record)),
		"media_task_id":                  strings.TrimSpace(textValue(payload["media_task_id"])),
		"voice_transcription_status":     strings.TrimSpace(textValue(payload["voice_transcription_status"])),
		"voice_transcription_error":      strings.TrimSpace(textValue(payload["voice_transcription_error"])),
		"voice_transcription_execute_id": strings.TrimSpace(textValue(payload["voice_transcription_execute_id"])),
		"voice_text":                     textValue(payload["voice_text"]),
		"updated_at":                     defaultText(textValue(payload["updated_at"]), fallbackTimestamp(record)),
	}
}

func messageIDValue(value any) any {
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return nil
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return nil
	}
	return parsed
}

func fallbackTimestamp(record outbox.Record) string {
	for _, candidate := range []time.Time{record.CreatedAt, record.OccurredAt, record.AvailableAt} {
		if !candidate.IsZero() {
			return candidate.UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func defaultText(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return strings.TrimSpace(fallback)
	}
	return trimmed
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil && strings.TrimSpace(textValue(value)) != "" {
			return value
		}
	}
	return nil
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "", "0", "false", "no", "off":
			return false
		default:
			return true
		}
	default:
		return true
	}
}
