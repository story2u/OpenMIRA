export const OBSERVABILITY_DASHBOARD_PATH = "/admin/observability/dashboard";
export const STAGE6_HEALTH_PATH = "/healthz/stage6";
export const ROOT_ROUTE_BASE_PATH = "";

export const OBSERVABILITY_HOURS_OPTIONS = [1, 3, 6, 12, 24, 48];
export const OBSERVABILITY_EVENT_HOURS_OPTIONS = [1, 6, 12, 24, 72, 168];

function cleanText(value) {
  return String(value ?? "").trim();
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function intValue(value, fallback = 0) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.trunc(parsed);
}

function boundedInt(value, fallback, minimum, maximum) {
  const parsed = intValue(value, fallback);
  if (parsed < minimum) return minimum;
  if (parsed > maximum) return maximum;
  return parsed;
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "ok", "healthy", "是"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "failed", "error", "否"].includes(normalized)) return false;
  return fallback;
}

function numberValue(value) {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function unwrapPayload(payload = {}) {
  return payload?.data && typeof payload.data === "object" ? payload.data : payload && typeof payload === "object" ? payload : {};
}

function listFromPayload(value) {
  return Array.isArray(value) ? value : [];
}

function objectFromPayload(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

export function defaultObservabilityFilters() {
  return {
    hours: "6",
    eventHours: "24",
  };
}

export function buildObservabilityDashboardRequest(filters = {}) {
  return {
    ok: true,
    method: "GET",
    path: OBSERVABILITY_DASHBOARD_PATH,
    params: {
      hours: boundedInt(filters.hours, 6, 1, 48),
      event_hours: boundedInt(firstDefined(filters.eventHours, filters.event_hours), 24, 1, 168),
    },
  };
}

export function buildStage6HealthRequest() {
  return {
    ok: true,
    method: "GET",
    path: STAGE6_HEALTH_PATH,
    basePath: ROOT_ROUTE_BASE_PATH,
  };
}

export function normalizeObservabilityDashboard(payload = {}) {
  const data = unwrapPayload(payload);
  const currentMetrics = normalizeMetricMap(data?.current_metrics || data?.currentMetrics);
  const recentEvents = listFromPayload(firstDefined(data?.recent_events, data?.recentEvents)).map(normalizeEvent);
  return {
    generatedAt: cleanText(firstDefined(data?.generated_at, data?.generatedAt)),
    currentMetrics,
    series: normalizeSeriesMap(data?.series),
    recentEvents,
    errorSummary: normalizeErrorSummary(data?.error_summary || data?.errorSummary, recentEvents),
    alerts: listFromPayload(data?.alerts).map(normalizeAlert).sort(sortByStatusAndTime),
    slowSpans: listFromPayload(firstDefined(data?.slow_spans, data?.slowSpans)).map(normalizeSpan),
    stageLatency: listFromPayload(firstDefined(data?.stage_latency, data?.stageLatency)).map(normalizeStageLatency),
    stage6: normalizeStage6Status(data?.stage6),
    raw: data,
  };
}

export function normalizeStage6Status(payload = {}) {
  const data = unwrapPayload(payload);
  const components = Object.entries(data)
    .filter(([key, value]) => !["ok", "success", "status", "generated_at", "generatedAt"].includes(key) && value && typeof value === "object" && !Array.isArray(value))
    .map(([key, value]) => normalizeStage6Component(key, value));
  const ok = parseBool(firstDefined(data?.ok, data?.success), components.length > 0 ? components.every((item) => item.ok) : false);
  return {
    ok,
    status: cleanText(data?.status) || (ok ? "ok" : "unknown"),
    generatedAt: cleanText(firstDefined(data?.generated_at, data?.generatedAt)),
    components,
    raw: data,
  };
}

export function observabilityStatusRank(status = "") {
  const normalized = cleanText(status).toLowerCase();
  if (["critical", "fatal", "error", "failed"].includes(normalized)) return 0;
  if (["warn", "warning", "degraded"].includes(normalized)) return 1;
  if (["normal", "ok", "healthy", "success"].includes(normalized)) return 2;
  return 3;
}

export function formatObservabilityValue(value, unit = "") {
  const numeric = numberValue(value);
  const suffix = cleanText(unit);
  if (numeric === null) return "-";
  const rounded = Math.abs(numeric) >= 100 ? Math.round(numeric) : Math.round(numeric * 100) / 100;
  return suffix ? `${rounded} ${suffix}` : String(rounded);
}

function normalizeMetricMap(value = {}) {
  return Object.entries(objectFromPayload(value))
    .map(([name, item]) => normalizeMetric(name, item))
    .filter((item) => item.name)
    .sort(sortByStatusAndName);
}

function normalizeMetric(name, value = {}) {
  const item = objectFromPayload(value);
  return {
    name: cleanText(firstDefined(item?.metric_name, item?.metricName, name)),
    group: cleanText(firstDefined(item?.metric_group, item?.metricGroup)),
    scope: cleanText(firstDefined(item?.metric_scope, item?.metricScope)) || "global",
    sourceType: cleanText(firstDefined(item?.source_type, item?.sourceType)) || "backend",
    unit: cleanText(item?.unit || "count"),
    value: numberValue(item?.value),
    numerator: numberValue(item?.numerator),
    denominator: numberValue(item?.denominator),
    thresholdValue: numberValue(firstDefined(item?.threshold_value, item?.thresholdValue)),
    status: cleanText(item?.status || "normal"),
    tenantID: cleanText(firstDefined(item?.tenant_id, item?.tenantID)),
    deviceID: cleanText(firstDefined(item?.device_id, item?.deviceID)),
    observedAt: cleanText(firstDefined(item?.observed_at, item?.observedAt)),
    raw: item,
  };
}

function normalizeSeriesMap(value = {}) {
  return Object.entries(objectFromPayload(value))
    .map(([name, points]) => ({
      name,
      points: listFromPayload(points).map((point) => ({
        observedAt: cleanText(firstDefined(point?.observed_at, point?.observedAt)),
        value: numberValue(point?.value),
        status: cleanText(point?.status || "normal"),
        raw: point,
      })),
    }))
    .filter((series) => series.points.length > 0);
}

function normalizeEvent(value = {}) {
  return {
    eventID: cleanText(firstDefined(value?.event_id, value?.eventID)),
    traceID: cleanText(firstDefined(value?.trace_id, value?.traceID)),
    level: cleanText(value?.level || "ERROR"),
    sourceType: cleanText(firstDefined(value?.source_type, value?.sourceType)) || "backend",
    category: cleanText(firstDefined(value?.event_category, value?.eventCategory)) || "exception",
    code: cleanText(firstDefined(value?.event_code, value?.eventCode)),
    module: cleanText(value?.module),
    action: cleanText(value?.action),
    deviceID: cleanText(firstDefined(value?.device_id, value?.deviceID)),
    taskID: cleanText(firstDefined(value?.task_id, value?.taskID)),
    errorType: cleanText(firstDefined(value?.error_type, value?.errorType)),
    errorMessage: cleanText(firstDefined(value?.error_message, value?.errorMessage)),
    occurredAt: cleanText(firstDefined(value?.occurred_at, value?.occurredAt)),
    raw: value,
  };
}

function normalizeAlert(value = {}) {
  return {
    source: cleanText(value?.source),
    status: cleanText(value?.status || "warn"),
    name: cleanText(value?.name),
    detail: cleanText(value?.detail),
    observedAt: cleanText(firstDefined(value?.observed_at, value?.observedAt)),
    value: numberValue(value?.value),
    unit: cleanText(value?.unit),
    raw: value,
  };
}

function normalizeSpan(value = {}) {
  return {
    spanID: cleanText(firstDefined(value?.span_id, value?.spanID)),
    traceID: cleanText(firstDefined(value?.trace_id, value?.traceID)),
    pipelineType: cleanText(firstDefined(value?.pipeline_type, value?.pipelineType)),
    stageName: cleanText(firstDefined(value?.stage_name, value?.stageName)),
    status: cleanText(value?.status || "ok"),
    errorMessage: cleanText(firstDefined(value?.error_message, value?.errorMessage)),
    durationMS: numberValue(firstDefined(value?.duration_ms, value?.durationMS)),
    startedAt: cleanText(firstDefined(value?.started_at, value?.startedAt)),
    finishedAt: cleanText(firstDefined(value?.finished_at, value?.finishedAt)),
    raw: value,
  };
}

function normalizeStageLatency(value = {}) {
  return {
    pipelineType: cleanText(firstDefined(value?.pipeline_type, value?.pipelineType)),
    stageName: cleanText(firstDefined(value?.stage_name, value?.stageName)),
    avgDurationMS: numberValue(firstDefined(value?.avg_duration_ms, value?.avgDurationMS)),
    maxDurationMS: numberValue(firstDefined(value?.max_duration_ms, value?.maxDurationMS)),
    sampleCount: intValue(firstDefined(value?.sample_count, value?.sampleCount), 0),
    raw: value,
  };
}

function normalizeErrorSummary(value = {}, events = []) {
  const data = objectFromPayload(value);
  return {
    total: intValue(data?.total, events.length),
    sourceCounts: normalizeCountItems(firstDefined(data?.source_counts, data?.sourceCounts)),
    levelCounts: normalizeCountItems(firstDefined(data?.level_counts, data?.levelCounts)),
    categoryCounts: normalizeCountItems(firstDefined(data?.category_counts, data?.categoryCounts)),
    raw: data,
  };
}

function normalizeCountItems(value = []) {
  return listFromPayload(value)
    .map((item) => ({
      name: cleanText(item?.name),
      count: intValue(item?.count, 0),
    }))
    .filter((item) => item.name);
}

function normalizeStage6Component(name, value = {}) {
  const item = objectFromPayload(value);
  const ok = parseBool(firstDefined(item?.ok, item?.success, item?.healthy), false);
  return {
    name: cleanText(name),
    ok,
    status: cleanText(item?.status) || (ok ? "ok" : "unknown"),
    detail: cleanText(firstDefined(item?.detail, item?.message, item?.error)),
    connections: intValue(item?.connections, 0),
    raw: item,
  };
}

function sortByStatusAndName(left, right) {
  const statusDelta = observabilityStatusRank(left.status) - observabilityStatusRank(right.status);
  if (statusDelta !== 0) return statusDelta;
  return left.name.localeCompare(right.name);
}

function sortByStatusAndTime(left, right) {
  const statusDelta = observabilityStatusRank(left.status) - observabilityStatusRank(right.status);
  if (statusDelta !== 0) return statusDelta;
  return cleanText(right.observedAt).localeCompare(cleanText(left.observedAt));
}
