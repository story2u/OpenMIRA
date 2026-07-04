export const AI_REPLY_LOGS_PATH = "/admin/ai-config/reply-logs";
export const AI_REPLY_OVERVIEW_PATH = "/admin/stats/ai-replies/overview";
export const AI_REPLY_TREND_PATH = "/admin/stats/ai-replies/trend";
export const AI_REPLY_BREAKDOWN_PATH = "/admin/stats/ai-replies/breakdown";

export const AI_REPLY_STATUS_OPTIONS = [
  { value: "all", label: "全部状态" },
  { value: "processing", label: "处理中" },
  { value: "sending", label: "发送中" },
  { value: "sent", label: "已发送" },
  { value: "success", label: "成功" },
  { value: "failed", label: "失败" },
  { value: "send_failed", label: "发送失败" },
  { value: "unreplyable", label: "不可回复" },
  { value: "superseded", label: "已跳过" },
];

export const AI_REPLY_PAGE_SIZE_OPTIONS = [20, 50, 100];

const STATUS_LABELS = Object.fromEntries(AI_REPLY_STATUS_OPTIONS.map((item) => [item.value, item.label]));

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

function positiveInt(value, fallback = 1) {
  const parsed = intValue(value, fallback);
  return parsed > 0 ? parsed : fallback;
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
  if (["true", "1", "yes", "on", "是"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否"].includes(normalized)) return false;
  return fallback;
}

function nullableNumber(value) {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function compactParams(values = {}, keepKeys = []) {
  const params = {};
  const keep = new Set(keepKeys);
  Object.entries(values).forEach(([key, value]) => {
    const normalized = cleanText(value);
    if (!normalized) return;
    if (!keep.has(key) && normalized === "all") return;
    params[key] = normalized;
  });
  return params;
}

function listFromPayload(payload = {}, key = "items") {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.[key])) return payload[key];
  if (Array.isArray(payload?.data?.[key])) return payload.data[key];
  if (key === "data" && Array.isArray(payload?.data)) return payload.data;
  return [];
}

export function defaultAIReplyLogFilters() {
  return {
    scope: "local",
    status: "all",
    keyword: "",
    date: "",
    page: "1",
    pageSize: "50",
  };
}

export function defaultAIReplyStatsFilters() {
  return {
    date: "",
    days: "7",
  };
}

export function buildAIReplyLogsRequest(filters = {}) {
  const page = positiveInt(firstDefined(filters.page, 1), 1);
  const pageSize = boundedInt(firstDefined(filters.pageSize, filters.page_size, 50), 50, 1, 100);
  const params = compactParams({
    scope: firstDefined(filters.scope, "local"),
    keyword: filters.keyword,
    status: firstDefined(filters.status, "all"),
    date: filters.date,
    page,
    page_size: pageSize,
  }, ["scope", "page", "page_size"]);
  return {
    ok: true,
    method: "GET",
    path: AI_REPLY_LOGS_PATH,
    params,
  };
}

export function buildAIReplyOverviewRequest(filters = {}) {
  return {
    ok: true,
    method: "GET",
    path: AI_REPLY_OVERVIEW_PATH,
    params: compactParams({ date: filters.date }),
  };
}

export function buildAIReplyTrendRequest(filters = {}) {
  const days = boundedInt(firstDefined(filters.days, 7), 7, 1, 90);
  return {
    ok: true,
    method: "GET",
    path: AI_REPLY_TREND_PATH,
    params: { days: String(days) },
  };
}

export function buildAIReplyBreakdownRequest(filters = {}) {
  return {
    ok: true,
    method: "GET",
    path: AI_REPLY_BREAKDOWN_PATH,
    params: compactParams({ date: filters.date }),
  };
}

export function normalizeAIReplyLogs(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data) ? payload.data : payload;
  return {
    logs: listFromPayload(source, "logs").map(normalizeAIReplyLog).filter(Boolean),
    pagination: normalizePagination(source?.pagination || payload?.pagination),
    scope: cleanText(firstDefined(source?.scope, payload?.scope, "local")) || "local",
    filters: normalizeAIReplyLogFilters(source?.filters || payload?.filters),
  };
}

