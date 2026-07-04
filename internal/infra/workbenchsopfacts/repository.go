// Package workbenchsopfacts reads SOP delivery fact analytics for admin pages.
// It is read-only and does not create dispatch tasks, resend failures, mutate
// SOP configs, or participate in platform media/test side effects.
package workbenchsopfacts

import (
	"context"
	"database/sql"
	"fmt"
	"math"
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

// Queryer is the database/sql shape needed by the SOP fact repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Execer is implemented by DB handles that can update SOP fact rows.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads sop_delivery_facts analytics rows for admin candidates.
type Repository struct {
	DB  Queryer
	Now func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// MarkCustomerReply attributes an incoming customer message to the latest successful SOP fact.
func (repository *Repository) MarkCustomerReply(ctx context.Context, tenantID string, conversationID string, externalUserID string, replyTraceID string, replyMsgID string, repliedAt time.Time) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench sop fact database is not configured")
	}
	execer, ok := repository.DB.(Execer)
	if !ok {
		return false, fmt.Errorf("workbench sop fact database cannot execute updates")
	}
	conversationID = text(conversationID, 255)
	if conversationID == "" {
		return false, nil
	}
	if repliedAt.IsZero() {
		repliedAt = repository.now()
	}
	repliedAt = repliedAt.UTC()
	factID, existingTraceID, err := repository.findCustomerReplyFact(ctx, tenantID, conversationID, externalUserID, repliedAt)
	if err != nil || factID == "" {
		return false, err
	}
	replyTraceID = text(replyTraceID, 255)
	if replyTraceID != "" && existingTraceID == replyTraceID {
		return true, nil
	}
	_, err = execer.ExecContext(ctx, `
UPDATE sop_delivery_facts
SET customer_replied = 1,
    first_customer_reply_trace_id = CASE WHEN COALESCE(first_customer_reply_trace_id, '') = '' THEN ? ELSE first_customer_reply_trace_id END,
    first_customer_reply_msgid = CASE WHEN COALESCE(first_customer_reply_msgid, '') = '' THEN ? ELSE first_customer_reply_msgid END,
    first_customer_reply_at = CASE WHEN first_customer_reply_at IS NULL THEN ? ELSE first_customer_reply_at END,
    customer_reply_message_count = customer_reply_message_count + 1,
    updated_at = ?
WHERE fact_id = ?`,
		replyTraceID,
		text(replyMsgID, 255),
		repliedAt,
		repository.now(),
		factID,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (repository *Repository) findCustomerReplyFact(ctx context.Context, tenantID string, conversationID string, externalUserID string, repliedAt time.Time) (string, string, error) {
	where := []string{"conversation_id = ?", "delivery_status = 'success'", "delivered_at <= ?"}
	args := []any{conversationID, repliedAt}
	tenantID = text(tenantID, 255)
	if tenantID != "" {
		where = append([]string{"tenant_id = ?"}, where...)
		args = append([]any{tenantID}, args...)
	}
	factID, traceID, ok, err := repository.selectCustomerReplyFact(ctx, "SELECT fact_id, first_customer_reply_trace_id FROM sop_delivery_facts WHERE "+strings.Join(where, " AND ")+" ORDER BY delivered_at DESC, updated_at DESC LIMIT 1", args...)
	if err != nil || ok {
		return factID, traceID, err
	}
	customerID := text(externalUserID, 255)
	if customerID == "" {
		customerID = customerIDFromConversationID(conversationID)
	}
	if customerID == "" {
		return "", "", nil
	}
	fallbackWhere := []string{"delivery_status = 'success'", "delivered_at <= ?", "stat_date = ?"}
	fallbackArgs := []any{repliedAt, repliedAt.Format("2006-01-02")}
	if tenantID != "" {
		fallbackWhere = append([]string{"tenant_id = ?"}, fallbackWhere...)
		fallbackArgs = append([]any{tenantID}, fallbackArgs...)
	}
	fallbackArgs = append(fallbackArgs, customerID, "%:"+escapeLike(customerID))
	factID, traceID, _, err = repository.selectCustomerReplyFact(ctx, "SELECT fact_id, first_customer_reply_trace_id FROM sop_delivery_facts WHERE "+strings.Join(fallbackWhere, " AND ")+" AND (external_userid = ? OR conversation_id LIKE ? ESCAPE '!') ORDER BY delivered_at DESC, updated_at DESC LIMIT 1", fallbackArgs...)
	return factID, traceID, err
}

func (repository *Repository) selectCustomerReplyFact(ctx context.Context, query string, args ...any) (string, string, bool, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return "", "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", false, rows.Err()
	}
	var factID any
	var traceID any
	if err := rows.Scan(&factID, &traceID); err != nil {
		return "", "", false, err
	}
	return stringFromDB(factID), stringFromDB(traceID), true, rows.Err()
}

// SummarizeSOPStageDaily returns customer-level metrics grouped by stage.
func (repository *Repository) SummarizeSOPStageDaily(ctx context.Context, query workbench.SOPStageStatsQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench sop fact database is not configured")
	}
	where := []string{"stat_date = ?"}
	args := []any{dateValue(query.Date)}
	if text(query.FlowID, 255) != "" {
		where = append(where, "flow_id = ?")
		args = append(args, text(query.FlowID, 255))
	}
	sql := `
SELECT
    flow_id,
    stage_unique_id,
    MAX(stage_name) AS stage_name,
    MIN(stage_index) AS stage_index,
    MAX(day_stage) AS day_stage,
    MAX(customer_state) AS customer_state,
    SUM(CASE WHEN delivery_status = 'success' THEN 1 ELSE 0 END) AS delivered_task_count,
    COUNT(DISTINCT CASE WHEN delivery_status = 'success' THEN conversation_id END) AS delivered_customer_count,
    SUM(CASE WHEN delivery_status = 'success' THEN message_count ELSE 0 END) AS delivered_message_count,
    COUNT(DISTINCT CASE WHEN delivery_status = 'success' AND customer_replied = 1 THEN conversation_id END) AS customer_open_count,
    SUM(CASE WHEN delivery_status = 'success' AND customer_replied = 1 THEN customer_reply_message_count ELSE 0 END) AS customer_reply_message_count,
    SUM(CASE WHEN delivery_status = 'success' AND customer_replied = 1 AND ai_reply_status = 'sent' THEN 1 ELSE 0 END) AS ai_reply_fact_count,
    COUNT(DISTINCT CASE WHEN delivery_status = 'success' AND customer_replied = 1 AND ai_reply_status = 'sent' THEN conversation_id END) AS ai_reply_count
FROM sop_delivery_facts
WHERE ` + strings.Join(where, " AND ") + `
GROUP BY flow_id, stage_unique_id
ORDER BY MIN(stage_index), stage_unique_id`
	rows, err := repository.DB.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workbench.ProjectionRow, 0)
	for rows.Next() {
		var flowID any
		var stageUniqueID any
		var stageName any
		var stageIndex any
		var dayStage any
		var customerState any
		var deliveredTaskCount any
		var deliveredCustomerCount any
		var deliveredMessageCount any
		var customerOpenCount any
		var customerReplyMessageCount any
		var aiReplyFactCount any
		var aiReplyCount any
		if err := rows.Scan(&flowID, &stageUniqueID, &stageName, &stageIndex, &dayStage, &customerState, &deliveredTaskCount, &deliveredCustomerCount, &deliveredMessageCount, &customerOpenCount, &customerReplyMessageCount, &aiReplyFactCount, &aiReplyCount); err != nil {
			return nil, err
		}
		delivered := intFromDB(deliveredCustomerCount)
		opened := intFromDB(customerOpenCount)
		aiReplied := intFromDB(aiReplyCount)
		aiReplyRate := ratio(aiReplied, opened)
		items = append(items, workbench.ProjectionRow{
			"flow_id":                      stringFromDB(flowID),
			"stage_unique_id":              stringFromDB(stageUniqueID),
			"stage_name":                   stringFromDB(stageName),
			"stage_index":                  intFromDB(stageIndex),
			"day_stage":                    stringFromDB(dayStage),
			"customer_state":               stringFromDB(customerState),
			"delivered_task_count":         intFromDB(deliveredTaskCount),
			"delivered_customer_count":     delivered,
			"delivered_message_count":      intFromDB(deliveredMessageCount),
			"customer_open_count":          opened,
			"customer_reply_message_count": intFromDB(customerReplyMessageCount),
			"ai_reply_fact_count":          intFromDB(aiReplyFactCount),
			"ai_reply_customer_count":      aiReplied,
			"ai_reply_count":               aiReplied,
			"customer_open_rate":           ratio(opened, delivered),
			"ai_reply_rate":                aiReplyRate,
			"ai_takeover_rate":             aiReplyRate,
			"ai_reply_delivery_rate":       ratio(aiReplied, delivered),
		})
	}
	return items, rows.Err()
}

