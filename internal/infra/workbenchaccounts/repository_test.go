// Package workbenchaccounts tests the read-only wework_accounts adapter.
// Fakes capture SQL and row mapping so workbench scope can be verified without
// a concrete database driver in the phase-three harness.
package workbenchaccounts

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"im-go/internal/workbench"
)

func TestListAccountsMapsScopeFields(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), "flow-a", int64(1), "09:00", "18:00", "coze", "vip", "2026-01-01", "2026-01-02"},
		{[]byte("acc-002"), []byte("账号二"), nil, nil, []byte("DY-1802"), "cs-002", nil, "ent-b", []byte("0"), nil, nil, nil, nil, nil, nil, nil, nil},
	}}}
	repository := &Repository{DB: db}

	accounts, err := repository.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts returned error: %v", err)
	}
	if db.query != accountSelectColumns+" FROM wework_accounts ORDER BY updated_at DESC" {
		t.Fatalf("unexpected query: %s", db.query)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %+v, want two rows", accounts)
	}
	if accounts[0].AccountID != "acc-001" || accounts[0].AccountName != "账号一" || accounts[0].AssigneeName != "消息端一" || accounts[0].DeviceID != "device-1" || accounts[0].AgentID != "sdk:device-1" || !accounts[0].AIEnabled {
		t.Fatalf("first account = %+v", accounts[0])
	}
	if accounts[0].SOPFlowID != "flow-a" || accounts[0].SOPEnabled == nil || !*accounts[0].SOPEnabled || accounts[0].AIModel != "coze" || accounts[0].KnowledgeTag != "vip" || accounts[0].CreatedAt != "2026-01-01" {
		t.Fatalf("first account optional fields = %+v", accounts[0])
	}
	if accounts[1].AccountID != "acc-002" || accounts[1].DeviceID != "" || accounts[1].AIEnabled {
		t.Fatalf("second account = %+v", accounts[1])
	}
}

func TestListAccountsByAssigneeFiltersAndTrimsAssignee(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", "cs-001", "消息端一", "ent-a", "true", nil, nil, nil, nil, nil, nil, nil, nil}}}}
	repository := &Repository{DB: db}

	accounts, err := repository.ListAccountsByAssignee(context.Background(), " cs-001 ")
	if err != nil {
		t.Fatalf("ListAccountsByAssignee returned error: %v", err)
	}
	if !strings.Contains(db.query, "WHERE assignee_id = ? ORDER BY updated_at DESC") {
		t.Fatalf("unexpected query: %s", db.query)
	}
	if len(db.args) != 1 || db.args[0] != "cs-001" {
		t.Fatalf("args = %#v", db.args)
	}
	if len(accounts) != 1 || !accounts[0].AIEnabled {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestListAccountsByAssigneeSkipsBlankAssignee(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	accounts, err := repository.ListAccountsByAssignee(context.Background(), " ")
	if err != nil {
		t.Fatalf("ListAccountsByAssignee returned error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("accounts = %+v, want empty", accounts)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no query", db.query)
	}
}

func TestFindAccountsByIdentityUsesExactIndexedPredicates(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{{"acc-001", "alice", "device-1", "sdk:device-1", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, nil, nil}}}}
	repository := &Repository{DB: db}

	accounts, err := repository.FindAccountsByIdentity(context.Background(), " DY-1801 ", 999)
	if err != nil {
		t.Fatalf("FindAccountsByIdentity returned error: %v", err)
	}
	for _, want := range []string{"WHERE account_name = ? OR account_id = ? OR LOWER(REPLACE(wework_user_id, '-', '')) = ?", "ORDER BY updated_at DESC LIMIT ?"} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("query %q missing %q", db.query, want)
		}
	}
	if strings.Contains(db.query, "SELECT *") {
		t.Fatalf("query should stay column-explicit: %s", db.query)
	}
	wantArgs := []any{"DY-1801", "DY-1801", "dy1801", 100}
	if len(db.args) != len(wantArgs) {
		t.Fatalf("args = %#v", db.args)
	}
	for index, want := range wantArgs {
		if db.args[index] != want {
			t.Fatalf("arg[%d] = %#v, want %#v; all args=%#v", index, db.args[index], want, db.args)
		}
	}
	if len(accounts) != 1 || accounts[0].AccountID != "acc-001" || accounts[0].WeWorkUserID != "DY-1801" {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestFindAccountsByIdentitySkipsBlankIdentity(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	accounts, err := repository.FindAccountsByIdentity(context.Background(), " ", 20)
	if err != nil {
		t.Fatalf("FindAccountsByIdentity returned error: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("accounts = %+v, want empty", accounts)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no query", db.query)
	}
}

func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil)
	_, err := repository.ListAccounts(context.Background())
	if err == nil {
		t.Fatal("ListAccounts error = nil, want nil sql db error")
	}
}

