// Package workbenchprojection reads the conversation overview projection.
// The repository keeps SQL construction isolated from workbench services so the
// bootstrap path can prove scope, pagination, and filter behavior first.
package workbenchprojection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"im-go/internal/workbench"
)

// ErrScopeRequired prevents accidental full projection scans from Go candidates.
var ErrScopeRequired = errors.New("workbench projection scope is required")

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the projection repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Repository reads conversation_overview_projection rows and scoped counts.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// ListRows reads scoped projection rows for workbench views.
func (repository *Repository) ListRows(ctx context.Context, query workbench.ProjectionQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	normalized := normalizeQuery(query)
	if !normalized.hasScope() {
		return nil, ErrScopeRequired
	}

	sqlText, args := repository.listRowsSQL(normalized)
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

// CountScoped returns count_scoped-compatible totals for the current scope.
func (repository *Repository) CountScoped(ctx context.Context, query workbench.ProjectionQuery) (workbench.ProjectionStats, error) {
	if repository.DB == nil {
		return workbench.ProjectionStats{}, fmt.Errorf("workbench projection database is not configured")
	}
	normalized := normalizeQuery(query)
	if !normalized.hasScope() {
		return workbench.ProjectionStats{}, ErrScopeRequired
	}

	sqlText, args := repository.countScopedSQL(normalized)
	var total any
	var unread any
	var assigned any
	err := repository.DB.QueryRowContext(ctx, sqlText, args...).Scan(&total, &unread, &assigned)
	if err != nil {
		return workbench.ProjectionStats{}, err
	}
	return workbench.ProjectionStats{
		ConversationCount: intFromDB(total),
		UnreadCount:       intFromDB(unread),
		AssignedCount:     intFromDB(assigned),
	}, nil
}

// ListConversationRows returns the bounded /api/v1/conversations scan.
func (repository *Repository) ListConversationRows(ctx context.Context, query workbench.ConversationListQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench projection database is not configured")
	}
	sqlText, args := conversationListSQL(query)
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

func (repository *Repository) listRowsSQL(query normalizedQuery) (string, []any) {
	args := make([]any, 0)
	stateClauses := buildStateFilterClauses(query.modeFilter, query.statusFilter)

	appendKeyset := func(sqlText string, hasWhere bool) string {
		if !hasCursor(query.cursorLastMessageAt) {
			return sqlText
		}
		prefix := " WHERE "
		if hasWhere {
			prefix = " AND "
		}
		args = append(args, query.cursorLastMessageAt, query.cursorLastMessageAt, query.cursorConversationID)
		return sqlText + prefix + "((last_message_at < ?) OR (last_message_at = ? AND conversation_id > ?))"
	}

	if len(query.conversationIDs) > 0 {
		sqlText := "SELECT * FROM conversation_overview_projection WHERE conversation_id IN (" + placeholders(len(query.conversationIDs)) + ")"
		args = append(args, stringsToAny(query.conversationIDs)...)
		if query.tenantID != "" {
			sqlText += " AND tenant_id = ?"
			args = append(args, query.tenantID)
		}
		if len(stateClauses) > 0 {
			sqlText += " AND " + strings.Join(stateClauses, " AND ")
		}
		sqlText = appendKeyset(sqlText, true)
		sqlText += " ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?"
		args = append(args, query.limit)
		return sqlText, args
	}

	if (len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0) && query.assigneeID != "" {
		accountScopeSQL, accountScopeArgs := buildAccountScopeQuery(query.tenantID, query.deviceIDs, query.weworkUserIDs)
		assigneeClauses, assigneeArgs := buildScopeClauses(query.tenantID, "assignee_id = ?")
		sqlText := "SELECT * FROM (" + accountScopeSQL + " UNION SELECT * FROM conversation_overview_projection WHERE " + strings.Join(assigneeClauses, " AND ") + ") projection_rows"
		args = append(args, accountScopeArgs...)
		args = append(args, assigneeArgs...)
		args = append(args, query.assigneeID)
		hasWhere := len(stateClauses) > 0
		if hasWhere {
			sqlText += " WHERE " + strings.Join(stateClauses, " AND ")
		}
		sqlText = appendKeyset(sqlText, hasWhere)
		sqlText += " ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?"
		args = append(args, query.limit)
		return sqlText, args
	}

	sqlText := "SELECT * FROM conversation_overview_projection"
	clauses := make([]string, 0)
	if query.tenantID != "" && (len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0) {
		clauses = append(clauses, "(tenant_id = ? OR tenant_id = '')")
		args = append(args, query.tenantID)
	} else if query.tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, query.tenantID)
	}
	scopeClauses := scopeClauses(query.deviceIDs, query.weworkUserIDs, &args)
	if len(scopeClauses) > 0 {
		clauses = append(clauses, "("+strings.Join(scopeClauses, " OR ")+")")
	} else if query.assigneeID != "" {
		clauses = append(clauses, "assignee_id = ?")
		args = append(args, query.assigneeID)
	}
	clauses = append(clauses, stateClauses...)
	hasWhere := len(clauses) > 0
	if hasWhere {
		sqlText += " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlText = appendKeyset(sqlText, hasWhere)
	sqlText += " ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?"
	args = append(args, query.limit)
	return sqlText, args
}

