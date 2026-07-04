// Package workbenchassignments reads current conversation assignment rows.
// Assigned-sessions scope still hydrates final conversations from projection,
// while phase-four assignment list/detail routes return assignment records.
package workbenchassignments

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the assignment repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads assigned conversation ids for CS workbench scope.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// GetAssignment returns one assignment record by conversation id.
func (repository *Repository) GetAssignment(ctx context.Context, conversationID string, tenantID string) (*workbench.AssignmentRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench assignment database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, nil
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT tenant_id, conversation_id, assignee_id, assignee_name, assigned_at, updated_at FROM conversation_assignments WHERE conversation_id = ?", conversationID)
	if err != nil {
		return nil, err
	}
	records, err := scanAssignmentRecords(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	if normalizedTenant := strings.TrimSpace(tenantID); normalizedTenant != "" && records[0].TenantID != "" && records[0].TenantID != normalizedTenant {
		return nil, nil
	}
	return &records[0], nil
}

// ListAssignmentsByAssignee returns bounded assignment records by assignee.
func (repository *Repository) ListAssignmentsByAssignee(ctx context.Context, assigneeID string, tenantID string, limit int) ([]workbench.AssignmentRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench assignment database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return []workbench.AssignmentRecord{}, nil
	}
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 200
	}
	args := []any{assigneeID}
	query := "SELECT tenant_id, conversation_id, assignee_id, assignee_name, assigned_at, updated_at FROM conversation_assignments WHERE assignee_id = ?"
	if strings.TrimSpace(tenantID) != "" {
		query += " AND tenant_id = ?"
		args = append(args, strings.TrimSpace(tenantID))
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, normalizedLimit)
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanAssignmentRecords(rows, normalizedLimit)
}

// ListAssignedConversationIDs returns bounded assignment ids ordered by recency.
func (repository *Repository) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench assignment database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return []string{}, nil
	}
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 10000
	}
	args := []any{assigneeID}
	query := "SELECT conversation_id FROM conversation_assignments WHERE assignee_id = ?"
	if strings.TrimSpace(tenantID) != "" {
		query += " AND tenant_id = ?"
		args = append(args, strings.TrimSpace(tenantID))
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, normalizedLimit)
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversationIDs := make([]string, 0)
	seen := make(map[string]bool)
	for rows.Next() {
		var conversationID any
		if err := rows.Scan(&conversationID); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(conversationID)
		if normalizedID == "" || seen[normalizedID] {
			continue
		}
		seen[normalizedID] = true
		conversationIDs = append(conversationIDs, normalizedID)
	}
	return conversationIDs, rows.Err()
}

// CountByAssigneeIDs returns current assignment counts for visible CS users.
func (repository *Repository) CountByAssigneeIDs(ctx context.Context, assigneeIDs []string, tenantID string) (map[string]int, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench assignment database is not configured")
	}
	normalizedIDs := normalizeAssigneeIDs(assigneeIDs)
	if len(normalizedIDs) == 0 {
		return map[string]int{}, nil
	}
	args := stringsToAny(normalizedIDs)
	query := "SELECT assignee_id, COUNT(*) FROM conversation_assignments WHERE assignee_id IN (" + placeholders(len(normalizedIDs)) + ")"
	if strings.TrimSpace(tenantID) != "" {
		query += " AND tenant_id = ?"
		args = append(args, strings.TrimSpace(tenantID))
	}
	query += " GROUP BY assignee_id"
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var assigneeID any
		var count any
		if err := rows.Scan(&assigneeID, &count); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(assigneeID)
		if normalizedID != "" {
			counts[normalizedID] = intFromDB(count)
		}
	}
	return counts, rows.Err()
}

// ClaimAssignment creates or updates one conversation assignment.
func (repository *Repository) ClaimAssignment(ctx context.Context, command workbench.AssignmentClaimCommand) (workbench.AssignmentRecord, error) {
	if repository.DB == nil {
		return workbench.AssignmentRecord{}, fmt.Errorf("workbench assignment database is not configured")
	}
	conversationID := strings.TrimSpace(command.ConversationID)
	assigneeID := strings.TrimSpace(command.AssigneeID)
	if conversationID == "" || assigneeID == "" {
		return workbench.AssignmentRecord{}, workbench.AssignmentConflictError{Detail: "conversation_id and assignee_id are required"}
	}
	tenantID := strings.TrimSpace(command.TenantID)
	current, err := repository.GetAssignment(ctx, conversationID, "")
	if err != nil {
		return workbench.AssignmentRecord{}, err
	}
	if current != nil && tenantID != "" && current.TenantID != "" && current.TenantID != tenantID {
		return workbench.AssignmentRecord{}, workbench.AssignmentConflictError{Detail: "conversation assigned under another tenant"}
	}
	if current != nil && strings.TrimSpace(current.AssigneeID) != assigneeID && !command.Force {
		return workbench.AssignmentRecord{}, workbench.AssignmentConflictError{Detail: "conversation already assigned"}
	}
	if current == nil {
		if _, err := repository.DB.ExecContext(
			ctx,
			"INSERT INTO conversation_assignments (conversation_id, tenant_id, assignee_id, assignee_name, assigned_at, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
			conversationID,
			tenantID,
			assigneeID,
			strings.TrimSpace(command.AssigneeName),
		); err != nil {
			return workbench.AssignmentRecord{}, err
		}
	} else {
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversation_assignments SET tenant_id = CASE WHEN ? != '' THEN ? ELSE tenant_id END, assignee_id = ?, assignee_name = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?",
			tenantID,
			tenantID,
			assigneeID,
			strings.TrimSpace(command.AssigneeName),
			conversationID,
		); err != nil {
			return workbench.AssignmentRecord{}, err
		}
	}
	updated, err := repository.GetAssignment(ctx, conversationID, tenantID)
	if err != nil {
		return workbench.AssignmentRecord{}, err
	}
	if updated == nil {
		return workbench.AssignmentRecord{}, workbench.AssignmentConflictError{Detail: "conversation assignment was not saved"}
	}
	if err := repository.updateProjectionAssignment(ctx, *updated); err != nil {
		return workbench.AssignmentRecord{}, err
	}
	return *updated, nil
}

