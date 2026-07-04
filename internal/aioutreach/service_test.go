package aioutreach

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/messages"
	"wework-go/internal/workbench"
)

func TestQueryConversationResolvesAccountConversationAndMessages(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	accounts := &fakeAccountStore{accounts: []workbench.AccountRecord{{
		AccountID:    "acc-1",
		AccountName:  "wechat-a",
		DeviceID:     "device-1",
		WeWorkUserID: "DY-1801",
		EnterpriseID: "ent-a",
	}}}
	conversations := &fakeConversationStore{conversation: Conversation{
		ConversationID: "ww:DY-1801:external-1",
		TenantID:       "ent-a",
		WeWorkUserID:   "dy-1801",
	}}
	messagesStore := &fakeMessageStore{records: []messages.Record{
		{
			ArchiveMsgID:   "archive-1",
			TraceID:        "trace-1",
			Direction:      "incoming",
			MessageOrigin:  "archive_history",
			MsgType:        "text",
			Content:        " hello ",
			Timestamp:      createdAt,
			ConversationID: "ww:DY-1801:external-1",
		},
		{
			TraceID:        "trace-2",
			Direction:      "outgoing",
			MsgType:        "",
			Content:        "staff reply",
			Timestamp:      createdAt.Add(time.Minute),
			ConversationID: "ww:DY-1801:external-1",
		},
	}}
	service := Service{
		Accounts:      accounts,
		Enterprises:   &fakeEnterpriseStore{corpIDs: map[string]string{"ent-a": "corp-a"}},
		Conversations: conversations,
		Messages:      messagesStore,
		Now:           func() time.Time { return createdAt },
	}

	result, err := service.QueryConversation(context.Background(), ConversationRequest{
		CorpID:         "corp-a",
		CustomerID:     "customer-ignored",
		ExternalUserID: " external-1 ",
		Wechat:         " wechat-a ",
		Limit:          999,
	})
	if err != nil {
		t.Fatalf("QueryConversation returned error: %v", err)
	}
	if accounts.identity != "wechat-a" || accounts.limit != defaultAccountLookupLimit {
		t.Fatalf("account lookup identity=%q limit=%d", accounts.identity, accounts.limit)
	}
	if conversations.conversationID != "ww:DY-1801:external-1" {
		t.Fatalf("conversationID = %q", conversations.conversationID)
	}
	if messagesStore.conversationID != "ww:DY-1801:external-1" || messagesStore.limit != maxConversationLimit {
		t.Fatalf("messages conversation=%q limit=%d", messagesStore.conversationID, messagesStore.limit)
	}
	if result.ConversationID != "ww:DY-1801:external-1" || len(result.Messages) != 2 {
		t.Fatalf("result = %#v", result)
	}
	first := result.Messages[0]
	if first.MsgID != "archive-1" || first.From != "customer" || first.Source != "archive_history" || first.MsgType != "text" || first.Content != "hello" || first.MsgTime != createdAt.UnixMilli() {
		t.Fatalf("first message = %#v", first)
	}
	second := result.Messages[1]
	if second.MsgID != "trace-2" || second.From != "staff" || second.Source != "unknown" || second.MsgType != "text" || second.MsgTime != createdAt.Add(time.Minute).UnixMilli() {
		t.Fatalf("second message = %#v", second)
	}
}

func TestQueryConversationReturnsAccountBusinessErrors(t *testing.T) {
	tests := []struct {
		name     string
		service  Service
		request  ConversationRequest
		wantCode int
	}{
		{
			name:     "wechat required",
			service:  Service{},
			request:  ConversationRequest{CorpID: "corp-a"},
			wantCode: CodeWechatRequired,
		},
		{
			name:     "account not found",
			service:  Service{Accounts: &fakeAccountStore{}},
			request:  ConversationRequest{CorpID: "corp-a", Wechat: "wechat-a"},
			wantCode: CodeAccountNotFound,
		},
		{
			name: "multiple accounts",
			service: Service{Accounts: &fakeAccountStore{accounts: []workbench.AccountRecord{
				{AccountID: "acc-1", DeviceID: "device-1", WeWorkUserID: "dy-1"},
				{AccountID: "acc-2", DeviceID: "device-2", WeWorkUserID: "dy-2"},
			}}},
			request:  ConversationRequest{Wechat: "wechat-a"},
			wantCode: CodeMultipleAccounts,
		},
		{
			name:     "missing device or wework user",
			service:  Service{Accounts: &fakeAccountStore{accounts: []workbench.AccountRecord{{AccountID: "acc-1", DeviceID: "", WeWorkUserID: "dy-1"}}}},
			request:  ConversationRequest{Wechat: "wechat-a"},
			wantCode: CodeAccountMissingDevice,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.service.QueryConversation(context.Background(), tt.request)
			assertOutreachCode(t, err, tt.wantCode)
		})
	}
}

