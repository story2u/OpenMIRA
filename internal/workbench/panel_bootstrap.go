// Panel bootstrap composes management workbench account stats, conversations,
// and assignment user summaries from projection-backed read models. It keeps
// the candidate route read-only and fails closed when scoped stores are absent.
package workbench

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	// ErrPanelRowsStoreUnavailable means assignment-aware projection rows are unavailable.
	ErrPanelRowsStoreUnavailable = errors.New("workbench panel rows store is unavailable")
)

// CSUserRecord is the minimal customer-service user fact for assignment panels.
type CSUserRecord struct {
	AssigneeID   string
	AssigneeName string
	Role         string
	Enabled      bool
	AIEnabled    bool
	MaxSessions  int
	HasPassword  bool
	LastSeenAt   any
	CreatedAt    any
	UpdatedAt    any
}

// PanelBootstrap builds /api/v1/conversations/panel-bootstrap from read models.
func (service Service) PanelBootstrap(ctx context.Context, request PanelBootstrapRequest) (Payload, error) {
	return service.panelPayload(ctx, request, true)
}

// PanelSnapshot builds /api/v1/conversations/panel-snapshot from read models.
func (service Service) PanelSnapshot(ctx context.Context, request PanelSnapshotRequest) (Payload, error) {
	return service.panelPayload(ctx, request.PanelBootstrapRequest, false)
}

func (service Service) panelPayload(ctx context.Context, request PanelBootstrapRequest, includeAccountStatsReady bool) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	statsStore := service.accountStatsStore()
	if statsStore == nil {
		return nil, ErrAccountStatsStoreUnavailable
	}
	rowsStore := service.panelRowsStore()
	if rowsStore == nil {
		return nil, ErrPanelRowsStoreUnavailable
	}
	statsRequest := panelStatsRequest(request)
	resolvedAssigneeID, err := resolveAccountStatsAssignee(statsRequest)
	if err != nil {
		return nil, err
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	scopedAccounts, explicitAccountScope, directDeviceIDs, directChannelUserIDs := resolveAccountStatsAccountScope(accounts, statsRequest)
	scopeAccounts := accountsForStatsScope(accounts, scopedAccounts, statsRequest, resolvedAssigneeID)
	tenantID := ResolveScopeTenantID(ScopeTenantInput{
		SessionTenantID:          panelSessionClaim(request, "tenant_id"),
		SessionOrganizationName:  panelSessionClaim(request, "organization_name"),
		HasExplicitSessionTenant: panelHasSessionClaim(request, "tenant_id") || panelHasSessionClaim(request, "organization_name"),
		ScopedAccounts:           scopeAccounts,
	})
	statsRows, err := service.panelAccountStats(ctx, statsStore, statsRequest, scopeAccounts, explicitAccountScope, directDeviceIDs, directChannelUserIDs, tenantID, resolvedAssigneeID)
	if err != nil {
		return nil, err
	}
	statsPayload := accountStatsPayload(buildAccountStatsRows(statsRows, accounts), request.AccountQuery)
	accountStats, _ := statsPayload["accounts"].([]ProjectionRow)
	summary, _ := statsPayload["summary"].(ProjectionRow)
	cursorLastMessageAt, cursorConversationID, err := DecodeConversationCursor(request.ConversationCursor)
	if err != nil {
		return nil, err
	}

	selectedAccount, hasSelectedAccount := selectPanelAccount(accountStats, scopeAccounts, scopedAccounts)
	panelQuery := panelRowsQuery(request, selectedAccount, hasSelectedAccount, scopeAccounts, tenantID, resolvedAssigneeID, directDeviceIDs, directChannelUserIDs)
	panelQuery.CursorLastMessageAt = cursorLastMessageAt
	panelQuery.CursorConversationID = cursorConversationID
	rawRows, err := rowsStore.ListPanelRows(ctx, panelQuery)
	if err != nil {
		return nil, err
	}
	filteredRows := filterPanelConversationRows(rawRows, request.ConversationQuery)
	pageRows, hasMore := pagePanelRows(filteredRows, request.ConversationLimit)
	nextCursor := ""
	if hasMore && len(pageRows) > 0 {
		nextCursor = EncodeConversationCursor(pageRows[len(pageRows)-1])
	}

	payload := Payload{
		"panel":         request.Panel,
		"account_name":  selectedPanelAccountName(selectedAccount, accountStats, hasSelectedAccount),
		"account_stats": accountStats,
		"summary":       summary,
		"conversations": serializeProjectionRows(pageRows),
		"conversation_page": map[string]any{
			"limit":        request.ConversationLimit,
			"has_more":     hasMore,
			"next_cursor":  nextCursor,
			"returned":     len(pageRows),
			"total":        rowInt(summary, "conversation_count"),
			"candidate_v1": true,
		},
	}
	if includeAccountStatsReady {
		payload["account_stats_ready"] = true
	}
	if request.Panel == "assignment" {
		payload["cs_users"] = service.panelCSUsers(ctx, tenantID)
	} else {
		payload["accounts"] = BuildAccountSummaryPayload(filterPanelAccounts(scopeAccounts, request.AccountQuery))
		payload["enterprises"] = buildPanelEnterprisePayload(scopeAccounts)
	}
	return payload, nil
}

