// Package workbenchobservability reads legacy runtime observability tables for
// the admin monitoring dashboard.
package workbenchobservability

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Queryer is the database/sql shape needed by the observability repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Repository reads dashboard facts from runtime_metric_snapshots, error_events,
// and pipeline_spans.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListObservabilityMetricRows returns metric snapshots ordered oldest first.
func (repository *Repository) ListObservabilityMetricRows(ctx context.Context, metricNames []string, since time.Time, limit int) ([]workbench.ObservabilityMetricRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench observability database is not configured")
	}
	names := normalizeMetricNames(metricNames)
	if len(names) == 0 {
		return []workbench.ObservabilityMetricRecord{}, nil
	}
	query := `
SELECT
    metric_group,
    metric_name,
    metric_scope,
    source_type,
    unit,
    value,
    numerator,
    denominator,
    threshold_value,
    status,
    tenant_id,
    device_id,
    dimensions_json,
    observed_at
FROM runtime_metric_snapshots
WHERE metric_name IN (` + placeholders(len(names)) + `)
  AND observed_at >= ?
ORDER BY observed_at ASC
LIMIT ?`
	args := make([]any, 0, len(names)+2)
	for _, name := range names {
		args = append(args, name)
	}
	args = append(args, repository.dbDatetimeParam(since), boundedLimit(limit))
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ObservabilityMetricRecord, 0)
	for rows.Next() {
		var metricGroup any
		var metricName any
		var metricScope any
		var sourceType any
		var unit any
		var value any
		var numerator any
		var denominator any
		var thresholdValue any
		var status any
		var tenantID any
		var deviceID any
		var dimensionsJSON any
		var observedAt any
		if err := rows.Scan(&metricGroup, &metricName, &metricScope, &sourceType, &unit, &value, &numerator, &denominator, &thresholdValue, &status, &tenantID, &deviceID, &dimensionsJSON, &observedAt); err != nil {
			return nil, err
		}
		records = append(records, workbench.ObservabilityMetricRecord{
			MetricGroup:    stringFromDB(metricGroup),
			MetricName:     stringFromDB(metricName),
			MetricScope:    stringFromDB(metricScope),
			SourceType:     stringFromDB(sourceType),
			Unit:           stringFromDB(unit),
			Value:          floatFromDB(value),
			Numerator:      floatPointerFromDB(numerator),
			Denominator:    floatPointerFromDB(denominator),
			ThresholdValue: floatPointerFromDB(thresholdValue),
			Status:         stringFromDB(status),
			TenantID:       stringFromDB(tenantID),
			DeviceID:       stringFromDB(deviceID),
			Dimensions:     jsonObjectFromDB(dimensionsJSON),
			ObservedAt:     timeFromDB(observedAt),
		})
	}
	return records, rows.Err()
}

// ListObservabilityRecentEvents returns recent error and warning events.
func (repository *Repository) ListObservabilityRecentEvents(ctx context.Context, since time.Time, limit int) ([]workbench.ObservabilityEventRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench observability database is not configured")
	}
	query := `
SELECT
    event_id,
    trace_id,
    level,
    source_type,
    event_category,
    event_code,
    module,
    action,
    device_id,
    tenant_id,
    conversation_id,
    task_id,
    wework_user_id,
    scope_type,
    scope_id,
    error_type,
    error_message,
    occurred_at
FROM error_events
WHERE occurred_at >= ?
ORDER BY occurred_at DESC
LIMIT ?`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(since), boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ObservabilityEventRecord, 0)
	for rows.Next() {
		var eventID any
		var traceID any
		var level any
		var sourceType any
		var eventCategory any
		var eventCode any
		var module any
		var action any
		var deviceID any
		var tenantID any
		var conversationID any
		var taskID any
		var weworkUserID any
		var scopeType any
		var scopeID any
		var errorType any
		var errorMessage any
		var occurredAt any
		if err := rows.Scan(&eventID, &traceID, &level, &sourceType, &eventCategory, &eventCode, &module, &action, &deviceID, &tenantID, &conversationID, &taskID, &weworkUserID, &scopeType, &scopeID, &errorType, &errorMessage, &occurredAt); err != nil {
			return nil, err
		}
		records = append(records, workbench.ObservabilityEventRecord{
			EventID:        stringFromDB(eventID),
			TraceID:        stringFromDB(traceID),
			Level:          stringFromDB(level),
			SourceType:     stringFromDB(sourceType),
			EventCategory:  stringFromDB(eventCategory),
			EventCode:      stringFromDB(eventCode),
			Module:         stringFromDB(module),
			Action:         stringFromDB(action),
			DeviceID:       stringFromDB(deviceID),
			TenantID:       stringFromDB(tenantID),
			ConversationID: stringFromDB(conversationID),
			TaskID:         stringFromDB(taskID),
			WeworkUserID:   stringFromDB(weworkUserID),
			ScopeType:      stringFromDB(scopeType),
			ScopeID:        stringFromDB(scopeID),
			ErrorType:      stringFromDB(errorType),
			ErrorMessage:   stringFromDB(errorMessage),
			OccurredAt:     timeFromDB(occurredAt),
		})
	}
	return records, rows.Err()
}

