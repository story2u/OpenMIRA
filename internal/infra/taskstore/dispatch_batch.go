package taskstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

var sdkDispatchBatchableTaskTypes = map[string]struct{}{
	"send_text":           {},
	"send_image":          {},
	"send_video":          {},
	"send_file":           {},
	"appointment_billing": {},
	"send_address":        {},
	"request_money":       {},
	"transfer_money":      {},
}

type sdkDispatchTargetKey [5]string

type sdkDispatchCreatedOrder struct {
	createdAt time.Time
	taskID    string
}

// SDKDispatchFollowupQuery describes a same-device followup selector.
type SDKDispatchFollowupQuery struct {
	FirstTask           tasks.Record
	TaskTypes           []string
	Limit               int
	ForUpdateSkipLocked bool
}

// SDKDispatchBatchClaimQuery describes durable same-chat followup claim options.
type SDKDispatchBatchClaimQuery struct {
	FirstTask           tasks.Record
	TaskTypes           []string
	WorkerID            string
	MaxSize             int
	SkipInterleaved     bool
	ForUpdateSkipLocked bool
}

// BuildSDKDispatchFollowupSelect mirrors Python _build_sdk_dispatch_followup_select.
func BuildSDKDispatchFollowupSelect(query SDKDispatchFollowupQuery) (string, []any, error) {
	taskTypes := cleanBatchableTaskTypes(query.TaskTypes)
	if len(taskTypes) == 0 {
		return "", nil, fmt.Errorf("batchable task_types is required for sdk dispatch followup claim")
	}
	deviceID := strings.TrimSpace(query.FirstTask.Target.DeviceID)
	if deviceID == "" {
		return "", nil, fmt.Errorf("first task device_id is required for sdk dispatch followup claim")
	}
	limit := query.Limit
	if limit < 1 {
		limit = 1
	}
	args := []any{
		string(tasks.StatusAccepted),
		deviceID,
		"sdk:%",
	}
	for _, taskType := range taskTypes {
		args = append(args, taskType)
	}
	args = append(args,
		`%"queue": "fast"%`,
		`%"queue":"fast"%`,
		`%"queue": "slow"%`,
		`%"queue":"slow"%`,
		limit,
	)
	sql := `
SELECT task_id, source, target_agent_id, target_device_id, task_type, payload_json,
       status, created_at, updated_at, trace_id, error, retry_count, next_retry_at,
       wework_user_id, enterprise_id, dispatched_at, script_started_at
FROM tasks
WHERE status = ?
  AND target_device_id = ?
  AND target_agent_id LIKE ?
  AND task_type IN (` + placeholders(len(taskTypes)) + `)
ORDER BY
    CASE
        WHEN payload_json LIKE ? OR payload_json LIKE ? THEN 0
        WHEN payload_json LIKE ? OR payload_json LIKE ? THEN 10
        ELSE 0
    END,
    created_at ASC,
    task_id ASC
LIMIT ?`
	if query.ForUpdateSkipLocked {
		sql += " FOR UPDATE SKIP LOCKED"
	}
	return sql, args, nil
}

