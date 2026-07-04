package incomingwrite

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"im-go/internal/incomingmodel"
	"im-go/internal/outbox"
)

// ChatIngestor is the durable chat write boundary used by Service.
type ChatIngestor interface {
	IngestIncomingMessage(ctx context.Context, message IncomingMessage) (bool, ConversationSnapshot, error)
}

// ResultChatIngestor can return the normalized message used by the durable store.
type ResultChatIngestor interface {
	IngestIncomingMessageWithResult(ctx context.Context, message IncomingMessage) (ChatIngestResult, error)
}

// ChatIngestResult carries the normalized write result back to outbox construction.
type ChatIngestResult struct {
	IsNew        bool
	Conversation ConversationSnapshot
	Message      IncomingMessage
}

// OutboxEnqueuer is the durable outbox append boundary used by Service.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// CustomerReplyMarker marks SOP delivery facts when a customer replies.
type CustomerReplyMarker interface {
	MarkCustomerReply(ctx context.Context, tenantID string, conversationID string, externalUserID string, replyTraceID string, replyMsgID string, repliedAt time.Time) (bool, error)
}

// ServiceResult mirrors IncomingMessageWriteService.ingest_incoming_message_async output.
type ServiceResult struct {
	IsNew           bool
	Conversation    ConversationSnapshot
	AutoReplyQueued bool
	OutboxRecords   []outbox.Record
}

// Service writes incoming messages and appends follow-up outbox events.
type Service struct {
	Chat            ChatIngestor
	Outbox          OutboxEnqueuer
	CustomerReplies CustomerReplyMarker
}

// Ingest writes a message via Chat and enqueues realtime/automation outbox events.
func (service Service) Ingest(ctx context.Context, message IncomingMessage, options BuildOptions) (ServiceResult, error) {
	if service.Chat == nil {
		return ServiceResult{}, fmt.Errorf("incoming write chat ingestor is not configured")
	}
	if service.Outbox == nil {
		return ServiceResult{}, fmt.Errorf("incoming write outbox enqueuer is not configured")
	}
	var isNew bool
	var conversation ConversationSnapshot
	if ingestor, ok := service.Chat.(ResultChatIngestor); ok {
		result, err := ingestor.IngestIncomingMessageWithResult(ctx, message)
		if err != nil {
			return ServiceResult{}, err
		}
		isNew = result.IsNew
		conversation = result.Conversation
		message = result.Message
	} else {
		var err error
		isNew, conversation, err = service.Chat.IngestIncomingMessage(ctx, message)
		if err != nil {
			return ServiceResult{}, err
		}
	}
	if isNew && service.CustomerReplies != nil {
		_, _ = service.CustomerReplies.MarkCustomerReply(
			ctx,
			defaultText(options.TenantID, message.TenantID),
			defaultText(conversation.ConversationID, message.ConversationID),
			firstNonBlank(conversation.ExternalUserID, message.ExternalUserID, message.SenderID),
			strings.TrimSpace(message.TraceID),
			messageIDText(message.MessageID),
			message.Timestamp,
		)
	}
	options.IsNew = isNew
	events := BuildIncomingEvents(message, conversation, options)
	records, err := service.Outbox.EnqueueMany(ctx, events.Events)
	if err != nil {
		return ServiceResult{}, err
	}
	return ServiceResult{
		IsNew:           isNew,
		Conversation:    conversation,
		AutoReplyQueued: events.AutoReplyQueued,
		OutboxRecords:   records,
	}, nil
}

// QueueArchiveSync enqueues one archive reconciliation request.
func (service Service) QueueArchiveSync(ctx context.Context, signal ArchiveSyncSignal) error {
	if service.Outbox == nil {
		return fmt.Errorf("incoming write outbox enqueuer is not configured")
	}
	event := BuildArchiveSyncSignal(signal)
	_, err := service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{event})
	return err
}

// MessageStore is the durable incoming message repository shape.
type MessageStore interface {
	AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error)
}

// StoreChatIngestor adapts an incomingmodel store to the incomingwrite boundary.
type StoreChatIngestor struct {
	Store         MessageStore
	NextMessageID func() int64
}

// IngestIncomingMessage keeps StoreChatIngestor compatible with ChatIngestor.
func (ingestor StoreChatIngestor) IngestIncomingMessage(ctx context.Context, message IncomingMessage) (bool, ConversationSnapshot, error) {
	result, err := ingestor.IngestIncomingMessageWithResult(ctx, message)
	if err != nil {
		return false, ConversationSnapshot{}, err
	}
	return result.IsNew, result.Conversation, nil
}

