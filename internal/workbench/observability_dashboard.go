package workbench

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrObservabilityDashboardStoreUnavailable means runtime observability tables cannot be read.
	ErrObservabilityDashboardStoreUnavailable = errors.New("workbench observability dashboard store is unavailable")
	// ErrInvalidObservabilityHours preserves FastAPI's hours query boundary.
	ErrInvalidObservabilityHours = errors.New("invalid hours, expected 1..48")
	// ErrInvalidObservabilityEventHours preserves FastAPI's event_hours query boundary.
	ErrInvalidObservabilityEventHours = errors.New("invalid event_hours, expected 1..168")
)

var observabilityCurrentMetrics = []string{
	"global_health_status",
	"active_websocket_connections",
	"websocket_queued_messages",
	"websocket_dropped_messages",
	"global_delivery_success_rate",
	"tracing_queue_fill_ratio",
	"error_capture_queue_fill_ratio",
	"incoming_queue_depth",
	"incoming_dead_letter_depth",
	"incoming_queue_retry_scheduled_total",
	"dispatch_pending_tasks",
	"device_saturation_rate",
	"dispatch_timeout_backlog",
	"dispatch_timeout_rate",
	"outbox_pending_events",
	"device_total_count",
	"device_online_rate",
	"wework_logged_in_rate",
	"wework_app_foreground_rate",
	"device_high_cpu_count",
	"device_high_memory_count",
	"circuit_breaker_open_count",
	"archive_sync_stale_seconds",
	"archive_seq_offset_lag",
	"archive_media_pending_tasks",
	"archive_media_running_tasks",
	"archive_media_retryable_tasks",
	"archive_media_oldest_pending_wait_seconds",
}

var observabilitySeriesMetrics = []string{
	"incoming_queue_depth",
	"dispatch_pending_tasks",
	"device_online_rate",
	"global_delivery_success_rate",
	"archive_seq_offset_lag",
	"archive_media_pending_tasks",
	"archive_media_oldest_pending_wait_seconds",
}

// ObservabilityDashboardStore reads the three legacy runtime observability tables.
type ObservabilityDashboardStore interface {
	ListObservabilityMetricRows(ctx context.Context, metricNames []string, since time.Time, limit int) ([]ObservabilityMetricRecord, error)
	ListObservabilityRecentEvents(ctx context.Context, since time.Time, limit int) ([]ObservabilityEventRecord, error)
	ListObservabilitySlowSpans(ctx context.Context, since time.Time, limit int) ([]ObservabilitySpanRecord, error)
	ListObservabilityStageLatency(ctx context.Context, since time.Time, limit int) ([]ObservabilityStageLatencyRecord, error)
}

// Stage6StatusProvider returns the runtime status block embedded by the legacy dashboard.
type Stage6StatusProvider interface {
	Stage6Status(ctx context.Context) (Payload, error)
}

// ObservabilityMetricRecord carries one runtime_metric_snapshots row.
type ObservabilityMetricRecord struct {
	MetricGroup    string
	MetricName     string
	MetricScope    string
	SourceType     string
	Unit           string
	Value          float64
	Numerator      *float64
	Denominator    *float64
	ThresholdValue *float64
	Status         string
	TenantID       string
	DeviceID       string
	Dimensions     map[string]any
	ObservedAt     time.Time
}

// ObservabilityEventRecord carries one error_events row.
type ObservabilityEventRecord struct {
	EventID        string
	TraceID        string
	Level          string
	SourceType     string
	EventCategory  string
	EventCode      string
	Module         string
	Action         string
	DeviceID       string
	TenantID       string
	ConversationID string
	TaskID         string
	WeworkUserID   string
	ScopeType      string
	ScopeID        string
	ErrorType      string
	ErrorMessage   string
	OccurredAt     time.Time
}

// ObservabilitySpanRecord carries one slow pipeline_spans row.
type ObservabilitySpanRecord struct {
	SpanID         string
	TraceID        string
	PipelineType   string
	StageName      string
	DeviceID       string
	TenantID       string
	ConversationID string
	TaskID         string
	WeworkUserID   string
	Status         string
	ErrorMessage   string
	DurationMS     float64
	StartedAt      time.Time
	FinishedAt     time.Time
}

// ObservabilityStageLatencyRecord carries one grouped pipeline stage latency row.
type ObservabilityStageLatencyRecord struct {
	PipelineType  string
	StageName     string
	AvgDurationMS float64
	MaxDurationMS float64
	SampleCount   int
}

