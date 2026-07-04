package workbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/tasks"
)

var (
	ErrSOPResendFlowIDRequired       = errors.New("sop resend flow_id is required")
	ErrSOPResendTaskIDRequired       = errors.New("sop resend task_id is required")
	ErrSOPResendStoreUnavailable     = errors.New("sop delivery fact repository is unavailable")
	ErrSOPResendExecutorUnavailable  = errors.New("sop dispatch resend executor is unavailable")
	ErrSOPResendMissingPersistedData = errors.New("missing persisted actions")
)

// SOPDispatchResendBody mirrors the legacy JSON body.
type SOPDispatchResendBody struct {
	FlowID    any   `json:"flow_id"`
	Date      any   `json:"date"`
	AllFailed bool  `json:"all_failed"`
	TaskID    any   `json:"task_id"`
	TaskIDs   []any `json:"task_ids"`
	Limit     any   `json:"limit"`
}

// SOPDispatchResendQuery contains failed fact filters for manual resend.
type SOPDispatchResendQuery struct {
	Date      string
	FlowID    string
	AllFailed bool
	TaskIDs   []string
	Limit     int
}

// SOPDispatchResendRequest carries POST /sop/dispatch-tasks/resend input.
type SOPDispatchResendRequest struct {
	Session auth.Session
	Query   SOPDispatchResendQuery
}

// SOPDispatchResendGroup is one original task and its persisted actions.
type SOPDispatchResendGroup struct {
	Row     ProjectionRow
	Actions []map[string]string
}

// NewSOPDispatchResendRequest validates and normalizes the legacy body.
func NewSOPDispatchResendRequest(body SOPDispatchResendBody, session auth.Session) (SOPDispatchResendRequest, error) {
	flowID := jsonText(body.FlowID)
	if flowID == "" {
		return SOPDispatchResendRequest{}, ErrSOPResendFlowIDRequired
	}
	taskIDs := cleanStringList(body.TaskIDs)
	if taskID := jsonText(body.TaskID); taskID != "" {
		taskIDs = append(taskIDs, taskID)
	}
	taskIDs = dedupeStrings(taskIDs)
	if !body.AllFailed && len(taskIDs) == 0 {
		return SOPDispatchResendRequest{}, ErrSOPResendTaskIDRequired
	}
	limit := anyInt(body.Limit)
	if limit <= 0 {
		limit = 100
	}
	return SOPDispatchResendRequest{
		Session: session,
		Query: SOPDispatchResendQuery{
			Date:      jsonText(body.Date),
			FlowID:    flowID,
			AllFailed: body.AllFailed,
			TaskIDs:   taskIDs,
			Limit:     limit,
		},
	}, nil
}

// SOPDispatchTasksResend creates resend tasks for failed SOP fact groups.
func (service Service) SOPDispatchTasksResend(ctx context.Context, request SOPDispatchResendRequest) (Payload, error) {
	if service.SOPDispatchResendStore == nil {
		return nil, ErrSOPResendStoreUnavailable
	}
	if service.SOPDispatchResendExecutor == nil {
		return nil, ErrSOPResendExecutorUnavailable
	}
	query := request.Query
	query.Date = service.sopAnalyticsDate(query.Date)
	if query.AllFailed {
		query.TaskIDs = nil
	}
	rows, err := service.SOPDispatchResendStore.ListFailedSOPResendCandidates(ctx, query)
	if err != nil {
		return nil, err
	}
	groups := groupSOPResendRows(rows)
	results := make([]ProjectionRow, 0, len(groups))
	succeeded := 0
	for _, group := range groups {
		result := service.resendSOPGroup(ctx, group)
		if truthy(result["success"]) {
			succeeded++
		}
		results = append(results, result)
	}
	failed := len(results) - succeeded
	return Payload{
		"success":   failed == 0,
		"date":      query.Date,
		"flow_id":   query.FlowID,
		"requested": len(groups),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	}, nil
}