func (repository *Repository) countScopedSQL(query normalizedQuery) (string, []any) {
	stateClauses := buildStateFilterClauses(query.modeFilter, query.statusFilter)
	selectArgs := make([]any, 0)
	whereArgs := make([]any, 0)

	if (len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0) && query.assigneeID != "" {
		accountScopeSQL, accountScopeArgs := buildAccountScopeQuery(query.tenantID, query.deviceIDs, query.weworkUserIDs)
		assigneeClauses, assigneeArgs := buildScopeClauses(query.tenantID, "assignee_id = ?")
		innerSQL := accountScopeSQL + " UNION SELECT * FROM conversation_overview_projection WHERE " + strings.Join(assigneeClauses, " AND ")
		selectArgs = append(selectArgs, query.assigneeID)
		whereArgs = append(whereArgs, accountScopeArgs...)
		whereArgs = append(whereArgs, assigneeArgs...)
		whereArgs = append(whereArgs, query.assigneeID)
		outerWhere := ""
		if len(stateClauses) > 0 {
			outerWhere = " WHERE " + strings.Join(stateClauses, " AND ")
		}
		sqlText := "SELECT COUNT(1) AS total, SUM(COALESCE(unread_count, 0)) AS unread, SUM(CASE WHEN assignee_id = ? THEN 1 ELSE 0 END) AS assigned FROM (" + innerSQL + ") _cs_scope" + outerWhere
		return sqlText, append(selectArgs, whereArgs...)
	}

	clauses := make([]string, 0)
	if query.tenantID != "" && (len(query.deviceIDs) > 0 || len(query.weworkUserIDs) > 0) {
		clauses = append(clauses, "(tenant_id = ? OR tenant_id = '')")
		whereArgs = append(whereArgs, query.tenantID)
	} else if query.tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		whereArgs = append(whereArgs, query.tenantID)
	}
	scope := scopeClauses(query.deviceIDs, query.weworkUserIDs, &whereArgs)
	if len(scope) > 0 {
		clauses = append(clauses, "("+strings.Join(scope, " OR ")+")")
	} else if query.assigneeID != "" {
		clauses = append(clauses, "assignee_id = ?")
		whereArgs = append(whereArgs, query.assigneeID)
	}
	clauses = append(clauses, stateClauses...)
	whereSQL := ""
	if len(clauses) > 0 {
		whereSQL = " WHERE " + strings.Join(clauses, " AND ")
	}
	assignedSQL := "0"
	if query.assigneeID != "" {
		assignedSQL = "SUM(CASE WHEN assignee_id = ? THEN 1 ELSE 0 END)"
		selectArgs = append(selectArgs, query.assigneeID)
	}
	sqlText := "SELECT COUNT(1) AS total, SUM(COALESCE(unread_count, 0)) AS unread, " + assignedSQL + " AS assigned FROM conversation_overview_projection" + whereSQL
	return sqlText, append(selectArgs, whereArgs...)
}

