package workbench

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceDiagnosticOrphanConversationsBuildsPythonShape keeps diagnostic payloads stable.
func TestServiceDiagnosticOrphanConversationsBuildsPythonShape(t *testing.T) {
	store := &fakeDiagnosticConversationStore{records: []DiagnosticOrphanConversationRecord{
		{
			ConversationID:   " conv-1 ",
			TenantID:         "tenant-a",
			WeWorkUserID:     "",
			ExternalUserID:   "ext-1",
			DeviceID:         " device-a ",
			SenderID:         "sender-1",
			SenderName:       "张三",
			ConversationName: "会话一",
			LastMessageAt:    "2026-07-01 09:30:00",
			UnreadCount:      2,
		},
		{ConversationID: "conv-2", DeviceID: "missing-device"},
	}}
	service := Service{
		Accounts: &fakeAccountStore{accounts: []AccountRecord{
			{AccountID: "acc-a", DeviceID: "device-a"},
		}},
		DiagnosticConversationStore: store,
	}

	payload, err := service.DiagnosticOrphanConversations(context.Background(), NewDiagnosticOrphanConversationsRequest(auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("DiagnosticOrphanConversations returned error: %v", err)
	}
	if payload["total"] != 2 {
		t.Fatalf("payload = %+v", payload)
	}
	items := payload["items"].([]Payload)
	if items[0]["conversation_id"] != "conv-1" || items[0]["resolved_account_id"] != "acc-a" || items[0]["unread_count"] != 2 {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1]["resolved_account_id"] != "" {
		t.Fatalf("second item = %+v", items[1])
	}
}

// TestServiceDiagnosticOrphanConversationsFailsClosedWithoutStore keeps wiring explicit.
func TestServiceDiagnosticOrphanConversationsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).DiagnosticOrphanConversations(context.Background(), DiagnosticOrphanConversationsRequest{})
	if !errors.Is(err, ErrDiagnosticConversationStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticConversationStoreUnavailable)
	}
}

type fakeDiagnosticConversationStore struct {
	records []DiagnosticOrphanConversationRecord
	forked  []DiagnosticForkedConversationGroupRecord
	err     error
}

func (store *fakeDiagnosticConversationStore) ListDiagnosticOrphanConversations(ctx context.Context) ([]DiagnosticOrphanConversationRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.records, nil
}

func (store *fakeDiagnosticConversationStore) ListDiagnosticForkedConversations(ctx context.Context) ([]DiagnosticForkedConversationGroupRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.forked, nil
}
