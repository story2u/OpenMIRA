// Package incomingmodel contains pure incoming message write-model rules.
package incomingmodel

import (
	"regexp"
	"strings"
	"time"
)

const (
	DirectionIncoming       = "incoming"
	DirectionOutgoing       = "outgoing"
	OriginDeviceRealtime    = "device_realtime"
	DefaultMessageType      = "text"
	DefaultConversationType = "single"
	DefaultAIModeOverride   = "inherit"
	DefaultSOPRuntimeState  = "{}"
)

var (
	externalIDPattern    = regexp.MustCompile(`^(wm|wo|woan|wman)[A-Za-z0-9_-]{10,}$`)
	externalAliasPattern = regexp.MustCompile(`^external_userid_[A-Za-z0-9_-]+$`)
	shortAgentPattern    = regexp.MustCompile(`^[A-Za-z]{1,8}-?\d{3,8}$`)
)

// IncomingMessage is the normalized request shape for add_incoming_message.
type IncomingMessage struct {
	TenantID         string
	MessageID        int64
	ArchiveMsgID     string
	ConversationID   string
	ConversationKey  string
	AccountID        string
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
	TraceID          string
	MessageOrigin    string
	Direction        string
	TaskID           string
	SendStatus       string
	SendError        string
}

// ConversationSnapshot is the current conversations row needed by the write model.
type ConversationSnapshot struct {
	ConversationPK   *int64
	ConversationID   string
	ConversationKey  string
	TenantID         string
	AccountID        string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	DeviceID         string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
	FirstMessageAt   *time.Time
	LastIncomingAt   *time.Time
	LastOutgoingAt   *time.Time
	UnreadCount      int
	AIAutoReply      bool
	AIModeOverride   string
	SOPRuntimeState  string
}

// MessageRow is the messages row produced by an incoming write.
type MessageRow struct {
	MessageID        int64
	ConversationPK   *int64
	TenantID         string
	TraceID          string
	ArchiveMsgID     string
	ConversationID   string
	ConversationKey  string
	AccountID        string
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
	Direction        string
	MessageOrigin    string
	TaskID           string
	SendStatus       string
	SendError        string
	Timestamp        time.Time
	CreatedAt        time.Time
}

// ConversationRow is the conversations row produced by an incoming write.
type ConversationRow struct {
	ConversationPK   *int64
	ConversationID   string
	ConversationKey  string
	TenantID         string
	AccountID        string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	DeviceID         string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
	FirstMessageAt   time.Time
	LastContent      string
	LastMsgType      string
	LastMessageAt    time.Time
	LastIncomingAt   *time.Time
	LastOutgoingAt   *time.Time
	UnreadCount      int
	AIAutoReply      bool
	AIModeOverride   string
	SOPRuntimeState  string
	UpdatedAt        time.Time
}

// WritePlan contains the rows needed by a durable incoming message write.
type WritePlan struct {
	IsNewConversation bool
	Message           MessageRow
	Conversation      ConversationRow
}