// ReleaseAssignment deletes one conversation assignment when scope checks pass.
func (repository *Repository) ReleaseAssignment(ctx context.Context, command workbench.AssignmentReleaseCommand) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench assignment database is not configured")
	}
	conversationID := strings.TrimSpace(command.ConversationID)
	if conversationID == "" {
		return false, nil
	}
	current, err := repository.GetAssignment(ctx, conversationID, "")
	if err != nil || current == nil {
		return false, err
	}
	tenantID := strings.TrimSpace(command.TenantID)
	if tenantID != "" && current.TenantID != "" && current.TenantID != tenantID {
		return false, nil
	}
	assigneeID := strings.TrimSpace(command.AssigneeID)
	if assigneeID != "" && strings.TrimSpace(current.AssigneeID) != assigneeID && !command.Force {
		return false, workbench.AssignmentConflictError{Detail: "conversation assigned to another assignee"}
	}
	if _, err := repository.DB.ExecContext(ctx, "DELETE FROM conversation_assignments WHERE conversation_id = ?", conversationID); err != nil {
		return false, err
	}
	released := workbench.AssignmentRecord{ConversationID: conversationID, TenantID: firstNonBlank(tenantID, current.TenantID)}
	if err := repository.updateProjectionAssignment(ctx, released); err != nil {
		return false, err
	}
	return true, nil
}

// PurgeAssignments deletes assignments and clears projection assignee fields.
func (repository *Repository) PurgeAssignments(ctx context.Context, tenantID string) (workbench.AssignmentPurgeResult, error) {
	if repository.DB == nil {
		return workbench.AssignmentPurgeResult{}, fmt.Errorf("workbench assignment database is not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	var deleteResult sql.Result
	var err error
	if tenantID != "" {
		deleteResult, err = repository.DB.ExecContext(ctx, "DELETE FROM conversation_assignments WHERE tenant_id = ?", tenantID)
	} else {
		deleteResult, err = repository.DB.ExecContext(ctx, "DELETE FROM conversation_assignments")
	}
	if err != nil {
		return workbench.AssignmentPurgeResult{}, err
	}
	deleted, _ := deleteResult.RowsAffected()
	cleared, err := repository.clearProjectionAssignments(ctx, tenantID)
	if err != nil {
		return workbench.AssignmentPurgeResult{}, err
	}
	return workbench.AssignmentPurgeResult{Deleted: int(deleted), ClearedProjection: cleared}, nil
}

func (repository *Repository) updateProjectionAssignment(ctx context.Context, record workbench.AssignmentRecord) error {
	_, err := repository.DB.ExecContext(
		ctx,
		"UPDATE conversation_overview_projection SET assignee_id = ?, assignee_name = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?",
		strings.TrimSpace(record.AssigneeID),
		strings.TrimSpace(record.AssigneeName),
		strings.TrimSpace(record.ConversationID),
	)
	return err
}

func (repository *Repository) clearProjectionAssignments(ctx context.Context, tenantID string) (int, error) {
	var result sql.Result
	var err error
	if tenantID != "" {
		result, err = repository.DB.ExecContext(
			ctx,
			"UPDATE conversation_overview_projection SET assignee_id = '', assignee_name = '', updated_at = CURRENT_TIMESTAMP WHERE tenant_id = ? AND (COALESCE(assignee_id, '') != '' OR COALESCE(assignee_name, '') != '')",
			tenantID,
		)
	} else {
		result, err = repository.DB.ExecContext(
			ctx,
			"UPDATE conversation_overview_projection SET assignee_id = '', assignee_name = '', updated_at = CURRENT_TIMESTAMP WHERE COALESCE(assignee_id, '') != '' OR COALESCE(assignee_name, '') != ''",
		)
	}
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// scanAssignmentRecords converts SQL rows into assignment records.
func scanAssignmentRecords(rows RowsScanner, capacity int) ([]workbench.AssignmentRecord, error) {
	defer rows.Close()
	records := make([]workbench.AssignmentRecord, 0, capacity)
	for rows.Next() {
		var tenantID any
		var conversationID any
		var assigneeID any
		var assigneeName any
		var assignedAt any
		var updatedAt any
		if err := rows.Scan(&tenantID, &conversationID, &assigneeID, &assigneeName, &assignedAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedConversationID := stringFromDB(conversationID)
		if normalizedConversationID == "" {
			continue
		}
		records = append(records, workbench.AssignmentRecord{
			TenantID:       stringFromDB(tenantID),
			ConversationID: normalizedConversationID,
			AssigneeID:     stringFromDB(assigneeID),
			AssigneeName:   stringFromDB(assigneeName),
			AssignedAt:     timeFromDB(assignedAt),
			UpdatedAt:      timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func normalizeAssigneeIDs(values []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ",")
}

func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
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

// stringFromDB converts SQL scalar values into trimmed strings.
func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(typed)), "%d", &parsed)
		return parsed
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(typed)), "%d", &parsed)
		return parsed
	}
}

// timeFromDB converts SQL timestamp values into JSON-friendly strings.
func timeFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
