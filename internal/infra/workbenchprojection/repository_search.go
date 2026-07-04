// Projection search is isolated from list/count SQL to keep request-time
// search scope explicit and prevent message-body fallback scans from creeping
// into the workbench candidate repository.
package workbenchprojection

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"im-go/internal/workbench"
)

// SearchRows searches scoped projection contact fields without touching messages.content.
func (repository *Repository) SearchRows(ctx context.Context, query workbench.ProjectionSearchQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	normalized := normalizeSearchQuery(query)
	if normalized.keyword == "" {
		return []workbench.ProjectionRow{}, nil
	}
	if !normalized.hasScope() {
		return nil, ErrScopeRequired
	}
	merged := map[string]workbench.ProjectionRow{}
	if err := repository.runSearchPattern(ctx, normalized, normalized.keyword+"%", merged); err != nil {
		return nil, err
	}
	if len(merged) == 0 {
		if err := repository.runSearchPattern(ctx, normalized, "%"+normalized.keyword+"%", merged); err != nil {
			return nil, err
		}
	}
	result := make([]workbench.ProjectionRow, 0, len(merged))
	for _, row := range merged {
		result = append(result, row)
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftWeight := intFromDB(result[left]["match_weight"])
		rightWeight := intFromDB(result[right]["match_weight"])
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
		leftTime := strings.TrimSpace(fmt.Sprint(result[left]["last_message_at"]))
		rightTime := strings.TrimSpace(fmt.Sprint(result[right]["last_message_at"]))
		if leftTime != rightTime {
			return leftTime < rightTime
		}
		return strings.TrimSpace(fmt.Sprint(result[left]["conversation_id"])) < strings.TrimSpace(fmt.Sprint(result[right]["conversation_id"]))
	})
	if len(result) > normalized.limit {
		result = result[:normalized.limit]
	}
	return result, nil
}

func (repository *Repository) runSearchPattern(ctx context.Context, query normalizedSearchQuery, pattern string, merged map[string]workbench.ProjectionRow) error {
	for _, field := range searchFields {
		clauses, args := buildSearchBaseClauses(query)
		clauses = append(clauses, "COALESCE("+field.name+", '') != ''")
		clauses = append(clauses, "COALESCE("+field.name+", '') LIKE ?")
		args = append(args, pattern)
		sqlText := "SELECT * FROM conversation_overview_projection WHERE " + strings.Join(clauses, " AND ") + " ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?"
		args = append(args, query.limit)
		rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return err
		}
		result, scanErr := scanProjectionRows(rows)
		closeErr := rows.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, row := range result {
			conversationID := strings.TrimSpace(fmt.Sprint(row["conversation_id"]))
			if conversationID == "" {
				continue
			}
			current, exists := merged[conversationID]
			if !exists || field.weight > intFromDB(current["match_weight"]) {
				row["match_weight"] = field.weight
				merged[conversationID] = row
			}
		}
	}
	return nil
}

type normalizedSearchQuery struct {
	keyword       string
	deviceIDs     []string
	weworkUserIDs []string
	assigneeID    string
	tenantID      string
	modeFilter    string
	statusFilter  string
	limit         int
}

type searchField struct {
	name   string
	weight int
}

var searchFields = []searchField{
	{name: "customer_name", weight: 5},
	{name: "sender_remark", weight: 4},
	{name: "sender_name", weight: 3},
}

func normalizeSearchQuery(query workbench.ProjectionSearchQuery) normalizedSearchQuery {
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	return normalizedSearchQuery{
		keyword:       strings.TrimSpace(query.Keyword),
		deviceIDs:     normalizeStrings(query.DeviceIDs),
		weworkUserIDs: normalizeChannelScopeIDs(query.ChannelUserIDs, query.WeWorkUserIDs),
		assigneeID:    strings.TrimSpace(query.AssigneeID),
		tenantID:      strings.TrimSpace(query.TenantID),
		modeFilter:    defaultLower(query.ModeFilter, "all"),
		statusFilter:  defaultLower(query.StatusFilter, "all"),
		limit:         limit,
	}
}

func (query normalizedSearchQuery) hasScope() bool {
	return len(query.deviceIDs) > 0 ||
		len(query.weworkUserIDs) > 0 ||
		query.assigneeID != "" ||
		query.tenantID != ""
}

func buildSearchBaseClauses(query normalizedSearchQuery) ([]string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if query.tenantID != "" && (len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0) {
		clauses = append(clauses, "(tenant_id = ? OR tenant_id = '')")
		args = append(args, query.tenantID)
	} else if query.tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, query.tenantID)
	}

	scope := scopeClauses(query.deviceIDs, query.weworkUserIDs, &args)
	if len(scope) > 0 {
		scopeSQL := "(" + strings.Join(scope, " OR ") + ")"
		if query.assigneeID != "" {
			scopeSQL = "(" + scopeSQL + " OR assignee_id = ?)"
			args = append(args, query.assigneeID)
		}
		clauses = append(clauses, scopeSQL)
	} else if query.assigneeID != "" {
		clauses = append(clauses, "(assignee_id = ? OR assignee_id = '' OR assignee_id IS NULL)")
		args = append(args, query.assigneeID)
	}
	clauses = append(clauses, buildStateFilterClauses(query.modeFilter, query.statusFilter)...)
	return clauses, args
}
