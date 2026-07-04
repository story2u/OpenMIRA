package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceAssignAccountPublishesAndAudits(t *testing.T) {
	store := &fakeAccountAssignStore{account: AccountRecord{
		AccountID:    "acc-001",
		AccountName:  "账号一",
		AssigneeID:   "cs-002",
		AssigneeName: "客服二",
		WeWorkUserID: "DY-1801",
		UpdatedAt:    "2026-07-01T09:00:00Z",
	}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Accounts: store, AccountEvents: events, AuditLogWriter: audit, ReadModelInvalidator: invalidator}

	payload, err := service.AssignAccount(context.Background(), NewAccountAssignRequest(" acc-001 ", AccountAssignBody{AssigneeID: " cs-002 ", AssigneeName: " 客服二 "}, auth.Session{Role: "admin", AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("AssignAccount returned error: %v", err)
	}
	if store.assignAccountID != "acc-001" || store.assignAssigneeID != "cs-002" || store.assignAssigneeName != "客服二" {
		t.Fatalf("store = %+v", store)
	}
	account := payload["account"].(ProjectionRow)
	if payload["success"] != true || account["account_id"] != "acc-001" || account["assignee_id"] != "cs-002" || account["assignee_name"] != "客服二" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].channel != "devices" || events.events[0].event != "account.assigned" || events.events[0].topic != "account.changed" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-001" || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "分配账号 acc-001 给 cs-002") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceUnassignAccountPublishesAndAudits(t *testing.T) {
	store := &fakeAccountAssignStore{account: AccountRecord{AccountID: "acc-001", AccountName: "账号一"}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Accounts: store, AccountEvents: events, AuditLogWriter: audit, ReadModelInvalidator: invalidator}

	payload, err := service.UnassignAccount(context.Background(), NewAccountUnassignRequest(" acc-001 ", auth.Session{Role: "supervisor", AssigneeID: "sup-001"}))
	if err != nil {
		t.Fatalf("UnassignAccount returned error: %v", err)
	}
	account := payload["account"].(ProjectionRow)
	if payload["success"] != true || account["account_id"] != "acc-001" || account["assignee_id"] != nil || account["assignee_name"] != nil {
		t.Fatalf("payload = %#v", payload)
	}
	if store.unassignAccountID != "acc-001" {
		t.Fatalf("store = %+v", store)
	}
	if len(events.events) != 1 || events.events[0].event != "account.unassigned" || events.events[0].topic != "account.changed" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "sup-001" || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "取消分配账号 acc-001") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceAssignAccountValidationAndNotFound(t *testing.T) {
	service := Service{Accounts: &fakeAccountAssignStore{}}

	_, err := service.AssignAccount(context.Background(), NewAccountAssignRequest("acc-001", AccountAssignBody{}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountAssigneeRequired) {
		t.Fatalf("err = %v, want assignee required", err)
	}

	_, err = service.AssignAccount(context.Background(), NewAccountAssignRequest("missing", AccountAssignBody{AssigneeID: "cs-001"}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("assign err = %v, want account not found", err)
	}

	_, err = service.UnassignAccount(context.Background(), NewAccountUnassignRequest("missing", auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unassign err = %v, want account not found", err)
	}
}

func TestServiceAssignAccountRequiresStore(t *testing.T) {
	service := Service{}

	_, err := service.AssignAccount(context.Background(), NewAccountAssignRequest("acc-001", AccountAssignBody{AssigneeID: "cs-001"}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("assign err = %v, want store unavailable", err)
	}

	_, err = service.UnassignAccount(context.Background(), NewAccountUnassignRequest("acc-001", auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("unassign err = %v, want store unavailable", err)
	}
}

type fakeAccountAssignStore struct {
	account            AccountRecord
	assignAccountID    string
	assignAssigneeID   string
	assignAssigneeName string
	assignErr          error
	unassignAccountID  string
	unassignErr        error
}

func (store *fakeAccountAssignStore) ListAccounts(ctx context.Context) ([]AccountRecord, error) {
	return nil, nil
}

func (store *fakeAccountAssignStore) AssignAccount(ctx context.Context, accountID string, assigneeID string, assigneeName string) (AccountRecord, bool, error) {
	store.assignAccountID = accountID
	store.assignAssigneeID = assigneeID
	store.assignAssigneeName = assigneeName
	if store.assignErr != nil {
		return AccountRecord{}, false, store.assignErr
	}
	if strings.TrimSpace(store.account.AccountID) == "" {
		return AccountRecord{}, false, nil
	}
	return store.account, true, nil
}

func (store *fakeAccountAssignStore) UnassignAccount(ctx context.Context, accountID string) (AccountRecord, bool, error) {
	store.unassignAccountID = accountID
	if store.unassignErr != nil {
		return AccountRecord{}, false, store.unassignErr
	}
	if strings.TrimSpace(store.account.AccountID) == "" {
		return AccountRecord{}, false, nil
	}
	return store.account, true, nil
}