func (service Service) panelRowsStore() PanelRowsStore {
	if service.Projection == nil {
		return nil
	}
	store, ok := service.Projection.(PanelRowsStore)
	if !ok {
		return nil
	}
	return store
}

func panelStatsRequest(request PanelBootstrapRequest) AccountStatsRequest {
	statusFilter := "all"
	unassignedOnly := request.UnassignedOnly
	if request.Panel == "assignment" {
		statusFilter = "pending"
		unassignedOnly = true
	}
	return AccountStatsRequest{
		Session:        request.Session,
		AssigneeID:     request.AssigneeID,
		AccountName:    request.PreferredAccountName,
		AccountKey:     request.PreferredAccountKey,
		AccountQuery:   request.AccountQuery,
		UnreadOnly:     false,
		UnassignedOnly: unassignedOnly,
		StatusFilter:   statusFilter,
	}
}

func (service Service) panelAccountStats(ctx context.Context, store AccountStatsStore, request AccountStatsRequest, scopeAccounts []AccountRecord, explicitAccountScope bool, directDeviceIDs []string, directChannelUserIDs []string, tenantID string, assigneeID string) ([]ProjectionRow, error) {
	deviceIDs, channelUserIDs := accountStatsScopeIDs(scopeAccounts)
	deviceIDs = appendUniqueStrings(deviceIDs, directDeviceIDs...)
	channelUserIDs = appendUniqueStrings(channelUserIDs, directChannelUserIDs...)
	if !explicitAccountScope && tenantID != "" {
		deviceIDs = nil
		channelUserIDs = nil
	}
	return store.ListAccountStats(ctx, AccountStatsQuery{
		DeviceIDs:                    deviceIDs,
		ChannelUserIDs:               channelUserIDs,
		WeWorkUserIDs:                channelUserIDs,
		AssigneeID:                   assigneeID,
		TenantID:                     tenantID,
		UnreadOnly:                   request.UnreadOnly,
		UnassignedOnly:               request.UnassignedOnly,
		StatusFilter:                 request.StatusFilter,
		IncludeUnassignedForAssignee: true,
	})
}

func selectPanelAccount(stats []ProjectionRow, scopeAccounts []AccountRecord, explicitAccounts []AccountRecord) (AccountRecord, bool) {
	if len(explicitAccounts) > 0 {
		return explicitAccounts[0], true
	}
	lookup := accountStatsLookup(scopeAccounts)
	for _, row := range stats {
		account := resolveProjectionStatsAccount(row, lookup)
		if !emptyAccountRecord(account) {
			return account, true
		}
	}
	if len(scopeAccounts) > 0 {
		return scopeAccounts[0], true
	}
	return AccountRecord{}, false
}

func emptyAccountRecord(account AccountRecord) bool {
	return strings.TrimSpace(account.AccountID) == "" &&
		strings.TrimSpace(account.DeviceID) == "" &&
		strings.TrimSpace(account.ChannelUserID) == "" &&
		strings.TrimSpace(account.WeWorkUserID) == "" &&
		strings.TrimSpace(account.AccountName) == ""
}

func panelRowsQuery(request PanelBootstrapRequest, selectedAccount AccountRecord, hasSelectedAccount bool, scopeAccounts []AccountRecord, tenantID string, assigneeID string, directDeviceIDs []string, directChannelUserIDs []string) PanelRowsQuery {
	deviceIDs := appendUniqueStrings(nil, directDeviceIDs...)
	channelUserIDs := appendUniqueStrings(nil, directChannelUserIDs...)
	if hasSelectedAccount {
		if deviceID := strings.TrimSpace(selectedAccount.DeviceID); deviceID != "" {
			deviceIDs = appendUniqueStrings(deviceIDs, deviceID)
		}
		channelUserIDs = appendUniqueStrings(channelUserIDs, AccountChannelCandidates(selectedAccount)...)
		if tenantID == "" {
			tenantID = strings.TrimSpace(selectedAccount.EnterpriseID)
		}
	} else if len(deviceIDs) == 0 && len(channelUserIDs) == 0 && tenantID == "" {
		deviceIDs, channelUserIDs = accountStatsScopeIDs(scopeAccounts)
	}
	statusFilter := "all"
	unassignedOnly := request.UnassignedOnly
	if request.Panel == "assignment" {
		statusFilter = "pending"
		unassignedOnly = true
	}
	return PanelRowsQuery{
		DeviceIDs:      deviceIDs,
		ChannelUserIDs: channelUserIDs,
		WeWorkUserIDs:  channelUserIDs,
		AssigneeID:     assigneeID,
		TenantID:       tenantID,
		UnassignedOnly: unassignedOnly,
		StatusFilter:   statusFilter,
		Limit:          panelRowsFetchLimit(request.ConversationLimit, request.ConversationQuery),
	}
}

