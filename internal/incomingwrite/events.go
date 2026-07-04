// Package incomingwrite builds outbox events for incoming message writes.
package incomingwrite

import (
	"strings"
	"time"

	"im-go/internal/outbox"
)

const (
	EventArchiveSyncRequested       = "archive.sync.requested"
	EventConversationMessage        = "conversation.message.received"
	EventConversationAutoReply      = "conversation.auto_reply.requested"
	DefaultIngestSource             = "device_realtime"
	DefaultCanonicalSource          = "device_primary"
	DefaultArchiveSyncSource        = "self_decrypt"
	DefaultArchiveSyncTriggerReason = "device_message_received"
)

// IncomingMessage is the normalized request shape needed for outbox events.
type IncomingMessage struct {
	TraceID          string
	MessageID        any
	TenantID         string
	ArchiveMsgID     string
	ConversationID   string
	ConversationKey  string
	AccountID        string
	ChannelUserID    string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	DeviceID         string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	Content          string
	MsgType          string
	ConversationName string
	Timestamp        time.Time
	MessageOrigin    string
}

// ConversationSnapshot is the post-write conversation shape needed by the event builder.
type ConversationSnapshot struct {
	ConversationID   string
	ConversationKey  string
	AccountID        string
	ChannelUserID    string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
	FirstMessageAt   time.Time
}

// BuildOptions controls incoming outbox event construction.
type BuildOptions struct {
	TenantID              string
	PublishEventOverride  string
	AutoReplyWhen         string
	IngestSource          string
	CanonicalSource       string
	ReconciledFromArchive bool
	IsNew                 bool
	EffectiveAIAutoReply  bool
}

// BuildResult contains generated outbox events and the auto-reply decision.
type BuildResult struct {
	Events          []outbox.EventEnvelope
	AutoReplyQueued bool
	RealtimeEvent   string
}

// BuildIncomingEvents mirrors IncomingMessageWriteService outbox envelope construction.
func BuildIncomingEvents(message IncomingMessage, conversation ConversationSnapshot, options BuildOptions) BuildResult {
	timestamp := message.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	tenantID := defaultText(options.TenantID, message.TenantID)
	realtimeEvent := defaultText(options.PublishEventOverride, chooseRealtimeEvent(options.IsNew))
	conversationID := defaultText(conversation.ConversationID, message.ConversationID)
	conversationKey := defaultText(conversation.ConversationKey, conversationID)
	channelUserID := firstNonBlank(conversation.ChannelUserID, message.ChannelUserID, conversation.WeWorkUserID, message.WeWorkUserID)
	partitionKey := strings.TrimSpace(message.DeviceID) + ":" + strings.TrimSpace(message.SenderID)
	events := []outbox.EventEnvelope{
		{
			EventID:       strings.TrimSpace(message.TraceID) + ":realtime",
			EventType:     EventConversationMessage,
			AggregateType: "conversation",
			AggregateID:   conversationID,
			TenantID:      tenantID,
			PartitionKey:  partitionKey,
			TraceID:       strings.TrimSpace(message.TraceID),
			Payload: map[string]any{
				"message_id":               message.MessageID,
				"conversation_id":          conversationID,
				"conversation_key":         conversationKey,
				"resolved_conversation_id": conversationKey,
				"tenant_id":                tenantID,
				"trace_id":                 strings.TrimSpace(message.TraceID),
				"channel_user_id":          channelUserID,
				"wework_user_id":           channelUserID,
				"external_userid":          conversation.ExternalUserID,
				"room_id":                  conversation.RoomID,
				"conversation_type":        conversation.ConversationType,
				"sender_id":                strings.TrimSpace(message.SenderID),
				"sender_name":              conversation.SenderName,
				"sender_avatar":            conversation.SenderAvatar,
				"sender_remark":            conversation.SenderRemark,
				"conversation_name":        conversation.ConversationName,
				"content":                  message.Content,
				"msg_type":                 defaultText(message.MsgType, "text"),
				"direction":                "incoming",
				"device_id":                strings.TrimSpace(message.DeviceID),
				"timestamp":                timestamp.UTC().Format(time.RFC3339Nano),
				"created_at":               timestamp.UTC().Format(time.RFC3339Nano),
				"publish_event":            realtimeEvent,
				"ingest_source":            defaultText(options.IngestSource, DefaultIngestSource),
				"canonical_source":         defaultText(options.CanonicalSource, DefaultCanonicalSource),
				"reconciled_from_archive":  bool(options.ReconciledFromArchive),
			},
			OccurredAt:  timestamp,
			AvailableAt: timestamp,
		},
	}
	autoReplyQueued := shouldQueueAutoReply(conversation, options)
	if autoReplyQueued {
		events = append(events, outbox.EventEnvelope{
			EventID:       strings.TrimSpace(message.TraceID) + ":auto-reply",
			EventType:     EventConversationAutoReply,
			AggregateType: "conversation",
			AggregateID:   conversationID,
			TenantID:      tenantID,
			PartitionKey:  partitionKey,
			TraceID:       strings.TrimSpace(message.TraceID),
			Payload: map[string]any{
				"conversation_id":   conversationID,
				"tenant_id":         tenantID,
				"device_id":         strings.TrimSpace(message.DeviceID),
				"sender_id":         strings.TrimSpace(message.SenderID),
				"sender_name":       conversation.SenderName,
				"conversation_name": conversation.ConversationName,
				"content":           message.Content,
				"first_message_at":  timeOrNil(conversation.FirstMessageAt),
				"trigger_event":     "incoming_message",
			},
			OccurredAt:  timestamp,
			AvailableAt: timestamp.Add(time.Millisecond),
		})
	}
	return BuildResult{Events: events, AutoReplyQueued: autoReplyQueued, RealtimeEvent: realtimeEvent}
}

