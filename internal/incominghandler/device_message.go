// Package incominghandler adapts incoming queue payloads to write services.
package incominghandler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"im-go/internal/archivereconcile"
	"im-go/internal/incomingqueue"
	"im-go/internal/incomingwrite"
)

const (
	IngestSourceDeviceMessageReceived = "device_message_received"
	IngestSourceConnectorInbound      = "connector_inbound_message"
	CanonicalSourceDevicePrimary      = "device_primary"
	CanonicalSourceConnector          = "connector"
)

// IngestService is the incoming write service boundary used by device handlers.
type IngestService interface {
	Ingest(ctx context.Context, message incomingwrite.IncomingMessage, options incomingwrite.BuildOptions) (incomingwrite.ServiceResult, error)
}

// ArchiveEnterpriseStore reads archive reconcile fields for one enterprise.
type ArchiveEnterpriseStore interface {
	GetArchiveReconcileEnterprise(ctx context.Context, enterpriseID string) (*archivereconcile.Enterprise, error)
}

// ArchiveSyncService queues archive reconciliation requests.
type ArchiveSyncService interface {
	QueueArchiveSync(ctx context.Context, signal incomingwrite.ArchiveSyncSignal) error
}

// DeviceMessageInput is the normalized write input built from a queue payload.
type DeviceMessageInput struct {
	Message incomingwrite.IncomingMessage
	Options incomingwrite.BuildOptions
}

// DeviceMessageHandler consumes device.message.incoming queue payloads.
type DeviceMessageHandler struct {
	Service            IngestService
	ArchiveEnterprises ArchiveEnterpriseStore
	ArchiveSync        ArchiveSyncService
	Now                func() time.Time
}

// Handle maps one queue payload and writes it through Service.
func (handler DeviceMessageHandler) Handle(ctx context.Context, payload map[string]any) error {
	if handler.Service == nil {
		return fmt.Errorf("incoming device message service is not configured")
	}
	now := handler.now()
	input := BuildDeviceMessageInput(payload, now)
	config, err := handler.archiveConfig(ctx, input.Options.TenantID)
	if err != nil {
		return err
	}
	if archivereconcile.ShouldDirectIngest(config) {
		if _, err := handler.Service.Ingest(ctx, input.Message, input.Options); err != nil {
			return err
		}
		if config.ArchiveReconcileEnabled {
			_ = handler.queueArchiveSync(ctx, input, config, now)
		}
		return nil
	}
	if err := handler.queueArchiveSync(ctx, input, config, now); err != nil {
		_, ingestErr := handler.Service.Ingest(ctx, input.Message, input.Options)
		return ingestErr
	}
	return nil
}

