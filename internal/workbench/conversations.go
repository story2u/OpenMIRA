// Workbench conversation paging reuses the bootstrap projection pipeline.
// It returns the legacy cold-page payload shape without account/device hydrate.
package workbench

import (
	"context"
	"strings"
)

// Conversations builds the current projection-backed cold page candidate payload.
func (service Service) Conversations(ctx context.Context, request ConversationsRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	cursorLastMessageAt, cursorConversationID, err := DecodeConversationCursor(request.ConversationCursor)
	if err != nil {
		return nil, err
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := ResolveAccountScope(AccountScopeInput{
		AllAccounts:                accounts,
		AssigneeID:                 request.Session.AssigneeID,
		SelectedAccountID:          request.SelectedAccountID,
		SessionTenantID:            sessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id"),
		SessionOrganizationName:    sessionClaim(BootstrapRequest{Session: request.Session}, "organization_name"),
		HasExplicitSessionTenant:   hasSessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id"),
		HasExplicitOrganizationKey: hasSessionClaim(BootstrapRequest{Session: request.Session}, "organization_name"),
	})
	if err != nil {
		return nil, err
	}
	assignedConversationIDs, err := service.assignedConversationIDs(ctx, BootstrapRequest{
		Session:           request.Session,
		SelectedAccountID: request.SelectedAccountID,
		ModeFilter:        request.ModeFilter,
		StatusFilter:      request.StatusFilter,
	}, scope)
	if err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(request.ConversationID)
	query := service.projectionQuery(BootstrapRequest{
		Session:           request.Session,
		SelectedAccountID: request.SelectedAccountID,
		ModeFilter:        request.ModeFilter,
		StatusFilter:      request.StatusFilter,
	}, scope, request.ConversationLimit, assignedConversationIDs)
	query.CursorLastMessageAt = cursorLastMessageAt
	query.CursorConversationID = cursorConversationID
	if conversationID != "" {
		query.ConversationIDs = []string{conversationID}
		query.CursorLastMessageAt = nil
		query.CursorConversationID = ""
		query.Limit = 1
	}

	rows := []ProjectionRow{}
	canListRows := !scope.AssignedSessions || len(assignedConversationIDs) > 0
	if conversationID != "" && scope.AssignedSessions {
		canListRows = containsTrimmedString(assignedConversationIDs, conversationID)
	}
	if canListRows {
		rows, err = service.Projection.ListRows(ctx, query)
		if err != nil {
			return nil, err
		}
	}
	scopedRows := ApplyAccountAIEnabledToRows(rows, scope.Accounts)
	filteredRows := FilterRowsByWorkbenchFilters(scopedRows, request.ModeFilter, request.StatusFilter)
	stats := statsFromRows(filteredRows, scope, request.Session.AssigneeID)
	if service.canTrustCountScoped(BootstrapRequest{ModeFilter: request.ModeFilter}, scope) {
		if counted, err := service.Projection.CountScoped(ctx, query); err == nil {
			stats = counted
		}
	}
	pendingCount := stats.ConversationCount
	if !strings.EqualFold(request.StatusFilter, "pending") {
		pendingCount = service.pendingCount(ctx, query, scopedRows, BootstrapRequest{ModeFilter: request.ModeFilter}, scope)
	}
	sensitiveCount := service.sensitiveCount(ctx, query, scopedRows, scope)
	nextCursor := ""
	pageRows := filteredRows
	hasMore := false
	coldTotal := len(filteredRows)
	if conversationID == "" {
		_, _, coldRows := ResolvePriorityLayers(scope.SelectedAccountKey, filteredRows, request.StatusFilter, service.hotLimit(), service.warmLimit())
		pageRows = coldRows
		if len(pageRows) > request.ConversationLimit {
			pageRows = pageRows[:request.ConversationLimit]
		}
		hasMore = len(coldRows) > len(pageRows)
		coldTotal = len(coldRows)
		if hasMore && len(pageRows) > 0 {
			nextCursor = EncodeConversationCursor(pageRows[len(pageRows)-1])
		}
	} else if len(pageRows) > 1 {
		pageRows = pageRows[:1]
		hasMore = true
	}
	return Payload{
		"conversations": serializeProjectionRows(pageRows),
		"conversation_page": map[string]any{
			"has_more":     hasMore,
			"next_cursor":  nextCursor,
			"returned":     len(pageRows),
			"cold_total":   coldTotal,
			"total":        stats.ConversationCount,
			"candidate_v1": true,
		},
		"summary": map[string]any{
			"account_count":            len(scope.Accounts),
			"conversation_count":       stats.ConversationCount,
			"unread_count":             stats.UnreadCount,
			"assigned_count":           stats.AssignedCount,
			"pending_reply_count":      pendingCount,
			"sensitive_handoff_count":  sensitiveCount,
			"projection_candidate_v1":  true,
			"requires_payload_hydrate": true,
		},
	}, nil
}

func containsTrimmedString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

// EncodeConversationCursor builds the legacy last_message_at|conversation_id cursor.
func EncodeConversationCursor(row ProjectionRow) string {
	lastMessageAt := strings.ReplaceAll(rowText(row, "last_message_at"), "T", " ")
	conversationID := rowText(row, "conversation_id")
	if lastMessageAt == "" || conversationID == "" {
		return ""
	}
	return lastMessageAt + "|" + conversationID
}
