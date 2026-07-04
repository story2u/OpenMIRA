package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"im-go/internal/auth"
)

func TestServiceUpsertAccountPublishesAndAudits(t *testing.T) {
	aiEnabled := true
	store := &fakeAccountManageStore{account: AccountRecord{
		AccountID:    "acc-001",
		AccountName:  "账号一",
		DeviceID:     "device-1",
		WeWorkUserID: "DY-1801",
		AIEnabled:    true,
	}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Accounts: store, AccountEvents: events, AuditLogWriter: audit, ReadModelInvalidator: invalidator}

	payload, err := service.UpsertAccount(context.Background(), NewAccountUpsertRequest(AccountUpsertBody{AccountID: " acc-001 ", AccountName: " 账号一 ", DeviceID: " device-1 ", WeWorkUserID: " DY-1801 ", AIEnabled: &aiEnabled}, auth.Session{Role: "admin", AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("UpsertAccount returned error: %v", err)
	}
	if store.upsert.AccountID != "acc-001" || store.upsert.AccountName != "账号一" || store.upsert.DeviceID != "device-1" || store.upsert.WeWorkUserID != "DY-1801" || store.upsert.AIEnabled == nil || !*store.upsert.AIEnabled {
		t.Fatalf("upsert command = %+v", store.upsert)
	}
	account := payload["account"].(ProjectionRow)
	if payload["success"] != true || account["account_id"] != "acc-001" || account["account_name"] != "账号一" || account["ai_enabled"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].channel != "devices" || events.events[0].event != "account.updated" || events.events[0].topic != "account.changed" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-001" || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "创建/更新账号: 账号一") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceUpsertAccountGeneratesAccountID(t *testing.T) {
	store := &fakeAccountManageStore{}
	service := Service{Accounts: store}

	_, err := service.UpsertAccount(context.Background(), NewAccountUpsertRequest(AccountUpsertBody{AccountName: "账号一"}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("UpsertAccount returned error: %v", err)
	}
	if !strings.HasPrefix(store.upsert.AccountID, "acc-") {
		t.Fatalf("generated account id = %q", store.upsert.AccountID)
	}
}

func TestNewAccountUpsertRequestPrefersChannelUserID(t *testing.T) {
	request := NewAccountUpsertRequest(AccountUpsertBody{
		AccountName:   "账号一",
		ChannelUserID: " channel-1 ",
		WeWorkUserID:  " old-channel-1 ",
	}, auth.Session{Role: "admin"})

	if request.Command.WeWorkUserID != "channel-1" {
		t.Fatalf("WeWorkUserID compatibility value = %q, want channel-1", request.Command.WeWorkUserID)
	}
}

func TestServiceUpsertAccountValidation(t *testing.T) {
	service := Service{Accounts: &fakeAccountManageStore{}}

	_, err := service.UpsertAccount(context.Background(), NewAccountUpsertRequest(AccountUpsertBody{}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountNameRequired) {
		t.Fatalf("err = %v, want account name required", err)
	}
}

func TestServiceDeleteAccountPublishesOnlyWhenDeleted(t *testing.T) {
	store := &fakeAccountManageStore{deleted: true}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Accounts: store, AccountEvents: events, AuditLogWriter: audit, ReadModelInvalidator: invalidator}

	payload, err := service.DeleteAccount(context.Background(), NewAccountDeleteRequest(" acc-001 ", auth.Session{Role: "supervisor", AssigneeID: "sup-001"}))
	if err != nil {
		t.Fatalf("DeleteAccount returned error: %v", err)
	}
	if payload["success"] != true || store.deleteAccountID != "acc-001" {
		t.Fatalf("payload=%#v store=%+v", payload, store)
	}
	if len(events.events) != 1 || events.events[0].event != "account.deleted" || events.events[0].payload["account_id"] != "acc-001" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "sup-001" || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "删除账号: acc-001") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}

	store.deleted = false
	events.events = nil
	audit.entries = nil
	invalidator.namespaces = nil
	payload, err = service.DeleteAccount(context.Background(), NewAccountDeleteRequest("missing", auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("DeleteAccount no-op returned error: %v", err)
	}
	if payload["success"] != false || len(events.events) != 0 || len(audit.entries) != 0 || len(invalidator.namespaces) != 0 {
		t.Fatalf("no-op payload=%#v events=%+v audit=%+v invalidated=%+v", payload, events.events, audit.entries, invalidator.namespaces)
	}
}

func TestServiceBatchUpsertAccountsParsesCSVPublishesAndAudits(t *testing.T) {
	store := &fakeAccountManageStore{}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Accounts: store, AccountEvents: events, AuditLogWriter: audit, ReadModelInvalidator: invalidator}
	content := []byte("\ufeffaccount_name,agent_id,device_id,wework_user_id,enterprise_id,sop_enabled,ai_enabled,knowledge_tag\n账号一,agent-1,device-1,DY-1,ent-a,yes,on,vip\n,skip,,,,,,\n账号二,agent-2,device-2,DY-2,ent-b,0,false,\n")

	payload, err := service.BatchUpsertAccounts(context.Background(), NewAccountBatchUpsertRequest("accounts.csv", content, auth.Session{Role: "admin", AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("BatchUpsertAccounts returned error: %v", err)
	}
	if payload["success"] != true || payload["count"] != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	accounts := payload["accounts"].([]ProjectionRow)
	if len(accounts) != 2 || accounts[0]["account_name"] != "账号一" || accounts[1]["account_name"] != "账号二" {
		t.Fatalf("accounts = %#v", accounts)
	}
	if len(store.upserts) != 2 || store.upserts[0].AccountName != "账号一" || store.upserts[0].SOPEnabled == nil || !*store.upserts[0].SOPEnabled || store.upserts[1].AIEnabled == nil || *store.upserts[1].AIEnabled {
		t.Fatalf("upserts = %+v", store.upserts)
	}
	if !strings.HasPrefix(store.upserts[0].AccountID, "acc-") || !strings.HasPrefix(store.upserts[1].AccountID, "acc-") {
		t.Fatalf("generated account ids = %q %q", store.upserts[0].AccountID, store.upserts[1].AccountID)
	}
	if len(events.events) != 1 || events.events[0].event != "account.batch_imported" || events.events[0].payload["count"] != 2 {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "批量导入账号: 2 条") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceBatchUpsertAccountsValidation(t *testing.T) {
	service := Service{Accounts: &fakeAccountManageStore{}}

	cases := []struct {
		name     string
		filename string
		content  []byte
		want     error
	}{
		{name: "missing file", filename: "", content: []byte("account_name\n账号一\n"), want: ErrAccountBatchFileRequired},
		{name: "wrong extension", filename: "accounts.txt", content: []byte("account_name\n账号一\n"), want: ErrAccountBatchCSVOnly},
		{name: "empty", filename: "accounts.csv", content: nil, want: ErrAccountBatchCSVEmpty},
		{name: "decode", filename: "accounts.csv", content: []byte{0xff, 0xfe}, want: ErrAccountBatchCSVDecode},
		{name: "header", filename: "accounts.csv", content: []byte{}, want: ErrAccountBatchHeaderMissing},
	}

	for _, tc := range cases {
		if tc.name == "header" {
			tc.content = []byte("\n")
		}
		_, err := service.BatchUpsertAccounts(context.Background(), NewAccountBatchUpsertRequest(tc.filename, tc.content, auth.Session{Role: "admin"}))
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestServiceAccountManageRequiresStore(t *testing.T) {
	service := Service{}

	_, err := service.UpsertAccount(context.Background(), NewAccountUpsertRequest(AccountUpsertBody{AccountName: "账号一"}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("upsert err = %v, want store unavailable", err)
	}

	_, err = service.DeleteAccount(context.Background(), NewAccountDeleteRequest("acc-001", auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("delete err = %v, want store unavailable", err)
	}

	_, err = service.BatchUpsertAccounts(context.Background(), NewAccountBatchUpsertRequest("accounts.csv", []byte("account_name\n账号一\n"), auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("batch err = %v, want store unavailable", err)
	}
}

type fakeAccountManageStore struct {
	account         AccountRecord
	upsert          AccountUpsertCommand
	upserts         []AccountUpsertCommand
	upsertErr       error
	deleteAccountID string
	deleted         bool
	deleteErr       error
}

func (store *fakeAccountManageStore) ListAccounts(ctx context.Context) ([]AccountRecord, error) {
	return nil, nil
}

func (store *fakeAccountManageStore) UpsertAccount(ctx context.Context, command AccountUpsertCommand) (AccountRecord, error) {
	store.upsert = command
	store.upserts = append(store.upserts, command)
	if store.upsertErr != nil {
		return AccountRecord{}, store.upsertErr
	}
	account := store.account
	if strings.TrimSpace(account.AccountID) == "" {
		account = AccountRecord{AccountID: command.AccountID, AccountName: command.AccountName}
	}
	return account, nil
}

func (store *fakeAccountManageStore) DeleteAccount(ctx context.Context, accountID string) (bool, error) {
	store.deleteAccountID = accountID
	if store.deleteErr != nil {
		return false, store.deleteErr
	}
	return store.deleted, nil
}