// ListSOPFacts returns one page of SOP delivery fact detail rows.
func (repository *Repository) ListSOPFacts(ctx context.Context, query workbench.SOPFactsQuery) (workbench.SOPFactsPage, error) {
	if repository.DB == nil {
		return workbench.SOPFactsPage{}, fmt.Errorf("workbench sop fact database is not configured")
	}
	where, args := sopFactsWhere(query)
	whereSQL := strings.Join(where, " AND ")
	total, err := repository.count(ctx, "SELECT COUNT(*) AS total FROM sop_delivery_facts WHERE "+whereSQL, args...)
	if err != nil {
		return workbench.SOPFactsPage{}, err
	}
	page := maxInt(1, query.Page)
	pageSize := minInt(100, maxInt(1, query.PageSize))
	offset := (page - 1) * pageSize
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT "+strings.Join(sopFactColumns, ", ")+" FROM sop_delivery_facts WHERE "+whereSQL+" ORDER BY COALESCE(delivered_at, queued_at, created_at) DESC, fact_id DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return workbench.SOPFactsPage{}, err
	}
	items, err := scanFactRows(rows)
	if err != nil {
		return workbench.SOPFactsPage{}, err
	}
	return workbench.SOPFactsPage{
		Items: items,
		Pagination: workbench.ProjectionRow{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": maxInt(1, (total+pageSize-1)/pageSize),
		},
	}, nil
}

