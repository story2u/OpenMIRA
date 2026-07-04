// Account stats SQL stays separate from row pagination SQL because unread here
// means pending conversations. Keeping the aggregate builder isolated makes the
// phase-three harness catch regressions before the route is enabled.
package workbenchprojection

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/workbench"
)

const accountStatsPendingClause = "(COALESCE(p.last_direction, '') = 'incoming' OR (COALESCE(p.last_direction, '') = '' AND p.last_incoming_at IS NOT NULL AND (p.last_message_at IS NULL OR p.last_message_at = p.last_incoming_at)))"

// ListAccountStats returns projection account aggregates for management panels.
func (repository *Repository) ListAccountStats(ctx context.Context, query workbench.AccountStatsQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	normalized := normalizeAccountStatsQuery(query)
	if !normalized.hasScope() {
		return nil, ErrScopeRequired
	}

	sqlText, args := repository.accountStatsSQL(normalized)
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := scanProjectionRows(rows)
	if err != nil {
		return nil, err
	}
	return result, rows.Err()
}

func (repository *Repository) accountStatsSQL(query normalizedAccountStatsQuery) (string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if query.tenantID != "" {
		clauses = append(clauses, "p.tenant_id = ?")
		args = append(args, query.tenantID)
	}
	if query.unreadOnly {
		clauses = append(clauses, "COALESCE(p.unread_count, 0) > 0")
	}
	switch query.statusFilter {
	case "pending":
		clauses = append(clauses, accountStatsPendingClause)
	case "replied":
		clauses = append(clauses, "NOT "+accountStatsPendingClause)
	case "unread":
		clauses = append(clauses, "COALESCE(p.unread_count, 0) > 0")
	}
	if query.unassignedOnly {
		clauses = append(clauses, "(ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')")
	} else if query.assigneeID != "" {
		if query.includeUnassignedForAssignee {
			clauses = append(clauses, "(ca.assignee_id = ? OR ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')")
		} else {
			clauses = append(clauses, "ca.assignee_id = ?")
		}
		args = append(args, query.assigneeID)
	}
	scopeClauses := make([]string, 0)
	if len(query.deviceIDs) > 0 {
		scopeClauses = append(scopeClauses, "p.device_id IN ("+placeholders(len(query.deviceIDs))+")")
		args = append(args, stringsToAny(query.deviceIDs)...)
	}
	if len(query.weworkUserIDs) > 0 {
		scopeClauses = append(scopeClauses, "p.wework_user_id IN ("+placeholders(len(query.weworkUserIDs))+")")
		args = append(args, stringsToAny(query.weworkUserIDs)...)
	}
	if len(scopeClauses) > 0 {
		clauses = append(clauses, "("+strings.Join(scopeClauses, " OR ")+")")
	}
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlText := "SELECT COALESCE(p.wework_user_id, '') AS wework_user_id, COALESCE(p.device_id, '') AS device_id, COUNT(*) AS total, SUM(CASE WHEN " + accountStatsPendingClause + " THEN 1 ELSE 0 END) AS unread, SUM(CASE WHEN (ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '') AND " + accountStatsPendingClause + " THEN 1 ELSE 0 END) AS unassigned_unread, 0 AS max_pending, MAX(COALESCE(p.last_message_at, p.updated_at)) AS last_message_at FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id" + whereSQL + " GROUP BY COALESCE(p.wework_user_id, ''), COALESCE(p.device_id, '') ORDER BY unread DESC, total DESC, last_message_at DESC"
	return sqlText, args
}

type normalizedAccountStatsQuery struct {
	deviceIDs                    []string
	weworkUserIDs                []string
	assigneeID                   string
	tenantID                     string
	unreadOnly                   bool
	unassignedOnly               bool
	statusFilter                 string
	includeUnassignedForAssignee bool
}

func normalizeAccountStatsQuery(query workbench.AccountStatsQuery) normalizedAccountStatsQuery {
	return normalizedAccountStatsQuery{
		deviceIDs:                    normalizeStrings(query.DeviceIDs),
		weworkUserIDs:                normalizeStrings(query.WeWorkUserIDs),
		assigneeID:                   strings.TrimSpace(query.AssigneeID),
		tenantID:                     strings.TrimSpace(query.TenantID),
		unreadOnly:                   query.UnreadOnly,
		unassignedOnly:               query.UnassignedOnly,
		statusFilter:                 defaultLower(query.StatusFilter, "all"),
		includeUnassignedForAssignee: query.IncludeUnassignedForAssignee,
	}
}

func (query normalizedAccountStatsQuery) hasScope() bool {
	return len(query.deviceIDs) > 0 ||
		len(query.weworkUserIDs) > 0 ||
		query.assigneeID != "" ||
		query.tenantID != ""
}