// PrepareIncoming builds the message and conversation rows for a non-duplicate incoming message.
func PrepareIncoming(message IncomingMessage, current *ConversationSnapshot, generatedMessageID int64, now time.Time) WritePlan {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	message = NormalizeIncomingMessage(message, generatedMessageID, now)
	var conversationPK *int64
	var firstMessageAt time.Time
	accountID := message.AccountID
	aiAutoReply := false
	aiModeOverride := DefaultAIModeOverride
	sopRuntimeState := DefaultSOPRuntimeState
	unreadCount := 0
	senderName := message.SenderName
	var lastIncomingAt *time.Time
	var lastOutgoingAt *time.Time
	if message.Direction == DirectionIncoming {
		value := message.Timestamp
		lastIncomingAt = &value
		unreadCount = 1
	} else {
		value := message.Timestamp
		lastOutgoingAt = &value
	}
	isNew := current == nil || strings.TrimSpace(current.ConversationID) == ""
	if current != nil {
		conversationPK = cloneInt64(current.ConversationPK)
		if accountID == "" {
			accountID = strings.TrimSpace(current.AccountID)
		}
		aiAutoReply = current.AIAutoReply
		aiModeOverride = defaultText(current.AIModeOverride, DefaultAIModeOverride)
		sopRuntimeState = defaultText(current.SOPRuntimeState, DefaultSOPRuntimeState)
		unreadCount = current.UnreadCount
		if message.Direction == DirectionIncoming {
			unreadCount = current.UnreadCount + 1
		}
		senderName = ResolveSenderNameForUpsert(message.SenderName, current.SenderName)
		if current.FirstMessageAt != nil {
			firstMessageAt = *current.FirstMessageAt
		}
		if current.LastIncomingAt != nil {
			if lastIncomingAt == nil || current.LastIncomingAt.After(*lastIncomingAt) {
				lastIncomingAt = cloneTime(current.LastIncomingAt)
			}
		}
		if current.LastOutgoingAt != nil {
			if lastOutgoingAt == nil || current.LastOutgoingAt.After(*lastOutgoingAt) {
				lastOutgoingAt = cloneTime(current.LastOutgoingAt)
			}
		}
	}
	if firstMessageAt.IsZero() {
		firstMessageAt = message.Timestamp
	}
	messageRow := MessageRow{
		MessageID:        message.MessageID,
		ConversationPK:   cloneInt64(conversationPK),
		TenantID:         message.TenantID,
		TraceID:          message.TraceID,
		ArchiveMsgID:     message.ArchiveMsgID,
		ConversationID:   message.ConversationID,
		ConversationKey:  message.ConversationKey,
		AccountID:        accountID,
		WeWorkUserID:     message.WeWorkUserID,
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
		Direction:        message.Direction,
		MessageOrigin:    message.MessageOrigin,
		TaskID:           message.TaskID,
		SendStatus:       message.SendStatus,
		SendError:        message.SendError,
		Timestamp:        message.Timestamp,
		CreatedAt:        message.Timestamp,
	}
	return WritePlan{
		IsNewConversation: isNew,
		Message:           messageRow,
		Conversation: ConversationRow{
			ConversationPK:   cloneInt64(conversationPK),
			ConversationID:   message.ConversationID,
			ConversationKey:  message.ConversationKey,
			TenantID:         message.TenantID,
			AccountID:        accountID,
			WeWorkUserID:     message.WeWorkUserID,
			ExternalUserID:   message.ExternalUserID,
			RoomID:           message.RoomID,
			ConversationType: message.ConversationType,
			DeviceID:         message.DeviceID,
			SenderID:         message.SenderID,
			SenderName:       senderName,
			SenderAvatar:     message.SenderAvatar,
			SenderRemark:     message.SenderRemark,
			ConversationName: message.ConversationName,
			FirstMessageAt:   firstMessageAt,
			LastContent:      message.Content,
			LastMsgType:      message.MsgType,
			LastMessageAt:    message.Timestamp,
			LastIncomingAt:   lastIncomingAt,
			LastOutgoingAt:   lastOutgoingAt,
			UnreadCount:      unreadCount,
			AIAutoReply:      aiAutoReply,
			AIModeOverride:   aiModeOverride,
			SOPRuntimeState:  sopRuntimeState,
			UpdatedAt:        now.UTC(),
		},
	}
}

