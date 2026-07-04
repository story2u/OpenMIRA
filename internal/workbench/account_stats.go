// Account stats service code keeps management reads projection-backed.
// It resolves auth scope and account metadata in the domain layer, while SQL remains
// in infra so the route can stay read-only and fail closed.
package workbench

import (
	"context"
	"errors"
	"strings"
)

var (
	// ErrAccountStatsStoreUnavailable means account aggregate rows cannot be loaded.
	ErrAccountStatsStoreUnavailable = errors.New("workbench account stats store is unavailable")
	// ErrCSSessionMissingAssignee enforces the CS read-scope guard.
	ErrCSSessionMissingAssignee = errors.New("current cs session is missing assignee_id")
	// ErrCSAssigneeScope enforces the CS cross-assignee guard.
	ErrCSAssigneeScope = errors.New("cs cannot query conversations of another assignee")
)

// AccountStatsStore loads projection-backed account aggregate rows.
type AccountStatsStore interface {
	ListAccountStats(ctx context.Context, query AccountStatsQuery) ([]ProjectionRow, error)
}

// AccountStats builds /api/v1/conversations/account-stats from projection aggregates.
func (service Service) AccountStats(ctx context.Context, request AccountStatsRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	store := service.accountStatsStore()
	if store == nil {
		return nil, ErrAccountStatsStoreUnavailable
	}
	resolvedAssigneeID, err := resolveAccountStatsAssignee(request)
	if err != nil {
		return nil, err
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	scopedAccounts, explicitAccountScope, directDeviceIDs, directChannelUserIDs := resolveAccountStatsAccountScope(accounts, request)
	if explicitAccountScope && len(scopedAccounts) == 0 && len(directDeviceIDs) == 0 && len(directChannelUserIDs) == 0 {
		return accountStatsPayload(nil, request.AccountQuery), nil
	}
	scopeAccounts := accountsForStatsScope(accounts, scopedAccounts, request, resolvedAssigneeID)
	tenantID := ResolveScopeTenantID(ScopeTenantInput{
		SessionTenantID:          sessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id"),
		SessionOrganizationName:  sessionClaim(BootstrapRequest{Session: request.Session}, "organization_name"),
		HasExplicitSessionTenant: hasSessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id") || hasSessionClaim(BootstrapRequest{Session: request.Session}, "organization_name"),
		ScopedAccounts:           scopeAccounts,
	})
	deviceIDs, channelUserIDs := accountStatsScopeIDs(scopeAccounts)
	deviceIDs = appendUniqueStrings(deviceIDs, directDeviceIDs...)
	channelUserIDs = appendUniqueStrings(channelUserIDs, directChannelUserIDs...)
	if !explicitAccountScope && tenantID != "" {
		deviceIDs = nil
		channelUserIDs = nil
	}

	rows, err := store.ListAccountStats(ctx, AccountStatsQuery{
		DeviceIDs:                    deviceIDs,
		ChannelUserIDs:               channelUserIDs,
		WeWorkUserIDs:                channelUserIDs,
		AssigneeID:                   resolvedAssigneeID,
		TenantID:                     tenantID,
		UnreadOnly:                   request.UnreadOnly,
		UnassignedOnly:               request.UnassignedOnly,
		StatusFilter:                 request.StatusFilter,
		IncludeUnassignedForAssignee: true,
	})
	if err != nil {
		return nil, err
	}
	stats := buildAccountStatsRows(rows, accounts)
	return accountStatsPayload(stats, request.AccountQuery), nil
}

func (service Service) accountStatsStore() AccountStatsStore {
	if service.Projection == nil {
		return nil
	}
	store, ok := service.Projection.(AccountStatsStore)
	if !ok {
		return nil
	}
	return store
}

func resolveAccountStatsAssignee(request AccountStatsRequest) (string, error) {
	requestedAssigneeID := strings.TrimSpace(request.AssigneeID)
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		sessionAssigneeID := strings.TrimSpace(request.Session.AssigneeID)
		if sessionAssigneeID == "" {
			return "", ErrCSSessionMissingAssignee
		}
		if requestedAssigneeID != "" && requestedAssigneeID != sessionAssigneeID {
			return "", ErrCSAssigneeScope
		}
		return sessionAssigneeID, nil
	}
	return requestedAssigneeID, nil
}

