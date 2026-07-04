package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// SDKDispatchClaimQuery describes the durable SDK task claim selector.
type SDKDispatchClaimQuery struct {
	DeviceIDs           []string
	TaskTypes           []string
	ForUpdateSkipLocked bool
}

// BuildSDKDispatchClaimSelect mirrors Python _build_sdk_dispatch_claim_select.
func BuildSDKDispatchClaimSelect(query SDKDispatchClaimQuery) (string, []any, error) {
	taskTypes := cleanStrings(query.TaskTypes)
	if len(taskTypes) == 0 {
		return "", nil, fmt.Errorf("task_types is required for sdk dispatch claim")
	}
	deviceIDs := cleanStrings(query.DeviceIDs)
	if query.DeviceIDs != nil && len(deviceIDs) == 0 {
		return "", nil, fmt.Errorf("device_ids cannot be empty when provided")
	}

	clauses := []string{"status = ?", "target_agent_id LIKE ?"}
	args := []any{"accepted", "sdk:%"}
	clauses = append(clauses, "task_type IN ("+placeholders(len(taskTypes))+")")
	for _, taskType := range taskTypes {
		args = append(args, taskType)
	}
	if query.DeviceIDs != nil {
		clauses = append(clauses, "target_device_id IN ("+placeholders(len(deviceIDs))+")")
		for _, deviceID := range deviceIDs {
			args = append(args, deviceID)
		}
	}
	args = append(args,
		`%"queue": "fast"%`,
		`%"queue":"fast"%`,
		`%"queue": "slow"%`,
		`%"queue":"slow"%`,
	)
	sql := `
SELECT task_id, source, target_agent_id, target_device_id, task_type, payload_json,
       status, created_at, updated_at, trace_id, error, retry_count, next_retry_at,
       wework_user_id, enterprise_id, dispatched_at, script_started_at
FROM tasks
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY
    CASE
        WHEN payload_json LIKE ? OR payload_json LIKE ? THEN 0
        WHEN payload_json LIKE ? OR payload_json LIKE ? THEN 10
        ELSE 0
    END,
    created_at ASC,
    task_id ASC
LIMIT 1`
	if query.ForUpdateSkipLocked {
		sql += " FOR UPDATE SKIP LOCKED"
	}
	return sql, args, nil
}

// ClaimSDKDispatchRow marks one selected accepted SDK task as running.
func (repository *Repository) ClaimSDKDispatchRow(ctx context.Context, taskID string, workerID string, now time.Time) (tasks.Record, bool, error) {
	return claimSDKDispatchRow(ctx, repository.DB, taskID, workerID, now)
}

// ClaimNextSDKDispatchTask atomically selects and claims the next accepted SDK task.
func (repository *Repository) ClaimNextSDKDispatchTask(ctx context.Context, query SDKDispatchClaimQuery, workerID string, now time.Time) (tasks.Record, bool, error) {
	if repository.Tx == nil {
		return tasks.Record{}, false, fmt.Errorf("task transaction database is not configured")
	}
	sqlText, args, err := BuildSDKDispatchClaimSelect(query)
	if err != nil {
		return tasks.Record{}, false, nil
	}
	tx, err := repository.Tx.BeginTaskStoreTx(ctx)
	if err != nil {
		return tasks.Record{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	selected, err := scanTask(tx.QueryRowContext(ctx, sqlText, args...))
	if err != nil {
		if err == sql.ErrNoRows {
			if err := tx.Commit(); err != nil {
				return tasks.Record{}, false, err
			}
			committed = true
			return tasks.Record{}, false, nil
		}
		return tasks.Record{}, false, err
	}
	claimed, ok, err := claimSDKDispatchRow(ctx, tx, selected.TaskID, workerID, now)
	if err != nil {
		return tasks.Record{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return tasks.Record{}, false, err
	}
	committed = true
	return claimed, ok, nil
}

func claimSDKDispatchRow(ctx context.Context, db Queryer, taskID string, workerID string, now time.Time) (tasks.Record, bool, error) {
	if db == nil {
		return tasks.Record{}, false, fmt.Errorf("task database is not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return tasks.Record{}, false, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "unknown"
	}
	result, err := db.ExecContext(ctx, `
UPDATE tasks
SET status = ?, updated_at = ?, error = ?
WHERE task_id = ? AND status = ?`,
		string(tasks.StatusRunning),
		dbTime(now),
		fmt.Sprintf("claimed by sdk dispatcher worker_id=%s", workerID),
		taskID,
		string(tasks.StatusAccepted),
	)
	if err != nil {
		return tasks.Record{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return tasks.Record{}, false, err
	}
	if affected != 1 {
		return tasks.Record{}, false, nil
	}
	claimed, err := scanTask(db.QueryRowContext(ctx, selectTaskSQL("task_id = ?", "LIMIT 1"), taskID))
	if err != nil {
		if err == sql.ErrNoRows {
			return tasks.Record{}, false, nil
		}
		return tasks.Record{}, false, err
	}
	return claimed, true, nil
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func placeholders(count int) string {
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ", ")
}
