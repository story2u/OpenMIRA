package incomingmodel

import (
	"testing"
	"time"
)

func TestBuildConversationIDUsesStableWeWorkIdentity(t *testing.T) {
	message := IncomingMessage{WeWorkUserID: "WX-USER", ExternalUserID: "WM-External", SenderID: "ignored"}
	if got := BuildConversationID(message); got != "ww:wxuser:wm-external" {
		t.Fatalf("conversation id = %q", got)
	}
	message = IncomingMessage{WeWorkUserID: "wx-user", RoomID: "ROOM-1", ExternalUserID: "wm-external"}
	if got := BuildConversationID(message); got != "ww:wxuser:room:room-1" {
		t.Fatalf("room conversation id = %q", got)
	}
}

func TestBuildConversationIDFallsBackToPendingIdentity(t *testing.T) {
	message := IncomingMessage{ExternalUserID: "External-1", ConversationName: "VIP Alice"}
	if got := BuildConversationID(message); got != "pending:external-1:vip_alice" {
		t.Fatalf("pending external id = %q", got)
	}
	message = IncomingMessage{DeviceID: "Device A", ConversationName: ""}
	if got := BuildConversationID(message); got != "pending:device_a:unknown" {
		t.Fatalf("pending device id = %q", got)
	}
}

func TestPrepareIncomingCreatesNewConversationPlan(t *testing.T) {
	messageAt := fixedTime(10)
	now := fixedTime(11)
	plan := PrepareIncoming(IncomingMessage{
		TenantID:         " tenant-1 ",
		WeWorkUserID:     "WX-1",
		ExternalUserID:   "Ext-1",
		RoomID:           "Room-1",
		DeviceID:         "device-1",
		SenderID:         "sender-1",
		SenderName:       "Alice",
		Content:          "hello",
		ConversationName: "Group A",
		Timestamp:        messageAt,
		TraceID:          " trace-1 ",
	}, nil, 42, now)

	if !plan.IsNewConversation {
		t.Fatal("new conversation flag = false")
	}
	if plan.Message.MessageID != 42 || plan.Message.MessageOrigin != OriginDeviceRealtime || plan.Message.CreatedAt != messageAt {
		t.Fatalf("message defaults = %+v", plan.Message)
	}
	if plan.Conversation.ConversationID != "ww:wx1:room:room-1" || plan.Conversation.ConversationType != "room" {
		t.Fatalf("conversation identity = %+v", plan.Conversation)
	}
	if plan.Conversation.UnreadCount != 1 || plan.Conversation.LastIncomingAt == nil || !plan.Conversation.LastIncomingAt.Equal(messageAt) {
		t.Fatalf("incoming state = %+v", plan.Conversation)
	}
	if !plan.Conversation.UpdatedAt.Equal(now) || plan.Conversation.FirstMessageAt != messageAt {
		t.Fatalf("conversation times = %+v", plan.Conversation)
	}
}

func TestPrepareIncomingMergesExistingConversationState(t *testing.T) {
	pk := int64(99)
	firstMessageAt := fixedTime(1)
	lastIncomingAt := fixedTime(20)
	lastOutgoingAt := fixedTime(9)
	current := &ConversationSnapshot{
		ConversationPK:  &pk,
		ConversationID:  "conv-1",
		AccountID:       "account-old",
		SenderName:      "Real Alice",
		FirstMessageAt:  &firstMessageAt,
		LastIncomingAt:  &lastIncomingAt,
		LastOutgoingAt:  &lastOutgoingAt,
		UnreadCount:     3,
		AIAutoReply:     true,
		AIModeOverride:  "manual",
		SOPRuntimeState: `{"stage":"warm"}`,
	}

	plan := PrepareIncoming(IncomingMessage{
		ConversationID:   "conv-1",
		ConversationKey:  "key-1",
		TenantID:         "tenant-1",
		DeviceID:         "device-1",
		SenderID:         "sender-1",
		SenderName:       "wmExternalUser12345",
		Content:          "new",
		MsgType:          "image",
		ConversationName: "Alice",
		Timestamp:        fixedTime(10),
		TraceID:          "trace-1",
	}, current, 100, fixedTime(11))

	if plan.IsNewConversation {
		t.Fatal("existing conversation marked new")
	}
	if plan.Conversation.ConversationPK == nil || *plan.Conversation.ConversationPK != pk {
		t.Fatalf("conversation pk = %#v", plan.Conversation.ConversationPK)
	}
	if plan.Conversation.AccountID != "account-old" || plan.Message.AccountID != "account-old" {
		t.Fatalf("account binding not preserved: message=%+v conversation=%+v", plan.Message, plan.Conversation)
	}
	if plan.Conversation.SenderName != "Real Alice" {
		t.Fatalf("low-quality sender should keep current name, got %q", plan.Conversation.SenderName)
	}
	if plan.Conversation.UnreadCount != 4 || !plan.Conversation.FirstMessageAt.Equal(firstMessageAt) {
		t.Fatalf("conversation counters/times = %+v", plan.Conversation)
	}
	if plan.Conversation.LastIncomingAt == nil || !plan.Conversation.LastIncomingAt.Equal(lastIncomingAt) {
		t.Fatalf("last incoming should keep newer current value: %#v", plan.Conversation.LastIncomingAt)
	}
	if plan.Conversation.LastOutgoingAt == nil || !plan.Conversation.LastOutgoingAt.Equal(lastOutgoingAt) {
		t.Fatalf("last outgoing not preserved: %#v", plan.Conversation.LastOutgoingAt)
	}
	if !plan.Conversation.AIAutoReply || plan.Conversation.AIModeOverride != "manual" || plan.Conversation.SOPRuntimeState != `{"stage":"warm"}` {
		t.Fatalf("runtime flags not preserved: %+v", plan.Conversation)
	}
}