func conversationListSQL(query workbench.ConversationListQuery) (string, []any) {
	args := make([]any, 0)
	clauses := make([]string, 0)
	if tenantID := strings.TrimSpace(query.TenantID); tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, tenantID)
	}
	if assigneeID := strings.TrimSpace(query.AssigneeID); assigneeID != "" {
		clauses = append(clauses, "assignee_id = ?")
		args = append(args, assigneeID)
	}
	if accountName := strings.TrimSpace(query.AccountName); accountName != "" {
		clauses = append(clauses, "(account_name = ? OR account_id = ? OR account_device_id = ? OR device_id = ? OR account_wework_user_id = ? OR wework_user_id = ?)")
		args = append(args, accountName, accountName, accountName, accountName, accountName, accountName)
	}
	if query.UnreadOnly {
		clauses = append(clauses, "COALESCE(unread_count, 0) > 0")
	}
	if query.UnassignedOnly {
		clauses = append(clauses, "COALESCE(assignee_id, '') = ''")
	}
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		keywordClause := conversationListKeywordClause()
		clauses = append(clauses, keywordClause)
		like := "%" + keyword + "%"
		for range conversationListKeywordColumns() {
			args = append(args, like)
		}
	}

	sqlText := "SELECT * FROM conversation_overview_projection"
	if len(clauses) > 0 {
		sqlText += " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlText += " ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?"
	args = append(args, boundedConversationListLimit(query.Limit))
	return sqlText, args
}