func TestSetAccountAIEnabledUpdatesAndReturnsAccount(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, "2026-01-01", "2026-01-02"}},
	}}
	repository := &Repository{DB: db}

	account, updated, err := repository.SetAccountAIEnabled(context.Background(), " acc-001 ", true)
	if err != nil {
		t.Fatalf("SetAccountAIEnabled returned error: %v", err)
	}
	if !updated || account.AccountID != "acc-001" || !account.AIEnabled {
		t.Fatalf("account=%+v updated=%t", account, updated)
	}
	if len(db.execs) != 1 || db.execs[0].query != "UPDATE wework_accounts SET ai_enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?" {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != 1 || db.execs[0].args[1] != "acc-001" {
		t.Fatalf("exec args = %#v", db.execs[0].args)
	}
}

func TestUpsertAccountUpdatesExistingAccountWithPreserveSemantics(t *testing.T) {
	enabled := true
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "旧账号", "old-device", "old-agent", "DY-1801", "cs-001", "消息端一", "ent-a", int64(0), "flow-a", int64(0), "09:00", "18:00", "old-model", "old-tag", "2026-01-01", "2026-01-02"}},
		{{"acc-001", "新账号", "", "", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), "flow-a", int64(1), "09:00", "18:00", "deepseek", "old-tag", "2026-01-01", "2026-01-03"}},
	}}
	repository := &Repository{DB: db}

	account, err := repository.UpsertAccount(context.Background(), workbench.AccountUpsertCommand{
		AccountID:   " acc-001 ",
		AccountName: " 新账号 ",
		AIEnabled:   &enabled,
		AIModel:     " deepseek ",
	})
	if err != nil {
		t.Fatalf("UpsertAccount returned error: %v", err)
	}
	if account.AccountName != "新账号" || !account.AIEnabled || account.AIModel != "deepseek" || account.KnowledgeTag != "old-tag" {
		t.Fatalf("account = %+v", account)
	}
	if len(db.execs) != 1 || !strings.HasPrefix(db.execs[0].query, "UPDATE wework_accounts SET account_name = ?") {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != "新账号" || db.execs[0].args[1] != nil || db.execs[0].args[2] != nil || db.execs[0].args[9] != 1 || db.execs[0].args[10] != "deepseek" || db.execs[0].args[12] != "acc-001" {
		t.Fatalf("update args = %#v", db.execs[0].args)
	}
}

func TestUpsertAccountPreservesDisplayNameWhenIncomingNameIsUserID(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "张三", "device-1", "agent-1", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, "2026-01-01", "2026-01-02"}},
		{{"acc-001", "张三", "device-1", "agent-1", "DY-1801", "cs-001", "消息端一", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, "2026-01-01", "2026-01-03"}},
	}}
	repository := &Repository{DB: db}

	account, err := repository.UpsertAccount(context.Background(), workbench.AccountUpsertCommand{
		AccountID:    "acc-001",
		AccountName:  " dy1801 ",
		WeWorkUserID: "DY-1801",
	})
	if err != nil {
		t.Fatalf("UpsertAccount returned error: %v", err)
	}
	if account.AccountName != "张三" {
		t.Fatalf("account = %+v, want preserved display name", account)
	}
	if len(db.execs) != 1 || db.execs[0].args[0] != "张三" {
		t.Fatalf("update execs = %+v", db.execs)
	}
}