// ObservabilityDashboardRequest carries normalized dashboard query parameters.
type ObservabilityDashboardRequest struct {
	Session    auth.Session
	Hours      int
	EventHours int
}

// NewObservabilityDashboardRequest validates FastAPI-compatible query bounds.
func NewObservabilityDashboardRequest(values url.Values, session auth.Session) (ObservabilityDashboardRequest, error) {
	hours, err := parseObservabilityBoundedInt(values.Get("hours"), 6, 1, 48)
	if err != nil {
		return ObservabilityDashboardRequest{}, ErrInvalidObservabilityHours
	}
	eventHours, err := parseObservabilityBoundedInt(values.Get("event_hours"), 24, 1, 168)
	if err != nil {
		return ObservabilityDashboardRequest{}, ErrInvalidObservabilityEventHours
	}
	return ObservabilityDashboardRequest{Session: session, Hours: hours, EventHours: eventHours}, nil
}

// ObservabilityDashboard builds /api/v1/admin/observability/dashboard.
func (service Service) ObservabilityDashboard(ctx context.Context, request ObservabilityDashboardRequest) (Payload, error) {
	if service.ObservabilityDashboardStore == nil {
		return nil, ErrObservabilityDashboardStoreUnavailable
	}
	now := service.now().UTC()
	metricSince := now.Add(-time.Duration(maxInt(1, request.Hours)) * time.Hour)
	eventSince := now.Add(-time.Duration(maxInt(1, request.EventHours)) * time.Hour)

	latestMetricRows, err := service.ObservabilityDashboardStore.ListObservabilityMetricRows(ctx, observabilityCurrentMetrics, now.Add(-48*time.Hour), maxInt(400, len(observabilityCurrentMetrics)*24))
	if err != nil {
		return nil, err
	}
	seriesRows, err := service.ObservabilityDashboardStore.ListObservabilityMetricRows(ctx, observabilitySeriesMetrics, metricSince, maxInt(600, len(observabilitySeriesMetrics)*maxInt(12, request.Hours)*12))
	if err != nil {
		return nil, err
	}
	recentEvents, err := service.ObservabilityDashboardStore.ListObservabilityRecentEvents(ctx, eventSince, 80)
	if err != nil {
		return nil, err
	}
	slowSpans, err := service.ObservabilityDashboardStore.ListObservabilitySlowSpans(ctx, metricSince, 12)
	if err != nil {
		return nil, err
	}
	stageLatency, err := service.ObservabilityDashboardStore.ListObservabilityStageLatency(ctx, metricSince, 12)
	if err != nil {
		return nil, err
	}
	stage6, err := service.observabilityStage6(ctx)
	if err != nil {
		return nil, err
	}

	latestMetrics := observabilityLatestMetricMap(latestMetricRows)
	eventsPayload := observabilityEventPayloads(recentEvents)
	return Payload{
		"generated_at":    serializeObservabilityTime(now),
		"current_metrics": latestMetrics,
		"series":          observabilitySeriesPayload(seriesRows),
		"recent_events":   eventsPayload,
		"error_summary":   observabilityErrorSummary(eventsPayload),
		"slow_spans":      observabilitySpanPayloads(slowSpans),
		"stage_latency":   observabilityStageLatencyPayloads(stageLatency),
		"alerts":          observabilityAlerts(latestMetrics, eventsPayload),
		"stage6":          stage6,
	}, nil
}

func (service Service) observabilityStage6(ctx context.Context) (Payload, error) {
	if service.Stage6StatusProvider != nil {
		return service.Stage6StatusProvider.Stage6Status(ctx)
	}
	return Payload{
		"ok":                       true,
		"ingest_queue":             Payload{},
		"outbox_relay":             Payload{},
		"search_projection":        Payload{},
		"pipeline_tracing":         Payload{},
		"error_capture":            Payload{},
		"runtime_metric_snapshots": Payload{},
		"conversation_rows":        0,
		"api_ws_hub":               Payload{},
		"archive_media_storage":    Payload{},
	}, nil
}

// Stage6Status builds the legacy /healthz/stage6 runtime status payload.
func (service Service) Stage6Status(ctx context.Context) (Payload, error) {
	return service.observabilityStage6(ctx)
}

