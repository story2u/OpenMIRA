import assert from "node:assert/strict";
import test from "node:test";

import {
  OBSERVABILITY_DASHBOARD_PATH,
  ROOT_ROUTE_BASE_PATH,
  STAGE6_HEALTH_PATH,
  buildObservabilityDashboardRequest,
  buildStage6HealthRequest,
  defaultObservabilityFilters,
  formatObservabilityValue,
  normalizeObservabilityDashboard,
  normalizeStage6Status,
  observabilityStatusRank,
} from "./adminObservability.js";

test("observability dashboard request keeps legacy path and query bounds", () => {
  const defaults = defaultObservabilityFilters();
  assert.deepEqual(defaults, { hours: "6", eventHours: "24" });

  const request = buildObservabilityDashboardRequest({ hours: "99", eventHours: "0" });
  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, OBSERVABILITY_DASHBOARD_PATH);
  assert.deepEqual(request.params, {
    hours: 48,
    event_hours: 1,
  });
});

test("stage6 health request uses root legacy route", () => {
  const request = buildStage6HealthRequest();
  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, STAGE6_HEALTH_PATH);
  assert.equal(request.basePath, ROOT_ROUTE_BASE_PATH);
});

test("normalizeObservabilityDashboard keeps metrics, events, alerts and stage6", () => {
  const dashboard = normalizeObservabilityDashboard({
    generated_at: "2026-07-02T10:00:00Z",
    current_metrics: {
      incoming_queue_depth: {
        metric_group: "ingest",
        metric_name: "incoming_queue_depth",
        value: 7,
        unit: "count",
        status: "warn",
        observed_at: "2026-07-02T09:59:00Z",
      },
      outbox_lag_seconds: {
        value: 1.25,
        unit: "seconds",
        status: "normal",
      },
    },
    series: {
      incoming_queue_depth: [{ observed_at: "2026-07-02T09:00:00Z", value: 5, status: "normal" }],
    },
    recent_events: [{
      event_id: "evt-1",
      level: "ERROR",
      event_category: "exception",
      module: "incoming",
      error_message: "timeout",
      occurred_at: "2026-07-02T09:58:00Z",
    }],
    error_summary: {
      total: 1,
      level_counts: [{ name: "ERROR", count: 1 }],
    },
    alerts: [{ source: "metric", status: "critical", name: "runtime", detail: "down", observed_at: "2026-07-02T09:57:00Z" }],
    slow_spans: [{ span_id: "span-1", pipeline_type: "incoming", stage_name: "store", duration_ms: 123.456, error_message: "timeout" }],
    stage_latency: [{ pipeline_type: "incoming", stage_name: "store", avg_duration_ms: 88.13, max_duration_ms: 120, sample_count: 2 }],
    stage6: { ok: true, api_ws_hub: { ok: true, connections: 2 } },
  });

  assert.equal(dashboard.generatedAt, "2026-07-02T10:00:00Z");
  assert.equal(dashboard.currentMetrics[0].name, "incoming_queue_depth");
  assert.equal(dashboard.currentMetrics[0].status, "warn");
  assert.equal(dashboard.series[0].points.length, 1);
  assert.equal(dashboard.recentEvents[0].errorMessage, "timeout");
  assert.equal(dashboard.errorSummary.total, 1);
  assert.equal(dashboard.errorSummary.levelCounts[0].name, "ERROR");
  assert.equal(dashboard.alerts[0].name, "runtime");
  assert.equal(dashboard.slowSpans[0].durationMS, 123.456);
  assert.equal(dashboard.stageLatency[0].sampleCount, 2);
  assert.equal(dashboard.stage6.ok, true);
  assert.equal(dashboard.stage6.components[0].connections, 2);
});

test("normalizeStage6Status derives component state from stable payload keys", () => {
  const status = normalizeStage6Status({
    ok: false,
    api_ws_hub: { status: "degraded", detail: "redis unavailable" },
    archive_media: { ok: true, connections: 1 },
  });

  assert.equal(status.ok, false);
  assert.equal(status.components.length, 2);
  assert.equal(status.components[0].status, "degraded");
  assert.equal(status.components[1].ok, true);
});

test("observability formatting and rank helpers are stable", () => {
  assert.equal(formatObservabilityValue(12.345, "ms"), "12.35 ms");
  assert.equal(formatObservabilityValue(null, "count"), "-");
  assert.equal(observabilityStatusRank("critical") < observabilityStatusRank("warn"), true);
  assert.equal(observabilityStatusRank("warn") < observabilityStatusRank("normal"), true);
});
