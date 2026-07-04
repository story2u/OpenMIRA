package workbenchprojection

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/workbench"
)

// ListAutoAssignCandidates returns unassigned unread projection rows for bulk assignment.
func (repository *Repository) ListAutoAssignCandidates(ctx context.Context, tenantID string, limit int) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	normalizedTenantID := strings.TrimSpace(tenantID)
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}
	if normalizedLimit > 1000 {
		normalizedLimit = 1000
	}
	clauses := []string{
		"(ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"COALESCE(p.unread_count, 0) > 0",
	}
	args := make([]any, 0, 2)
	if normalizedTenantID != "" {
		clauses = append(clauses, "(p.tenant_id = ? OR p.tenant_id = '')")
		args = append(args, normalizedTenantID)
	}
	sqlText := "SELECT p.* FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id WHERE " + strings.Join(clauses, " AND ") + " ORDER BY p.last_message_at DESC, p.conversation_id ASC LIMIT ?"
	args = append(args, normalizedLimit)
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
