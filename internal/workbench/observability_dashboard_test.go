package workbench

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"wework-go/internal/auth"
)

// TestNewObservabilityDashboardRequestValidatesBounds keeps FastAPI Query limits.
func TestNewObservabilityDashboardRequestValidatesBounds(t *testing.T) {
	request, err := NewObservabilityDashboardRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewObservabilityDashboardRequest returned error: %v", err)
	}
	if request.Hours != 6 || request.EventHours != 24 || request.Session.Role != "admin" {
		t.Fatalf("request = %+v", request)
	}
	if _, err := NewObservabilityDashboardRequest(url.Values{"hours": {"49"}}, auth.Session{}); !errors.Is(err, ErrInvalidObservabilityHours) {
		t.Fatalf("hours error = %v, want %v", err, ErrInvalidObservabilityHours)
	}
	if _, err := NewObservabilityDashboardRequest(url.Values{"event_hours": {"0"}}, auth.Session{}); !errors.Is(err, ErrInvalidObservabilityEventHours) {
		t.Fatalf("event_hours error = %v, want %v", err, ErrInvalidObservabilityEventHours)
	}
}

// TestServiceObservabilityDashboardBuildsPayload preserves the Python dashboard shape.
func TestServiceObservabilityDashboardBuildsPayload(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	numerator := 7.0
	store := &fakeObservabilityDashboardStore{
		metricRows: [][]ObservabilityMetricRecord{
			{
				{MetricGroup: "queue", MetricName: "incoming_queue_depth", Unit: "count", Value: 3, Status: "normal", ObservedAt: now.Add(-2 * time.Hour)},
				{MetricGroup: "queue", MetricName: "incoming_queue_depth", Unit: "count", Value: 7, Numerator: &numerator, Status: "warn", ObservedAt: now.Add(-time.Hour)},
			},
			{
				{MetricName: "incoming_queue_depth", Value: 4, Status: "normal", ObservedAt: now.Add(-30 * time.Minute)},
			},
		},
		events: []ObservabilityEventRecord{
			{EventID: "evt-1", Level: "ERROR", SourceType: "frontend", EventCategory: "runtime", Module: "admin", ErrorMessage: "render failed", OccurredAt: now.Add(-20 * time.Minute)},
			{EventID: "evt-2", Level: "WARNING", SourceType: "backend", EventCategory: "queue", Action: "retry", OccurredAt: now.Add(-10 * time.Minute)},
		},
		spans: []ObservabilitySpanRecord{
			{SpanID: "span-1", TraceID: "trace-1", PipelineType: "dispatch", StageName: "send", Status: "error", ErrorMessage: "timeout", DurationMS: 123.456, StartedAt: now.Add(-time.Minute), FinishedAt: now},
		},
		latency: []ObservabilityStageLatencyRecord{
			{PipelineType: "dispatch", StageName: "send", AvgDurationMS: 88.129, MaxDurationMS: 123.456, SampleCount: 2},
		},
	}
	service := Service{
		ObservabilityDashboardStore: store,
		Stage6StatusProvider:        fakeStage6Provider{payload: Payload{"ok": true, "api_ws_hub": Payload{"connections": 2}}},
		Now: func() time.Time {
			return now
		},
	}

	payload, err := service.ObservabilityDashboard(context.Background(), ObservabilityDashboardRequest{Hours: 6, EventHours: 24})
	if err != nil {
		t.Fatalf("ObservabilityDashboard returned error: %v", err)
	}
	if payload["generated_at"] != "2026-06-29T10:00:00Z" {
		t.Fatalf("generated_at = %#v", payload["generated_at"])
	}
	current := payload["current_metrics"].(Payload)
	depth := current["incoming_queue_depth"].(Payload)
	if depth["value"] != 7.0 || depth["status"] != "warn" || depth["numerator"] != &numerator {
		t.Fatalf("incoming_queue_depth = %+v", depth)
	}
	series := payload["series"].(Payload)
	if len(series["incoming_queue_depth"].([]Payload)) != 1 {
		t.Fatalf("series = %+v", series)
	}
	summary := payload["error_summary"].(Payload)
	if summary["total"] != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	alerts := payload["alerts"].([]Payload)
	if len(alerts) != 3 || alerts[0]["status"] != "critical" || alerts[0]["name"] != "runtime" {
		t.Fatalf("alerts = %+v", alerts)
	}
	slowSpans := payload["slow_spans"].([]Payload)
	if slowSpans[0]["duration_ms"] != 123.46 || slowSpans[0]["error_message"] != "timeout" {
		t.Fatalf("slow_spans = %+v", slowSpans)
	}
	stageLatency := payload["stage_latency"].([]Payload)
	if stageLatency[0]["avg_duration_ms"] != 88.13 || stageLatency[0]["sample_count"] != 2 {
		t.Fatalf("stage_latency = %+v", stageLatency)
	}
	stage6 := payload["stage6"].(Payload)
	if stage6["ok"] != true {
		t.Fatalf("stage6 = %+v", stage6)
	}
	if len(store.metricCalls) != 2 || store.metricCalls[0].limit != maxInt(400, len(observabilityCurrentMetrics)*24) || store.metricCalls[1].limit != maxInt(600, len(observabilitySeriesMetrics)*12*12) {
		t.Fatalf("metric calls = %+v", store.metricCalls)
	}
	if !store.eventSince.Equal(now.Add(-24*time.Hour)) || store.eventLimit != 80 || store.spanLimit != 12 || store.latencyLimit != 12 {
		t.Fatalf("store windows: event=%s/%d span=%d latency=%d", store.eventSince, store.eventLimit, store.spanLimit, store.latencyLimit)
	}
}