func (service Service) resendSOPGroup(ctx context.Context, group SOPDispatchResendGroup) ProjectionRow {
	originalTaskID := rowText(group.Row, "task_id")
	if originalTaskID == "" {
		originalTaskID = rowText(group.Row, "fact_id")
	}
	if len(group.Actions) == 0 {
		return ProjectionRow{"success": false, "original_task_id": originalTaskID, "error": ErrSOPResendMissingPersistedData.Error()}
	}
	record, err := service.SOPDispatchResendExecutor.ResendSOPDispatch(ctx, group)
	if err != nil {
		return ProjectionRow{"success": false, "original_task_id": originalTaskID, "error": err.Error()}
	}
	resendTaskID := strings.TrimSpace(record.TaskID)
	if resendTaskID == "" {
		resendTaskID = originalTaskID
	}
	_ = service.SOPDispatchResendStore.MarkSOPResendQueued(ctx, originalTaskID, resendTaskID)
	return ProjectionRow{
		"success":          true,
		"original_task_id": originalTaskID,
		"resend_task_id":   resendTaskID,
		"status":           string(record.Status),
	}
}

func groupSOPResendRows(rows []ProjectionRow) []SOPDispatchResendGroup {
	grouped := map[string]int{}
	groups := make([]SOPDispatchResendGroup, 0)
	actionKeys := make([]map[string]struct{}, 0)
	for _, row := range rows {
		taskID := rowText(row, "task_id")
		if taskID == "" {
			taskID = rowText(row, "fact_id")
		}
		if taskID == "" {
			continue
		}
		index, ok := grouped[taskID]
		if !ok {
			index = len(groups)
			grouped[taskID] = index
			groups = append(groups, SOPDispatchResendGroup{Row: cloneProjectionRow(row), Actions: []map[string]string{}})
			actionKeys = append(actionKeys, map[string]struct{}{})
		}
		for _, action := range parseSOPFactActions(row) {
			key := actionKey(action)
			if _, exists := actionKeys[index][key]; exists {
				continue
			}
			actionKeys[index][key] = struct{}{}
			groups[index].Actions = append(groups[index].Actions, action)
		}
	}
	return groups
}

// SOPDispatchTaskExecutor creates send_mixed_messages tasks from persisted facts.
type SOPDispatchTaskExecutor struct {
	Tasks TaskCreator
	Now   func() time.Time
	NewID func(prefix string) string
}

// ResendSOPDispatch builds one durable send_mixed_messages task.
func (executor SOPDispatchTaskExecutor) ResendSOPDispatch(ctx context.Context, group SOPDispatchResendGroup) (tasks.Record, error) {
	if executor.Tasks == nil {
		return tasks.Record{}, ErrSOPResendExecutorUnavailable
	}
	row := group.Row
	originalTaskID := firstNonEmpty(rowText(row, "task_id"), rowText(row, "fact_id"))
	deviceID := rowText(row, "device_id")
	if deviceID == "" {
		return tasks.Record{}, errors.New("missing device_id")
	}
	receiver := firstNonEmpty(rowText(row, "conversation_key"), rowText(row, "external_userid"), rowText(row, "conversation_id"))
	entity := rowText(row, "enterprise_id")
	if receiver == "" || entity == "" {
		return tasks.Record{}, errors.New("missing receiver or entity")
	}
	flowID := firstNonEmpty(rowText(row, "flow_id"), "default")
	taskID := executor.newID("sop-resend-")
	traceID := executor.newID("platform-pull-send-")
	payload := executor.payload(row, group.Actions, taskID, originalTaskID, receiver, entity, flowID)
	enterpriseID := entity
	return executor.Tasks.Create(ctx, tasks.CreateRequest{
		TaskID:       taskID,
		Source:       "system",
		Target:       tasks.Target{AgentID: firstNonEmpty(rowText(row, "agent_id"), "sdk:"+deviceID), DeviceID: deviceID},
		TaskType:     "send_mixed_messages",
		Payload:      payload,
		CreatedAt:    executor.now(),
		TraceID:      &traceID,
		EnterpriseID: &enterpriseID,
	})
}

