package projectionupdate

import (
	"strings"
	"testing"
	"time"
)

func TestApplyMessageEventCreatesIncomingProjectionRow(t *testing.T) {
	messageAt := fixedTime(10)
	row := ApplyMessageEvent(Row{}, MessageEvent{
		ConversationID: " conv-1 ",
		TenantID:       " tenant-1 ",
		DeviceID:       " device-1 ",
		WeWorkUserID:   " wx-1 ",
		ExternalUserID: " ext-1 ",
		SenderID:       " customer-1 ",
		SenderName:     " Alice ",
		Content:        strings.Repeat("x", MaxLastContentLength+5),
		Timestamp:      messageAt,
		UpdatedAt:      fixedTime(11),
	})

	if row.ConversationID != "conv-1" || row.TenantID != "tenant-1" || row.DeviceID != "device-1" {
		t.Fatalf("identity fields were not normalized: %+v", row)
	}
	if row.ConversationType != DefaultConversationType || row.LastMsgType != DefaultMessageType {
		t.Fatalf("defaults not applied: conversation_type=%q msg_type=%q", row.ConversationType, row.LastMsgType)
	}
	if row.LastDirection != "incoming" || row.UnreadCount != 1 {
		t.Fatalf("incoming event should start unread state, got direction=%q unread=%d", row.LastDirection, row.UnreadCount)
	}
	if row.LastIncomingAt == nil || !row.LastIncomingAt.Equal(messageAt) {
		t.Fatalf("last_incoming_at should default to message timestamp, got %#v", row.LastIncomingAt)
	}
	if len(row.LastContent) != MaxLastContentLength {
		t.Fatalf("last content was not truncated to %d, got %d", MaxLastContentLength, len(row.LastContent))
	}
}

func TestApplyMessageEventKeepsNewestMessageFieldsButMergesIdentity(t *testing.T) {
	currentIncoming := fixedTime(20)
	current := Row{
		ConversationID:   "conv-1",
		TenantID:         "tenant-old",
		DeviceID:         "device-old",
		WeWorkUserID:     "wx-old",
		SenderID:         "sender-old",
		SenderName:       "Old Name",
		LastContent:      "newest content",
		LastMsgType:      "image",
		LastDirection:    "incoming",
		LastMessageAt:    fixedTime(20),
		LastIncomingAt:   &currentIncoming,
		UnreadCount:      4,
		ConversationName: "current conversation",
	}

	row := ApplyMessageEvent(current, MessageEvent{
		ConversationID:   "conv-1",
		TenantID:         "tenant-new",
		DeviceID:         "device-new",
		WeWorkUserID:     "wx-new",
		SenderID:         "sender-new",
		SenderName:       "New Name",
		ConversationName: "",
		Content:          "older content",
		MsgType:          "text",
		Direction:        "incoming",
		Timestamp:        fixedTime(19),
		UpdatedAt:        fixedTime(21),
	})

	if row.LastContent != "newest content" || row.LastMsgType != "image" || row.UnreadCount != 4 {
		t.Fatalf("older message should not replace latest fields or unread count: %+v", row)
	}
	if row.TenantID != "tenant-new" || row.DeviceID != "device-new" || row.WeWorkUserID != "wx-new" {
		t.Fatalf("identity columns should still merge independent of timestamp: %+v", row)
	}
	if row.SenderID != "sender-new" || row.SenderName != "New Name" {
		t.Fatalf("sender identity did not merge: %+v", row)
	}
	if row.ConversationName != "current conversation" {
		t.Fatalf("blank conversation name should keep current value, got %q", row.ConversationName)
	}
	if row.LastIncomingAt == nil || !row.LastIncomingAt.Equal(currentIncoming) {
		t.Fatalf("last_incoming_at regressed: %#v", row.LastIncomingAt)
	}
}

func TestApplyMessageEventUnreadRules(t *testing.T) {
	current := Row{
		ConversationID: "conv-1",
		LastContent:    "previous",
		LastDirection:  "incoming",
		LastMessageAt:  fixedTime(10),
		UnreadCount:    2,
	}

	incoming := ApplyMessageEvent(current, MessageEvent{
		ConversationID: "conv-1",
		Content:        "new incoming",
		Direction:      "incoming",
		Timestamp:      fixedTime(11),
		UpdatedAt:      fixedTime(11),
	})
	if incoming.UnreadCount != 3 || incoming.LastContent != "new incoming" {
		t.Fatalf("new incoming should increment unread and replace last content: %+v", incoming)
	}

	outgoing := ApplyMessageEvent(incoming, MessageEvent{
		ConversationID: "conv-1",
		Content:        "reply",
		Direction:      "outgoing",
		Timestamp:      fixedTime(12),
		UpdatedAt:      fixedTime(12),
	})
	if outgoing.UnreadCount != 0 || outgoing.LastDirection != "outgoing" || outgoing.LastContent != "reply" {
		t.Fatalf("new outgoing should clear unread and become latest message: %+v", outgoing)
	}

	olderOutgoing := ApplyMessageEvent(incoming, MessageEvent{
		ConversationID: "conv-1",
		Content:        "old reply",
		Direction:      "outgoing",
		Timestamp:      fixedTime(9),
		UpdatedAt:      fixedTime(13),
	})
	if olderOutgoing.UnreadCount != incoming.UnreadCount || olderOutgoing.LastContent != incoming.LastContent {
		t.Fatalf("older outgoing should not clear unread or replace last content: %+v", olderOutgoing)
	}
}

