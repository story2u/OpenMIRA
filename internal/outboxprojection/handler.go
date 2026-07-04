// Package outboxprojection maps outbox records to projection write side effects.
package outboxprojection

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/outbox"
	"wework-go/internal/projectionupdate"
	"wework-go/internal/readmodelcache"
)

const (
	EventArchiveMessageIngested      = "archive.message.ingested"
	EventConversationMessageReceived = "conversation.message.received"
	EventConversationOutbound        = "conversation.message.outbound_recorded"
	EventConversationAssignment      = "conversation.assignment.changed"
	EventContactProfileUpdated       = "contact.profile.updated"
)

// Store is the projection writer shape needed by Handler.
type Store interface {
	UpsertMessageEvent(ctx context.Context, event projectionupdate.MessageEvent) error
	UpsertAssignment(ctx context.Context, assignment projectionupdate.Assignment) error
}

type SensitiveHandoffClearer interface {
	ClearSensitiveHandoff(ctx context.Context, conversationID string) error
}

type ContactProfileUpdater interface {
	UpdateIdentity(ctx context.Context, update projectionupdate.IdentityUpdate) error
}

type ReadModelInvalidator interface {
	InvalidateNamespaces(ctx context.Context, namespaces ...string) error
}

// Handler applies projection side effects for supported outbox event types.
type Handler struct {
	Store                Store
	ReadModelInvalidator ReadModelInvalidator
}

// Dispatch handles one outbox record.
func (handler Handler) Dispatch(ctx context.Context, record outbox.Record) error {
	switch strings.TrimSpace(record.EventType) {
	case EventConversationMessageReceived:
		event, ok := BuildMessageEvent(record)
		if !ok {
			return nil
		}
		if handler.Store == nil {
			return fmt.Errorf("outbox projection store is not configured")
		}
		if err := handler.Store.UpsertMessageEvent(ctx, event); err != nil {
			return err
		}
		handler.invalidateReadModels(ctx)
		return nil
	case EventArchiveMessageIngested:
		if !truthy(record.Payload["message_created"]) {
			return nil
		}
		event, ok := BuildMessageEvent(record)
		if !ok {
			return nil
		}
		if handler.Store == nil {
			return fmt.Errorf("outbox projection store is not configured")
		}
		if err := handler.Store.UpsertMessageEvent(ctx, event); err != nil {
			return err
		}
		handler.invalidateReadModels(ctx)
		return nil
	case EventConversationOutbound:
		event, ok := BuildOutboundMessageEvent(record)
		if !ok {
			return nil
		}
		if handler.Store == nil {
			return fmt.Errorf("outbox projection store is not configured")
		}
		if err := handler.Store.UpsertMessageEvent(ctx, event); err != nil {
			return err
		}
		if shouldClearSensitiveHandoff(record) {
			if clearer, ok := handler.Store.(SensitiveHandoffClearer); ok {
				if err := clearer.ClearSensitiveHandoff(ctx, event.ConversationID); err != nil {
					handler.invalidateReadModels(ctx)
					return err
				}
			}
		}
		handler.invalidateReadModels(ctx)
		return nil
	case EventConversationAssignment:
		assignment, ok := BuildAssignment(record)
		if !ok {
			return nil
		}
		if handler.Store == nil {
			return fmt.Errorf("outbox projection store is not configured")
		}
		if err := handler.Store.UpsertAssignment(ctx, assignment); err != nil {
			return err
		}
		handler.invalidateReadModels(ctx)
		return nil
	case EventContactProfileUpdated:
		update, ok := BuildContactProfileUpdate(record)
		if !ok {
			return nil
		}
		if handler.Store == nil {
			return fmt.Errorf("outbox projection store is not configured")
		}
		updater, ok := handler.Store.(ContactProfileUpdater)
		if !ok {
			return fmt.Errorf("outbox projection contact profile updater is not configured")
		}
		if err := updater.UpdateIdentity(ctx, update); err != nil {
			return err
		}
		handler.invalidateReadModels(ctx)
		return nil
	default:
		return nil
	}
}