// IngestIncomingMessageWithResult writes through the store and returns the normalized message.
func (ingestor StoreChatIngestor) IngestIncomingMessageWithResult(ctx context.Context, message IncomingMessage) (ChatIngestResult, error) {
	if ingestor.Store == nil {
		return ChatIngestResult{}, fmt.Errorf("incoming message store is not configured")
	}
	modelMessage := incomingModelMessage(message)
	if modelMessage.MessageID <= 0 && ingestor.NextMessageID != nil {
		modelMessage.MessageID = ingestor.NextMessageID()
		message.MessageID = modelMessage.MessageID
	}
	modelMessage = incomingmodel.NormalizeIncomingMessage(modelMessage, modelMessage.MessageID, modelMessage.Timestamp)
	isNew, conversation, err := ingestor.Store.AddIncomingMessage(ctx, modelMessage)
	if err != nil {
		return ChatIngestResult{}, err
	}
	return ChatIngestResult{
		IsNew:        isNew,
		Conversation: conversationSnapshot(conversation),
		Message:      normalizeOutgoingMessage(message, modelMessage),
	}, nil
}

func incomingModelMessage(message IncomingMessage) incomingmodel.IncomingMessage {
	return incomingmodel.IncomingMessage{
		TenantID:         message.TenantID,
		MessageID:        int64MessageID(message.MessageID),
		ArchiveMsgID:     message.ArchiveMsgID,
		ConversationID:   message.ConversationID,
		ConversationKey:  message.ConversationKey,
		AccountID:        message.AccountID,
		WeWorkUserID:     firstNonBlank(message.ChannelUserID, message.WeWorkUserID),
		ExternalUserID:   message.ExternalUserID,
		RoomID:           message.RoomID,
		ConversationType: message.ConversationType,
		DeviceID:         message.DeviceID,
		SenderID:         message.SenderID,
		SenderName:       message.SenderName,
		SenderAvatar:     message.SenderAvatar,
		SenderRemark:     message.SenderRemark,
		Content:          message.Content,
		MsgType:          message.MsgType,
		ConversationName: message.ConversationName,
		Timestamp:        message.Timestamp,
		TraceID:          message.TraceID,
		MessageOrigin:    message.MessageOrigin,
	}
}

func normalizeOutgoingMessage(message IncomingMessage, modelMessage incomingmodel.IncomingMessage) IncomingMessage {
	normalized := modelMessage
	if message.MessageID == nil && normalized.MessageID > 0 {
		message.MessageID = normalized.MessageID
	}
	message.TenantID = firstNonBlank(message.TenantID, normalized.TenantID)
	message.AccountID = firstNonBlank(message.AccountID, normalized.AccountID)
	message.ConversationID = firstNonBlank(message.ConversationID, normalized.ConversationID)
	message.ConversationKey = firstNonBlank(message.ConversationKey, normalized.ConversationKey)
	message.ChannelUserID = firstNonBlank(message.ChannelUserID, normalized.WeWorkUserID)
	message.WeWorkUserID = firstNonBlank(message.WeWorkUserID, normalized.WeWorkUserID)
	message.ExternalUserID = firstNonBlank(message.ExternalUserID, normalized.ExternalUserID)
	message.RoomID = firstNonBlank(message.RoomID, normalized.RoomID)
	message.ConversationType = firstNonBlank(message.ConversationType, normalized.ConversationType)
	message.MsgType = firstNonBlank(message.MsgType, normalized.MsgType)
	message.MessageOrigin = firstNonBlank(message.MessageOrigin, normalized.MessageOrigin)
	if message.Timestamp.IsZero() {
		message.Timestamp = normalized.Timestamp
	}
	return message
}

func conversationSnapshot(snapshot incomingmodel.ConversationSnapshot) ConversationSnapshot {
	firstMessageAt := timeOrZero(snapshot.FirstMessageAt)
	return ConversationSnapshot{
		ConversationID:   snapshot.ConversationID,
		ConversationKey:  snapshot.ConversationKey,
		AccountID:        snapshot.AccountID,
		ChannelUserID:    snapshot.WeWorkUserID,
		WeWorkUserID:     snapshot.WeWorkUserID,
		ExternalUserID:   snapshot.ExternalUserID,
		RoomID:           snapshot.RoomID,
		ConversationType: snapshot.ConversationType,
		SenderName:       snapshot.SenderName,
		SenderAvatar:     snapshot.SenderAvatar,
		SenderRemark:     snapshot.SenderRemark,
		ConversationName: snapshot.ConversationName,
		FirstMessageAt:   firstMessageAt,
	}
}

func int64MessageID(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func messageIDText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "<nil>" {
			return ""
		}
		return text
	}
}