func TestApplyMessageEventExactUnreadOverridesDelta(t *testing.T) {
	exactUnread := 0
	current := Row{
		ConversationID: "conv-1",
		LastMessageAt:  fixedTime(10),
		UnreadCount:    9,
	}
	row := ApplyMessageEvent(current, MessageEvent{
		ConversationID: "conv-1",
		Direction:      "incoming",
		Timestamp:      fixedTime(11),
		UnreadCount:    &exactUnread,
		UpdatedAt:      fixedTime(11),
	})
	if row.UnreadCount != 0 {
		t.Fatalf("explicit unread_count should override delta, got %d", row.UnreadCount)
	}

	ignoreExact := -1
	row = ApplyMessageEvent(current, MessageEvent{
		ConversationID: "conv-1",
		Direction:      "incoming",
		Timestamp:      fixedTime(11),
		UnreadCount:    &ignoreExact,
		UpdatedAt:      fixedTime(11),
	})
	if row.UnreadCount != 10 {
		t.Fatalf("negative unread hint should fall back to delta, got %d", row.UnreadCount)
	}
}

func TestNormalizeLastContentCountsRunes(t *testing.T) {
	content := strings.Repeat("界", MaxLastContentLength+2)
	normalized := NormalizeLastContent(content)
	if len([]rune(normalized)) != MaxLastContentLength {
		t.Fatalf("normalized content should keep %d runes, got %d", MaxLastContentLength, len([]rune(normalized)))
	}
	if !strings.HasSuffix(normalized, "界") {
		t.Fatalf("normalized content should not cut a UTF-8 rune: %q", normalized)
	}
}

func TestApplyAssignmentCreatesAndUpdatesAssignee(t *testing.T) {
	assignedAt := fixedTime(20)
	row := ApplyAssignment(Row{}, Assignment{
		ConversationID: " conv-1 ",
		TenantID:       " tenant-1 ",
		AssigneeID:     " cs-1 ",
		AssigneeName:   " Agent One ",
		UpdatedAt:      assignedAt,
	})
	if row.ConversationID != "conv-1" || row.TenantID != "tenant-1" {
		t.Fatalf("assignment insert did not normalize identity: %+v", row)
	}
	if row.LastMsgType != DefaultMessageType || row.LastDirection != DefaultDirection || !row.LastMessageAt.Equal(assignedAt) {
		t.Fatalf("assignment insert defaults not aligned with legacy projection: %+v", row)
	}

	row = ApplyAssignment(row, Assignment{
		ConversationID: "conv-1",
		TenantID:       "",
		AssigneeID:     "cs-2",
		AssigneeName:   "Agent Two",
		UpdatedAt:      fixedTime(21),
	})
	if row.TenantID != "tenant-1" || row.AssigneeID != "cs-2" || row.AssigneeName != "Agent Two" {
		t.Fatalf("assignment update did not preserve tenant or replace assignee: %+v", row)
	}
}

func TestApplyIdentityUpdateReplacesDisplayFields(t *testing.T) {
	row := ApplyIdentityUpdate(Row{
		ConversationID: "conv-1",
		CustomerName:   "Old Customer",
		SenderName:     "Old Nick",
		SenderRemark:   "Old Remark",
		SenderAvatar:   "old.png",
	}, IdentityUpdate{
		EnterpriseID: " ent-1 ",
		SenderID:     " wm-1 ",
		DisplayName:  " Display ",
		RemarkName:   " Remark ",
		Nickname:     " Nick ",
		AvatarURL:    " avatar.png ",
		WeWorkUserID: " user-1 ",
		UpdatedAt:    fixedTime(22),
	})
	if row.CustomerName != "Display" || row.SenderName != "Nick" || row.SenderRemark != "Remark" || row.SenderAvatar != "avatar.png" {
		t.Fatalf("identity fields = %+v", row)
	}
	if !row.UpdatedAt.Equal(fixedTime(22)) {
		t.Fatalf("updated_at = %s", row.UpdatedAt)
	}
}

func TestMarkReadAndOutgoingReplyState(t *testing.T) {
	current := Row{
		ConversationID: "conv-1",
		LastDirection:  "incoming",
		LastMessageAt:  fixedTime(10),
		UnreadCount:    5,
	}

	read := MarkRead(current, fixedTime(20))
	if read.UnreadCount != 0 || read.LastDirection != "incoming" {
		t.Fatalf("mark read should only clear unread and update timestamp: %+v", read)
	}

	replyState := ApplyOutgoingReplyState(current, fixedTime(21))
	if replyState.UnreadCount != 0 || replyState.LastDirection != "outgoing" || !replyState.LastMessageAt.Equal(current.LastMessageAt) {
		t.Fatalf("outgoing reply state should clear unread and preserve last message time: %+v", replyState)
	}
}

func fixedTime(minute int) time.Time {
	return time.Date(2026, 6, 30, 10, minute, 0, 0, time.UTC)
}