func parseObservabilityBoundedInt(raw string, fallback int, minimum int, maximum int) (int, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(text)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func observabilityLatestMetricMap(rows []ObservabilityMetricRecord) Payload {
	latest := Payload{}
	for _, row := range rows {
		name := strings.TrimSpace(row.MetricName)
		if name == "" {
			continue
		}
		latest[name] = observabilityMetricPayload(row)
	}
	return latest
}

func observabilityMetricPayload(row ObservabilityMetricRecord) Payload {
	return Payload{
		"metric_group":    strings.TrimSpace(row.MetricGroup),
		"metric_name":     strings.TrimSpace(row.MetricName),
		"metric_scope":    observabilityTextDefault(row.MetricScope, "global"),
		"source_type":     observabilityTextDefault(row.SourceType, "backend"),
		"unit":            observabilityTextDefault(row.Unit, "count"),
		"value":           row.Value,
		"numerator":       row.Numerator,
		"denominator":     row.Denominator,
		"threshold_value": row.ThresholdValue,
		"status":          observabilityTextDefault(row.Status, "normal"),
		"tenant_id":       strings.TrimSpace(row.TenantID),
		"device_id":       strings.TrimSpace(row.DeviceID),
		"dimensions":      cloneObservabilityMap(row.Dimensions),
		"observed_at":     serializeObservabilityTime(row.ObservedAt),
	}
}

func observabilitySeriesPayload(rows []ObservabilityMetricRecord) Payload {
	seriesNames := map[string]struct{}{}
	for _, name := range observabilitySeriesMetrics {
		seriesNames[name] = struct{}{}
	}
	series := map[string][]Payload{}
	for _, row := range rows {
		name := strings.TrimSpace(row.MetricName)
		if _, ok := seriesNames[name]; !ok {
			continue
		}
		series[name] = append(series[name], Payload{
			"observed_at": serializeObservabilityTime(row.ObservedAt),
			"value":       row.Value,
			"status":      observabilityTextDefault(row.Status, "normal"),
		})
	}
	payload := Payload{}
	for _, name := range observabilitySeriesMetrics {
		if len(series[name]) > 0 {
			payload[name] = series[name]
		}
	}
	return payload
}

func observabilityEventPayloads(records []ObservabilityEventRecord) []Payload {
	payloads := make([]Payload, 0, len(records))
	for _, row := range records {
		payloads = append(payloads, Payload{
			"event_id":        strings.TrimSpace(row.EventID),
			"trace_id":        strings.TrimSpace(row.TraceID),
			"level":           observabilityTextDefault(row.Level, "ERROR"),
			"source_type":     observabilityTextDefault(row.SourceType, "backend"),
			"event_category":  observabilityTextDefault(row.EventCategory, "exception"),
			"event_code":      strings.TrimSpace(row.EventCode),
			"module":          strings.TrimSpace(row.Module),
			"action":          strings.TrimSpace(row.Action),
			"device_id":       strings.TrimSpace(row.DeviceID),
			"tenant_id":       strings.TrimSpace(row.TenantID),
			"conversation_id": strings.TrimSpace(row.ConversationID),
			"task_id":         strings.TrimSpace(row.TaskID),
			"wework_user_id":  strings.TrimSpace(row.WeworkUserID),
			"scope_type":      strings.TrimSpace(row.ScopeType),
			"scope_id":        strings.TrimSpace(row.ScopeID),
			"error_type":      strings.TrimSpace(row.ErrorType),
			"error_message":   strings.TrimSpace(row.ErrorMessage),
			"occurred_at":     serializeObservabilityTime(row.OccurredAt),
		})
	}
	return payloads
}

func observabilitySpanPayloads(records []ObservabilitySpanRecord) []Payload {
	payloads := make([]Payload, 0, len(records))
	for _, row := range records {
		var errorMessage any
		if strings.TrimSpace(row.ErrorMessage) != "" {
			errorMessage = strings.TrimSpace(row.ErrorMessage)
		}
		payloads = append(payloads, Payload{
			"span_id":         strings.TrimSpace(row.SpanID),
			"trace_id":        strings.TrimSpace(row.TraceID),
			"pipeline_type":   strings.TrimSpace(row.PipelineType),
			"stage_name":      strings.TrimSpace(row.StageName),
			"device_id":       strings.TrimSpace(row.DeviceID),
			"tenant_id":       strings.TrimSpace(row.TenantID),
			"conversation_id": strings.TrimSpace(row.ConversationID),
			"task_id":         strings.TrimSpace(row.TaskID),
			"wework_user_id":  strings.TrimSpace(row.WeworkUserID),
			"status":          observabilityTextDefault(row.Status, "ok"),
			"error_message":   errorMessage,
			"duration_ms":     roundObservability(row.DurationMS, 2),
			"started_at":      serializeObservabilityTime(row.StartedAt),
			"finished_at":     serializeObservabilityTime(row.FinishedAt),
		})
	}
	return payloads
}

func observabilityStageLatencyPayloads(records []ObservabilityStageLatencyRecord) []Payload {
	payloads := make([]Payload, 0, len(records))
	for _, row := range records {
		payloads = append(payloads, Payload{
			"pipeline_type":   strings.TrimSpace(row.PipelineType),
			"stage_name":      strings.TrimSpace(row.StageName),
			"avg_duration_ms": roundObservability(row.AvgDurationMS, 2),
			"max_duration_ms": roundObservability(row.MaxDurationMS, 2),
			"sample_count":    row.SampleCount,
		})
	}
	return payloads
}

func observabilityErrorSummary(events []Payload) Payload {
	return Payload{
		"total":           len(events),
		"source_counts":   observabilityCountItems(events, "source_type", "backend", 0),
		"level_counts":    observabilityCountItems(events, "level", "ERROR", 0),
		"category_counts": observabilityCountItems(events, "event_category", "exception", 8),
	}
}

func observabilityAlerts(metrics Payload, events []Payload) []Payload {
	alerts := make([]Payload, 0)
	for metricName, raw := range metrics {
		item, ok := raw.(Payload)
		if !ok {
			continue
		}
		status := strings.TrimSpace(textFromAny(item["status"]))
		if status != "warn" && status != "critical" {
			continue
		}
		alerts = append(alerts, Payload{
			"source":      "metric",
			"status":      status,
			"name":        metricName,
			"detail":      metricName,
			"observed_at": item["observed_at"],
			"value":       item["value"],
			"unit":        item["unit"],
		})
	}
	for index, item := range events {
		if index >= 12 {
			break
		}
		level := strings.ToUpper(strings.TrimSpace(textFromAny(item["level"])))
		if level != "WARN" && level != "WARNING" && level != "ERROR" && level != "CRITICAL" && level != "FATAL" {
			continue
		}
		status := "warn"
		if level == "ERROR" || level == "CRITICAL" || level == "FATAL" {
			status = "critical"
		}
		alerts = append(alerts, Payload{
			"source":      "event",
			"status":      status,
			"name":        firstNonEmptyText(item["event_category"], item["module"], "event"),
			"detail":      firstNonEmptyText(item["error_message"], item["action"], ""),
			"observed_at": item["occurred_at"],
			"value":       nil,
			"unit":        nil,
		})
	}
	sort.SliceStable(alerts, func(i int, j int) bool {
		leftStatus, rightStatus := textFromAny(alerts[i]["status"]), textFromAny(alerts[j]["status"])
		if leftStatus != rightStatus {
			return leftStatus == "critical"
		}
		leftObserved, rightObserved := textFromAny(alerts[i]["observed_at"]), textFromAny(alerts[j]["observed_at"])
		if leftObserved != rightObserved {
			return leftObserved < rightObserved
		}
		return textFromAny(alerts[i]["name"]) < textFromAny(alerts[j]["name"])
	})
	if len(alerts) > 20 {
		return alerts[:20]
	}
	return alerts
}

type observabilityCount struct {
	name  string
	value int
	first int
}

func observabilityCountItems(events []Payload, key string, fallback string, limit int) []Payload {
	counts := map[string]*observabilityCount{}
	for index, event := range events {
		name := strings.TrimSpace(textFromAny(event[key]))
		if name == "" {
			name = fallback
		}
		count, ok := counts[name]
		if !ok {
			count = &observabilityCount{name: name, first: index}
			counts[name] = count
		}
		count.value++
	}
	items := make([]observabilityCount, 0, len(counts))
	for _, count := range counts {
		items = append(items, *count)
	}
	sort.SliceStable(items, func(i int, j int) bool {
		if items[i].value != items[j].value {
			return items[i].value > items[j].value
		}
		return items[i].first < items[j].first
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	payloads := make([]Payload, 0, len(items))
	for _, item := range items {
		payloads = append(payloads, Payload{"name": item.name, "value": item.value})
	}
	return payloads
}

func observabilityTextDefault(value string, fallback string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return fallback
	}
	return text
}

func cloneObservabilityMap(input map[string]any) Payload {
	if input == nil {
		return Payload{}
	}
	output := Payload{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func serializeObservabilityTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func roundObservability(value float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(value)
	}
	scale := math.Pow10(digits)
	return math.Round(value*scale) / scale
}

func textFromAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstNonEmptyText(values ...any) string {
	for _, value := range values {
		text := textFromAny(value)
		if text != "" {
			return text
		}
	}
	return ""
}
