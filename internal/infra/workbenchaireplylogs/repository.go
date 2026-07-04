// Package workbenchaireplylogs reads AI reply attempt logs for admin pages.
// It mirrors Python's counted pagination over ai_reply_attempts and joined
// message/conversation/account facts without taking over write-side attempts.
package workbenchaireplylogs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the reply log repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Repository reads counted AI reply attempt log pages.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListAIReplyLogs returns one Python-compatible AI reply log page.
func (repository *Repository) ListAIReplyLogs(ctx context.Context, query workbench.AIReplyLogQuery) (workbench.AIReplyLogPage, error) {
	if repository.DB == nil {
		return workbench.AIReplyLogPage{}, fmt.Errorf("workbench ai reply log database is not configured")
	}
	page := normalizePage(query.Page)
	pageSize := normalizePageSize(query.PageSize)
	whereSQL, args := repository.whereClause(query)
	fromClause := `
FROM ai_reply_attempts a
LEFT JOIN messages m ON m.trace_id = a.outgoing_trace_id
LEFT JOIN messages incoming_m ON incoming_m.trace_id = a.incoming_trace_id
LEFT JOIN conversations c ON c.conversation_id = a.conversation_id
LEFT JOIN wework_accounts w ON w.account_id = a.account_id` + whereSQL

	countRows, err := repository.DB.QueryContext(ctx, "SELECT COUNT(1) AS total "+fromClause, args...)
	if err != nil {
		return workbench.AIReplyLogPage{}, err
	}
	total, err := scanTotal(countRows)
	if err != nil {
		return workbench.AIReplyLogPage{}, err
	}

	dataArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	dataRows, err := repository.DB.QueryContext(ctx, replyLogDataSQL(fromClause), dataArgs...)
	if err != nil {
		return workbench.AIReplyLogPage{}, err
	}
	logs, err := scanReplyLogRows(dataRows, pageSize)
	if err != nil {
		return workbench.AIReplyLogPage{}, err
	}
	return workbench.AIReplyLogPage{
		Logs: logs,
		Pagination: workbench.ProjectionRow{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages(total, pageSize),
		},
	}, nil
}

// whereClause builds Python-compatible filters and positional SQL args.
func (repository *Repository) whereClause(query workbench.AIReplyLogQuery) (string, []any) {
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 20)
	if query.LocalOnly {
		conditions = append(conditions, "LOWER(COALESCE(a.provider, '')) NOT IN ('coze', 'xiaobei')")
		conditions = append(conditions, "COALESCE(a.workflow_id, '') = ''")
	} else if workflowID := strings.TrimSpace(query.WorkflowID); workflowID != "" {
		conditions = append(conditions, "a.workflow_id = ?")
		args = append(args, workflowID)
	}
	if status := strings.ToLower(strings.TrimSpace(query.Status)); status != "" && status != "all" {
		conditions = append(conditions, "LOWER(COALESCE(a.status, '')) = ?")
		args = append(args, status)
	}
	replyTimeFilterSQL := repository.replyTimeFilterSQL()
	if query.Start != nil {
		conditions = append(conditions, replyTimeFilterSQL+" >= ?")
		args = append(args, repository.dbDatetimeParam(*query.Start))
	}
	if query.End != nil {
		conditions = append(conditions, replyTimeFilterSQL+" < ?")
		args = append(args, repository.dbDatetimeParam(*query.End))
	}
	if keyword := strings.ToLower(strings.TrimSpace(query.Keyword)); keyword != "" {
		fields := []string{
			"m.content",
			"incoming_m.content",
			"m.sender_name",
			"m.sender_remark",
			"c.sender_name",
			"c.sender_remark",
			"c.conversation_name",
			"w.account_name",
			"w.assignee_name",
			"a.status",
			"a.attempt_id",
			"a.conversation_id",
			"a.incoming_trace_id",
			"a.outgoing_trace_id",
		}
		parts := make([]string, 0, len(fields))
		for _, field := range fields {
			parts = append(parts, "LOWER(COALESCE("+field+", '')) LIKE ?")
			args = append(args, "%"+keyword+"%")
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "\nWHERE " + strings.Join(conditions, " AND "), args
}

// replyTimeFilterSQL returns the expression used by date filters.
func (repository *Repository) replyTimeFilterSQL() string {
	replyTimeSQL := "COALESCE(m.timestamp, a.finished_at, a.updated_at, a.started_at)"
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "sqlite") {
		return "datetime(" + replyTimeSQL + ")"
	}
	return replyTimeSQL
}

// dbDatetimeParam formats Beijing day bounds for the configured SQL dialect.
func (repository *Repository) dbDatetimeParam(value time.Time) string {
	beijing := value.In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return beijing.Format(time.RFC3339)
	}
	return beijing.Format("2006-01-02 15:04:05")
}

// replyLogDataSQL returns the joined row query for one reply log page.
func replyLogDataSQL(fromClause string) string {
	return `
SELECT
    a.attempt_id,
    a.tenant_id,
    a.account_id,
    a.device_id,
    a.wework_user_id,
    a.conversation_id,
    a.external_userid,
    a.incoming_trace_id,
    a.ai_trace_id,
    a.outgoing_trace_id,
    a.task_id,
    a.provider,
    a.workflow_id,
    a.model,
    a.trigger_event,
    a.status,
    a.failure_type,
    a.provider_error,
    a.user_facing_error,
    a.started_at,
    a.finished_at,
    a.updated_at,
    m.trace_id AS message_trace_id,
    m.timestamp AS message_timestamp,
    m.content AS message_content,
    incoming_m.trace_id AS customer_message_trace_id,
    incoming_m.content AS customer_message_content,
    m.sender_name AS message_sender_name,
    m.sender_remark AS message_sender_remark,
    c.sender_name AS conversation_sender_name,
    c.sender_remark AS conversation_sender_remark,
    c.conversation_name AS conversation_name,
    w.account_name AS account_name,
    w.assignee_id AS assignee_id,
    w.assignee_name AS assignee_name
` + fromClause + `
ORDER BY COALESCE(m.timestamp, a.finished_at, a.updated_at, a.started_at) DESC,
         a.updated_at DESC,
         a.attempt_id DESC
LIMIT ? OFFSET ?`
}