// ListSOPTaskBatches returns task-level groups for /admin/sop/dispatch-tasks.
func (repository *Repository) ListSOPTaskBatches(ctx context.Context, query workbench.SOPDispatchTasksQuery) (workbench.SOPTaskBatchesPage, error) {
	if repository.DB == nil {
		return workbench.SOPTaskBatchesPage{}, fmt.Errorf("workbench sop fact database is not configured")
	}
	where, args := sopTaskBatchesWhere(query)
	whereSQL := strings.Join(where, " AND ")
	batchKeyExpr := "COALESCE(NULLIF(task_id, ''), fact_id)"
	total, err := repository.count(ctx, "SELECT COUNT(DISTINCT "+batchKeyExpr+") AS total FROM sop_delivery_facts WHERE "+whereSQL, args...)
	if err != nil {
		return workbench.SOPTaskBatchesPage{}, err
	}
	page := maxInt(1, query.Page)
	pageSize := minInt(100, maxInt(1, query.PageSize))
	offset := (page - 1) * pageSize
	keyRows, err := repository.DB.QueryContext(
		ctx,
		"SELECT "+batchKeyExpr+" AS batch_key, MAX(COALESCE(delivered_at, failed_at, queued_at, created_at)) AS last_at FROM sop_delivery_facts WHERE "+whereSQL+" GROUP BY "+batchKeyExpr+" ORDER BY last_at DESC, batch_key DESC LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return workbench.SOPTaskBatchesPage{}, err
	}
	batchKeys, err := scanBatchKeys(keyRows)
	if err != nil {
		return workbench.SOPTaskBatchesPage{}, err
	}
	if len(batchKeys) == 0 {
		return workbench.SOPTaskBatchesPage{
			Items: []workbench.SOPTaskBatchGroup{},
			Pagination: workbench.ProjectionRow{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": maxInt(1, (total+pageSize-1)/pageSize),
			},
		}, nil
	}
	rowArgs := append([]any{}, args...)
	for _, key := range batchKeys {
		rowArgs = append(rowArgs, key)
	}
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT "+strings.Join(sopFactColumns, ", ")+" FROM sop_delivery_facts WHERE "+whereSQL+" AND "+batchKeyExpr+" IN ("+placeholders(len(batchKeys))+") ORDER BY COALESCE(delivered_at, failed_at, queued_at, created_at) DESC, fact_id DESC",
		rowArgs...,
	)
	if err != nil {
		return workbench.SOPTaskBatchesPage{}, err
	}
	facts, err := scanFactRows(rows)
	if err != nil {
		return workbench.SOPTaskBatchesPage{}, err
	}
	grouped := make(map[string][]workbench.ProjectionRow, len(batchKeys))
	for _, key := range batchKeys {
		grouped[key] = []workbench.ProjectionRow{}
	}
	for _, row := range facts {
		key := text(row["task_id"], 255)
		if key == "" {
			key = text(row["fact_id"], 255)
		}
		if _, ok := grouped[key]; ok {
			grouped[key] = append(grouped[key], row)
		}
	}
	items := make([]workbench.SOPTaskBatchGroup, 0, len(batchKeys))
	for _, key := range batchKeys {
		items = append(items, workbench.SOPTaskBatchGroup{BatchKey: key, Rows: grouped[key]})
	}
	return workbench.SOPTaskBatchesPage{
		Items: items,
		Pagination: workbench.ProjectionRow{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": maxInt(1, (total+pageSize-1)/pageSize),
		},
	}, nil
}