func resolveAccountStatsAccountScope(accounts []AccountRecord, request AccountStatsRequest) ([]AccountRecord, bool, []string, []string) {
	accountName := strings.TrimSpace(request.AccountName)
	accountKey := strings.TrimSpace(request.AccountKey)
	if accountName == "" && accountKey == "" {
		return nil, false, nil, nil
	}
	matched := make([]AccountRecord, 0)
	for _, account := range accounts {
		if accountStatsAccountMatches(account, accountName, accountKey) {
			matched = append(matched, account)
		}
	}
	deviceIDs := make([]string, 0)
	channelUserIDs := make([]string, 0)
	prefix, value, ok := splitAccountStatsIdentityKey(accountKey)
	if ok {
		switch prefix {
		case "device":
			deviceIDs = append(deviceIDs, value)
		case "channel", "account_user", "wework", "archive_user":
			channelUserIDs = appendUniqueStrings(channelUserIDs, value, strings.ToLower(value), NormalizeIDHint(value))
		}
	}
	return matched, true, normalizeStringsLocal(deviceIDs), normalizeStringsLocal(channelUserIDs)
}

func accountStatsAccountMatches(account AccountRecord, accountName string, accountKey string) bool {
	if accountName != "" && strings.TrimSpace(account.AccountName) == accountName {
		return true
	}
	if accountKey == "" {
		return false
	}
	key := strings.TrimSpace(accountKey)
	rawKey := strings.ToLower(key)
	trimmedKey := key
	if value, ok := strings.CutPrefix(rawKey, "account:"); ok {
		trimmedKey = strings.TrimSpace(key[len("account:"):])
		rawKey = strings.TrimSpace(value)
	}
	if value, ok := strings.CutPrefix(rawKey, "channel:"); ok {
		trimmedKey = strings.TrimSpace(key[len("channel:"):])
		rawKey = strings.TrimSpace(value)
	}
	if value, ok := strings.CutPrefix(rawKey, "account_user:"); ok {
		trimmedKey = strings.TrimSpace(key[len("account_user:"):])
		rawKey = strings.TrimSpace(value)
	}
	if value, ok := strings.CutPrefix(rawKey, "wework:"); ok {
		trimmedKey = strings.TrimSpace(key[len("wework:"):])
		rawKey = strings.TrimSpace(value)
	}
	if value, ok := strings.CutPrefix(rawKey, "archive_user:"); ok {
		trimmedKey = strings.TrimSpace(key[len("archive_user:"):])
		rawKey = strings.TrimSpace(value)
	}
	if value, ok := strings.CutPrefix(rawKey, "device:"); ok {
		trimmedKey = strings.TrimSpace(key[len("device:"):])
		rawKey = strings.TrimSpace(value)
	}
	normalizedKey := NormalizeIDHint(trimmedKey)
	for _, candidate := range []string{
		strings.TrimSpace(account.AccountID),
		strings.TrimSpace(account.DeviceID),
		strings.TrimSpace(account.ChannelUserID),
		strings.TrimSpace(account.WeWorkUserID),
		strings.TrimSpace(account.AccountName),
	} {
		if candidate == "" {
			continue
		}
		if strings.EqualFold(candidate, trimmedKey) || strings.EqualFold(candidate, rawKey) || NormalizeIDHint(candidate) == normalizedKey {
			return true
		}
	}
	return false
}

func accountsForStatsScope(allAccounts []AccountRecord, selectedAccounts []AccountRecord, request AccountStatsRequest, assigneeID string) []AccountRecord {
	if len(selectedAccounts) > 0 {
		return selectedAccounts
	}
	if strings.TrimSpace(request.AccountName) != "" || strings.TrimSpace(request.AccountKey) != "" {
		return nil
	}
	if strings.TrimSpace(assigneeID) == "" {
		return allAccounts
	}
	scoped := make([]AccountRecord, 0)
	for _, account := range allAccounts {
		if strings.TrimSpace(account.AssigneeID) == strings.TrimSpace(assigneeID) {
			scoped = append(scoped, account)
		}
	}
	return scoped
}

func accountStatsScopeIDs(accounts []AccountRecord) ([]string, []string) {
	deviceIDs := make([]string, 0)
	channelUserIDs := make([]string, 0)
	for _, account := range accounts {
		if deviceID := strings.TrimSpace(account.DeviceID); deviceID != "" {
			deviceIDs = append(deviceIDs, deviceID)
		}
		channelUserIDs = appendUniqueStrings(channelUserIDs, AccountChannelCandidates(account)...)
	}
	return normalizeStringsLocal(deviceIDs), normalizeStringsLocal(channelUserIDs)
}

func buildAccountStatsRows(rows []ProjectionRow, accounts []AccountRecord) []ProjectionRow {
	lookup := accountStatsLookup(accounts)
	stats := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		account := resolveProjectionStatsAccount(row, lookup)
		stats = append(stats, accountStatsRow(row, account))
	}
	return stats
}