func panelRowsFetchLimit(conversationLimit int, conversationQuery string) int {
	if strings.TrimSpace(conversationQuery) != "" {
		return 201
	}
	if conversationLimit >= 200 {
		return 201
	}
	return maxInt(1, conversationLimit) + 1
}

func filterPanelConversationRows(rows []ProjectionRow, query string) []ProjectionRow {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return rows
	}
	filtered := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		if panelConversationRowMatches(row, needle) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func panelConversationRowMatches(row ProjectionRow, query string) bool {
	for _, key := range []string{"conversation_id", "conversation_name", "customer_name", "sender_name", "sender_remark", "sender_id", "external_userid", "last_content"} {
		if strings.Contains(strings.ToLower(rowText(row, key)), query) {
			return true
		}
	}
	return false
}

func pagePanelRows(rows []ProjectionRow, limit int) ([]ProjectionRow, bool) {
	limit = maxInt(1, limit)
	if len(rows) <= limit {
		return rows, false
	}
	return rows[:limit], true
}

func selectedPanelAccountName(account AccountRecord, stats []ProjectionRow, hasSelectedAccount bool) string {
	if hasSelectedAccount {
		return strings.TrimSpace(account.AccountName)
	}
	if len(stats) > 0 {
		return rowText(stats[0], "account_name")
	}
	return ""
}

func filterPanelAccounts(accounts []AccountRecord, query string) []AccountRecord {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return accounts
	}
	filtered := make([]AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		if strings.Contains(strings.ToLower(strings.TrimSpace(account.AccountName)), needle) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(account.AccountID)), needle) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(account.DeviceID)), needle) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(account.WeWorkUserID)), needle) {
			filtered = append(filtered, account)
		}
	}
	return filtered
}

func buildPanelEnterprisePayload(accounts []AccountRecord) []ProjectionRow {
	type aggregate struct {
		id    string
		count int
	}
	byID := make(map[string]int)
	for _, account := range accounts {
		enterpriseID := strings.TrimSpace(account.EnterpriseID)
		if enterpriseID != "" {
			byID[enterpriseID]++
		}
	}
	aggregates := make([]aggregate, 0, len(byID))
	for enterpriseID, count := range byID {
		aggregates = append(aggregates, aggregate{id: enterpriseID, count: count})
	}
	sort.SliceStable(aggregates, func(left int, right int) bool {
		return aggregates[left].id < aggregates[right].id
	})
	payload := make([]ProjectionRow, 0, len(aggregates))
	for _, item := range aggregates {
		payload = append(payload, ProjectionRow{
			"enterprise_id": item.id,
			"name":          item.id,
			"account_count": item.count,
		})
	}
	return payload
}

func (service Service) panelCSUsers(ctx context.Context, tenantID string) []ProjectionRow {
	if service.CSUsers == nil {
		return []ProjectionRow{}
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return []ProjectionRow{}
	}
	counts := map[string]int{}
	if store := service.assignmentCountStore(); store != nil {
		assigneeIDs := make([]string, 0, len(users))
		for _, user := range users {
			assigneeIDs = appendUniqueStrings(assigneeIDs, user.AssigneeID)
		}
		if loaded, err := service.assignmentLoadMap(ctx, store, assigneeIDs, tenantID); err == nil {
			counts = loaded
		}
	}
	payload := make([]ProjectionRow, 0, len(users))
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		if assigneeID == "" {
			continue
		}
		payload = append(payload, ProjectionRow{
			"assignee_id":      assigneeID,
			"assignee_name":    strings.TrimSpace(user.AssigneeName),
			"enabled":          user.Enabled,
			"role":             strings.TrimSpace(user.Role),
			"max_sessions":     user.MaxSessions,
			"current_sessions": counts[assigneeID],
			"last_seen_at":     nilIfBlank(anyText(user.LastSeenAt)),
		})
	}
	sort.SliceStable(payload, func(left int, right int) bool {
		leftEnabled := rowBool(payload[left], "enabled")
		rightEnabled := rowBool(payload[right], "enabled")
		if leftEnabled != rightEnabled {
			return leftEnabled
		}
		leftSessions := rowInt(payload[left], "current_sessions")
		rightSessions := rowInt(payload[right], "current_sessions")
		if leftSessions != rightSessions {
			return leftSessions < rightSessions
		}
		return rowText(payload[left], "assignee_name") < rowText(payload[right], "assignee_name")
	})
	return payload
}

func (service Service) assignmentCountStore() AssignmentCountStore {
	if service.Assignments == nil {
		return nil
	}
	store, ok := service.Assignments.(AssignmentCountStore)
	if !ok {
		return nil
	}
	return store
}

func panelSessionClaim(request PanelBootstrapRequest, key string) string {
	return sessionClaim(BootstrapRequest{Session: request.Session}, key)
}

func panelHasSessionClaim(request PanelBootstrapRequest, key string) bool {
	return hasSessionClaim(BootstrapRequest{Session: request.Session}, key)
}

func anyText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
