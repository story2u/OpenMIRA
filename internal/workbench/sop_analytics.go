// SOP analytics exposes read-only delivery fact summaries and drill-down rows.
// The candidate keeps sop_delivery_facts as the fact source and may enrich the
// operator-facing display fields from local projection/account reads.
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrSOPAnalyticsStoreUnavailable means SOP fact rows cannot be loaded.
	ErrSOPAnalyticsStoreUnavailable = errors.New("workbench sop analytics store is unavailable")
	// ErrInvalidSOPAnalyticsPage preserves FastAPI's page lower bound.
	ErrInvalidSOPAnalyticsPage = errors.New("invalid page, expected >=1")
	// ErrInvalidSOPAnalyticsPageSize preserves FastAPI's page_size bounds.
	ErrInvalidSOPAnalyticsPageSize = errors.New("invalid page_size, expected 1..100")
)

// SOPStageStatsQuery contains the store-level stage aggregate filters.
type SOPStageStatsQuery struct {
	Date   string
	FlowID string
}

// SOPFactsQuery contains the store-level fact detail filters.
type SOPFactsQuery struct {
	Date          string
	FlowID        string
	StageUniqueID string
	Status        string
	Keyword       string
	Page          int
	PageSize      int
}

// SOPDispatchTasksQuery contains the persisted fact filters for dispatch tasks.
type SOPDispatchTasksQuery struct {
	Date     string
	FlowID   string
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// SOPFactsPage carries fact rows and legacy pagination metadata.
type SOPFactsPage struct {
	Items      []ProjectionRow
	Pagination ProjectionRow
}

// SOPTaskBatchGroup carries one task_id/fact_id group from sop_delivery_facts.
type SOPTaskBatchGroup struct {
	BatchKey string
	Rows     []ProjectionRow
}

// SOPTaskBatchesPage carries dispatch-task batch groups and pagination metadata.
type SOPTaskBatchesPage struct {
	Items      []SOPTaskBatchGroup
	Pagination ProjectionRow
}

// SOPStageStatsRequest carries /api/v1/admin/sop/analytics/stage-stats input.
type SOPStageStatsRequest struct {
	Session auth.Session
	Query   SOPStageStatsQuery
}

// SOPFactsRequest carries /api/v1/admin/sop/analytics/facts input.
type SOPFactsRequest struct {
	Session auth.Session
	Query   SOPFactsQuery
}

// SOPDispatchTasksRequest carries /api/v1/admin/sop/dispatch-tasks input.
type SOPDispatchTasksRequest struct {
	Session auth.Session
	Query   SOPDispatchTasksQuery
}

// NewSOPStageStatsRequest normalizes stage-stats query parameters.
func NewSOPStageStatsRequest(values url.Values, session auth.Session) SOPStageStatsRequest {
	return SOPStageStatsRequest{
		Session: session,
		Query: SOPStageStatsQuery{
			Date:   strings.TrimSpace(values.Get("date")),
			FlowID: strings.TrimSpace(values.Get("flow_id")),
		},
	}
}

// NewSOPFactsRequest validates and normalizes fact list query parameters.
func NewSOPFactsRequest(values url.Values, session auth.Session) (SOPFactsRequest, error) {
	page, err := boundedQueryInt(values, "page", 1, 1, int(^uint(0)>>1))
	if err != nil {
		return SOPFactsRequest{}, ErrInvalidSOPAnalyticsPage
	}
	pageSize, err := boundedQueryInt(values, "page_size", 50, 1, 100)
	if err != nil {
		return SOPFactsRequest{}, ErrInvalidSOPAnalyticsPageSize
	}
	return SOPFactsRequest{
		Session: session,
		Query: SOPFactsQuery{
			Date:          strings.TrimSpace(values.Get("date")),
			FlowID:        strings.TrimSpace(values.Get("flow_id")),
			StageUniqueID: strings.TrimSpace(values.Get("stage_unique_id")),
			Status:        strings.TrimSpace(values.Get("status")),
			Keyword:       strings.TrimSpace(values.Get("keyword")),
			Page:          page,
			PageSize:      pageSize,
		},
	}, nil
}

// NewSOPDispatchTasksRequest validates and normalizes dispatch task filters.
func NewSOPDispatchTasksRequest(values url.Values, session auth.Session) (SOPDispatchTasksRequest, error) {
	page, err := boundedQueryInt(values, "page", 1, 1, int(^uint(0)>>1))
	if err != nil {
		return SOPDispatchTasksRequest{}, ErrInvalidSOPAnalyticsPage
	}
	pageSize, err := boundedQueryInt(values, "page_size", 30, 1, 100)
	if err != nil {
		return SOPDispatchTasksRequest{}, ErrInvalidSOPAnalyticsPageSize
	}
	status := strings.ToLower(strings.TrimSpace(values.Get("status")))
	if status == "" {
		status = "all"
	}
	return SOPDispatchTasksRequest{
		Session: session,
		Query: SOPDispatchTasksQuery{
			Date:     strings.TrimSpace(values.Get("date")),
			FlowID:   strings.TrimSpace(values.Get("flow_id")),
			Status:   status,
			Keyword:  strings.TrimSpace(values.Get("keyword")),
			Page:     page,
			PageSize: pageSize,
		},
	}, nil
}

// SOPAnalyticsStageStats builds /api/v1/admin/sop/analytics/stage-stats.
func (service Service) SOPAnalyticsStageStats(ctx context.Context, request SOPStageStatsRequest) (Payload, error) {
	if service.SOPAnalyticsStore == nil {
		return nil, ErrSOPAnalyticsStoreUnavailable
	}
	query := request.Query
	query.Date = service.sopAnalyticsDate(query.Date)
	items, err := service.SOPAnalyticsStore.SummarizeSOPStageDaily(ctx, query)
	if err != nil {
		return nil, err
	}
	return Payload{"date": query.Date, "flow_id": query.FlowID, "items": items}, nil
}

// SOPAnalyticsFacts builds /api/v1/admin/sop/analytics/facts.
func (service Service) SOPAnalyticsFacts(ctx context.Context, request SOPFactsRequest) (Payload, error) {
	if service.SOPAnalyticsStore == nil {
		return nil, ErrSOPAnalyticsStoreUnavailable
	}
	query := request.Query
	query.Date = service.sopAnalyticsDate(query.Date)
	page, err := service.SOPAnalyticsStore.ListSOPFacts(ctx, query)
	if err != nil {
		return nil, err
	}
	return Payload{"items": page.Items, "pagination": page.Pagination}, nil
}

// SOPDispatchTasks builds /api/v1/admin/sop/dispatch-tasks from persisted facts.
func (service Service) SOPDispatchTasks(ctx context.Context, request SOPDispatchTasksRequest) (Payload, error) {
	if service.SOPAnalyticsStore == nil {
		return nil, ErrSOPAnalyticsStoreUnavailable
	}
	query := request.Query
	query.Date = service.sopAnalyticsDate(query.Date)
	page, err := service.SOPAnalyticsStore.ListSOPTaskBatches(ctx, query)
	if err != nil {
		return nil, err
	}
	batches := make([]ProjectionRow, 0, len(page.Items))
	tasks := make([]ProjectionRow, 0)
	for _, group := range page.Items {
		batch := sopFactGroupToDispatchBatch(group)
		batches = append(batches, batch)
		if details, ok := batch["details"].([]ProjectionRow); ok {
			tasks = append(tasks, details...)
		}
	}
	service.applySOPAutoResendPendingState(ctx, batches)
	service.enrichSOPDispatchTaskRows(ctx, batches)
	return Payload{"batches": batches, "tasks": tasks, "pagination": page.Pagination}, nil
}

// sopAnalyticsDate mirrors Python's Beijing-today default without validation.
func (service Service) sopAnalyticsDate(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	now := service.now().In(statsBeijingLocation)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, statsBeijingLocation).Format("2006-01-02")
}