func (handler Handler) invalidateReadModels(ctx context.Context) {
	if handler.ReadModelInvalidator == nil {
		return
	}
	_ = handler.ReadModelInvalidator.InvalidateNamespaces(ctx, readmodelcache.AllNamespaces()...)
}

func shouldClearSensitiveHandoff(record outbox.Record) bool {
	message := mapValue(record.Payload["message"])
	switch strings.ToLower(strings.TrimSpace(textValue(message["message_origin"]))) {
	case "manual_reply", "manual_send":
		return strings.TrimSpace(textValue(message["conversation_id"])) != ""
	default:
		return false
	}
}

// BuildMessageEvent maps inbound/archive message outbox payloads to projection input.
func BuildMessageEvent(record outbox.Record) (projectionupdate.MessageEvent, bool) {
	payload := cloneMap(record.Payload)
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	if conversationID == "" {
		return projectionupdate.MessageEvent{}, false
	}
	senderName := firstText(payload["sender_display_name"], payload["sender_name"])
	customerName := firstText(payload["customer_name"], payload["sender_display_name"], payload["sender_name"], payload["sender_remark"])
	return projectionupdate.MessageEvent{
		ConversationID:   conversationID,
		TenantID:         defaultText(textValue(payload["tenant_id"]), record.TenantID),
		DeviceID:         textValue(payload["device_id"]),
		WeWorkUserID:     textValue(payload["wework_user_id"]),
		ExternalUserID:   firstText(payload["external_userid"], payload["sender_id"]),
		RoomID:           textValue(payload["room_id"]),
		ConversationType: textValue(payload["conversation_type"]),
		SenderID:         textValue(payload["sender_id"]),
		SenderName:       senderName,
		SenderRemark:     textValue(payload["sender_remark"]),
		SenderAvatar:     firstText(payload["sender_avatar_display"], payload["sender_avatar"]),
		CustomerName:     customerName,
		ConversationName: firstText(payload["conversation_name"], customerName, senderName),
		Content:          textValue(payload["content"]),
		MsgType:          defaultText(textValue(payload["msg_type"]), projectionupdate.DefaultMessageType),
		Direction:        defaultText(textValue(payload["direction"]), projectionupdate.DefaultDirection),
		IsSystemEvent:    truthy(payload["is_system_event"]),
		Timestamp:        payloadTime(payload["timestamp"], recordFallbackTime(record)),
		LastIncomingAt:   optionalPayloadTime(payload["last_incoming_at"]),
		UnreadCount:      optionalInt(payload["unread_count"]),
	}, true
}

// BuildOutboundMessageEvent maps outbound_recorded payloads to projection input.
func BuildOutboundMessageEvent(record outbox.Record) (projectionupdate.MessageEvent, bool) {
	payload := cloneMap(record.Payload)
	message := cloneMap(mapValue(payload["message"]))
	conversationID := strings.TrimSpace(textValue(message["conversation_id"]))
	if conversationID == "" {
		return projectionupdate.MessageEvent{}, false
	}
	tenantID := defaultText(firstText(message["tenant_id"], payload["tenant_id"]), record.TenantID)
	return projectionupdate.MessageEvent{
		ConversationID:   conversationID,
		TenantID:         tenantID,
		DeviceID:         textValue(message["device_id"]),
		WeWorkUserID:     textValue(message["wework_user_id"]),
		ExternalUserID:   firstText(message["external_userid"], message["sender_id"]),
		RoomID:           textValue(message["room_id"]),
		ConversationType: textValue(message["conversation_type"]),
		SenderID:         textValue(message["sender_id"]),
		SenderName:       textValue(message["sender_name"]),
		SenderRemark:     textValue(message["sender_remark"]),
		SenderAvatar:     textValue(message["sender_avatar"]),
		Content:          textValue(message["content"]),
		MsgType:          defaultText(textValue(message["msg_type"]), projectionupdate.DefaultMessageType),
		Direction:        defaultText(textValue(message["direction"]), "outgoing"),
		IsSystemEvent:    truthy(message["is_system_event"]),
		Timestamp:        payloadTime(message["timestamp"], recordFallbackTime(record)),
		UnreadCount:      optionalInt(message["unread_count"]),
	}, true
}