// BuildArchiveSyncSignal mirrors queue_archive_sync_signal.
func BuildArchiveSyncSignal(input ArchiveSyncSignal) outbox.EventEnvelope {
	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	enterpriseID := defaultText(input.EnterpriseID, "default")
	source := defaultText(input.Source, DefaultArchiveSyncSource)
	traceID := strings.TrimSpace(input.TraceID)
	return outbox.EventEnvelope{
		EventID:       "archive-sync:" + enterpriseID + ":" + source + ":" + traceID,
		EventType:     EventArchiveSyncRequested,
		AggregateType: "archive_sync",
		AggregateID:   enterpriseID + ":" + source,
		TenantID:      enterpriseID,
		PartitionKey:  enterpriseID + ":" + source,
		TraceID:       traceID,
		Payload: map[string]any{
			"enterprise_id":  enterpriseID,
			"source":         source,
			"device_id":      defaultText(input.DeviceID, "unknown_device"),
			"sender_id":      defaultText(input.SenderID, "unknown_sender"),
			"trigger_reason": defaultText(input.TriggerReason, DefaultArchiveSyncTriggerReason),
		},
		OccurredAt:  occurredAt,
		AvailableAt: occurredAt.Add(time.Second),
	}
}

// ArchiveSyncSignal is the input for archive.sync.requested events.
type ArchiveSyncSignal struct {
	EnterpriseID  string
	Source        string
	TraceID       string
	DeviceID      string
	SenderID      string
	OccurredAt    time.Time
	TriggerReason string
}

func shouldQueueAutoReply(conversation ConversationSnapshot, options BuildOptions) bool {
	if strings.TrimSpace(conversation.ConversationID) == "" || strings.TrimSpace(conversation.AccountID) == "" {
		return false
	}
	if !options.EffectiveAIAutoReply {
		return false
	}
	autoReplyWhen := defaultText(options.AutoReplyWhen, "always")
	return autoReplyWhen == "always" || (autoReplyWhen == "new_only" && options.IsNew)
}

func chooseRealtimeEvent(isNew bool) string {
	if isNew {
		return "conversation.incoming"
	}
	return "conversation.message"
}

func timeOrNil(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}