func TestUpsertAccountInsertsNewAccount(t *testing.T) {
	sopEnabled := false
	db := &fakeDB{rowsByQuery: [][][]any{
		{},
		{{"acc-002", "账号二", "device-2", "agent-2", "DY-1802", "", "", "ent-b", nil, "flow-b", int64(0), nil, nil, nil, nil, "2026-01-01", "2026-01-01"}},
	}}
	repository := &Repository{DB: db}

	account, err := repository.UpsertAccount(context.Background(), workbench.AccountUpsertCommand{
		AccountID:    "acc-002",
		AccountName:  "账号二",
		AgentID:      "agent-2",
		DeviceID:     "device-2",
		WeWorkUserID: "DY-1802",
		EnterpriseID: "ent-b",
		SOPFlowID:    "flow-b",
		SOPEnabled:   &sopEnabled,
	})
	if err != nil {
		t.Fatalf("UpsertAccount returned error: %v", err)
	}
	if account.AccountID != "acc-002" || account.AccountName != "账号二" || account.SOPEnabled == nil || *account.SOPEnabled {
		t.Fatalf("account = %+v", account)
	}
	if len(db.execs) != 1 || !strings.HasPrefix(db.execs[0].query, "INSERT INTO wework_accounts") {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != "acc-002" || db.execs[0].args[1] != "账号二" || db.execs[0].args[7] != 0 {
		t.Fatalf("insert args = %#v", db.execs[0].args)
	}
}

func TestDeleteAccountUsesRowsAffected(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteAccount(context.Background(), " acc-001 ")
	if err != nil {
		t.Fatalf("DeleteAccount returned error: %v", err)
	}
	if !deleted || len(db.execs) != 1 || db.execs[0].query != "DELETE FROM wework_accounts WHERE account_id = ?" || db.execs[0].args[0] != "acc-001" {
		t.Fatalf("deleted=%t execs=%+v", deleted, db.execs)
	}
}

func TestAssignAccountUpdatesAssigneeAndReturnsAccount(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", "cs-002", "消息端二", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, "2026-01-01", "2026-01-02"}},
	}}
	repository := &Repository{DB: db}

	account, updated, err := repository.AssignAccount(context.Background(), " acc-001 ", " cs-002 ", " 消息端二 ")
	if err != nil {
		t.Fatalf("AssignAccount returned error: %v", err)
	}
	if !updated || account.AccountID != "acc-001" || account.AssigneeID != "cs-002" || account.AssigneeName != "消息端二" {
		t.Fatalf("account=%+v updated=%t", account, updated)
	}
	if len(db.execs) != 1 || db.execs[0].query != "UPDATE wework_accounts SET assignee_id = ?, assignee_name = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?" {
		t.Fatalf("execs = %+v", db.execs)
	}
	wantArgs := []any{"cs-002", "消息端二", "acc-001"}
	for index, want := range wantArgs {
		if db.execs[0].args[index] != want {
			t.Fatalf("arg[%d]=%#v want %#v; args=%#v", index, db.execs[0].args[index], want, db.execs[0].args)
		}
	}
}

func TestAssignAccountWritesNullNameWhenBlank(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", "cs-002", nil, "ent-a", int64(1), nil, nil, nil, nil, nil, nil, nil, nil}},
	}}
	repository := &Repository{DB: db}

	_, updated, err := repository.AssignAccount(context.Background(), "acc-001", "cs-002", " ")
	if err != nil {
		t.Fatalf("AssignAccount returned error: %v", err)
	}
	if !updated || db.execs[0].args[1] != nil {
		t.Fatalf("updated=%t args=%#v", updated, db.execs[0].args)
	}
}

func TestUnassignAccountClearsAssigneeAndReturnsAccount(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "账号一", "device-1", "sdk:device-1", "DY-1801", nil, nil, "ent-a", int64(1), nil, nil, nil, nil, nil, nil, "2026-01-01", "2026-01-02"}},
	}}
	repository := &Repository{DB: db}

	account, updated, err := repository.UnassignAccount(context.Background(), " acc-001 ")
	if err != nil {
		t.Fatalf("UnassignAccount returned error: %v", err)
	}
	if !updated || account.AccountID != "acc-001" || account.AssigneeID != "" || account.AssigneeName != "" {
		t.Fatalf("account=%+v updated=%t", account, updated)
	}
	if len(db.execs) != 1 || db.execs[0].query != "UPDATE wework_accounts SET assignee_id = NULL, assignee_name = NULL, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?" {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != "acc-001" {
		t.Fatalf("exec args = %#v", db.execs[0].args)
	}
}

