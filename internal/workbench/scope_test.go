package workbench

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveAccountScopeDefaultsToAllForAssigneeAccounts(t *testing.T) {
	scope, err := ResolveAccountScope(AccountScopeInput{
		AssigneeID: "cs-001",
		AllAccounts: []AccountRecord{
			{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
			{AccountID: "acc-002", AssigneeID: "cs-001", WeWorkUserID: "dy1802", EnterpriseID: "ent-b"},
			{AccountID: "auto-orphan", AssigneeID: "cs-001"},
			{AccountID: "acc-003", AssigneeID: "cs-002", WeWorkUserID: "dy1803", EnterpriseID: "ent-c"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveAccountScope returned error: %v", err)
	}
	if scope.SelectedAccountKey != "all" || scope.AssignedSessions {
		t.Fatalf("scope key = %q assigned=%t, want all/false", scope.SelectedAccountKey, scope.AssignedSessions)
	}
	if len(scope.Accounts) != 2 {
		t.Fatalf("accounts = %+v, want two non-orphan assignee accounts", scope.Accounts)
	}
	if scope.TenantID != "" {
		t.Fatalf("tenant = %q, want empty for multi-enterprise all scope", scope.TenantID)
	}
	wantIDs := []string{"DY-1801", "dy-1801", "dy1801", "dy1802"}
	if !reflect.DeepEqual(scope.ChannelUserIDs, wantIDs) || !reflect.DeepEqual(scope.WeWorkUserIDs, wantIDs) {
		t.Fatalf("channel ids = %#v compatibility=%#v, want %#v", scope.ChannelUserIDs, scope.WeWorkUserIDs, wantIDs)
	}
}

func TestResolveAccountScopeDefaultsToAssignedSessionsWithoutAccounts(t *testing.T) {
	scope, err := ResolveAccountScope(AccountScopeInput{AssigneeID: "cs-404"})
	if err != nil {
		t.Fatalf("ResolveAccountScope returned error: %v", err)
	}
	if scope.SelectedAccountKey != "assigned-sessions" || !scope.AssignedSessions || len(scope.ChannelUserIDs) != 0 || len(scope.WeWorkUserIDs) != 0 {
		t.Fatalf("unexpected scope: %+v", scope)
	}
}

func TestResolveAccountScopeCanonicalizesHistoricalBareAccountValues(t *testing.T) {
	scope, err := ResolveAccountScope(AccountScopeInput{
		AssigneeID:        "cs-001",
		SelectedAccountID: "device-1",
		AllAccounts: []AccountRecord{
			{AccountID: "acc-001", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveAccountScope returned error: %v", err)
	}
	if scope.SelectedAccountKey != "account:acc-001" || scope.SelectedAccount == nil || scope.SelectedAccount.AccountID != "acc-001" {
		t.Fatalf("unexpected selected scope: %+v selected=%+v", scope, scope.SelectedAccount)
	}
	if scope.TenantID != "ent-a" {
		t.Fatalf("tenant = %q, want selected account tenant", scope.TenantID)
	}
}

func TestResolveAccountScopeRejectsUnknownBareAccountValues(t *testing.T) {
	_, err := ResolveAccountScope(AccountScopeInput{
		AssigneeID:        "cs-001",
		SelectedAccountID: "unknown-account",
		AllAccounts: []AccountRecord{
			{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801"},
		},
	})
	if !errors.Is(err, ErrInvalidAccountScope) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidAccountScope)
	}
}

func TestResolveAccountScopeIncludesExplicitVisibleAccount(t *testing.T) {
	scope, err := ResolveAccountScope(AccountScopeInput{
		AssigneeID:        "cs-001",
		SelectedAccountID: "account:acc-002",
		AllAccounts: []AccountRecord{
			{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
			{AccountID: "acc-002", AssigneeID: "cs-002", WeWorkUserID: "DY-1802", EnterpriseID: "ent-b"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveAccountScope returned error: %v", err)
	}
	if len(scope.Accounts) != 2 || scope.SelectedAccount == nil || scope.SelectedAccount.AccountID != "acc-002" {
		t.Fatalf("unexpected explicit account scope: %+v selected=%+v", scope.Accounts, scope.SelectedAccount)
	}
	if scope.TenantID != "ent-b" {
		t.Fatalf("tenant = %q, want explicit selected account tenant", scope.TenantID)
	}
	if !reflect.DeepEqual(scope.ChannelUserIDs, []string{"DY-1802", "dy-1802", "dy1802"}) {
		t.Fatalf("channel ids = %#v", scope.ChannelUserIDs)
	}
}

func TestResolveScopeTenantIDUsesExplicitSessionTenant(t *testing.T) {
	tenantID := ResolveScopeTenantID(ScopeTenantInput{
		SessionTenantID:          "session-tenant",
		HasExplicitSessionTenant: true,
		SelectedAccount:          AccountRecord{EnterpriseID: "account-tenant"},
		ScopedAccounts:           []AccountRecord{{EnterpriseID: "ent-a"}, {EnterpriseID: "ent-b"}},
	})
	if tenantID != "session-tenant" {
		t.Fatalf("tenant = %q, want explicit session tenant", tenantID)
	}
}

func TestResolveSelectedScopedAccountMatchesAutoDeviceID(t *testing.T) {
	account, ok := ResolveSelectedScopedAccount(
		[]AccountRecord{{AccountID: "auto-device-1", DeviceID: "device-1", AssigneeID: "cs-001"}},
		"account:auto-device-1",
	)
	if !ok || account.DeviceID != "device-1" {
		t.Fatalf("account = %+v ok=%t, want auto device match", account, ok)
	}
}

func TestAccountChannelCandidatesPreferExplicitChannelUserID(t *testing.T) {
	got := AccountChannelCandidates(AccountRecord{DeviceID: "device-1", ChannelUserID: "channel-001", WeWorkUserID: "DY-1801"})
	want := []string{"channel-001", "channel001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestAccountDeviceCandidatesKeepCompatibilityUserID(t *testing.T) {
	got := AccountDeviceCandidates(AccountRecord{DeviceID: "device-1", WeWorkUserID: "DY-1801"})
	want := []string{"DY-1801", "dy-1801", "dy1801"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
	if candidates := AccountDeviceCandidates(AccountRecord{DeviceID: "device-1"}); len(candidates) != 0 {
		t.Fatalf("candidates = %#v, want empty without explicit channel id", candidates)
	}
}