// scanReplyLogRows converts SQL rows into raw workbench projection rows.
func scanReplyLogRows(rows RowsScanner, capacity int) ([]workbench.ProjectionRow, error) {
	defer rows.Close()
	logs := make([]workbench.ProjectionRow, 0, capacity)
	for rows.Next() {
		var attemptID any
		var tenantID any
		var accountID any
		var deviceID any
		var weworkUserID any
		var conversationID any
		var externalUserID any
		var incomingTraceID any
		var aiTraceID any
		var outgoingTraceID any
		var taskID any
		var provider any
		var workflowID any
		var model any
		var triggerEvent any
		var status any
		var failureType any
		var providerError any
		var userFacingError any
		var startedAt any
		var finishedAt any
		var updatedAt any
		var messageTraceID any
		var messageTimestamp any
		var messageContent any
		var customerMessageTraceID any
		var customerMessageContent any
		var messageSenderName any
		var messageSenderRemark any
		var conversationSenderName any
		var conversationSenderRemark any
		var conversationName any
		var accountName any
		var assigneeID any
		var assigneeName any
		if err := rows.Scan(
			&attemptID,
			&tenantID,
			&accountID,
			&deviceID,
			&weworkUserID,
			&conversationID,
			&externalUserID,
			&incomingTraceID,
			&aiTraceID,
			&outgoingTraceID,
			&taskID,
			&provider,
			&workflowID,
			&model,
			&triggerEvent,
			&status,
			&failureType,
			&providerError,
			&userFacingError,
			&startedAt,
			&finishedAt,
			&updatedAt,
			&messageTraceID,
			&messageTimestamp,
			&messageContent,
			&customerMessageTraceID,
			&customerMessageContent,
			&messageSenderName,
			&messageSenderRemark,
			&conversationSenderName,
			&conversationSenderRemark,
			&conversationName,
			&accountName,
			&assigneeID,
			&assigneeName,
		); err != nil {
			return nil, err
		}
		logs = append(logs, workbench.ProjectionRow{
			"attempt_id":                 stringFromDB(attemptID),
			"tenant_id":                  stringFromDB(tenantID),
			"account_id":                 stringFromDB(accountID),
			"device_id":                  stringFromDB(deviceID),
			"wework_user_id":             stringFromDB(weworkUserID),
			"conversation_id":            stringFromDB(conversationID),
			"external_userid":            stringFromDB(externalUserID),
			"incoming_trace_id":          stringFromDB(incomingTraceID),
			"ai_trace_id":                stringFromDB(aiTraceID),
			"outgoing_trace_id":          stringFromDB(outgoingTraceID),
			"task_id":                    stringFromDB(taskID),
			"provider":                   stringFromDB(provider),
			"workflow_id":                stringFromDB(workflowID),
			"model":                      stringFromDB(model),
			"trigger_event":              stringFromDB(triggerEvent),
			"status":                     stringFromDB(status),
			"failure_type":               stringFromDB(failureType),
			"provider_error":             stringFromDB(providerError),
			"user_facing_error":          stringFromDB(userFacingError),
			"started_at":                 timeFromDB(startedAt),
			"finished_at":                timeFromDB(finishedAt),
			"updated_at":                 timeFromDB(updatedAt),
			"message_trace_id":           stringFromDB(messageTraceID),
			"message_timestamp":          timeFromDB(messageTimestamp),
			"message_content":            stringFromDB(messageContent),
			"customer_message_trace_id":  stringFromDB(customerMessageTraceID),
			"customer_message_content":   stringFromDB(customerMessageContent),
			"message_sender_name":        stringFromDB(messageSenderName),
			"message_sender_remark":      stringFromDB(messageSenderRemark),
			"conversation_sender_name":   stringFromDB(conversationSenderName),
			"conversation_sender_remark": stringFromDB(conversationSenderRemark),
			"conversation_name":          stringFromDB(conversationName),
			"account_name":               stringFromDB(accountName),
			"assignee_id":                stringFromDB(assigneeID),
			"assignee_name":              stringFromDB(assigneeName),
		})
	}
	return logs, rows.Err()
}

// scanTotal reads the COUNT(1) result row.
func scanTotal(rows RowsScanner) (int, error) {
	defer rows.Close()
	for rows.Next() {
		var total any
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
		return intFromDB(total), rows.Err()
	}
	return 0, rows.Err()
}

// stringFromDB trims nullable database values as strings.
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

// timeFromDB preserves time.Time values while normalizing blank timestamps.
func timeFromDB(value any) any {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed
	default:
		return stringFromDB(value)
	}
}

// intFromDB parses integer database values from common driver types.
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

// normalizePage mirrors Python's minimum page behavior for direct store calls.
func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

// normalizePageSize mirrors Python's page_size default and upper bound.
func normalizePageSize(pageSize int) int {
	if pageSize < 1 {
		return 50
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

// totalPages computes counted pagination while preserving at least one page.
func totalPages(total int, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

type sqlQueryer struct {
	db *sql.DB
}

// QueryContext delegates to database/sql after guarding nil DB handles.
func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}
