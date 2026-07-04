// Package projectionupdate contains pure conversation_overview_projection state rules.
package projectionupdate

import (
	"strings"
	"time"
)

const (
	DefaultConversationType = "single"
	DefaultMessageType      = "text"
	DefaultDirection        = "incoming"
	MaxLastContentLength    = 255
)

// Row mirrors the mutable columns controlled by projection write events.
type Row struct {
	ConversationID   string
	TenantID         string
	DeviceID         string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	SenderID         string
	SenderName       string
	SenderRemark     string
	SenderAvatar     string
	CustomerName     string
	ConversationName string
	LastContent      string
	LastMsgType      string
	IsSystemEvent    bool
	LastDirection    string
	LastMessageAt    time.Time
	LastIncomingAt   *time.Time
	UnreadCount      int
	AIAutoReply      bool
	AssigneeID       string
	AssigneeName     string
	UpdatedAt        time.Time
}

// MessageEvent is the normalized input for one projection message upsert.
type MessageEvent struct {
	ConversationID   string
	TenantID         string
	DeviceID         string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	SenderID         string
	SenderName       string
	SenderRemark     string
	SenderAvatar     string
	CustomerName     string
	ConversationName string
	Content          string
	MsgType          string
	Direction        string
	IsSystemEvent    bool
	Timestamp        time.Time
	LastIncomingAt   *time.Time
	UnreadCount      *int
	UpdatedAt        time.Time
}

// Assignment is the normalized input for one assignment projection upsert.
type Assignment struct {
	ConversationID string
	TenantID       string
	AssigneeID     string
	AssigneeName   string
	UpdatedAt      time.Time
}

// IdentityUpdate is the normalized input for one contact profile projection update.
type IdentityUpdate struct {
	EnterpriseID string
	SenderID     string
	DisplayName  string
	RemarkName   string
	Nickname     string
	AvatarURL    string
	WeWorkUserID string
	UpdatedAt    time.Time
}

// ApplyMessageEvent applies the legacy projection upsert conflict rules.
func ApplyMessageEvent(current Row, event MessageEvent) Row {
	event = NormalizeMessageEvent(event)
	if strings.TrimSpace(current.ConversationID) == "" {
		return rowFromMessageEvent(event)
	}

	previousLastMessageAt := current.LastMessageAt
	next := current
	next.ConversationID = defaultText(event.ConversationID, current.ConversationID)
	next.TenantID = keepIfBlank(event.TenantID, current.TenantID)
	next.DeviceID = event.DeviceID
	next.WeWorkUserID = keepIfBlank(event.WeWorkUserID, current.WeWorkUserID)
	next.ExternalUserID = keepIfBlank(event.ExternalUserID, current.ExternalUserID)
	next.RoomID = keepIfBlank(event.RoomID, current.RoomID)
	next.ConversationType = keepIfBlank(event.ConversationType, current.ConversationType)
	next.SenderID = event.SenderID
	next.SenderName = keepIfBlank(event.SenderName, current.SenderName)
	next.SenderRemark = keepIfBlank(event.SenderRemark, current.SenderRemark)
	next.SenderAvatar = keepIfBlank(event.SenderAvatar, current.SenderAvatar)
	next.CustomerName = keepIfBlank(event.CustomerName, current.CustomerName)
	next.ConversationName = keepIfBlank(event.ConversationName, current.ConversationName)
	if messageAtLeastCurrent(event.Timestamp, previousLastMessageAt) {
		next.LastContent = NormalizeLastContent(event.Content)
		next.LastMsgType = event.MsgType
		next.IsSystemEvent = event.IsSystemEvent
		next.LastDirection = event.Direction
	}
	next.LastMessageAt = maxTime(previousLastMessageAt, event.Timestamp)
	next.LastIncomingAt = maxOptionalTime(current.LastIncomingAt, eventIncomingMarker(event))
	next.UnreadCount = applyUnreadCount(current.UnreadCount, previousLastMessageAt, event)
	next.UpdatedAt = event.UpdatedAt
	return next
}