// BuildDeviceMessageInput maps queued realtime or connector events to the write model.
func BuildDeviceMessageInput(payload map[string]any, now time.Time) DeviceMessageInput {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	deviceID := firstText(payload["device_id"], data["device_id"])
	senderName := firstText(data["sender"], data["sender_name"], "unknown")
	senderID := firstText(data["sender_id"], senderName)
	conversationName := firstText(data["conversation_name"], senderName)
	traceID := firstText(payload["trace_id"], payload["event_id"])
	if traceID == "" {
		traceID = fallbackTraceID(deviceID, now)
	}
	ingestSource := IngestSourceDeviceMessageReceived
	canonicalSource := CanonicalSourceDevicePrimary
	eventType := strings.TrimSpace(textValue(payload["event_type"]))
	kind := strings.TrimSpace(textValue(payload["kind"]))
	if eventType == incomingqueue.EventTypeConnectorInbound || kind == "connector.inbound_message" {
		ingestSource = IngestSourceConnectorInbound
		canonicalSource = CanonicalSourceConnector
	}
	messageTime := parseTime(firstText(data["timestamp"], payload["occurred_at"]), now)
	tenantID := firstText(payload["tenant_id"], data["tenant_id"])
	msgType := safeMessageType(firstText(data["msg_type"], "text"))
	channelUserID := firstText(data["channel_user_id"], data["account_user_id"], data["wework_user_id"])
	externalUserID := firstText(data["channel_contact_id"], data["external_user_id"], data["external_userid"], senderID)
	message := incomingwrite.IncomingMessage{
		TraceID:          traceID,
		MessageID:        messageIDValue(data["message_id"]),
		TenantID:         tenantID,
		ArchiveMsgID:     firstText(data["archive_msgid"]),
		ConversationID:   firstText(data["conversation_id"]),
		ConversationKey:  firstText(data["conversation_key"]),
		AccountID:        firstText(data["account_id"]),
		ChannelUserID:    channelUserID,
		WeWorkUserID:     channelUserID,
		ExternalUserID:   externalUserID,
		RoomID:           firstText(data["room_id"]),
		ConversationType: firstText(data["conversation_type"]),
		DeviceID:         deviceID,
		SenderID:         senderID,
		SenderName:       senderName,
		SenderAvatar:     firstText(data["sender_avatar"]),
		SenderRemark:     firstText(data["sender_remark"]),
		Content:          textValue(data["content"]),
		MsgType:          msgType,
		ConversationName: conversationName,
		Timestamp:        messageTime,
		MessageOrigin:    firstText(data["message_origin"]),
	}
	return DeviceMessageInput{
		Message: message,
		Options: incomingwrite.BuildOptions{
			TenantID:              tenantID,
			IngestSource:          ingestSource,
			CanonicalSource:       canonicalSource,
			ReconciledFromArchive: false,
		},
	}
}

func (handler DeviceMessageHandler) now() time.Time {
	if handler.Now == nil {
		return time.Now().UTC()
	}
	return handler.Now().UTC()
}

func (handler DeviceMessageHandler) archiveConfig(ctx context.Context, tenantID string) (archivereconcile.Config, error) {
	if handler.ArchiveEnterprises == nil {
		return archivereconcile.BuildConfig(nil), nil
	}
	enterprise, err := handler.ArchiveEnterprises.GetArchiveReconcileEnterprise(ctx, tenantID)
	if err != nil {
		return archivereconcile.Config{}, err
	}
	return archivereconcile.BuildConfig(enterprise), nil
}

func (handler DeviceMessageHandler) queueArchiveSync(ctx context.Context, input DeviceMessageInput, config archivereconcile.Config, occurredAt time.Time) error {
	service := handler.archiveSyncService()
	if service == nil {
		return fmt.Errorf("incoming archive sync service is not configured")
	}
	return service.QueueArchiveSync(ctx, incomingwrite.ArchiveSyncSignal{
		EnterpriseID:  input.Options.TenantID,
		Source:        config.ArchiveSource,
		TraceID:       input.Message.TraceID,
		DeviceID:      input.Message.DeviceID,
		SenderID:      input.Message.SenderID,
		OccurredAt:    occurredAt,
		TriggerReason: archivereconcile.ArchiveSyncReason(config),
	})
}

func (handler DeviceMessageHandler) archiveSyncService() ArchiveSyncService {
	if handler.ArchiveSync != nil {
		return handler.ArchiveSync
	}
	service, _ := handler.Service.(ArchiveSyncService)
	return service
}

func fallbackTraceID(deviceID string, now time.Time) string {
	return "device-" + strings.TrimSpace(deviceID) + "-" + now.UTC().Format("20060102150405000000")
}

func parseTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback.UTC()
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback.UTC()
	}
	return parsed.UTC()
}

func safeMessageType(value string) string {
	switch strings.TrimSpace(value) {
	case "text", "image", "video", "voice", "file", "unknown":
		return strings.TrimSpace(value)
	default:
		return "text"
	}
}

func messageIDValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		if typed <= 0 {
			return nil
		}
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil && parsed > 0 {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return nil
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
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
