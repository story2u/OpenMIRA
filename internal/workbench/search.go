// Workbench search composes account scope with projection search results.
// This candidate only covers conversation/contact fields in the projection;
// ES, message-body search, friend events, and Redis caching remain separate.
package workbench

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

const searchResultBuildLimit = 80

// Search builds the current projection-backed CS workbench search candidate.
func (service Service) Search(ctx context.Context, request SearchRequest) (Payload, error) {
	pageStart, err := searchPageStart(request.Cursor)
	if err != nil {
		return nil, err
	}
	pageLimit := maxInt(1, request.Limit)
	keyword := strings.TrimSpace(request.Keyword)
	if keyword == "" {
		return emptySearchPayload(pageLimit, pageStart), nil
	}
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	searcher, ok := service.Projection.(ProjectionSearchStore)
	if !ok {
		return nil, ErrProjectionStoreUnavailable
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
	if scope.AssignedSessions {
		return emptySearchPayload(pageLimit, pageStart), nil
	}
	assigneeID := strings.TrimSpace(request.Session.AssigneeID)
	if scope.SelectedAccount != nil && strings.TrimSpace(scope.SelectedAccount.AssigneeID) == assigneeID {
		assigneeID = ""
	}
	rows, err := searcher.SearchRows(ctx, ProjectionSearchQuery{
		Keyword:       keyword,
		DeviceIDs:     DeviceIDsForAccounts(scope.Accounts),
		WeWorkUserIDs: scope.WeWorkUserIDs,
		AssigneeID:    assigneeID,
		TenantID:      scope.TenantID,
		ModeFilter:    projectionModeFilter(request.ModeFilter, scope),
		StatusFilter:  request.StatusFilter,
		Limit:         searchResultBuildLimit,
	})
	if err != nil {
		return nil, err
	}
	filteredRows := FilterRowsByWorkbenchFilters(ApplyAccountAIEnabledToRows(rows, scope.Accounts), request.ModeFilter, request.StatusFilter)
	sort.SliceStable(filteredRows, func(left, right int) bool {
		leftTime := rowText(filteredRows[left], "last_message_at")
		rightTime := rowText(filteredRows[right], "last_message_at")
		if leftTime != rightTime {
			return leftTime > rightTime
		}
		return rowText(filteredRows[left], "conversation_id") < rowText(filteredRows[right], "conversation_id")
	})
	payloadRows := serializeProjectionRows(filteredRows)
	for _, row := range payloadRows {
		row["search_kind"] = "conversation"
		row["has_history"] = true
	}
	return searchPayloadFromRows(payloadRows, pageLimit, pageStart), nil
}

func searchPageStart(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(cursor)
	if err != nil {
		return 0, ErrInvalidSearchCursor
	}
	return maxInt(0, value), nil
}

func emptySearchPayload(limit int, cursor int) Payload {
	return Payload{
		"results":     []ProjectionRow{},
		"has_more":    false,
		"next_cursor": "",
		"search_page": map[string]any{
			"limit":       limit,
			"cursor":      strconv.Itoa(cursor),
			"next_cursor": "",
			"has_more":    false,
			"returned":    0,
		},
	}
}

func searchPayloadFromRows(rows []ProjectionRow, limit int, cursor int) Payload {
	if cursor > len(rows) {
		cursor = len(rows)
	}
	end := cursor + limit
	if end > len(rows) {
		end = len(rows)
	}
	pagedRows := rows[cursor:end]
	hasMore := end < len(rows)
	nextCursor := ""
	if hasMore {
		nextCursor = strconv.Itoa(end)
	}
	return Payload{
		"results":     pagedRows,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
		"search_page": map[string]any{
			"limit":       limit,
			"cursor":      strconv.Itoa(cursor),
			"next_cursor": nextCursor,
			"has_more":    hasMore,
			"returned":    len(pagedRows),
			"total":       len(rows),
		},
	}
}