func sopFactGroupToDispatchBatch(group SOPTaskBatchGroup) ProjectionRow {
	rows := make([]ProjectionRow, 0, len(group.Rows))
	for _, row := range group.Rows {
		if row != nil {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return ProjectionRow{"task_id": strings.TrimSpace(group.BatchKey), "details": []ProjectionRow{}}
	}
	details := make([]ProjectionRow, 0, len(rows))
	statuses := make([]string, 0, len(rows))
	statusCounts := map[string]int{}
	actionPreview := make([]ProjectionRow, 0, 3)
	for _, row := range rows {
		details = append(details, sopFactToDispatchTask(row))
		status := strings.ToLower(strings.TrimSpace(rowText(row, "delivery_status")))
		statuses = append(statuses, status)
		if status == "" {
			status = "pending"
		}
		statusCounts[status]++
		if len(actionPreview) < 3 {
			for _, item := range sopFactActionPreview(row, 3-len(actionPreview)) {
				actionPreview = append(actionPreview, item)
			}
		}
	}
	first := rows[0]
	taskID := rowText(first, "task_id")
	if taskID == "" {
		taskID = strings.TrimSpace(group.BatchKey)
	}
	batchStatus := resolveSOPBatchStatus(statuses)
	taskError := ""
	for _, row := range rows {
		taskError = rowText(row, "delivery_error")
		if taskError != "" {
			break
		}
	}
	taskError = cleanSOPTaskError(batchStatus, taskError)
	stageIDs := map[string]struct{}{}
	recipients := map[string]struct{}{}
	actionCount := 0
	sourcePayloadJSON := ""
	autoResendMeta := ProjectionRow{}
	for _, row := range rows {
		if stageID := rowText(row, "stage_unique_id"); stageID != "" {
			stageIDs[stageID] = struct{}{}
		}
		if conversationID := rowText(row, "conversation_id"); conversationID != "" {
			recipients[conversationID] = struct{}{}
		}
		actionCount += rowInt(row, "message_count")
		if sourcePayloadJSON == "" {
			sourcePayloadJSON = rowText(row, "source_payload_json")
		}
		if len(autoResendMeta) == 0 {
			autoResendMeta = sopFactAutoResendMeta(row)
		}
	}
	stageCount := len(stageIDs)
	if stageCount == 0 {
		stageCount = len(rows)
	}
	recipientCount := len(recipients)
	if recipientCount == 0 {
		recipientCount = 1
	}
	row := ProjectionRow{
		"task_id":         taskID,
		"batch_id":        firstNonEmpty(rowText(first, "batch_id"), rowText(first, "task_id"), strings.TrimSpace(group.BatchKey)),
		"ai_trace_id":     rowText(first, "message_trace_id"),
		"flow_id":         rowText(first, "flow_id"),
		"flow_name":       rowText(first, "flow_name"),
		"conversation_id": rowText(first, "conversation_id"),
		"sender_name":     firstNonEmpty(rowText(first, "conversation_key"), rowText(first, "external_userid")),
		"assignee_id":     rowText(first, "assignee_id"),
		"assignee_name":   rowText(first, "assignee_name"),
		"account_id":      rowText(first, "account_id"),
		"device_id":       rowText(first, "device_id"),
		"wework_user_id":  rowText(first, "wework_user_id"),
		"entity":          rowText(first, "enterprise_id"),
		"day_stage":       rowText(first, "day_stage"),
		"customer_state":  rowText(first, "customer_state"),
		"stage_tag":       rowText(first, "customer_stage_tag"),
		"stage_count":     stageCount,
		"recipient_count": recipientCount,
		"action_count":    actionCount,
		"status_counts":   statusCounts,
		"dispatch_queue":  "slow",
		"task_status":     batchStatus,
		"task_error":      taskError,
		"action_preview":  actionPreview,
		"created_at":      sopFactTime(first, "queued_at", "created_at"),
		"completed_at":    sopFactTime(first, "delivered_at", "failed_at"),
		"event":           "sop_delivery_fact_batch",
		"trigger_event":   sopFactResendTriggerEvent(autoResendMeta),
		"details":         details,
	}
	for key, value := range autoResendMeta {
		row[key] = value
	}
	for key, value := range buildSOPResendState(batchStatus, taskID, sourcePayloadJSON, "fact") {
		row[key] = value
	}
	return row
}

func (service Service) enrichSOPDispatchTaskRows(ctx context.Context, batches []ProjectionRow) {
	rows := collectSOPDispatchRows(batches)
	if len(rows) == 0 {
		return
	}
	service.enrichSOPDispatchReceivers(ctx, rows)
	service.enrichSOPDispatchAccounts(ctx, rows)
}

func collectSOPDispatchRows(batches []ProjectionRow) []ProjectionRow {
	rows := make([]ProjectionRow, 0, len(batches))
	for _, batch := range batches {
		if batch == nil {
			continue
		}
		rows = append(rows, batch)
		switch details := batch["details"].(type) {
		case []ProjectionRow:
			for _, detail := range details {
				if detail != nil {
					rows = append(rows, detail)
				}
			}
		case []any:
			for _, item := range details {
				if detail, ok := item.(ProjectionRow); ok && detail != nil {
					rows = append(rows, detail)
				}
			}
		}
	}
	return rows
}

func (service Service) enrichSOPDispatchReceivers(ctx context.Context, rows []ProjectionRow) {
	if service.Projection == nil || len(rows) == 0 {
		return
	}
	conversationIDs := uniqueSOPDispatchRowTexts(rows, "conversation_id")
	if len(conversationIDs) == 0 {
		return
	}
	projectionRows, err := service.Projection.ListRows(ctx, ProjectionQuery{
		ConversationIDs: conversationIDs,
		ModeFilter:      "all",
		StatusFilter:    "all",
		Limit:           len(conversationIDs),
	})
	if err != nil {
		for _, row := range rows {
			if rowText(row, "conversation_id") != "" {
				applySOPReceiverDisplay(row, nil)
			}
		}
		return
	}
	byConversationID := make(map[string]ProjectionRow, len(projectionRows))
	for _, row := range projectionRows {
		conversationID := rowText(row, "conversation_id")
		if conversationID != "" {
			byConversationID[conversationID] = row
		}
	}
	for _, row := range rows {
		conversationID := rowText(row, "conversation_id")
		if conversationID == "" {
			continue
		}
		applySOPReceiverDisplay(row, byConversationID[conversationID])
	}
}

func applySOPReceiverDisplay(row ProjectionRow, conversation ProjectionRow) {
	if row == nil {
		return
	}
	if conversation == nil {
		if fallbackName := firstNonEmpty(rowText(row, "sender_name"), rowText(row, "conversation_key"), rowText(row, "external_userid")); fallbackName != "" {
			setSOPRowDefault(row, "receiver_display_name", fallbackName)
			setSOPRowDefault(row, "receiver_name", fallbackName)
		}
		return
	}
	senderRemark := rowText(conversation, "sender_remark")
	senderName := rowText(conversation, "sender_name")
	conversationName := rowText(conversation, "conversation_name")
	externalUserID := firstNonEmpty(rowText(conversation, "external_userid"), rowText(conversation, "sender_id"))
	displayName := firstNonEmpty(senderRemark, senderName, conversationName, externalUserID)
	if displayName != "" {
		row["sender_name"] = displayName
		row["receiver_display_name"] = displayName
	}
	row["receiver_name"] = firstNonEmpty(senderName, conversationName, displayName)
	row["receiver_remark"] = senderRemark
	row["conversation_name"] = conversationName
	if externalUserID != "" && rowText(row, "external_userid") == "" {
		row["external_userid"] = externalUserID
	}
}

func (service Service) enrichSOPDispatchAccounts(ctx context.Context, rows []ProjectionRow) {
	if service.Accounts == nil || len(rows) == 0 {
		return
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return
	}
	accountByID := make(map[string]AccountRecord, len(accounts))
	accountByDevice := make(map[string]AccountRecord, len(accounts))
	accountByWeWork := make(map[string]AccountRecord, len(accounts))
	for _, account := range accounts {
		if accountID := strings.TrimSpace(account.AccountID); accountID != "" {
			accountByID[accountID] = account
		}
		if deviceID := strings.TrimSpace(account.DeviceID); deviceID != "" {
			accountByDevice[deviceID] = account
		}
		if weworkUserID := normalizeSOPWeWorkUserID(account.WeWorkUserID); weworkUserID != "" {
			accountByWeWork[weworkUserID] = account
		}
	}
	for _, row := range rows {
		account, ok := resolveSOPDispatchAccount(row, accountByID, accountByDevice, accountByWeWork)
		if ok {
			applySOPAccountDisplay(row, account)
			continue
		}
		applySOPAccountDisplay(row, AccountRecord{})
	}
}

func resolveSOPDispatchAccount(row ProjectionRow, accountByID map[string]AccountRecord, accountByDevice map[string]AccountRecord, accountByWeWork map[string]AccountRecord) (AccountRecord, bool) {
	if accountID := rowText(row, "account_id"); accountID != "" {
		if account, ok := accountByID[accountID]; ok {
			return account, true
		}
	}
	if weworkUserID := normalizeSOPWeWorkUserID(sopRowWeWorkUserID(row)); weworkUserID != "" {
		if account, ok := accountByWeWork[weworkUserID]; ok {
			return account, true
		}
	}
	if deviceID := rowText(row, "device_id"); deviceID != "" {
		if account, ok := accountByDevice[deviceID]; ok {
			return account, true
		}
	}
	return AccountRecord{}, false
}

func applySOPAccountDisplay(row ProjectionRow, account AccountRecord) {
	if row == nil {
		return
	}
	accountName := firstNonEmpty(account.AccountName, rowText(row, "account_name"))
	weworkUserID := firstNonEmpty(account.WeWorkUserID, sopRowWeWorkUserID(row))
	accountID := firstNonEmpty(account.AccountID, rowText(row, "account_id"))
	deviceID := firstNonEmpty(account.DeviceID, rowText(row, "device_id"))
	if accountID != "" {
		row["account_id"] = accountID
	}
	if accountName != "" {
		row["account_name"] = accountName
	}
	if weworkUserID != "" {
		row["wework_user_id"] = weworkUserID
	}
	if deviceID != "" {
		row["device_id"] = deviceID
	}
	if displayName := formatSOPAccountDisplay(accountName, weworkUserID); displayName != "" {
		row["account_display_name"] = displayName
	}
}

func sopRowWeWorkUserID(row ProjectionRow) string {
	explicit := rowText(row, "wework_user_id")
	if explicit != "" {
		return explicit
	}
	parts := strings.Split(rowText(row, "conversation_id"), ":")
	if len(parts) >= 3 && strings.EqualFold(parts[0], "ww") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func formatSOPAccountDisplay(accountName string, weworkUserID string) string {
	name := strings.TrimSpace(accountName)
	userID := strings.TrimSpace(weworkUserID)
	if name != "" && userID != "" && !strings.Contains(normalizeSOPWeWorkUserID(name), normalizeSOPWeWorkUserID(userID)) {
		return name + "-" + userID
	}
	if name != "" {
		return name
	}
	return userID
}

func normalizeSOPWeWorkUserID(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func setSOPRowDefault(row ProjectionRow, key string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if rowText(row, key) == "" {
		row[key] = value
	}
}

func uniqueSOPDispatchRowTexts(rows []ProjectionRow, key string) []string {
	seen := make(map[string]struct{}, len(rows))
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value := rowText(row, key)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func (service Service) applySOPAutoResendPendingState(ctx context.Context, batches []ProjectionRow) {
	if service.SOPAutoResendPendingStore == nil || len(batches) == 0 {
		return
	}
	rows := collectSOPDispatchRows(batches)
	taskIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, row := range rows {
		if !truthy(row["can_resend"]) {
			continue
		}
		taskID := rowText(row, "task_id")
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		taskIDs = append(taskIDs, taskID)
	}
	if len(taskIDs) == 0 {
		return
	}
	pending := map[string]struct{}{}
	for _, taskID := range taskIDs {
		ok, err := service.SOPAutoResendPendingStore.IsSOPAutoResendPending(ctx, taskID)
		if err != nil {
			continue
		}
		if ok {
			pending[taskID] = struct{}{}
		}
	}
	if len(pending) == 0 {
		return
	}
	const reason = "自动补发排队中，请等待自动补发结果"
	for _, row := range rows {
		if _, ok := pending[rowText(row, "task_id")]; !ok {
			continue
		}
		row["can_resend"] = false
		row["resend_block_reason"] = reason
	}
}

func sopFactToDispatchTask(row ProjectionRow) ProjectionRow {
	status := strings.ToLower(strings.TrimSpace(rowText(row, "delivery_status")))
	if status == "" {
		status = "pending"
	}
	taskID := rowText(row, "task_id")
	sourcePayloadJSON := rowText(row, "source_payload_json")
	autoResendMeta := sopFactAutoResendMeta(row)
	item := ProjectionRow{
		"task_id":                 taskID,
		"ai_trace_id":             rowText(row, "message_trace_id"),
		"flow_id":                 rowText(row, "flow_id"),
		"flow_name":               rowText(row, "flow_name"),
		"conversation_id":         rowText(row, "conversation_id"),
		"sender_name":             firstNonEmpty(rowText(row, "conversation_key"), rowText(row, "external_userid")),
		"account_id":              rowText(row, "account_id"),
		"device_id":               rowText(row, "device_id"),
		"wework_user_id":          rowText(row, "wework_user_id"),
		"day_stage":               rowText(row, "day_stage"),
		"customer_state":          rowText(row, "customer_state"),
		"stage_tag":               rowText(row, "customer_stage_tag"),
		"stage_unique_id":         rowText(row, "stage_unique_id"),
		"stage_name":              rowText(row, "stage_name"),
		"trigger_event":           sopFactResendTriggerEvent(autoResendMeta),
		"action_count":            rowInt(row, "message_count"),
		"action_preview":          []ProjectionRow{},
		"message_details":         sopFactMessageDetails(row),
		"dispatch_queue":          "slow",
		"task_status":             normalizeSuccessStatus(status),
		"task_error":              cleanSOPTaskError(status, rowText(row, "delivery_error")),
		"assignee_id":             rowText(row, "assignee_id"),
		"assignee_name":           rowText(row, "assignee_name"),
		"entity":                  rowText(row, "enterprise_id"),
		"created_at":              sopFactTime(row, "queued_at", "created_at", "delivered_at", "failed_at"),
		"completed_at":            sopFactTime(row, "delivered_at", "failed_at"),
		"event":                   "sop_delivery_fact",
		"customer_replied":        rowBool(row, "customer_replied"),
		"first_customer_reply_at": sopFactTime(row, "first_customer_reply_at"),
		"ai_reply_status":         rowText(row, "ai_reply_status"),
		"ai_reply_at":             sopFactTime(row, "ai_reply_at"),
	}
	for key, value := range autoResendMeta {
		item[key] = value
	}
	for key, value := range buildSOPResendState(status, taskID, sourcePayloadJSON, "fact") {
		item[key] = value
	}
	return item
}

func sopFactActionPreview(row ProjectionRow, limit int) []ProjectionRow {
	preview := make([]ProjectionRow, 0, limit)
	for _, action := range parseSOPFactActions(row) {
		content := strings.TrimSpace(action["content"])
		if content == "" {
			continue
		}
		actionType := strings.ToLower(strings.TrimSpace(action["type"]))
		if actionType == "" {
			actionType = "text"
		}
		preview = append(preview, ProjectionRow{"type": actionType, "content_preview": truncateRunes(content, 80)})
		if len(preview) >= limit {
			break
		}
	}
	return preview
}

func sopFactMessageDetails(row ProjectionRow) []ProjectionRow {
	status := strings.ToLower(strings.TrimSpace(rowText(row, "delivery_status")))
	if status == "" {
		status = "pending"
	}
	taskError := cleanSOPTaskError(status, rowText(row, "delivery_error"))
	details := make([]ProjectionRow, 0)
	for index, action := range parseSOPFactActions(row) {
		content := firstNonEmpty(action["content"], action["content_preview"], action["text"])
		messageType := strings.ToLower(strings.TrimSpace(action["type"]))
		if messageType == "" {
			messageType = "text"
		}
		stageUniqueID := firstNonEmpty(action["stage_unique_id"], rowText(row, "stage_unique_id"))
		details = append(details, ProjectionRow{
			"message_index":   index + 1,
			"type":            messageType,
			"content":         content,
			"stage_unique_id": stageUniqueID,
			"task_status":     normalizeSuccessStatus(status),
			"task_error":      taskError,
		})
	}
	return details
}

func parseSOPFactActions(row ProjectionRow) []map[string]string {
	payload := sopFactSourcePayload(row)
	rawActions, ok := payload["actions"].([]any)
	if !ok {
		return nil
	}
	actions := make([]map[string]string, 0, len(rawActions))
	for _, raw := range rawActions {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(item["content"])) == "" {
			continue
		}
		action := map[string]string{}
		for key, value := range item {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				action[key] = text
			}
		}
		actions = append(actions, action)
	}
	return actions
}

func sopFactSourcePayload(row ProjectionRow) map[string]any {
	raw := rowText(row, "source_payload_json")
	if raw == "" {
		return map[string]any{}
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func sopFactAutoResendMeta(row ProjectionRow) ProjectionRow {
	payload := sopFactSourcePayload(row)
	originalTaskID := strings.TrimSpace(fmt.Sprint(payload["original_task_id"]))
	if originalTaskID == "<nil>" {
		originalTaskID = ""
	}
	attempt := anyInt(payload["auto_resend_attempt"])
	if originalTaskID == "" && attempt <= 0 {
		return ProjectionRow{}
	}
	return ProjectionRow{
		"original_task_id":           originalTaskID,
		"auto_resend_attempt":        attempt,
		"auto_resend_reason":         jsonText(payload["auto_resend_reason"]),
		"auto_resend_original_error": jsonText(payload["auto_resend_original_error"]),
	}
}

func sopFactResendTriggerEvent(meta ProjectionRow) string {
	if strings.TrimSpace(fmt.Sprint(meta["original_task_id"])) == "" || strings.TrimSpace(fmt.Sprint(meta["original_task_id"])) == "<nil>" {
		return "platform_pull"
	}
	if anyInt(meta["auto_resend_attempt"]) > 0 || jsonText(meta["auto_resend_reason"]) != "" || jsonText(meta["auto_resend_original_error"]) != "" {
		return "auto_resend"
	}
	return "manual_resend"
}

func buildSOPResendState(taskStatus string, taskID string, sourcePayloadJSON string, sourceKind string) ProjectionRow {
	status := strings.ToLower(strings.TrimSpace(taskStatus))
	normalizedTaskID := strings.TrimSpace(taskID)
	hasSourcePayload := strings.TrimSpace(sourcePayloadJSON) != ""
	if status != "failed" {
		return ProjectionRow{"can_resend": false, "resend_block_reason": "只有失败任务才需要补发"}
	}
	if normalizedTaskID == "" {
		return ProjectionRow{"can_resend": false, "resend_block_reason": "历史审计记录缺少任务 ID 和原始下发内容，无法补发"}
	}
	if sourceKind == "audit" || !hasSourcePayload {
		return ProjectionRow{"can_resend": false, "resend_block_reason": "历史审计记录只保留摘要，缺少原始下发内容，无法补发"}
	}
	return ProjectionRow{"can_resend": true, "resend_block_reason": ""}
}

func resolveSOPBatchStatus(statuses []string) string {
	normalized := make([]string, 0, len(statuses))
	for _, status := range statuses {
		text := strings.ToLower(strings.TrimSpace(status))
		if text != "" {
			normalized = append(normalized, text)
		}
	}
	if len(normalized) == 0 {
		return "pending"
	}
	if containsText(normalized, "failed") {
		return "failed"
	}
	if containsText(normalized, "resent") {
		return "resent"
	}
	allSuccessful := true
	for _, status := range normalized {
		if status != "success" && status != "sent" && status != "completed" {
			allSuccessful = false
			break
		}
	}
	if allSuccessful {
		return "success"
	}
	if containsText(normalized, "dispatched") {
		return "dispatched"
	}
	if containsText(normalized, "accepted") {
		return "accepted"
	}
	return normalized[0]
}

func sopFactTime(row ProjectionRow, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if formatted := formatBeijingAPIISO(value); formatted != "" {
				return formatted
			}
		}
	}
	return ""
}

func cleanSOPTaskError(status string, taskError string) string {
	if successfulSOPStatus(status) && strings.HasPrefix(strings.TrimSpace(taskError), "dispatched via ") {
		return ""
	}
	return strings.TrimSpace(taskError)
}

func successfulSOPStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "dispatched", "success", "completed", "sent":
		return true
	default:
		return false
	}
}

func normalizeSuccessStatus(status string) string {
	if strings.ToLower(strings.TrimSpace(status)) == "success" {
		return "success"
	}
	return strings.ToLower(strings.TrimSpace(status))
}

func containsText(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func jsonText(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}