func TestSetAccountConversationAIModeUpdatesConversationsAndProjection(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{
			{"conv-2", "tenant-a", "acc-001", int64(1), "auto"},
			{"conv-1", "tenant-a", "acc-001", int64(1), "auto"},
		},
	}}
	repository := &Repository{DB: db}

	conversations, err := repository.SetAccountConversationAIMode(context.Background(), " acc-001 ", true)
	if err != nil {
		t.Fatalf("SetAccountConversationAIMode returned error: %v", err)
	}
	if len(conversations) != 2 || conversations[0].ConversationID != "conv-2" || conversations[0].AIModeOverride != "auto" || !conversations[0].AIAutoReply {
		t.Fatalf("conversations = %+v", conversations)
	}
	if len(db.execs) != 2 {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].query != "UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?" {
		t.Fatalf("conversation update = %s", db.execs[0].query)
	}
	if db.execs[0].args[0] != "auto" || db.execs[0].args[1] != 1 || db.execs[0].args[2] != "acc-001" {
		t.Fatalf("conversation args = %#v", db.execs[0].args)
	}
	if !strings.Contains(db.execs[1].query, "UPDATE conversation_overview_projection SET ai_auto_reply = ?, ai_mode_override = ?") {
		t.Fatalf("projection update = %s", db.execs[1].query)
	}
	wantProjectionArgs := []any{1, "auto", "conv-2", "conv-1"}
	if len(db.execs[1].args) != len(wantProjectionArgs) {
		t.Fatalf("projection args = %#v", db.execs[1].args)
	}
	for index, want := range wantProjectionArgs {
		if db.execs[1].args[index] != want {
			t.Fatalf("projection arg[%d]=%#v want %#v; args=%#v", index, db.execs[1].args[index], want, db.execs[1].args)
		}
	}
}

func TestSetConversationAIModeOverrideUpdatesConversationProjectionAndRuntime(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"conv-1", "tenant-a", "acc-001", int64(0), "manual", `{"ai_reply_status":"failed"}`}},
		{{"conv-1", "tenant-a", "acc-001", int64(1), "auto", `{"ai_reply_status":"failed"}`}},
	}}
	repository := &Repository{DB: db}

	conversation, updated, err := repository.SetConversationAIModeOverride(context.Background(), " conv-1 ", "auto", false)
	if err != nil {
		t.Fatalf("SetConversationAIModeOverride returned error: %v", err)
	}
	if !updated || conversation.ConversationID != "conv-1" || !conversation.AIAutoReply || conversation.AIModeOverride != "auto" || conversation.SOPRuntimeState["ai_reply_status"] != "failed" {
		t.Fatalf("conversation=%+v updated=%t", conversation, updated)
	}
	if len(db.execs) != 2 {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].query != "UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?" {
		t.Fatalf("conversation update = %s", db.execs[0].query)
	}
	if db.execs[0].args[0] != "auto" || db.execs[0].args[1] != 1 || db.execs[0].args[2] != "conv-1" {
		t.Fatalf("conversation args = %#v", db.execs[0].args)
	}
	if !strings.Contains(db.execs[1].query, "UPDATE conversation_overview_projection SET ai_auto_reply = ?, ai_mode_override = ?") || db.execs[1].args[0] != 1 || db.execs[1].args[1] != "auto" {
		t.Fatalf("projection exec = %+v", db.execs[1])
	}

	err = repository.UpdateConversationRuntimeState(context.Background(), " conv-1 ", map[string]any{"handoff_status": "auto_active"})
	if err != nil {
		t.Fatalf("UpdateConversationRuntimeState returned error: %v", err)
	}
	last := db.execs[len(db.execs)-1]
	if last.query != "UPDATE conversations SET sop_runtime_state = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?" || last.args[1] != "conv-1" || !strings.Contains(last.args[0].(string), "auto_active") {
		t.Fatalf("runtime exec = %+v", last)
	}
}

func TestSetConversationAIModeOverrideBulkUpdatesConversationsAndProjection(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{
			{"conv-2", "tenant-a", "acc-002", int64(0), "manual", `{}`},
			{"conv-1", "tenant-a", "acc-001", int64(0), "manual", `{}`},
		},
	}}
	repository := &Repository{DB: db}

	conversations, err := repository.SetConversationAIModeOverrideBulk(context.Background(), []string{"conv-1", "conv-2", "conv-1"}, "manual", true)
	if err != nil {
		t.Fatalf("SetConversationAIModeOverrideBulk returned error: %v", err)
	}
	if len(conversations) != 2 || conversations[0].ConversationID != "conv-1" || conversations[1].ConversationID != "conv-2" {
		t.Fatalf("conversations = %+v", conversations)
	}
	if len(db.execs) != 2 {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].args[0] != "manual" || db.execs[0].args[1] != 0 {
		t.Fatalf("bulk update args = %#v", db.execs[0].args)
	}
	if db.execs[1].args[0] != 0 || db.execs[1].args[1] != "manual" {
		t.Fatalf("projection args = %#v", db.execs[1].args)
	}
}