// BuildAssignment maps assignment.changed payloads to projection input.
func BuildAssignment(record outbox.Record) (projectionupdate.Assignment, bool) {
	payload := cloneMap(record.Payload)
	assignmentPayload := cloneMap(mapValue(payload["assignment"]))
	conversationID := strings.TrimSpace(textValue(assignmentPayload["conversation_id"]))
	if conversationID == "" {
		return projectionupdate.Assignment{}, false
	}
	return projectionupdate.Assignment{
		ConversationID: conversationID,
		TenantID:       defaultText(firstText(assignmentPayload["tenant_id"], payload["tenant_id"]), record.TenantID),
		AssigneeID:     firstText(assignmentPayload["to_assignee_id"], assignmentPayload["assignee_id"]),
		AssigneeName:   firstText(assignmentPayload["to_assignee_name"], assignmentPayload["assignee_name"]),
		UpdatedAt:      payloadTime(firstAny(assignmentPayload["updated_at"], payload["updated_at"]), recordFallbackTime(record)),
	}, true
}

// BuildContactProfileUpdate maps contact.profile.updated payloads to projection input.
func BuildContactProfileUpdate(record outbox.Record) (projectionupdate.IdentityUpdate, bool) {
	payload := cloneMap(record.Payload)
	update := projectionupdate.IdentityUpdate{
		EnterpriseID: defaultText(firstText(payload["enterprise_id"], payload["tenant_id"]), record.TenantID),
		SenderID:     firstText(payload["sender_id"], payload["external_userid"]),
		DisplayName:  firstText(payload["identity_display_name"], payload["customer_name"]),
		RemarkName:   firstText(payload["identity_remark_name"], payload["sender_remark"]),
		Nickname:     firstText(payload["identity_nickname"], payload["sender_name"]),
		AvatarURL:    firstText(payload["identity_avatar_url"], payload["sender_avatar"], payload["customer_avatar"]),
		WeWorkUserID: textValue(payload["wework_user_id"]),
		UpdatedAt:    payloadTime(payload["identity_profile_verified_at"], recordFallbackTime(record)),
	}
	update = projectionupdate.NormalizeIdentityUpdate(update)
	if update.EnterpriseID == "" || update.SenderID == "" || update.WeWorkUserID == "" {
		return projectionupdate.IdentityUpdate{}, false
	}
	return update, true
}

func recordFallbackTime(record outbox.Record) time.Time {
	for _, candidate := range []time.Time{record.CreatedAt, record.OccurredAt, record.AvailableAt} {
		if !candidate.IsZero() {
			return candidate.UTC()
		}
	}
	return time.Time{}
}

func optionalPayloadTime(value any) *time.Time {
	parsed := payloadTime(value, time.Time{})
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func payloadTime(value any, fallback time.Time) time.Time {
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return fallback.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return fallback.UTC()
}

func optionalInt(value any) *int {
	switch typed := value.(type) {
	case nil:
		return nil
	case int:
		return &typed
	case int64:
		converted := int(typed)
		return &converted
	case float64:
		converted := int(typed)
		return &converted
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "" {
			return nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return nil
		}
		return &parsed
	}
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
		if text := strings.TrimSpace(textValue(value)); text != "" {
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

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

var supportedEventTypes = []string{
	EventConversationMessageReceived,
	EventArchiveMessageIngested,
	EventConversationOutbound,
	EventConversationAssignment,
	EventContactProfileUpdated,
}

// SupportedEventTypes returns outbox event types that can update projection rows.
func SupportedEventTypes() []string {
	return append([]string(nil), supportedEventTypes...)
}