// NormalizeIncomingMessage applies ChatService incoming defaults before storage.
func NormalizeIncomingMessage(message IncomingMessage, generatedMessageID int64, now time.Time) IncomingMessage {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	message.TenantID = strings.TrimSpace(message.TenantID)
	message.ArchiveMsgID = strings.TrimSpace(message.ArchiveMsgID)
	message.ConversationID = strings.TrimSpace(message.ConversationID)
	message.ConversationKey = strings.TrimSpace(message.ConversationKey)
	message.AccountID = strings.TrimSpace(message.AccountID)
	message.WeWorkUserID = normalizeWeWorkUserID(message.WeWorkUserID)
	message.ExternalUserID = normalizeExternalUserID(firstNonBlank(message.ExternalUserID, message.SenderID))
	message.RoomID = normalizeRoomID(message.RoomID)
	message.ConversationType = defaultText(message.ConversationType, conversationTypeForRoom(message.RoomID))
	message.DeviceID = strings.TrimSpace(message.DeviceID)
	message.SenderID = strings.TrimSpace(message.SenderID)
	message.SenderName = strings.TrimSpace(message.SenderName)
	message.SenderAvatar = strings.TrimSpace(message.SenderAvatar)
	message.SenderRemark = strings.TrimSpace(message.SenderRemark)
	message.MsgType = defaultText(message.MsgType, DefaultMessageType)
	message.ConversationName = strings.TrimSpace(message.ConversationName)
	message.TraceID = strings.TrimSpace(message.TraceID)
	message.MessageOrigin = defaultText(message.MessageOrigin, OriginDeviceRealtime)
	message.Direction = normalizeDirection(message.Direction)
	message.TaskID = strings.TrimSpace(message.TaskID)
	message.SendStatus = strings.ToLower(strings.TrimSpace(message.SendStatus))
	message.SendError = strings.TrimSpace(message.SendError)
	if message.Timestamp.IsZero() {
		message.Timestamp = now.UTC()
	} else {
		message.Timestamp = message.Timestamp.UTC()
	}
	if message.MessageID <= 0 {
		message.MessageID = generatedMessageID
	}
	if message.ConversationID == "" {
		message.ConversationID = BuildConversationID(message)
	}
	if message.ConversationKey == "" {
		message.ConversationKey = message.ConversationID
	}
	return message
}

func normalizeDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DirectionOutgoing:
		return DirectionOutgoing
	default:
		return DirectionIncoming
	}
}

// BuildConversationID mirrors ChatService + repository stable identity preference.
func BuildConversationID(message IncomingMessage) string {
	weworkUserID := normalizeWeWorkUserID(message.WeWorkUserID)
	externalUserID := normalizeExternalUserID(firstNonBlank(message.ExternalUserID, message.SenderID))
	roomID := normalizeRoomID(message.RoomID)
	if weworkUserID != "" {
		if roomID != "" {
			return "ww:" + weworkUserID + ":room:" + roomID
		}
		if externalUserID != "" {
			return "ww:" + weworkUserID + ":" + externalUserID
		}
	}
	safeName := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(message.ConversationName)), " ", "_")
	if externalUserID != "" {
		if safeName == "" {
			safeName = externalUserID
		}
		return "pending:" + externalUserID + ":" + safeName
	}
	senderID := strings.ToLower(strings.TrimSpace(message.SenderID))
	if senderID != "" {
		return "pending:" + senderID
	}
	deviceID := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(message.DeviceID)), " ", "_")
	if deviceID == "" {
		deviceID = "unknown"
	}
	if safeName == "" {
		safeName = "unknown"
	}
	return "pending:" + deviceID + ":" + safeName
}

// ShouldInsertMessage mirrors trace/archive duplicate checks before insert.
func ShouldInsertMessage(message IncomingMessage, existingTrace bool, existingArchiveMsgID bool) bool {
	if strings.TrimSpace(message.ArchiveMsgID) != "" && existingArchiveMsgID {
		return false
	}
	return !existingTrace
}

// ResolveSenderNameForUpsert avoids replacing a good nickname with a low-quality identifier.
func ResolveSenderNameForUpsert(incoming string, current string) string {
	incoming = strings.TrimSpace(incoming)
	current = strings.TrimSpace(current)
	if IsLowQualitySenderName(incoming) && current != "" && !IsLowQualitySenderName(current) {
		return current
	}
	return incoming
}

// IsLowQualitySenderName mirrors the legacy nickname quality guard.
func IsLowQualitySenderName(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return true
	}
	switch text {
	case "企微客户", "企微用户", "未知客户", "unknown_sender", "-", "--", "—", "——":
		return true
	}
	if externalIDPattern.MatchString(text) || externalAliasPattern.MatchString(text) {
		return true
	}
	if shortAgentPattern.MatchString(text) && !containsCJK(text) {
		return true
	}
	return false
}

func conversationTypeForRoom(roomID string) string {
	if strings.TrimSpace(roomID) != "" {
		return "room"
	}
	return DefaultConversationType
}

func normalizeExternalUserID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRoomID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeWeWorkUserID(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "")
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