// ListFailedSOPResendCandidates returns failed fact rows eligible for manual resend.
func (repository *Repository) ListFailedSOPResendCandidates(ctx context.Context, query workbench.SOPDispatchResendQuery) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench sop fact database is not configured")
	}
	where := []string{"stat_date = ?", "delivery_status = 'failed'", "COALESCE(source_payload_json, '') != ''"}
	args := []any{dateValue(query.Date)}
	if flowID := text(query.FlowID, 255); flowID != "" {
		where = append(where, "flow_id = ?")
		args = append(args, flowID)
	}
	taskIDs := cleanTaskIDs(query.TaskIDs)
	if len(taskIDs) > 0 {
		where = append(where, "task_id IN ("+placeholders(len(taskIDs))+")")
		for _, taskID := range taskIDs {
			args = append(args, taskID)
		}
	}
	rowLimit := minInt(500, maxInt(1, query.Limit)*5)
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT "+strings.Join(sopFactColumns, ", ")+" FROM sop_delivery_facts WHERE "+strings.Join(where, " AND ")+" ORDER BY COALESCE(failed_at, queued_at, created_at) DESC, fact_id DESC LIMIT ?",
		append(args, rowLimit)...,
	)
	if err != nil {
		return nil, err
	}
	return scanFactRows(rows)
}

// MarkSOPResendQueued marks the original failed facts as manually resent.
func (repository *Repository) MarkSOPResendQueued(ctx context.Context, originalTaskID string, resendTaskID string) error {
	if repository.DB == nil {
		return fmt.Errorf("workbench sop fact database is not configured")
	}
	execer, ok := repository.DB.(Execer)
	if !ok {
		return fmt.Errorf("workbench sop fact database cannot execute updates")
	}
	originalTaskID = text(originalTaskID, 255)
	resendTaskID = text(resendTaskID, 255)
	if originalTaskID == "" || resendTaskID == "" {
		return nil
	}
	_, err := execer.ExecContext(ctx, `
UPDATE sop_delivery_facts
SET delivery_status = 'resent',
    delivery_error = ?,
    updated_at = ?
WHERE task_id = ?
  AND delivery_status = 'failed'`,
		"resent via "+resendTaskID,
		repository.now(),
		originalTaskID,
	)
	return err
}

func sopFactsWhere(query workbench.SOPFactsQuery) ([]string, []any) {
	where := []string{"stat_date = ?"}
	args := []any{dateValue(query.Date)}
	if text(query.FlowID, 255) != "" {
		where = append(where, "flow_id = ?")
		args = append(args, text(query.FlowID, 255))
	}
	if text(query.StageUniqueID, 255) != "" {
		where = append(where, "stage_unique_id = ?")
		args = append(args, text(query.StageUniqueID, 255))
	}
	status := text(query.Status, 32)
	if status != "" && status != "all" {
		switch status {
		case "opened":
			where = append(where, "customer_replied = 1")
		case "ai_sent":
			where = append(where, "ai_reply_status = 'sent'")
		default:
			where = append(where, "delivery_status = ?")
			args = append(args, status)
		}
	}
	if text(query.Keyword, 128) != "" {
		like := "%" + text(query.Keyword, 128) + "%"
		where = append(where, "(conversation_id LIKE ? OR assignee_name LIKE ? OR first_customer_reply_trace_id LIKE ? OR task_id LIKE ?)")
		args = append(args, like, like, like, like)
	}
	return where, args
}

