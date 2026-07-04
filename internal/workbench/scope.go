// Account scope helpers keep CS workbench account selection independent from SQL.
// They mirror the legacy rules that decide which account ids become projection
// scope, while leaving canonical account enrichment for later service layers.
package workbench

import (
	"errors"
	"strings"
)

// ErrInvalidAccountScope means a historical bare selected_account_id is unknown.
var ErrInvalidAccountScope = errors.New("cs workbench account scope is invalid")

// AccountRecord is the minimal account fact needed to resolve workbench scope.
type AccountRecord struct {
	AccountID           string
	AccountName         string
	DeviceID            string
	AgentID             string
	WeWorkUserID        string
	AssigneeID          string
	AssigneeName        string
	EnterpriseID        string
	SOPFlowID           string
	SOPEnabled          *bool
	SOPReplyWindowStart string
	SOPReplyWindowEnd   string
	AIEnabled           bool
	AIModel             string
	KnowledgeTag        string
	CreatedAt           string
	UpdatedAt           string
}

// AccountConversationAIRecord is the response/event shape for account AI sync.
type AccountConversationAIRecord struct {
	ConversationID string
	TenantID       string
	AccountID      string
	AIAutoReply    bool
	AIModeOverride string
}

// AccountScopeInput carries account and session facts for workbench scope.
type AccountScopeInput struct {
	AllAccounts                []AccountRecord
	AssigneeID                 string
	SelectedAccountID          string
	SessionTenantID            string
	SessionOrganizationName    string
	HasExplicitSessionTenant   bool
	HasExplicitOrganizationKey bool
}

// AccountScope is the resolved account boundary for projection reads.
type AccountScope struct {
	SelectedAccountKey string
	Accounts           []AccountRecord
	SelectedAccount    *AccountRecord
	TenantID           string
	WeWorkUserIDs      []string
	AssignedSessions   bool
}

// ResolveAccountScope resolves the CS workbench selected account and tenant.
func ResolveAccountScope(input AccountScopeInput) (AccountScope, error) {
	assigneeID := strings.TrimSpace(input.AssigneeID)
	scopedAccounts := make([]AccountRecord, 0)
	for _, account := range input.AllAccounts {
		if strings.TrimSpace(account.AssigneeID) == assigneeID {
			scopedAccounts = append(scopedAccounts, account)
		}
	}
	selectedKey, err := ResolveDefaultAccountKey(scopedAccounts, input.SelectedAccountID, input.AllAccounts)
	if err != nil {
		return AccountScope{}, err
	}
	scopedAccounts = collapseOrphanAutoAccounts(scopedAccounts)
	scopedAccounts = includeExplicitAccount(scopedAccounts, input.AllAccounts, selectedKey)
	selectedAccount, ok := ResolveSelectedScopedAccount(scopedAccounts, selectedKey)
	tenantID := ResolveScopeTenantID(ScopeTenantInput{
		SessionTenantID:          input.SessionTenantID,
		SessionOrganizationName:  input.SessionOrganizationName,
		HasExplicitSessionTenant: input.HasExplicitSessionTenant || input.HasExplicitOrganizationKey,
		SelectedAccount:          selectedAccount,
		ScopedAccounts:           scopedAccounts,
	})
	weworkUserIDs := make([]string, 0)
	if ok {
		weworkUserIDs = AccountDeviceCandidates(selectedAccount)
	} else if selectedKey != "assigned-sessions" {
		seen := make(map[string]bool)
		for _, account := range scopedAccounts {
			for _, candidate := range AccountDeviceCandidates(account) {
				if !seen[candidate] {
					seen[candidate] = true
					weworkUserIDs = append(weworkUserIDs, candidate)
				}
			}
		}
	}
	scope := AccountScope{
		SelectedAccountKey: selectedKey,
		Accounts:           scopedAccounts,
		TenantID:           tenantID,
		WeWorkUserIDs:      weworkUserIDs,
		AssignedSessions:   selectedKey == "assigned-sessions",
	}
	if ok {
		accountCopy := selectedAccount
		scope.SelectedAccount = &accountCopy
	}
	return scope, nil
}

// CanonicalAccountKey normalizes historical bare account selector values.
func CanonicalAccountKey(selectedAccountKey string, accounts []AccountRecord) string {
	normalizedKey := strings.TrimSpace(selectedAccountKey)
	if normalizedKey == "" || normalizedKey == "all" || normalizedKey == "assigned-sessions" || strings.HasPrefix(normalizedKey, "account:") {
		return normalizedKey
	}
	lookup := strings.ToLower(normalizedKey)
	normalizedLookup := NormalizeIDHint(normalizedKey)
	for _, account := range accounts {
		accountID := strings.TrimSpace(account.AccountID)
		deviceID := strings.TrimSpace(account.DeviceID)
		weworkUserID := strings.TrimSpace(account.WeWorkUserID)
		if (accountID != "" && strings.ToLower(accountID) == lookup) ||
			(deviceID != "" && strings.ToLower(deviceID) == lookup) ||
			(weworkUserID != "" && strings.ToLower(weworkUserID) == lookup) ||
			(normalizedLookup != "" && NormalizeIDHint(weworkUserID) == normalizedLookup) {
			return "account:" + firstNonBlank(accountID, deviceID, weworkUserID)
		}
	}
	return normalizedKey
}

