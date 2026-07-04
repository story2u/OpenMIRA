package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"im-go/internal/auth"
)

func TestServiceToggleAccountAIEnabledPublishesAuditsAndSyncsConversations(t *testing.T) {
	writeStore := &fakeAccountAIWriteStore{
		account: AccountRecord{
			AccountID:    "acc-001",
			AccountName:  "账号一",
			AssigneeID:   "cs-001",
			AIEnabled:    true,
			UpdatedAt:    "2026-07-01T01:02:03Z",
			WeWorkUserID: "DY-1801",
		},
		conversations: []AccountConversationAIRecord{
			{ConversationID: "conv-1", TenantID: "tenant-a", AccountID: "acc-001", AIAutoReply: true, AIModeOverride: "auto"},
			{ConversationID: "conv-2", TenantID: "tenant-a", AccountID: "acc-001", AIAutoReply: true, AIModeOverride: "auto"},
		},
	}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Accounts:             &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001", AccountName: "账号一", AssigneeID: "cs-001"}}},
		AccountAIWriteStore:  writeStore,
		AccountEvents:        events,
		AuditLogWriter:       audit,
		ReadModelInvalidator: invalidator,
	}
	enabled := true

	payload, err := service.ToggleAccountAIEnabled(context.Background(), NewAccountAIEnabledRequest(" acc-001 ", AccountAIEnabledBody{Enabled: &enabled}, auth.Session{Role: "admin", AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("ToggleAccountAIEnabled returned error: %v", err)
	}
	if writeStore.accountID != "acc-001" || !writeStore.enabled || writeStore.conversationAccountID != "acc-001" || !writeStore.conversationEnabled {
		t.Fatalf("write store state = %+v", writeStore)
	}
	if payload["success"] != true || payload["enabled"] != true || payload["updated_count"] != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	account := payload["account"].(ProjectionRow)
	if account["account_id"] != "acc-001" || account["ai_enabled"] != true || account["channel_user_id"] != "DY-1801" || account["wework_user_id"] != "DY-1801" {
		t.Fatalf("account payload = %#v", account)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 2 || conversations[0]["conversation_id"] != "conv-1" || conversations[0]["ai_mode_override"] != "auto" || conversations[0]["ai_auto_reply"] != true {
		t.Fatalf("conversation payload = %#v", conversations)
	}
	if len(events.events) != 3 {
		t.Fatalf("events = %+v", events.events)
	}
	if events.events[0].channel != "conversations" || events.events[0].event != "conversation.ai_auto_reply" || events.events[0].payload["account_ai_enabled"] != true {
		t.Fatalf("conversation event = %+v", events.events[0])
	}
	if events.events[2].channel != "devices" || events.events[2].event != "account.updated" || events.events[2].topic != "account.changed" {
		t.Fatalf("account event = %+v", events.events[2])
	}
	if audit.entry.Operator != "admin-001" || audit.entry.ActionType != "account" || !strings.Contains(audit.entry.Detail, "切换账号AI托管 acc-001: 开启，同步会话 2 条") {
		t.Fatalf("audit entry = %+v", audit.entry)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceToggleAccountAIEnabledEnforcesCSScope(t *testing.T) {
	writeStore := &fakeAccountAIWriteStore{account: AccountRecord{AccountID: "acc-001"}}
	service := Service{
		Accounts:            &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001", AssigneeID: "cs-owner"}}},
		AccountAIWriteStore: writeStore,
	}
	enabled := false

	_, err := service.ToggleAccountAIEnabled(context.Background(), NewAccountAIEnabledRequest("acc-001", AccountAIEnabledBody{Enabled: &enabled}, auth.Session{Role: "cs", AssigneeID: "cs-other"}))
	if !errors.Is(err, auth.ErrPermissionDenied) {
		t.Fatalf("err = %v, want permission denied", err)
	}
	if writeStore.accountID != "" {
		t.Fatalf("write store should not be called: %+v", writeStore)
	}
}

func TestServiceToggleAccountAIEnabledValidationAndNotFound(t *testing.T) {
	service := Service{
		Accounts:            &fakeAccountStore{accounts: []AccountRecord{{AccountID: "acc-001"}}},
		AccountAIWriteStore: &fakeAccountAIWriteStore{},
	}
	_, err := service.ToggleAccountAIEnabled(context.Background(), NewAccountAIEnabledRequest("acc-001", AccountAIEnabledBody{}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountAIEnabledRequired) {
		t.Fatalf("err = %v, want enabled required", err)
	}
	enabled := true
	_, err = service.ToggleAccountAIEnabled(context.Background(), NewAccountAIEnabledRequest("missing", AccountAIEnabledBody{Enabled: &enabled}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("err = %v, want account not found", err)
	}
}

type fakeAccountAIWriteStore struct {
	account               AccountRecord
	conversations         []AccountConversationAIRecord
	accountID             string
	enabled               bool
	conversationAccountID string
	conversationEnabled   bool
	syncAccountID         string
	syncEnabled           bool
	syncReset             bool
}

func (store *fakeAccountAIWriteStore) SetAccountAIEnabled(ctx context.Context, accountID string, enabled bool) (AccountRecord, bool, error) {
	store.accountID = accountID
	store.enabled = enabled
	account := store.account
	if strings.TrimSpace(account.AccountID) == "" {
		account.AccountID = accountID
	}
	account.AIEnabled = enabled
	return account, true, nil
}

func (store *fakeAccountAIWriteStore) SetAccountConversationAIMode(ctx context.Context, accountID string, enabled bool) ([]AccountConversationAIRecord, error) {
	store.conversationAccountID = accountID
	store.conversationEnabled = enabled
	return store.conversations, nil
}

func (store *fakeAccountAIWriteStore) SyncAccountAIEnabled(ctx context.Context, account AccountRecord, enabled bool, resetOverrideToInherit bool) (AccountAIDefaultSyncResult, error) {
	store.syncAccountID = strings.TrimSpace(account.AccountID)
	store.syncEnabled = enabled
	store.syncReset = resetOverrideToInherit
	return AccountAIDefaultSyncResult{Conversations: store.conversations}, nil
}