func TestPrepareIncomingRespectsOutgoingDirection(t *testing.T) {
	firstMessageAt := fixedTime(1)
	lastIncomingAt := fixedTime(8)
	lastOutgoingAt := fixedTime(7)
	current := &ConversationSnapshot{
		ConversationID:  "conv-1",
		FirstMessageAt:  &firstMessageAt,
		LastIncomingAt:  &lastIncomingAt,
		LastOutgoingAt:  &lastOutgoingAt,
		UnreadCount:     3,
		SOPRuntimeState: `{"stage":"warm"}`,
	}

	plan := PrepareIncoming(IncomingMessage{
		ConversationID:   "conv-1",
		TenantID:         "tenant-1",
		DeviceID:         "device-1",
		SenderID:         "sender-1",
		SenderName:       "Alice",
		Content:          "reply",
		Direction:        DirectionOutgoing,
		Timestamp:        fixedTime(10),
		TraceID:          "trace-outgoing",
		MessageOrigin:    "archive_history",
		ConversationName: "Alice",
	}, current, 101, fixedTime(11))

	if plan.Message.Direction != DirectionOutgoing || plan.Message.MessageOrigin != "archive_history" {
		t.Fatalf("message = %+v", plan.Message)
	}
	if plan.Conversation.UnreadCount != 3 {
		t.Fatalf("unread count = %d", plan.Conversation.UnreadCount)
	}
	if plan.Conversation.LastIncomingAt == nil || !plan.Conversation.LastIncomingAt.Equal(lastIncomingAt) {
		t.Fatalf("last incoming = %#v", plan.Conversation.LastIncomingAt)
	}
	if plan.Conversation.LastOutgoingAt == nil || !plan.Conversation.LastOutgoingAt.Equal(fixedTime(10)) {
		t.Fatalf("last outgoing = %#v", plan.Conversation.LastOutgoingAt)
	}
}

func TestShouldInsertMessageChecksArchiveAndTraceDuplicates(t *testing.T) {
	if ShouldInsertMessage(IncomingMessage{ArchiveMsgID: "archive-1"}, false, true) {
		t.Fatal("existing archive msgid should skip insert")
	}
	if ShouldInsertMessage(IncomingMessage{}, true, false) {
		t.Fatal("existing trace should skip insert")
	}
	if !ShouldInsertMessage(IncomingMessage{ArchiveMsgID: "archive-1"}, false, false) {
		t.Fatal("new archive/trace should insert")
	}
}

func TestLowQualitySenderNameGuard(t *testing.T) {
	if !IsLowQualitySenderName("企微客户") || !IsLowQualitySenderName("wmExternalUser12345") || !IsLowQualitySenderName("AB-12345") {
		t.Fatal("expected low-quality sender names")
	}
	if IsLowQualitySenderName("张三") {
		t.Fatal("Chinese display name should be high quality")
	}
	if got := ResolveSenderNameForUpsert("wmExternalUser12345", "Real Alice"); got != "Real Alice" {
		t.Fatalf("sender name = %q", got)
	}
}

func fixedTime(minute int) time.Time {
	return time.Date(2026, 6, 30, 10, minute, 0, 0, time.UTC)
}