// ApplyAssignment applies the legacy assignment upsert conflict rules.
func ApplyAssignment(current Row, assignment Assignment) Row {
	assignment = NormalizeAssignment(assignment)
	if strings.TrimSpace(current.ConversationID) == "" {
		return Row{
			ConversationID:   assignment.ConversationID,
			TenantID:         assignment.TenantID,
			ConversationType: DefaultConversationType,
			LastMsgType:      DefaultMessageType,
			LastDirection:    DefaultDirection,
			LastMessageAt:    assignment.UpdatedAt,
			AssigneeID:       assignment.AssigneeID,
			AssigneeName:     assignment.AssigneeName,
			UpdatedAt:        assignment.UpdatedAt,
		}
	}
	next := current
	next.ConversationID = defaultText(assignment.ConversationID, current.ConversationID)
	next.TenantID = keepIfBlank(assignment.TenantID, current.TenantID)
	next.AssigneeID = assignment.AssigneeID
	next.AssigneeName = assignment.AssigneeName
	next.UpdatedAt = assignment.UpdatedAt
	return next
}

// ApplyIdentityUpdate mirrors projection update_identity display-field rules.
func ApplyIdentityUpdate(current Row, update IdentityUpdate) Row {
	update = NormalizeIdentityUpdate(update)
	next := current
	if customerName := firstNonBlank(update.DisplayName, update.RemarkName, update.Nickname); customerName != "" {
		next.CustomerName = customerName
	}
	if senderName := firstNonBlank(update.Nickname, update.DisplayName, update.RemarkName); senderName != "" {
		next.SenderName = senderName
	}
	if senderRemark := firstNonBlank(update.RemarkName, update.DisplayName); senderRemark != "" {
		next.SenderRemark = senderRemark
	}
	if update.AvatarURL != "" {
		next.SenderAvatar = update.AvatarURL
	}
	next.UpdatedAt = update.UpdatedAt
	return next
}

// MarkRead mirrors projection mark_read by clearing unread_count only.
func MarkRead(current Row, updatedAt time.Time) Row {
	next := current
	next.UnreadCount = 0
	next.UpdatedAt = updatedAt
	return next
}

// ApplyOutgoingReplyState mirrors update_reply_state_on_outgoing.
func ApplyOutgoingReplyState(current Row, updatedAt time.Time) Row {
	next := current
	next.LastDirection = "outgoing"
	next.UnreadCount = 0
	next.UpdatedAt = updatedAt
	return next
}

// NormalizeLastContent keeps conversation list previews inside the legacy column budget.
func NormalizeLastContent(value string) string {
	runes := []rune(value)
	if len(runes) <= MaxLastContentLength {
		return value
	}
	return string(runes[:MaxLastContentLength])
}

func rowFromMessageEvent(event MessageEvent) Row {
	initialUnreadCount := 0
	if event.UnreadCount != nil {
		initialUnreadCount = *event.UnreadCount
	} else if event.Direction == "incoming" {
		initialUnreadCount = 1
	}
	return Row{
		ConversationID:   event.ConversationID,
		TenantID:         event.TenantID,
		DeviceID:         event.DeviceID,
		WeWorkUserID:     event.WeWorkUserID,
		ExternalUserID:   event.ExternalUserID,
		RoomID:           event.RoomID,
		ConversationType: event.ConversationType,
		SenderID:         event.SenderID,
		SenderName:       event.SenderName,
		SenderRemark:     event.SenderRemark,
		SenderAvatar:     event.SenderAvatar,
		CustomerName:     event.CustomerName,
		ConversationName: event.ConversationName,
		LastContent:      NormalizeLastContent(event.Content),
		LastMsgType:      event.MsgType,
		IsSystemEvent:    event.IsSystemEvent,
		LastDirection:    event.Direction,
		LastMessageAt:    event.Timestamp,
		LastIncomingAt:   eventIncomingMarker(event),
		UnreadCount:      initialUnreadCount,
		UpdatedAt:        event.UpdatedAt,
	}
}