// ListObservabilitySlowSpans returns the slowest recent pipeline spans.
func (repository *Repository) ListObservabilitySlowSpans(ctx context.Context, since time.Time, limit int) ([]workbench.ObservabilitySpanRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench observability database is not configured")
	}
	query := `
SELECT
    span_id,
    trace_id,
    pipeline_type,
    stage_name,
    device_id,
    tenant_id,
    conversation_id,
    task_id,
    wework_user_id,
    status,
    error_message,
    duration_ms,
    started_at,
    finished_at
FROM pipeline_spans
WHERE created_at >= ?
ORDER BY duration_ms DESC, created_at DESC
LIMIT ?`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(since), boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ObservabilitySpanRecord, 0)
	for rows.Next() {
		var spanID any
		var traceID any
		var pipelineType any
		var stageName any
		var deviceID any
		var tenantID any
		var conversationID any
		var taskID any
		var weworkUserID any
		var status any
		var errorMessage any
		var durationMS any
		var startedAt any
		var finishedAt any
		if err := rows.Scan(&spanID, &traceID, &pipelineType, &stageName, &deviceID, &tenantID, &conversationID, &taskID, &weworkUserID, &status, &errorMessage, &durationMS, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		records = append(records, workbench.ObservabilitySpanRecord{
			SpanID:         stringFromDB(spanID),
			TraceID:        stringFromDB(traceID),
			PipelineType:   stringFromDB(pipelineType),
			StageName:      stringFromDB(stageName),
			DeviceID:       stringFromDB(deviceID),
			TenantID:       stringFromDB(tenantID),
			ConversationID: stringFromDB(conversationID),
			TaskID:         stringFromDB(taskID),
			WeworkUserID:   stringFromDB(weworkUserID),
			Status:         stringFromDB(status),
			ErrorMessage:   stringFromDB(errorMessage),
			DurationMS:     floatFromDB(durationMS),
			StartedAt:      timeFromDB(startedAt),
			FinishedAt:     timeFromDB(finishedAt),
		})
	}
	return records, rows.Err()
}

// ListObservabilityStageLatency returns grouped average stage latency.
func (repository *Repository) ListObservabilityStageLatency(ctx context.Context, since time.Time, limit int) ([]workbench.ObservabilityStageLatencyRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench observability database is not configured")
	}
	query := `
SELECT
    pipeline_type,
    stage_name,
    AVG(duration_ms) AS avg_duration_ms,
    MAX(duration_ms) AS max_duration_ms,
    COUNT(*) AS sample_count
FROM pipeline_spans
WHERE created_at >= ?
GROUP BY pipeline_type, stage_name
ORDER BY avg_duration_ms DESC, sample_count DESC
LIMIT ?`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(since), boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ObservabilityStageLatencyRecord, 0)
	for rows.Next() {
		var pipelineType any
		var stageName any
		var avgDuration any
		var maxDuration any
		var sampleCount any
		if err := rows.Scan(&pipelineType, &stageName, &avgDuration, &maxDuration, &sampleCount); err != nil {
			return nil, err
		}
		records = append(records, workbench.ObservabilityStageLatencyRecord{
			PipelineType:  stringFromDB(pipelineType),
			StageName:     stringFromDB(stageName),
			AvgDurationMS: floatFromDB(avgDuration),
			MaxDurationMS: floatFromDB(maxDuration),
			SampleCount:   intFromDB(sampleCount),
		})
	}
	return records, rows.Err()
}

func normalizeMetricNames(input []string) []string {
	output := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		output = append(output, name)
	}
	return output
}

func (repository *Repository) dbDatetimeParam(value time.Time) string {
	beijing := value.In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return beijing.Format(time.RFC3339)
	}
	return beijing.Format("2006-01-02 15:04:05")
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ", ")
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	return limit
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
	case float64:
		return int(typed)
	case []byte:
		return parseIntText(string(typed))
	case string:
		return parseIntText(typed)
	default:
		return parseIntText(fmt.Sprint(typed))
	}
}

func floatFromDB(value any) float64 {
	if pointer := floatPointerFromDB(value); pointer != nil {
		return *pointer
	}
	return 0
}

func floatPointerFromDB(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		return &typed
	case float32:
		converted := float64(typed)
		return &converted
	case int:
		converted := float64(typed)
		return &converted
	case int64:
		converted := float64(typed)
		return &converted
	case []byte:
		return parseFloatPointerText(string(typed))
	case string:
		return parseFloatPointerText(typed)
	default:
		return parseFloatPointerText(fmt.Sprint(typed))
	}
}

func jsonObjectFromDB(value any) map[string]any {
	text := stringFromDB(value)
	if text == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		if typed.IsZero() {
			return time.Time{}
		}
		if typed.Location() == time.Local {
			return typed.UTC()
		}
		return typed.UTC()
	case []byte:
		return parseDBTimeText(string(typed))
	case string:
		return parseDBTimeText(typed)
	default:
		return parseDBTimeText(fmt.Sprint(typed))
	}
}

func parseDBTimeText(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func parseFloatPointerText(value string) *float64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	var parsed float64
	if _, err := fmt.Sscanf(text, "%f", &parsed); err != nil {
		return nil
	}
	return &parsed
}

func parseIntText(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

type sqlQueryer struct {
	db *sql.DB
}

// QueryContext delegates to database/sql while preserving a tiny test boundary.
func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}
