// Package workbenchsopflows reads and writes SOP flow-level configs for admin
// candidates. Platform task execution remains with Python during migration.
package workbenchsopflows

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

// Queryer is the database/sql shape needed by the SOP flow repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes sop_flow_configs rows for admin candidates.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := ""
	if len(dialect) > 0 {
		resolvedDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// ListSOPFlows returns flow configs with default flow first.
func (repository *Repository) ListSOPFlows(ctx context.Context) ([]workbench.SOPFlowRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench sop flow database is not configured")
	}
	query := "SELECT flow_id, flow_name, target_audience, execution_mode, day_count, platform_pull_driver, platform_task_limit, platform_dispatch_queue, platform_task_url, execution_time_windows, enabled, human_handoff_rule, risk_keywords, created_at, updated_at FROM sop_flow_configs ORDER BY CASE WHEN flow_id='default' THEN 0 ELSE 1 END, flow_id ASC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanFlowRows(rows)
}

// UpsertSOPFlow creates or updates one SOP flow config.
func (repository *Repository) UpsertSOPFlow(ctx context.Context, command workbench.SOPFlowCommand) (workbench.SOPFlowRecord, error) {
	if repository.DB == nil {
		return workbench.SOPFlowRecord{}, fmt.Errorf("workbench sop flow database is not configured")
	}
	flowID := defaultString(command.FlowID, "default")
	flowName := defaultString(command.FlowName, flowID)
	platformPullDriver := normalizePlatformPullDriver(command.PlatformPullDriver)
	now := dbNow(repository.Dialect)
	if _, err := repository.DB.ExecContext(
		ctx,
		repository.upsertSQL(),
		flowID,
		flowName,
		strings.TrimSpace(command.TargetAudience),
		defaultString(command.ExecutionMode, "local_days"),
		maxInt(1, command.DayCount),
		platformPullDriver,
		maxInt(1, command.PlatformTaskLimit),
		defaultString(command.PlatformDispatchQueue, "slow"),
		strings.TrimSpace(command.PlatformTaskURL),
		strings.TrimSpace(command.ExecutionTimeWindows),
		boolInt(command.Enabled),
		strings.TrimSpace(command.HumanHandoffRule),
		strings.TrimSpace(command.RiskKeywords),
		now,
		now,
	); err != nil {
		return workbench.SOPFlowRecord{}, err
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT flow_id, flow_name, target_audience, execution_mode, day_count, platform_pull_driver, platform_task_limit, platform_dispatch_queue, platform_task_url, execution_time_windows, enabled, human_handoff_rule, risk_keywords, created_at, updated_at FROM sop_flow_configs WHERE flow_id = ?", flowID)
	if err != nil {
		return workbench.SOPFlowRecord{}, err
	}
	records, err := scanFlowRows(rows)
	if err != nil {
		return workbench.SOPFlowRecord{}, err
	}
	if len(records) == 0 {
		return workbench.SOPFlowRecord{}, fmt.Errorf("sop flow was not found after upsert")
	}
	return records[0], nil
}

// DeleteSOPFlow removes one flow and its policies, reporting whether anything existed.
func (repository *Repository) DeleteSOPFlow(ctx context.Context, flowID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench sop flow database is not configured")
	}
	normalizedFlowID := strings.TrimSpace(flowID)
	flowResult, err := repository.DB.ExecContext(ctx, "DELETE FROM sop_flow_configs WHERE flow_id = ?", normalizedFlowID)
	if err != nil {
		return false, err
	}
	policyResult, err := repository.DB.ExecContext(ctx, "DELETE FROM sop_policies WHERE flow_id = ?", normalizedFlowID)
	if err != nil {
		return false, err
	}
	flowAffected, _ := flowResult.RowsAffected()
	policyAffected, _ := policyResult.RowsAffected()
	return flowAffected+policyAffected > 0, nil
}