func conversationListKeywordClause() string {
	parts := make([]string, 0, len(conversationListKeywordColumns()))
	for _, column := range conversationListKeywordColumns() {
		parts = append(parts, "LOWER(COALESCE("+column+", '')) LIKE ?")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func conversationListKeywordColumns() []string {
	return []string{
		"conversation_id",
		"conversation_name",
		"customer_name",
		"sender_name",
		"sender_remark",
		"sender_id",
		"external_userid",
		"wework_user_id",
		"account_name",
		"last_content",
		"assignee_name",
	}
}

func boundedConversationListLimit(limit int) int {
	if limit <= 0 {
		return 5000
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

type normalizedQuery struct {
	deviceIDs            []string
	weworkUserIDs        []string
	conversationIDs      []string
	assigneeID           string
	tenantID             string
	cursorLastMessageAt  any
	cursorConversationID string
	modeFilter           string
	statusFilter         string
	limit                int
}

func normalizeQuery(query workbench.ProjectionQuery) normalizedQuery {
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	return normalizedQuery{
		deviceIDs:            normalizeStrings(query.DeviceIDs),
		weworkUserIDs:        normalizeChannelScopeIDs(query.ChannelUserIDs, query.WeWorkUserIDs),
		conversationIDs:      normalizeStrings(query.ConversationIDs),
		assigneeID:           strings.TrimSpace(query.AssigneeID),
		tenantID:             strings.TrimSpace(query.TenantID),
		cursorLastMessageAt:  normalizeCursor(query.CursorLastMessageAt),
		cursorConversationID: strings.TrimSpace(query.CursorConversationID),
		modeFilter:           defaultLower(query.ModeFilter, "all"),
		statusFilter:         defaultLower(query.StatusFilter, "all"),
		limit:                limit,
	}
}

func (query normalizedQuery) hasScope() bool {
	return len(query.deviceIDs) > 0 ||
		len(query.weworkUserIDs) > 0 ||
		len(query.conversationIDs) > 0 ||
		query.assigneeID != "" ||
		query.tenantID != ""
}

func buildAccountScopeQuery(tenantID string, deviceIDs []string, weworkUserIDs []string) (string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if len(deviceIDs) > 0 {
		if tenantID != "" {
			clauses = append(clauses, "SELECT * FROM conversation_overview_projection WHERE (tenant_id = ? OR tenant_id = '') AND device_id IN ("+placeholders(len(deviceIDs))+")")
			args = append(args, tenantID)
		} else {
			clauses = append(clauses, "SELECT * FROM conversation_overview_projection WHERE device_id IN ("+placeholders(len(deviceIDs))+")")
		}
		args = append(args, stringsToAny(deviceIDs)...)
	}
	if len(weworkUserIDs) > 0 {
		whereParts := make([]string, 0)
		if tenantID != "" {
			whereParts = append(whereParts, "(tenant_id = ? OR tenant_id = '')")
			args = append(args, tenantID)
		}
		whereParts = append(whereParts, "wework_user_id IN ("+placeholders(len(weworkUserIDs))+")")
		clauses = append(clauses, "SELECT * FROM conversation_overview_projection WHERE "+strings.Join(whereParts, " AND "))
		args = append(args, stringsToAny(weworkUserIDs)...)
	}
	return strings.Join(clauses, " UNION "), args
}

func buildScopeClauses(tenantID string, extraClause string) ([]string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if tenantID != "" {
		clauses = append(clauses, "tenant_id = ?")
		args = append(args, tenantID)
	}
	if extraClause != "" {
		clauses = append(clauses, extraClause)
	}
	return clauses, args
}

func buildStateFilterClauses(modeFilter string, statusFilter string) []string {
	clauses := make([]string, 0)
	switch defaultLower(modeFilter, "all") {
	case "manual":
		clauses = append(clauses, "COALESCE(ai_auto_reply, 0) = 0")
	case "ai":
		clauses = append(clauses, "COALESCE(ai_auto_reply, 0) = 1")
	case "sensitive":
		clauses = append(clauses, "COALESCE(sensitive_handoff_pending, 0) = 1")
	}
	pendingClause := "(COALESCE(last_direction, '') = 'incoming' OR (COALESCE(last_direction, '') = '' AND last_incoming_at IS NOT NULL AND (last_message_at IS NULL OR last_message_at = last_incoming_at)))"
	switch defaultLower(statusFilter, "all") {
	case "pending":
		clauses = append(clauses, pendingClause)
	case "unread":
		clauses = append(clauses, "COALESCE(unread_count, 0) > 0")
	case "replied":
		clauses = append(clauses, "NOT "+pendingClause)
	}
	return clauses
}

func scanProjectionRows(rows RowsScanner) ([]workbench.ProjectionRow, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]workbench.ProjectionRow, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for index := range values {
			targets[index] = &values[index]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		row := make(workbench.ProjectionRow, len(columns))
		for index, column := range columns {
			row[column] = normalizeDBValue(values[index])
		}
		result = append(result, row)
	}
	return result, nil
}

func scopeClauses(deviceIDs []string, weworkUserIDs []string, args *[]any) []string {
	clauses := make([]string, 0)
	if len(deviceIDs) > 0 {
		clauses = append(clauses, "device_id IN ("+placeholders(len(deviceIDs))+")")
		*args = append(*args, stringsToAny(deviceIDs)...)
	}
	if len(weworkUserIDs) > 0 {
		clauses = append(clauses, "wework_user_id IN ("+placeholders(len(weworkUserIDs))+")")
		*args = append(*args, stringsToAny(weworkUserIDs)...)
	}
	return clauses
}

func normalizeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func normalizeChannelScopeIDs(channelUserIDs []string, compatibilityUserIDs []string) []string {
	normalized := normalizeStrings(channelUserIDs)
	if len(normalized) > 0 {
		return normalized
	}
	return normalizeStrings(compatibilityUserIDs)
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func defaultLower(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func normalizeCursor(value any) any {
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		return text
	}
	return value
}

func hasCursor(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func normalizeDBValue(value any) any {
	if data, ok := value.([]byte); ok {
		return string(data)
	}
	return value
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint64:
		return int(typed)
	case []byte:
		return parseIntString(string(typed))
	case string:
		return parseIntString(typed)
	default:
		return 0
	}
}

func parseIntString(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
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

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is nil")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