export function normalizeAIReplyLog(record = {}) {
  const attemptId = cleanText(firstDefined(record?.attempt_id, record?.attemptId));
  const traceId = cleanText(firstDefined(record?.trace_id, record?.traceId));
  const taskId = cleanText(firstDefined(record?.task_id, record?.taskId));
  const status = normalizeAIReplyStatus(record?.status);
  const identity = attemptId || traceId || taskId || cleanText(record?.conversation_id);
  if (!identity && !cleanText(record?.content) && !cleanText(record?.customer_message)) return null;
  return {
    attemptId,
    traceId,
    incomingTraceId: cleanText(firstDefined(record?.incoming_trace_id, record?.incomingTraceId)),
    taskId,
    workflowId: cleanText(firstDefined(record?.workflow_id, record?.workflowId)),
    model: cleanText(record?.model),
    triggerEvent: cleanText(firstDefined(record?.trigger_event, record?.triggerEvent)),
    replyTime: cleanText(firstDefined(record?.reply_time, record?.replyTime)),
    startedAt: cleanText(firstDefined(record?.started_at, record?.startedAt)),
    finishedAt: cleanText(firstDefined(record?.finished_at, record?.finishedAt)),
    updatedAt: cleanText(firstDefined(record?.updated_at, record?.updatedAt)),
    assigneeId: cleanText(firstDefined(record?.assignee_id, record?.assigneeId)),
    assigneeName: cleanText(firstDefined(record?.assignee_name, record?.assigneeName)),
    accountId: cleanText(firstDefined(record?.account_id, record?.accountId)),
    accountName: cleanText(firstDefined(record?.account_name, record?.accountName)),
    receiverName: cleanText(firstDefined(record?.receiver_name, record?.receiverName)),
    conversationId: cleanText(firstDefined(record?.conversation_id, record?.conversationId)),
    customerMessage: cleanText(firstDefined(record?.customer_message, record?.customerMessage)),
    content: cleanText(record?.content),
    status,
    statusLabel: aiReplyStatusLabel(status),
    failureType: cleanText(firstDefined(record?.failure_type, record?.failureType)),
    providerError: cleanText(firstDefined(record?.provider_error, record?.providerError)),
    userFacingError: cleanText(firstDefined(record?.user_facing_error, record?.userFacingError)),
    messageMissing: parseBool(firstDefined(record?.message_missing, record?.messageMissing), false),
    customerMessageMissing: parseBool(firstDefined(record?.customer_message_missing, record?.customerMessageMissing), false),
    raw: record,
  };
}

export function normalizeAIReplyOverview(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data) ? payload.data : payload;
  return normalizeAIReplyStatsRow(source);
}

export function normalizeAIReplyTrend(payload = {}) {
  return listFromPayload(payload, "data").map(normalizeAIReplyStatsRow).filter(Boolean);
}

export function normalizeAIReplyBreakdown(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data) ? payload.data : payload;
  return {
    date: source?.date === null ? "" : cleanText(source?.date),
    items: listFromPayload(source, "items")
      .map((item = {}) => ({
        failureType: cleanText(firstDefined(item?.failure_type, item?.failureType)) || "unknown",
        count: intValue(item?.count, 0),
        raw: item,
      }))
      .filter((item) => item.failureType || item.count > 0),
  };
}

export function formatAIDurationMS(value) {
  const number = nullableNumber(value);
  if (number === null) return "-";
  return `${Math.round(number)}ms`;
}

export function formatAIRate(value) {
  const number = nullableNumber(value);
  if (number === null) return "-";
  return `${(number * 100).toFixed(1)}%`;
}

export function aiReplyStatusLabel(status = "") {
  const normalized = normalizeAIReplyStatus(status);
  return STATUS_LABELS[normalized] || normalized || "-";
}

function normalizeAIReplyLogFilters(filters = {}) {
  return {
    keyword: cleanText(filters?.keyword),
    status: normalizeAIReplyStatus(firstDefined(filters?.status, "all")),
    date: cleanText(filters?.date),
  };
}

function normalizeAIReplyStatus(status = "") {
  const normalized = cleanText(status).toLowerCase();
  return normalized || "all";
}

function normalizeAIReplyStatsRow(record = {}) {
  if (!record || typeof record !== "object") return null;
  const attempts = intValue(record?.attempts, 0);
  const successCount = intValue(firstDefined(record?.success_count, record?.successCount), 0);
  const sentCount = intValue(firstDefined(record?.sent_count, record?.sentCount), 0);
  const failedCount = intValue(firstDefined(record?.failed_count, record?.failedCount), 0);
  const sendFailedCount = intValue(firstDefined(record?.send_failed_count, record?.sendFailedCount), 0);
  const unreplyableCount = intValue(firstDefined(record?.unreplyable_count, record?.unreplyableCount), 0);
  return {
    day: cleanText(record?.day),
    date: cleanText(record?.date),
    attempts,
    successCount,
    sentCount,
    unreplyableCount,
    failedCount,
    sendFailedCount,
    avgAICallDurationMS: nullableNumber(firstDefined(record?.avg_ai_call_duration_ms, record?.avgAICallDurationMS)),
    avgTotalDurationMS: nullableNumber(firstDefined(record?.avg_total_duration_ms, record?.avgTotalDurationMS)),
    successRate: attempts > 0 ? successCount / attempts : 0,
    sentRate: attempts > 0 ? sentCount / attempts : 0,
    raw: record,
  };
}

function normalizePagination(value = {}) {
  const source = value && typeof value === "object" ? value : {};
  return {
    page: positiveInt(source.page, 1),
    pageSize: positiveInt(firstDefined(source.page_size, source.pageSize), 50),
    total: intValue(source.total, 0),
    totalPages: positiveInt(firstDefined(source.total_pages, source.totalPages), 1),
  };
}