func TestQueryConversationReturnsConversationBusinessErrors(t *testing.T) {
	account := workbench.AccountRecord{AccountID: "acc-1", DeviceID: "device-1", WeWorkUserID: "dy-1", EnterpriseID: "ent-a"}
	tests := []struct {
		name         string
		conversation Conversation
		found        bool
		request      ConversationRequest
		wantCode     int
	}{
		{
			name:     "external id required",
			found:    true,
			request:  ConversationRequest{Wechat: "wechat-a"},
			wantCode: CodeExternalIDRequired,
		},
		{
			name:     "conversation not found",
			found:    false,
			request:  ConversationRequest{Wechat: "wechat-a", CustomerID: "customer-1"},
			wantCode: CodeConversationNotFound,
		},
		{
			name:         "wrong account",
			conversation: Conversation{ConversationID: "ww:dy-1:customer-1", TenantID: "ent-a", WeWorkUserID: "dy-2"},
			found:        true,
			request:      ConversationRequest{Wechat: "wechat-a", CustomerID: "customer-1"},
			wantCode:     CodeConversationAccount,
		},
		{
			name:         "wrong corp",
			conversation: Conversation{ConversationID: "ww:dy-1:customer-1", TenantID: "ent-b", WeWorkUserID: "DY-1"},
			found:        true,
			request:      ConversationRequest{CorpID: "corp-a", Wechat: "wechat-a", CustomerID: "customer-1"},
			wantCode:     CodeConversationCorp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := Service{
				Accounts:      &fakeAccountStore{accounts: []workbench.AccountRecord{account}},
				Enterprises:   &fakeEnterpriseStore{corpIDs: map[string]string{"ent-a": "corp-a", "ent-b": "corp-b"}},
				Conversations: &fakeConversationStore{conversation: tt.conversation, found: tt.found},
				Messages:      &fakeMessageStore{},
			}
			_, err := service.QueryConversation(context.Background(), tt.request)
			assertOutreachCode(t, err, tt.wantCode)
		})
	}
}

func assertOutreachCode(t *testing.T, err error, wantCode int) {
	t.Helper()
	var outreachErr Error
	if !errors.As(err, &outreachErr) {
		t.Fatalf("error = %v, want outreach error code %d", err, wantCode)
	}
	if outreachErr.Code != wantCode {
		t.Fatalf("error code = %d, want %d; err=%v", outreachErr.Code, wantCode, err)
	}
}

type fakeAccountStore struct {
	accounts []workbench.AccountRecord
	identity string
	limit    int
	err      error
}

func (store *fakeAccountStore) FindAccountsByIdentity(ctx context.Context, identity string, limit int) ([]workbench.AccountRecord, error) {
	store.identity = identity
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return append([]workbench.AccountRecord(nil), store.accounts...), nil
}

type fakeEnterpriseStore struct {
	corpIDs map[string]string
	err     error
}

func (store *fakeEnterpriseStore) GetCorpID(ctx context.Context, enterpriseID string) (string, bool, error) {
	if store.err != nil {
		return "", false, store.err
	}
	corpID, ok := store.corpIDs[enterpriseID]
	return corpID, ok, nil
}

type fakeConversationStore struct {
	conversation   Conversation
	found          bool
	conversationID string
	err            error
}

func (store *fakeConversationStore) GetConversation(ctx context.Context, conversationID string) (Conversation, bool, error) {
	store.conversationID = conversationID
	if store.err != nil {
		return Conversation{}, false, store.err
	}
	if !store.found && store.conversation.ConversationID == "" {
		return Conversation{}, false, nil
	}
	conversation := store.conversation
	if conversation.ConversationID == "" {
		conversation.ConversationID = conversationID
	}
	return conversation, true, nil
}

type fakeMessageStore struct {
	records        []messages.Record
	conversationID string
	limit          int
	err            error
}

func (store *fakeMessageStore) ListLatestMessages(ctx context.Context, conversationID string, limit int) ([]messages.Record, error) {
	store.conversationID = conversationID
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return append([]messages.Record(nil), store.records...), nil
}
