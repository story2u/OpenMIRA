export const AUDIT_LOGS_PATH = "/admin/audit-logs";
export const SYSTEM_LOGS_PATH = "/admin/system-logs";

export const AUDIT_LOG_PAGE_SIZE_OPTIONS = [20, 50, 100];
export const SYSTEM_LOG_LIMIT_OPTIONS = [20, 50, 100, 200, 500];
export const SYSTEM_LOG_LEVEL_OPTIONS = ["all", "debug", "info", "warn", "error"];

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

function listFromPayload(payload = {}, key = "items") {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.[key])) return payload[key];
  if (Array.isArray(payload?.data?.[key])) return payload.data[key];
  return [];
}

function unwrapPayload(payload = {}) {
  return payload?.data && typeof payload.data === "object" ? payload.data : payload && typeof payload === "object" ? payload : {};
}

export function defaultAuditLogFilters() {
  return {
    operator: "",
    actionType: "all",
    date: "",
    page: "1",
    pageSize: "20",
  };
}

export function defaultSystemLogFilters() {
  return {
    date: "",
    level: "all",
    module: "",
    keyword: "",
    limit: "200",
    offset: "0",
  };
}

export function buildAuditLogsRequest(filters = {}) {
  const params = {
    page: boundedInt(filters.page, 1, 1, Number.MAX_SAFE_INTEGER),
    page_size: boundedInt(firstDefined(filters.pageSize, filters.page_size), 20, 1, 100),
  };
  const operator = cleanText(filters.operator);
  if (operator) params.operator = operator;
  const actionType = cleanText(firstDefined(filters.actionType, filters.action_type));
  if (actionType && actionType !== "all") params.action_type = actionType;
  const date = cleanText(filters.date);
  if (date) params.date = date;
  return {
    ok: true,
    method: "GET",
    path: AUDIT_LOGS_PATH,
    params,
  };
}

export function buildSystemLogsRequest(filters = {}) {
  const params = {
    limit: boundedInt(filters.limit, 200, 1, 500),
    offset: Math.max(0, intValue(filters.offset, 0)),
  };
  const date = cleanText(filters.date);
  if (date) params.date = date;
  const level = cleanText(filters.level);
  if (level && level !== "all") params.level = level;
  const module = cleanText(filters.module);
  if (module) params.module = module;
  const keyword = cleanText(filters.keyword);
  if (keyword) params.keyword = keyword;
  return {
    ok: true,
    method: "GET",
    path: SYSTEM_LOGS_PATH,
    params,
  };
}

export function normalizeAuditLogs(payload = {}) {
  const data = unwrapPayload(payload);
  const pagination = normalizeAuditPagination(data?.pagination);
  return {
    logs: listFromPayload(data, "logs").map(normalizeAuditLog),
    pagination,
    raw: data,
  };
}

export function normalizeSystemLogs(payload = {}, request = {}) {
  const data = unwrapPayload(payload);
  const limit = boundedInt(firstDefined(request.limit, request.params?.limit), 200, 1, 500);
  const offset = Math.max(0, intValue(firstDefined(request.offset, request.params?.offset), 0));
  const items = listFromPayload(data, "items").map(normalizeSystemLog);
  const total = Math.max(0, intValue(data?.total, items.length));
  return {
    items,
    total,
    date: cleanText(data?.date),
    limit,
    offset,
    hasPrevious: offset > 0,
    hasNext: offset + items.length < total,
    raw: data,
  };
}

function normalizeAuditLog(value = {}) {
  return {
    logID: cleanText(firstDefined(value?.log_id, value?.logID)),
    operator: cleanText(value?.operator),
    actionType: cleanText(firstDefined(value?.action_type, value?.actionType)),
    detail: cleanText(value?.detail),
    ip: cleanText(value?.ip),
    createdAt: cleanText(firstDefined(value?.created_at, value?.createdAt)),
    raw: value,
  };
}

function normalizeAuditPagination(value = {}) {
  return {
    page: boundedInt(value?.page, 1, 1, Number.MAX_SAFE_INTEGER),
    pageSize: boundedInt(firstDefined(value?.page_size, value?.pageSize), 20, 1, 100),
    total: Math.max(0, intValue(value?.total, 0)),
    totalPages: boundedInt(firstDefined(value?.total_pages, value?.totalPages), 1, 1, Number.MAX_SAFE_INTEGER),
  };
}

function normalizeSystemLog(value = {}) {
  return {
    timestamp: cleanText(firstDefined(value?.ts, value?.timestamp, value?.time)),
    level: cleanText(value?.level),
    module: cleanText(value?.module),
    action: cleanText(value?.action),
    detail: cleanText(firstDefined(value?.detail, value?.message)),
    operator: cleanText(value?.operator),
    traceID: cleanText(firstDefined(value?.trace_id, value?.traceID)),
    extra: value?.extra && typeof value.extra === "object" ? value.extra : null,
    raw: value,
  };
}
