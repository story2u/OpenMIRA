package workbenchobservability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestRepositoryReadsDashboardTables keeps SQL bindings aligned to Python's dashboard reader.
func TestRepositoryReadsDashboardTables(t *testing.T) {
	db := &fakeObservabilityDB{rows: []*fakeObservabilityRows{
		{values: [][]any{{
			"queue", "incoming_queue_depth", "global", "backend", "count", []byte("7.5"), nil, "10", nil, "warn", "tenant-a", "device-a", []byte(`{"shard":"a"}`), "2026-06-29T09:00:00Z",
		}}},
		{values: [][]any{{
			"evt-1", "trace-1", "ERROR", "frontend", "runtime", "E_RUNTIME", "admin", "render", "device-a", "tenant-a", "conv-1", "task-1", "ww-1", "conversation", "conv-1", "TypeError", "render failed", []byte("2026-06-29T09:10:00Z"),
		}}},
		{values: [][]any{{
			"span-1", "trace-1", "dispatch", "send", "device-a", "tenant-a", "conv-1", "task-1", "ww-1", "error", "timeout", 123.4, "2026-06-29T09:00:00Z", "2026-06-29T09:00:01Z",
		}}},
		{values: [][]any{{
			"dispatch", "send", []byte("88.5"), 123.4, int64(2),
		}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	since := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	metrics, err := repository.ListObservabilityMetricRows(context.Background(), []string{" incoming_queue_depth ", "incoming_queue_depth"}, since, 20)
	if err != nil {
		t.Fatalf("ListObservabilityMetricRows returned error: %v", err)
	}
	if len(metrics) != 1 || metrics[0].MetricName != "incoming_queue_depth" || metrics[0].Value != 7.5 || metrics[0].Denominator == nil || *metrics[0].Denominator != 10 {
		t.Fatalf("metrics = %+v", metrics)
	}
	if metrics[0].Dimensions["shard"] != "a" || metrics[0].ObservedAt.Format(time.RFC3339) != "2026-06-29T09:00:00Z" {
		t.Fatalf("metric details = %+v", metrics[0])
	}

	events, err := repository.ListObservabilityRecentEvents(context.Background(), since, 80)
	if err != nil {
		t.Fatalf("ListObservabilityRecentEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" || events[0].ErrorMessage != "render failed" {
		t.Fatalf("events = %+v", events)
	}

	spans, err := repository.ListObservabilitySlowSpans(context.Background(), since, 12)
	if err != nil {
		t.Fatalf("ListObservabilitySlowSpans returned error: %v", err)
	}
	if len(spans) != 1 || spans[0].SpanID != "span-1" || spans[0].DurationMS != 123.4 {
		t.Fatalf("spans = %+v", spans)
	}

	latency, err := repository.ListObservabilityStageLatency(context.Background(), since, 12)
	if err != nil {
		t.Fatalf("ListObservabilityStageLatency returned error: %v", err)
	}
	if len(latency) != 1 || latency[0].AvgDurationMS != 88.5 || latency[0].SampleCount != 2 {
		t.Fatalf("latency = %+v", latency)
	}

	if len(db.queries) != 4 {
		t.Fatalf("queries = %d, want 4", len(db.queries))
	}
	for _, want := range []string{"FROM runtime_metric_snapshots", "FROM error_events", "FROM pipeline_spans", "GROUP BY pipeline_type, stage_name"} {
		if !containsQuery(db.queries, want) {
			t.Fatalf("missing query fragment %q in %#v", want, db.queries)
		}
	}
	if db.args[0][0] != "incoming_queue_depth" || db.args[0][1] != "2026-06-29 17:00:00" || db.args[0][2] != 20 {
		t.Fatalf("metric args = %#v", db.args[0])
	}
}

// TestRepositoryPostgresDateParamsUseBeijingISO mirrors existing DB parameter policy.
func TestRepositoryPostgresDateParamsUseBeijingISO(t *testing.T) {
	db := &fakeObservabilityDB{rows: []*fakeObservabilityRows{{}}}
	repository := &Repository{DB: db, Dialect: "postgres"}
	_, err := repository.ListObservabilityRecentEvents(context.Background(), time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("ListObservabilityRecentEvents returned error: %v", err)
	}
	if db.args[0][0] != "2026-06-29T17:00:00+08:00" {
		t.Fatalf("postgres time arg = %#v", db.args[0][0])
	}
}

func containsQuery(queries []string, fragment string) bool {
	for _, query := range queries {
		if strings.Contains(query, fragment) {
			return true
		}
	}
	return false
}

type fakeObservabilityDB struct {
	rows    []*fakeObservabilityRows
	queries []string
	args    [][]any
}

func (db *fakeObservabilityDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	if len(db.rows) == 0 {
		return &fakeObservabilityRows{}, nil
	}
	next := db.rows[0]
	db.rows = db.rows[1:]
	return next, nil
}

type fakeObservabilityRows struct {
	values [][]any
	index  int
	closed bool
	err    error
}

func (rows *fakeObservabilityRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeObservabilityRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return errors.New("scan after EOF")
	}
	current := rows.values[rows.index]
	rows.index++
	if len(dest) != len(current) {
		return errors.New("destination count mismatch")
	}
	for index, value := range current {
		target, ok := dest[index].(*any)
		if !ok {
			return errors.New("destination must be *any")
		}
		*target = value
	}
	return nil
}

func (rows *fakeObservabilityRows) Close() error {
	rows.closed = true
	return nil
}

func (rows *fakeObservabilityRows) Err() error {
	return rows.err
}
