package workbench

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceDiagnosticForkedConversationsBuildsPythonShape keeps grouped payloads stable.
func TestServiceDiagnosticForkedConversationsBuildsPythonShape(t *testing.T) {
	store := &fakeDiagnosticConversationStore{forked: []DiagnosticForkedConversationGroupRecord{{
		WeWorkUserID:      " ww-a ",
		ExternalUserID:    " ext-a ",
		ConversationCount: 2,
		Conversations: []DiagnosticForkedConversationMemberRecord{
			{ConversationID: "conv-1", DeviceID: "device-a", ConversationName: "会话一", LastMessageAt: "2026-07-01 09:30:00", UnreadCount: 1},
			{ConversationID: " conv-2 ", DeviceID: " device-b ", ConversationName: " 会话二 ", UnreadCount: 0},
		},
	}}}
	service := Service{DiagnosticConversationStore: store}

	payload, err := service.DiagnosticForkedConversations(context.Background(), NewDiagnosticForkedConversationsRequest(auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("DiagnosticForkedConversations returned error: %v", err)
	}
	if payload["total"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	items := payload["items"].([]Payload)
	if items[0]["wework_user_id"] != "ww-a" || items[0]["external_userid"] != "ext-a" || items[0]["conversation_count"] != 2 {
		t.Fatalf("item = %+v", items[0])
	}
	members := items[0]["conversation_ids"].([]Payload)
	if members[1]["conversation_id"] != "conv-2" || members[1]["device_id"] != "device-b" {
		t.Fatalf("members = %+v", members)
	}
}

// TestServiceDiagnosticForkedConversationsFailsClosedWithoutStore keeps wiring explicit.
func TestServiceDiagnosticForkedConversationsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).DiagnosticForkedConversations(context.Background(), DiagnosticForkedConversationsRequest{})
	if !errors.Is(err, ErrDiagnosticConversationStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticConversationStoreUnavailable)
	}
}