func TestListAssigneeScopedConversationAIIDsUsesProjectionAndAccountDevices(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"acc-001", "账号一", "device-1", "sdk:device-1", "wx-1", "cs-001", "消息端一", "ent-a", int64(1), nil, nil, nil, nil, nil, nil, nil, nil}},
		{{"conv-1"}, {"conv-2"}},
	}}
	repository := &Repository{DB: db}

	ids, err := repository.ListAssigneeScopedConversationAIIDs(context.Background(), " cs-001 ", " ent-a ")
	if err != nil {
		t.Fatalf("ListAssigneeScopedConversationAIIDs returned error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "conv-1" || ids[1] != "conv-2" {
		t.Fatalf("ids = %+v", ids)
	}
	if !strings.Contains(db.query, "conversation_overview_projection") || !strings.Contains(db.query, "assignee_id = ?") || !strings.Contains(db.query, "device_id IN") || !strings.Contains(db.query, "tenant_id = ?") {
		t.Fatalf("query = %s", db.query)
	}
	wantArgs := []any{"cs-001", "device-1", "ent-a"}
	for index, want := range wantArgs {
		if db.args[index] != want {
			t.Fatalf("arg[%d]=%#v want %#v; args=%#v", index, db.args[index], want, db.args)
		}
	}
}

func TestMarkConversationReadClearsConversationAndProjection(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"conv-1", "key-1", "tenant-a", "acc-001", "device-1", "wx-1", "ext-1", "客户一", int64(3), "2026-07-01T09:00:00Z", "2026-07-01T09:01:00Z"}},
		{{"conv-1", "key-1", "tenant-a", "acc-001", "device-1", "wx-1", "ext-1", "客户一", int64(0), "2026-07-01T09:00:00Z", "2026-07-01T09:02:00Z"}},
	}}
	repository := &Repository{DB: db}

	conversation, updated, err := repository.MarkConversationRead(context.Background(), " key-1 ")
	if err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}
	if !updated || conversation.ConversationID != "conv-1" || conversation.ConversationKey != "key-1" || conversation.UnreadCount != 0 {
		t.Fatalf("conversation=%+v updated=%t", conversation, updated)
	}
	if len(db.execs) != 2 {
		t.Fatalf("execs = %+v", db.execs)
	}
	if db.execs[0].query != "UPDATE conversations SET unread_count = 0, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?" || db.execs[0].args[0] != "conv-1" {
		t.Fatalf("conversation exec = %+v", db.execs[0])
	}
	if db.execs[1].query != "UPDATE conversation_overview_projection SET unread_count = 0, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?" || db.execs[1].args[0] != "conv-1" {
		t.Fatalf("projection exec = %+v", db.execs[1])
	}
}

func TestGetConversationReadMapsStableConversationColumns(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"conv-1", "key-1", "tenant-a", "acc-001", "device-1", "wx-1", "ext-1", "客户一", []byte("7"), "2026-07-01T09:00:00Z", "2026-07-01T09:01:00Z"}},
	}}
	repository := &Repository{DB: db}

	conversation, ok, err := repository.GetConversationRead(context.Background(), " conv-1 ")
	if err != nil {
		t.Fatalf("GetConversationRead returned error: %v", err)
	}
	if !ok || conversation.UnreadCount != 7 || conversation.ExternalUserID != "ext-1" || conversation.AssigneeID != "" {
		t.Fatalf("conversation=%+v ok=%t", conversation, ok)
	}
	if !strings.Contains(db.query, "WHERE conversation_id = ? OR conversation_key = ?") {
		t.Fatalf("query = %s", db.query)
	}
	if len(db.args) != 2 || db.args[0] != "conv-1" || db.args[1] != "conv-1" {
		t.Fatalf("args = %#v", db.args)
	}
}

type fakeDB struct {
	rows        *fakeRows
	rowsByQuery [][][]any
	query       string
	args        []any
	execs       []fakeExec
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if len(db.rowsByQuery) > 0 {
		values := db.rowsByQuery[0]
		db.rowsByQuery = db.rowsByQuery[1:]
		return &fakeRows{values: values}, nil
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: args})
	return fakeResult(1), nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}