func scanFlowRows(rows RowsScanner) ([]workbench.SOPFlowRecord, error) {
	defer rows.Close()
	records := make([]workbench.SOPFlowRecord, 0)
	for rows.Next() {
		var flowID any
		var flowName any
		var targetAudience any
		var executionMode any
		var dayCount any
		var platformPullDriver any
		var platformTaskLimit any
		var platformDispatchQueue any
		var platformTaskURL any
		var executionTimeWindows any
		var enabled any
		var humanHandoffRule any
		var riskKeywords any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&flowID, &flowName, &targetAudience, &executionMode, &dayCount, &platformPullDriver, &platformTaskLimit, &platformDispatchQueue, &platformTaskURL, &executionTimeWindows, &enabled, &humanHandoffRule, &riskKeywords, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedFlowID := defaultString(stringFromDB(flowID), "default")
		records = append(records, workbench.SOPFlowRecord{
			FlowID:                normalizedFlowID,
			FlowName:              defaultString(stringFromDB(flowName), normalizedFlowID),
			TargetAudience:        stringFromDB(targetAudience),
			ExecutionMode:         defaultString(stringFromDB(executionMode), "local_days"),
			DayCount:              maxInt(1, intFromDB(dayCount, 1)),
			PlatformPullDriver:    normalizePlatformPullDriver(stringFromDB(platformPullDriver)),
			PlatformTaskLimit:     maxInt(1, intFromDB(platformTaskLimit, 20)),
			PlatformDispatchQueue: defaultString(stringFromDB(platformDispatchQueue), "slow"),
			PlatformTaskURL:       stringFromDB(platformTaskURL),
			ExecutionTimeWindows:  stringFromDB(executionTimeWindows),
			Enabled:               boolFromDB(enabled, true),
			HumanHandoffRule:      stringFromDB(humanHandoffRule),
			RiskKeywords:          stringFromDB(riskKeywords),
			CreatedAt:             timeFromDB(createdAt),
			UpdatedAt:             timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO sop_flow_configs (
    flow_id, flow_name, target_audience, execution_mode, day_count, platform_pull_driver, platform_task_limit, platform_dispatch_queue, platform_task_url, execution_time_windows, enabled, human_handoff_rule, risk_keywords, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(flow_id) DO UPDATE SET
    flow_name = EXCLUDED.flow_name,
    target_audience = EXCLUDED.target_audience,
    execution_mode = EXCLUDED.execution_mode,
    day_count = EXCLUDED.day_count,
    platform_pull_driver = EXCLUDED.platform_pull_driver,
    platform_task_limit = EXCLUDED.platform_task_limit,
    platform_dispatch_queue = EXCLUDED.platform_dispatch_queue,
    platform_task_url = EXCLUDED.platform_task_url,
    execution_time_windows = EXCLUDED.execution_time_windows,
    enabled = EXCLUDED.enabled,
    human_handoff_rule = EXCLUDED.human_handoff_rule,
    risk_keywords = EXCLUDED.risk_keywords,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO sop_flow_configs (
    flow_id, flow_name, target_audience, execution_mode, day_count, platform_pull_driver, platform_task_limit, platform_dispatch_queue, platform_task_url, execution_time_windows, enabled, human_handoff_rule, risk_keywords, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    flow_name = VALUES(flow_name),
    target_audience = VALUES(target_audience),
    execution_mode = VALUES(execution_mode),
    day_count = VALUES(day_count),
    platform_pull_driver = VALUES(platform_pull_driver),
    platform_task_limit = VALUES(platform_task_limit),
    platform_dispatch_queue = VALUES(platform_dispatch_queue),
    platform_task_url = VALUES(platform_task_url),
    execution_time_windows = VALUES(execution_time_windows),
    enabled = VALUES(enabled),
    human_handoff_rule = VALUES(human_handoff_rule),
    risk_keywords = VALUES(risk_keywords),
    updated_at = VALUES(updated_at)`
}

func normalizePlatformPullDriver(value string) string {
	normalized := defaultString(value, "conversation")
	if normalized == "conversation" || normalized == "platform_task" {
		return normalized
	}
	return "conversation"
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

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

func boolFromDB(value any, fallback bool) bool {
	switch typed := value.(type) {
	case nil:
		return fallback
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	}
	return fallback
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func intFromDB(value any, fallback int) int {
	switch typed := value.(type) {
	case nil:
		return fallback
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(typed)), "%d", &parsed); err == nil {
			return parsed
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func dbNow(dialect string) any {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
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