// TestServiceObservabilityDashboardFailsClosedWithoutStore keeps missing wiring explicit.
func TestServiceObservabilityDashboardFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).ObservabilityDashboard(context.Background(), ObservabilityDashboardRequest{})
	if !errors.Is(err, ErrObservabilityDashboardStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrObservabilityDashboardStoreUnavailable)
	}
}

type fakeObservabilityMetricCall struct {
	names []string
	since time.Time
	limit int
}

type fakeObservabilityDashboardStore struct {
	metricRows   [][]ObservabilityMetricRecord
	events       []ObservabilityEventRecord
	spans        []ObservabilitySpanRecord
	latency      []ObservabilityStageLatencyRecord
	metricCalls  []fakeObservabilityMetricCall
	eventSince   time.Time
	eventLimit   int
	spanSince    time.Time
	spanLimit    int
	latencySince time.Time
	latencyLimit int
}

func (store *fakeObservabilityDashboardStore) ListObservabilityMetricRows(ctx context.Context, metricNames []string, since time.Time, limit int) ([]ObservabilityMetricRecord, error) {
	store.metricCalls = append(store.metricCalls, fakeObservabilityMetricCall{names: append([]string(nil), metricNames...), since: since, limit: limit})
	if len(store.metricRows) == 0 {
		return []ObservabilityMetricRecord{}, nil
	}
	next := store.metricRows[0]
	store.metricRows = store.metricRows[1:]
	return next, nil
}

func (store *fakeObservabilityDashboardStore) ListObservabilityRecentEvents(ctx context.Context, since time.Time, limit int) ([]ObservabilityEventRecord, error) {
	store.eventSince = since
	store.eventLimit = limit
	return store.events, nil
}

func (store *fakeObservabilityDashboardStore) ListObservabilitySlowSpans(ctx context.Context, since time.Time, limit int) ([]ObservabilitySpanRecord, error) {
	store.spanSince = since
	store.spanLimit = limit
	return store.spans, nil
}

func (store *fakeObservabilityDashboardStore) ListObservabilityStageLatency(ctx context.Context, since time.Time, limit int) ([]ObservabilityStageLatencyRecord, error) {
	store.latencySince = since
	store.latencyLimit = limit
	return store.latency, nil
}

type fakeStage6Provider struct {
	payload Payload
}

func (provider fakeStage6Provider) Stage6Status(ctx context.Context) (Payload, error) {
	return provider.payload, nil
}
