// Panel row SQL joins current assignment facts before returning projection rows.
// This avoids treating stale projection assignee fields as the source of truth
// for management assignment panels.
package workbenchprojection

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/workbench"
)

// ListPanelRows returns bounded projection rows for management panel bootstrap.
func (repository *Repository) ListPanelRows(ctx context.Context, query workbench.PanelRowsQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	normalized := normalizePanelRowsQuery(query)
	if !normalized.hasScope() {
		return nil, ErrScopeRequired
	}
	sqlText, args := repository.panelRowsSQL(normalized)
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

func (repository *Repository) panelRowsSQL(query normalizedPanelRowsQuery) (string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if query.tenantID != "" {
		clauses = append(clauses, "p.tenant_id = ?")
		args = append(args, query.tenantID)
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
		clauses = append(clauses, "(ca.assignee_id = ? OR ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')")
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
	if query.cursorLastMessageAt != "" && query.cursorConversationID != "" {
		clauses = append(clauses, "((p.last_message_at < ?) OR (p.last_message_at = ? AND p.conversation_id > ?))")
		args = append(args, query.cursorLastMessageAt, query.cursorLastMessageAt, query.cursorConversationID)
	}
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlText := "SELECT p.* FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id" + whereSQL + " ORDER BY p.last_message_at DESC, p.conversation_id ASC LIMIT ?"
	args = append(args, query.limit)
	return sqlText, args
}

type normalizedPanelRowsQuery struct {
	deviceIDs            []string
	weworkUserIDs        []string
	assigneeID           string
	tenantID             string
	cursorLastMessageAt  string
	cursorConversationID string
	unassignedOnly       bool
	statusFilter         string
	limit                int
}

func normalizePanelRowsQuery(query workbench.PanelRowsQuery) normalizedPanelRowsQuery {
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	if limit > 201 {
		limit = 201
	}
	return normalizedPanelRowsQuery{
		deviceIDs:            normalizeStrings(query.DeviceIDs),
		weworkUserIDs:        normalizeStrings(query.WeWorkUserIDs),
		assigneeID:           strings.TrimSpace(query.AssigneeID),
		tenantID:             strings.TrimSpace(query.TenantID),
		cursorLastMessageAt:  panelCursorText(query.CursorLastMessageAt),
		cursorConversationID: strings.TrimSpace(query.CursorConversationID),
		unassignedOnly:       query.UnassignedOnly,
		statusFilter:         defaultLower(query.StatusFilter, "all"),
		limit:                limit,
	}
}

func (query normalizedPanelRowsQuery) hasScope() bool {
	if len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0 || query.tenantID != "" {
		return true
	}
	return query.assigneeID != "" && !query.unassignedOnly
}

func panelCursorText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