func accountStatsLookup(accounts []AccountRecord) map[string]AccountRecord {
	lookup := make(map[string]AccountRecord)
	for _, account := range accounts {
		for _, key := range []string{
			strings.TrimSpace(account.AccountID),
			strings.TrimSpace(account.DeviceID),
			strings.TrimSpace(account.WeWorkUserID),
			NormalizeIDHint(account.WeWorkUserID),
		} {
			if key != "" {
				lookup[strings.ToLower(key)] = account
			}
		}
	}
	return lookup
}

func resolveProjectionStatsAccount(row ProjectionRow, lookup map[string]AccountRecord) AccountRecord {
	for _, key := range []string{
		rowText(row, "wework_user_id"),
		NormalizeIDHint(rowText(row, "wework_user_id")),
		rowText(row, "device_id"),
	} {
		if key == "" {
			continue
		}
		if account, ok := lookup[strings.ToLower(key)]; ok {
			return account
		}
	}
	return AccountRecord{}
}

func accountStatsRow(row ProjectionRow, account AccountRecord) ProjectionRow {
	accountID := strings.TrimSpace(account.AccountID)
	weworkUserID := firstNonBlank(strings.TrimSpace(account.WeWorkUserID), rowText(row, "wework_user_id"))
	deviceID := firstNonBlank(strings.TrimSpace(account.DeviceID), rowText(row, "device_id"))
	accountName := strings.TrimSpace(account.AccountName)
	if accountName == "" {
		accountName = "未归属账号"
	}
	return ProjectionRow{
		"account_name":           accountName,
		"account_id":             nilIfBlank(accountID),
		"account_key":            accountStatsKey(accountID, weworkUserID, deviceID),
		"device_id":              deviceID,
		"organization_name":      "",
		"enterprise_id":          strings.TrimSpace(account.EnterpriseID),
		"enterprise_bound":       strings.TrimSpace(account.EnterpriseID) != "",
		"total":                  rowInt(row, "total"),
		"unread":                 rowInt(row, "unread"),
		"unassigned_unread":      rowInt(row, "unassigned_unread"),
		"max_pending":            rowInt(row, "max_pending"),
		"account_wework_user_id": weworkUserID,
		"wework_user_id":         weworkUserID,
		"account_avatar":         "",
		"stats_ready":            true,
	}
}

func accountStatsPayload(stats []ProjectionRow, accountQuery string) Payload {
	filtered := filterAccountStatsRows(stats, accountQuery)
	if filtered == nil {
		filtered = []ProjectionRow{}
	}
	return Payload{
		"accounts": filtered,
		"summary":  accountStatsSummary(filtered),
	}
}

func filterAccountStatsRows(stats []ProjectionRow, accountQuery string) []ProjectionRow {
	query := strings.ToLower(strings.TrimSpace(accountQuery))
	if query == "" {
		return stats
	}
	filtered := make([]ProjectionRow, 0, len(stats))
	for _, row := range stats {
		if accountStatsRowMatchesQuery(row, query) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func accountStatsRowMatchesQuery(row ProjectionRow, query string) bool {
	for _, key := range []string{"account_name", "organization_name", "account_wework_user_id", "wework_user_id", "account_id", "account_key", "device_id"} {
		if strings.Contains(strings.ToLower(rowText(row, key)), query) {
			return true
		}
	}
	return false
}

func accountStatsSummary(stats []ProjectionRow) ProjectionRow {
	summary := ProjectionRow{
		"account_count":           len(stats),
		"conversation_count":      0,
		"unread_count":            0,
		"unassigned_unread_count": 0,
	}
	for _, row := range stats {
		summary["conversation_count"] = rowInt(summary, "conversation_count") + rowInt(row, "total")
		summary["unread_count"] = rowInt(summary, "unread_count") + rowInt(row, "unread")
		summary["unassigned_unread_count"] = rowInt(summary, "unassigned_unread_count") + rowInt(row, "unassigned_unread")
	}
	return summary
}

func accountStatsKey(accountID string, weworkUserID string, deviceID string) string {
	if accountID != "" {
		return "account:" + accountID
	}
	if normalized := NormalizeIDHint(weworkUserID); normalized != "" {
		return "wework:" + normalized
	}
	if deviceID != "" {
		return "device:" + deviceID
	}
	return "name:未归属账号"
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range append(append([]string{}, values...), additions...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeStringsLocal(values []string) []string {
	return appendUniqueStrings(nil, values...)
}

func splitAccountStatsIdentityKey(accountKey string) (string, string, bool) {
	prefix, value, ok := strings.Cut(strings.TrimSpace(accountKey), ":")
	if !ok {
		return "", "", false
	}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	value = strings.TrimSpace(value)
	if prefix == "" || value == "" {
		return "", "", false
	}
	return prefix, value, true
}