// NormalizeMessageEvent trims legacy text fields and applies insert defaults.
func NormalizeMessageEvent(event MessageEvent) MessageEvent {
	event.ConversationID = strings.TrimSpace(event.ConversationID)
	event.TenantID = strings.TrimSpace(event.TenantID)
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.WeWorkUserID = strings.TrimSpace(event.WeWorkUserID)
	event.ExternalUserID = strings.TrimSpace(event.ExternalUserID)
	event.RoomID = strings.TrimSpace(event.RoomID)
	event.ConversationType = defaultText(event.ConversationType, DefaultConversationType)
	event.SenderID = strings.TrimSpace(event.SenderID)
	event.SenderName = strings.TrimSpace(event.SenderName)
	event.SenderRemark = strings.TrimSpace(event.SenderRemark)
	event.SenderAvatar = strings.TrimSpace(event.SenderAvatar)
	event.CustomerName = strings.TrimSpace(event.CustomerName)
	event.ConversationName = strings.TrimSpace(event.ConversationName)
	event.MsgType = defaultText(event.MsgType, DefaultMessageType)
	event.Direction = defaultText(strings.ToLower(event.Direction), DefaultDirection)
	if event.Timestamp.IsZero() {
		event.Timestamp = event.UpdatedAt
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = event.Timestamp
	}
	return event
}

// NormalizeAssignment trims assignment identity fields.
func NormalizeAssignment(assignment Assignment) Assignment {
	assignment.ConversationID = strings.TrimSpace(assignment.ConversationID)
	assignment.TenantID = strings.TrimSpace(assignment.TenantID)
	assignment.AssigneeID = strings.TrimSpace(assignment.AssigneeID)
	assignment.AssigneeName = strings.TrimSpace(assignment.AssigneeName)
	return assignment
}

// NormalizeIdentityUpdate trims contact profile update fields.
func NormalizeIdentityUpdate(update IdentityUpdate) IdentityUpdate {
	update.EnterpriseID = strings.TrimSpace(update.EnterpriseID)
	update.SenderID = strings.TrimSpace(update.SenderID)
	update.DisplayName = strings.TrimSpace(update.DisplayName)
	update.RemarkName = strings.TrimSpace(update.RemarkName)
	update.Nickname = strings.TrimSpace(update.Nickname)
	update.AvatarURL = strings.TrimSpace(update.AvatarURL)
	update.WeWorkUserID = strings.TrimSpace(update.WeWorkUserID)
	return update
}

func applyUnreadCount(currentUnread int, currentLastMessageAt time.Time, event MessageEvent) int {
	if event.UnreadCount != nil && *event.UnreadCount >= 0 {
		return *event.UnreadCount
	}
	if !messageAtLeastCurrent(event.Timestamp, currentLastMessageAt) {
		return currentUnread
	}
	switch event.Direction {
	case "outgoing":
		return 0
	case "incoming":
		return currentUnread + 1
	default:
		return currentUnread
	}
}

func eventIncomingMarker(event MessageEvent) *time.Time {
	if event.LastIncomingAt != nil {
		value := *event.LastIncomingAt
		return &value
	}
	if event.Direction != "incoming" {
		return nil
	}
	value := event.Timestamp
	return &value
}

func maxOptionalTime(current *time.Time, incoming *time.Time) *time.Time {
	if incoming == nil {
		return cloneTime(current)
	}
	if current == nil {
		return cloneTime(incoming)
	}
	if incoming.After(*current) {
		return cloneTime(incoming)
	}
	return cloneTime(current)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func maxTime(left time.Time, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func messageAtLeastCurrent(messageAt time.Time, current time.Time) bool {
	return current.IsZero() || !messageAt.Before(current)
}

func keepIfBlank(value string, current string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return current
	}
	return value
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
