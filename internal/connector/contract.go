// Package connector defines channel-neutral IM connector contracts.
package connector

import "time"

const (
	ChannelInternalWebhook = "internal.webhook"

	MessageTypeText    = "text"
	MessageTypeImage   = "image"
	MessageTypeVideo   = "video"
	MessageTypeVoice   = "voice"
	MessageTypeFile    = "file"
	MessageTypeUnknown = "unknown"

	ReceiptAccepted  = "accepted"
	ReceiptSent      = "sent"
	ReceiptDelivered = "delivered"
	ReceiptRead      = "read"
	ReceiptFailed    = "failed"
	ReceiptRevoked   = "revoked"
)

// ContactIdentity is the external contact identity as seen by one connector.
type ContactIdentity struct {
	ContactID      string         `json:"contact_id,omitempty"`
	ExternalUserID string         `json:"external_user_id,omitempty"`
	DisplayName    string         `json:"display_name,omitempty"`
	AvatarURL      string         `json:"avatar_url,omitempty"`
	Remark         string         `json:"remark,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ConversationBinding links an external conversation to the local IM model.
type ConversationBinding struct {
	ConversationID         string         `json:"conversation_id,omitempty"`
	ConversationKey        string         `json:"conversation_key,omitempty"`
	ExternalConversationID string         `json:"external_conversation_id,omitempty"`
	RoomID                 string         `json:"room_id,omitempty"`
	Type                   string         `json:"type,omitempty"`
	DisplayName            string         `json:"display_name,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
}

// MediaAttachment is a connector-neutral media reference.
type MediaAttachment struct {
	AttachmentID string         `json:"attachment_id,omitempty"`
	Type         string         `json:"type,omitempty"`
	URL          string         `json:"url,omitempty"`
	ObjectKey    string         `json:"object_key,omitempty"`
	MIMEType     string         `json:"mime_type,omitempty"`
	Bytes        int64          `json:"bytes,omitempty"`
	SHA256       string         `json:"sha256,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// InboundEvent is the standard event shape a connector sends into IM core.
type InboundEvent struct {
	EventID        string              `json:"event_id"`
	TraceID        string              `json:"trace_id,omitempty"`
	ConnectorID    string              `json:"connector_id"`
	Channel        string              `json:"channel"`
	TenantID       string              `json:"tenant_id"`
	AccountID      string              `json:"account_id,omitempty"`
	ChannelUserID  string              `json:"channel_user_id,omitempty"`
	EndpointID     string              `json:"endpoint_id,omitempty"`
	Sender         ContactIdentity     `json:"sender"`
	Conversation   ConversationBinding `json:"conversation"`
	MessageID      any                 `json:"message_id,omitempty"`
	MessageType    string              `json:"message_type"`
	Content        string              `json:"content,omitempty"`
	Media          []MediaAttachment   `json:"media,omitempty"`
	OccurredAt     time.Time           `json:"occurred_at"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	Metadata       map[string]any      `json:"metadata,omitempty"`
}

// OutboundMessage is the standard send request shape consumed by connector workers.
type OutboundMessage struct {
	MessageID      string              `json:"message_id"`
	TraceID        string              `json:"trace_id,omitempty"`
	IdempotencyKey string              `json:"idempotency_key"`
	ConnectorID    string              `json:"connector_id"`
	Channel        string              `json:"channel"`
	TenantID       string              `json:"tenant_id"`
	AccountID      string              `json:"account_id,omitempty"`
	EndpointID     string              `json:"endpoint_id,omitempty"`
	Target         ContactIdentity     `json:"target"`
	Conversation   ConversationBinding `json:"conversation"`
	MessageType    string              `json:"message_type"`
	Content        string              `json:"content,omitempty"`
	Media          []MediaAttachment   `json:"media,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	Metadata       map[string]any      `json:"metadata,omitempty"`
}

// DeliveryReceipt reports an external delivery state back to IM core.
type DeliveryReceipt struct {
	ReceiptID          string         `json:"receipt_id"`
	TraceID            string         `json:"trace_id,omitempty"`
	ConnectorID        string         `json:"connector_id"`
	Channel            string         `json:"channel"`
	TenantID           string         `json:"tenant_id"`
	MessageID          string         `json:"message_id"`
	ConnectorMessageID string         `json:"connector_message_id,omitempty"`
	Status             string         `json:"status"`
	ErrorCode          string         `json:"error_code,omitempty"`
	ErrorMessage       string         `json:"error_message,omitempty"`
	OccurredAt         time.Time      `json:"occurred_at"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}