// ClaimSDKDispatchTaskBatchAfter claims accepted same-chat followups after an already-running task.
func (repository *Repository) ClaimSDKDispatchTaskBatchAfter(ctx context.Context, query SDKDispatchBatchClaimQuery, now time.Time) ([]tasks.Record, error) {
	if query.MaxSize <= 0 {
		return []tasks.Record{}, nil
	}
	firstKey, ok := sdkDispatchBatchGroupKey(query.FirstTask)
	if !ok {
		return []tasks.Record{}, nil
	}
	if repository.Tx == nil {
		return nil, fmt.Errorf("task transaction database is not configured")
	}
	selectLimit := query.MaxSize
	if query.SkipInterleaved {
		selectLimit = minInt(100, maxInt(query.MaxSize, query.MaxSize*8))
	}
	sqlText, args, err := BuildSDKDispatchFollowupSelect(SDKDispatchFollowupQuery{
		FirstTask:           query.FirstTask,
		TaskTypes:           query.TaskTypes,
		Limit:               selectLimit,
		ForUpdateSkipLocked: query.ForUpdateSkipLocked,
	})
	if err != nil {
		return nil, err
	}
	tx, err := repository.Tx.BeginTaskStoreTx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	rows, err := tx.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	candidates, err := scanTaskRows(rows)
	if err != nil {
		return nil, err
	}
	skipBoundary, hasSkipBoundary := sdkDispatchCreatedOrder{}, false
	if query.SkipInterleaved {
		skipBoundary, hasSkipBoundary = sdkDispatchFirstSkipBoundaryOrder(query.FirstTask, candidates)
	}
	firstTargetKey, _ := sdkDispatchTargetKeyFor(query.FirstTask)
	firstChannel := sdkDispatchQueueChannel(query.FirstTask)
	claimed := make([]tasks.Record, 0, query.MaxSize)
	for _, candidate := range candidates {
		if hasSkipBoundary {
			candidateTargetKey, candidateTargetOK := sdkDispatchTargetKeyFor(candidate)
			if candidateTargetOK && candidateTargetKey == firstTargetKey && !sdkDispatchCreatedOrderKey(candidate).less(skipBoundary) {
				break
			}
		}
		if sdkDispatchQueueChannel(candidate) != firstChannel {
			if query.SkipInterleaved {
				continue
			}
			break
		}
		candidateKey, candidateOK := sdkDispatchBatchGroupKey(candidate)
		if !candidateOK || candidateKey != firstKey {
			if query.SkipInterleaved {
				continue
			}
			break
		}
		claimedTask, ok, err := claimSDKDispatchRow(ctx, tx, candidate.TaskID, query.WorkerID, now)
		if err != nil {
			return nil, err
		}
		if ok {
			claimed = append(claimed, claimedTask)
		}
		if len(claimed) >= query.MaxSize {
			break
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return claimed, nil
}

func sdkDispatchBatchGroupKey(task tasks.Record) (sdkDispatchTargetKey, bool) {
	taskType := strings.TrimSpace(task.TaskType)
	if _, ok := sdkDispatchBatchableTaskTypes[taskType]; !ok {
		return sdkDispatchTargetKey{}, false
	}
	if taskType == "send_image" {
		return sdkDispatchTargetKey{}, false
	}
	if payloadTruthy(task.Payload["preserve_individual_send"]) {
		return sdkDispatchTargetKey{}, false
	}
	return sdkDispatchTargetKeyFor(task)
}

func sdkDispatchTargetKeyFor(task tasks.Record) (sdkDispatchTargetKey, bool) {
	receiver := payloadString(task.Payload, "receiver", "username")
	if receiver == "" {
		return sdkDispatchTargetKey{}, false
	}
	return sdkDispatchTargetKey{
		receiver,
		payloadString(task.Payload, "aliases"),
		payloadString(task.Payload, "entity"),
		payloadString(task.Payload, "conversation_id", "session_id"),
		payloadString(task.Payload, "sender_id"),
	}, true
}

func sdkDispatchBlocksInterleavedSkip(firstTask tasks.Record, candidate tasks.Record) bool {
	firstKey, firstOK := sdkDispatchTargetKeyFor(firstTask)
	candidateKey, candidateOK := sdkDispatchTargetKeyFor(candidate)
	if !firstOK || !candidateOK || candidateKey != firstKey {
		return false
	}
	return strings.TrimSpace(candidate.TaskType) == "send_image" || payloadTruthy(candidate.Payload["preserve_individual_send"])
}

func sdkDispatchCreatedOrderKey(task tasks.Record) sdkDispatchCreatedOrder {
	return sdkDispatchCreatedOrder{createdAt: normalizeTaskTime(task.CreatedAt), taskID: strings.TrimSpace(task.TaskID)}
}

func sdkDispatchFirstSkipBoundaryOrder(firstTask tasks.Record, candidates []tasks.Record) (sdkDispatchCreatedOrder, bool) {
	var selected sdkDispatchCreatedOrder
	found := false
	for _, candidate := range candidates {
		if !sdkDispatchBlocksInterleavedSkip(firstTask, candidate) {
			continue
		}
		order := sdkDispatchCreatedOrderKey(candidate)
		if !found || order.less(selected) {
			selected = order
			found = true
		}
	}
	return selected, found
}

func sdkDispatchQueueChannel(task tasks.Record) string {
	if strings.EqualFold(payloadString(task.Payload, "queue"), "slow") {
		return "slow"
	}
	return "fast"
}

func cleanBatchableTaskTypes(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := sdkDispatchBatchableTaskTypes[value]; value != "" && ok {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func scanTaskRows(rows RowsScanner) ([]tasks.Record, error) {
	defer rows.Close()
	records := make([]tasks.Record, 0)
	for rows.Next() {
		record, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func payloadTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return true
	}
}

func normalizeTaskTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}

func (order sdkDispatchCreatedOrder) less(other sdkDispatchCreatedOrder) bool {
	if order.createdAt.Equal(other.createdAt) {
		return order.taskID < other.taskID
	}
	return order.createdAt.Before(other.createdAt)
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
