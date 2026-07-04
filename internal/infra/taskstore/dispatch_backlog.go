package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SDKDispatchBacklogQuery describes accepted SDK backlog filters.
type SDKDispatchBacklogQuery struct {
	DeviceIDs []string
	TaskTypes []string
}

// SDKDispatchBacklogDeviceSummary is one device backlog row.
type SDKDispatchBacklogDeviceSummary struct {
	Accepted     int
	OldestAgeSec int
}

// SDKDispatchBacklogSummary mirrors Python summarize_sdk_dispatch_backlog.
type SDKDispatchBacklogSummary struct {
	AcceptedTotal        int
	OldestAcceptedAgeSec int
	ByDevice             map[string]SDKDispatchBacklogDeviceSummary
}

// BuildSDKDispatchBacklogSelect mirrors Python summarize_sdk_dispatch_backlog SQL.
func BuildSDKDispatchBacklogSelect(query SDKDispatchBacklogQuery) (string, []any, error) {
	taskTypes := cleanStrings(query.TaskTypes)
	if len(taskTypes) == 0 {
		return "", nil, fmt.Errorf("task_types is required for sdk dispatch backlog summary")
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
	sqlText := `
SELECT target_device_id, COUNT(*) AS accepted, MIN(created_at) AS oldest_created_at
FROM tasks
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY target_device_id
ORDER BY target_device_id ASC`
	return sqlText, args, nil
}

// SummarizeSDKDispatchBacklog returns accepted durable SDK send backlog grouped by device.
func (repository *Repository) SummarizeSDKDispatchBacklog(ctx context.Context, query SDKDispatchBacklogQuery, now time.Time) (SDKDispatchBacklogSummary, error) {
	summary := SDKDispatchBacklogSummary{ByDevice: map[string]SDKDispatchBacklogDeviceSummary{}}
	sqlText, args, err := BuildSDKDispatchBacklogSelect(query)
	if err != nil {
		return summary, nil
	}
	if repository.DB == nil {
		return summary, fmt.Errorf("task database is not configured")
	}
	if now.IsZero() {
		now = time.Now()
	}
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID sql.NullString
		var accepted int
		var oldestCreatedAt any
		if err := rows.Scan(&deviceID, &accepted, &oldestCreatedAt); err != nil {
			return summary, err
		}
		normalizedDeviceID := strings.TrimSpace(deviceID.String)
		oldestAgeSec := 0
		if oldest := timeFromDB(oldestCreatedAt); !oldest.IsZero() {
			age := now.UTC().Sub(oldest.UTC()).Seconds()
			if age > 0 {
				oldestAgeSec = int(age)
			}
		}
		summary.ByDevice[normalizedDeviceID] = SDKDispatchBacklogDeviceSummary{Accepted: accepted, OldestAgeSec: oldestAgeSec}
		summary.AcceptedTotal += accepted
		if oldestAgeSec > summary.OldestAcceptedAgeSec {
			summary.OldestAcceptedAgeSec = oldestAgeSec
		}
	}
	return summary, rows.Err()
}