// ResolveDefaultAccountKey applies legacy defaults for empty account selectors.
func ResolveDefaultAccountKey(scopedAccounts []AccountRecord, selectedAccountKey string, allAccounts []AccountRecord) (string, error) {
	lookupAccounts := make([]AccountRecord, 0, len(scopedAccounts)+len(allAccounts))
	lookupAccounts = append(lookupAccounts, scopedAccounts...)
	lookupAccounts = append(lookupAccounts, allAccounts...)
	normalizedKey := CanonicalAccountKey(selectedAccountKey, lookupAccounts)
	if normalizedKey != "" {
		if normalizedKey != "all" && normalizedKey != "assigned-sessions" && !strings.HasPrefix(normalizedKey, "account:") {
			return "", ErrInvalidAccountScope
		}
		return normalizedKey, nil
	}
	if len(scopedAccounts) > 0 {
		return "all", nil
	}
	return "assigned-sessions", nil
}

// ResolveSelectedScopedAccount maps account:{id} selectors to a scoped account.
func ResolveSelectedScopedAccount(scopedAccounts []AccountRecord, selectedAccountKey string) (AccountRecord, bool) {
	normalizedKey := strings.TrimSpace(selectedAccountKey)
	if !strings.HasPrefix(normalizedKey, "account:") {
		return AccountRecord{}, false
	}
	accountLookup := strings.TrimSpace(strings.TrimPrefix(normalizedKey, "account:"))
	if accountLookup == "" {
		return AccountRecord{}, false
	}
	if strings.HasPrefix(accountLookup, "auto-") {
		autoDeviceID := strings.TrimSpace(strings.TrimPrefix(accountLookup, "auto-"))
		for _, account := range scopedAccounts {
			if autoDeviceID != "" && strings.TrimSpace(account.DeviceID) == autoDeviceID {
				return account, true
			}
		}
	}
	for _, account := range scopedAccounts {
		if strings.TrimSpace(account.AccountID) == accountLookup ||
			strings.TrimSpace(account.DeviceID) == accountLookup ||
			strings.TrimSpace(account.WeWorkUserID) == accountLookup {
			return account, true
		}
	}
	return AccountRecord{}, false
}

// ScopeTenantInput carries tenant facts used after account scope is resolved.
type ScopeTenantInput struct {
	SessionTenantID          string
	SessionOrganizationName  string
	HasExplicitSessionTenant bool
	SelectedAccount          AccountRecord
	ScopedAccounts           []AccountRecord
}

// ResolveScopeTenantID mirrors the workbench projection tenant narrowing rule.
func ResolveScopeTenantID(input ScopeTenantInput) string {
	sessionTenantID := strings.TrimSpace(input.SessionTenantID)
	if input.HasExplicitSessionTenant || strings.TrimSpace(input.SessionOrganizationName) != "" {
		return sessionTenantID
	}
	if tenantID := strings.TrimSpace(input.SelectedAccount.EnterpriseID); tenantID != "" {
		return tenantID
	}
	tenantIDs := make(map[string]bool)
	for _, account := range input.ScopedAccounts {
		tenantID := strings.TrimSpace(account.EnterpriseID)
		if tenantID != "" {
			tenantIDs[tenantID] = true
		}
	}
	if len(tenantIDs) == 1 {
		for tenantID := range tenantIDs {
			return tenantID
		}
	}
	if len(tenantIDs) > 1 {
		return ""
	}
	return sessionTenantID
}

// AccountDeviceCandidates returns the explicit wework ids used by projection.
func AccountDeviceCandidates(account AccountRecord) []string {
	userID := strings.TrimSpace(account.WeWorkUserID)
	if userID == "" {
		return nil
	}
	seen := make(map[string]bool)
	candidates := make([]string, 0, 3)
	for _, candidate := range []string{userID, strings.ToLower(userID), NormalizeIDHint(userID)} {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

// NormalizeIDHint matches the Python userid/device hint comparison key.
func NormalizeIDHint(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func collapseOrphanAutoAccounts(scopedAccounts []AccountRecord) []AccountRecord {
	collapsed := make([]AccountRecord, 0, len(scopedAccounts))
	for _, account := range scopedAccounts {
		accountID := strings.TrimSpace(account.AccountID)
		if strings.HasPrefix(accountID, "auto-") && strings.TrimSpace(account.DeviceID) == "" && NormalizeIDHint(account.WeWorkUserID) == "" {
			continue
		}
		collapsed = append(collapsed, account)
	}
	return collapsed
}

func includeExplicitAccount(scopedAccounts []AccountRecord, allAccounts []AccountRecord, selectedAccountKey string) []AccountRecord {
	if !strings.HasPrefix(strings.TrimSpace(selectedAccountKey), "account:") {
		return scopedAccounts
	}
	if _, ok := ResolveSelectedScopedAccount(scopedAccounts, selectedAccountKey); ok {
		return scopedAccounts
	}
	selected, ok := ResolveSelectedScopedAccount(allAccounts, selectedAccountKey)
	if !ok {
		return scopedAccounts
	}
	seen := make(map[string]bool)
	merged := make([]AccountRecord, 0, len(scopedAccounts)+1)
	for _, account := range append(append([]AccountRecord{}, scopedAccounts...), selected) {
		key := firstNonBlank(strings.TrimSpace(account.AccountID), NormalizeIDHint(account.WeWorkUserID), strings.TrimSpace(account.DeviceID))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, account)
	}
	return merged
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