func (executor SOPDispatchTaskExecutor) payload(row ProjectionRow, actions []map[string]string, taskID string, originalTaskID string, receiver string, entity string, flowID string) map[string]any {
	stageUniqueIDs := uniqueStageIDs(actions)
	messages := make([]any, 0, len(actions))
	for _, action := range actions {
		item := map[string]any{
			"type":    firstNonEmpty(strings.ToLower(strings.TrimSpace(action["type"])), "text"),
			"content": strings.TrimSpace(action["content"]),
		}
		for _, key := range []string{"url", "title", "summary", "description", "icon", "image", "path", "username", "lat", "lng", "latitude", "longitude", "filename", "address", "store_name", "store_id", "tencent_map_store", "button_name", "money", "note", "reason", "stage_unique_id"} {
			if value := strings.TrimSpace(action[key]); value != "" {
				item[key] = value
			}
		}
		messages = append(messages, item)
	}
	sopAudit := map[string]any{
		"source":           "platform_pull",
		"flow_id":          flowID,
		"flow_name":        rowText(row, "flow_name"),
		"trigger_event":    "manual_resend",
		"assignee_id":      rowText(row, "assignee_id"),
		"assignee_name":    rowText(row, "assignee_name"),
		"conversation_id":  rowText(row, "conversation_id"),
		"ai_trace_id":      executor.newID("sop-resend-"),
		"stage_unique_ids": stageUniqueIDs,
		"day_stage":        rowText(row, "day_stage"),
		"customer_state":   rowText(row, "customer_state"),
		"stage_tag":        rowText(row, "customer_stage_tag"),
		"task_id":          taskID,
		"original_task_id": originalTaskID,
	}
	payload := map[string]any{
		"username":        receiver,
		"receiver":        receiver,
		"receiver_name":   receiver,
		"entity":          entity,
		"msg_id":          taskID,
		"queue":           "slow",
		"messages":        messages,
		"conversation_id": rowText(row, "conversation_id"),
		"sender_id":       rowText(row, "external_userid"),
		"sop_audit":       sopAudit,
		"_send_policy": map[string]any{
			"origin":          "sop",
			"source_enabled":  true,
			"conversation_id": rowText(row, "conversation_id"),
			"flow_id":         flowID,
			"trigger_event":   "manual_resend",
		},
	}
	if aliases := sopResendAliases(row); aliases != "" {
		payload["aliases"] = aliases
	}
	if len(stageUniqueIDs) == 1 {
		payload["stage_unique_id"] = stageUniqueIDs[0]
	}
	return payload
}

func sopResendAliases(row ProjectionRow) string {
	payload := sopFactSourcePayload(row)
	return firstNonEmpty(rowText(row, "aliases"), rowText(row, "sender_remark"), jsonText(payload["aliases"]))
}

func (executor SOPDispatchTaskExecutor) now() time.Time {
	if executor.Now != nil {
		return executor.Now().UTC()
	}
	return time.Now().UTC()
}

func (executor SOPDispatchTaskExecutor) newID(prefix string) string {
	if executor.NewID != nil {
		if value := strings.TrimSpace(executor.NewID(prefix)); value != "" {
			return value
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return prefix + "0000000000000000"
	}
	return prefix + hex.EncodeToString(random[:])
}

func cleanStringList(values []any) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if item := jsonText(value); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func actionKey(action map[string]string) string {
	keys := make([]string, 0, len(action))
	for key := range action {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+action[key])
	}
	return strings.Join(parts, "\x00")
}

func uniqueStageIDs(actions []map[string]string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, action := range actions {
		stageID := strings.TrimSpace(action["stage_unique_id"])
		if stageID == "" {
			continue
		}
		if _, ok := seen[stageID]; ok {
			continue
		}
		seen[stageID] = struct{}{}
		result = append(result, stageID)
	}
	return result
}

func truthy(value any) bool {
	result, ok := value.(bool)
	return ok && result
}