func sopTaskBatchesWhere(query workbench.SOPDispatchTasksQuery) ([]string, []any) {
	where := []string{"stat_date = ?"}
	args := []any{dateValue(query.Date)}
	if text(query.FlowID, 255) != "" {
		where = append(where, "flow_id = ?")
		args = append(args, text(query.FlowID, 255))
	}
	status := strings.ToLower(text(query.Status, 32))
	if status != "" && status != "all" {
		where = append(where, "delivery_status = ?")
		args = append(args, status)
	}
	if text(query.Keyword, 128) != "" {
		like := "%" + text(query.Keyword, 128) + "%"
		where = append(where, "(conversation_id LIKE ? OR conversation_key LIKE ? OR external_userid LIKE ? OR assignee_name LIKE ? OR device_id LIKE ? OR task_id LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	return where, args
}

func cleanTaskIDs(values []string) []string {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		item := text(value, 255)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		cleaned = append(cleaned, item)
	}
	return cleaned
}

func (repository *Repository) count(ctx context.Context, query string, args ...any) (int, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var count any
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
		return intFromDB(count), rows.Err()
	}
	return 0, rows.Err()
}

func scanBatchKeys(rows RowsScanner) ([]string, error) {
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key any
		var lastAt any
		if err := rows.Scan(&key, &lastAt); err != nil {
			return nil, err
		}
		if normalized := text(key, 255); normalized != "" {
			keys = append(keys, normalized)
		}
	}
	return keys, rows.Err()
}

func scanFactRows(rows RowsScanner) ([]workbench.ProjectionRow, error) {
	defer rows.Close()
	items := make([]workbench.ProjectionRow, 0)
	for rows.Next() {
		values := make([]any, len(sopFactColumns))
		destinations := make([]any, len(sopFactColumns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		row := make(workbench.ProjectionRow, len(sopFactColumns))
		for index, column := range sopFactColumns {
			row[column] = scalarFromDB(values[index])
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for index := range values {
		values[index] = "?"
	}
	return strings.Join(values, ", ")
}

func dateValue(value string) string {
	text := strings.TrimSpace(value)
	if len(text) >= len("2006-01-02") {
		return text[:len("2006-01-02")]
	}
	return text
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func customerIDFromConversationID(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	index := strings.LastIndex(conversationID, ":")
	if index < 0 || index == len(conversationID)-1 {
		return ""
	}
	return text(conversationID[index+1:], 255)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(strings.TrimSpace(value))
}

func text(value any, limit int) string {
	normalized := strings.TrimSpace(fmt.Sprint(value))
	if value == nil {
		return ""
	}
	if len(normalized) > limit {
		return normalized[:limit]
	}
	return normalized
}

func ratio(numerator int, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*10000) / 10000
}

func scalarFromDB(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	case []byte:
		return strings.TrimSpace(string(typed))
	case int, int32, int64, float32, float64, bool:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringFromDB(value any) string {
	scalar := scalarFromDB(value)
	if scalar == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(scalar))
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
	case float32:
		return int(typed)
	case float64:
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

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

var sopFactColumns = []string{
	"fact_id",
	"tenant_id",
	"enterprise_id",
	"stat_date",
	"flow_id",
	"flow_name",
	"stage_unique_id",
	"stage_name",
	"stage_index",
	"customer_stage_tag",
	"day_stage",
	"customer_state",
	"conversation_id",
	"conversation_key",
	"external_userid",
	"wework_user_id",
	"account_id",
	"device_id",
	"assignee_id",
	"assignee_name",
	"task_id",
	"platform_task_id",
	"batch_id",
	"message_trace_id",
	"archive_msgid",
	"message_count",
	"content_hash",
	"delivery_status",
	"queued_at",
	"dispatched_at",
	"delivered_at",
	"failed_at",
	"delivery_error",
	"customer_replied",
	"first_customer_reply_trace_id",
	"first_customer_reply_msgid",
	"first_customer_reply_at",
	"customer_reply_message_count",
	"ai_attempt_id",
	"ai_started_at",
	"ai_reply_status",
	"ai_reply_trace_id",
	"ai_reply_task_id",
	"ai_reply_at",
	"ai_failure_type",
	"ai_error",
	"source_payload_json",
	"confidence",
	"created_at",
	"updated_at",
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
