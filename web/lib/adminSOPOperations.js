export const SOP_DISPATCH_TASKS_PATH = "/admin/sop/dispatch-tasks";
export const SOP_DISPATCH_RESEND_PATH = "/admin/sop/dispatch-tasks/resend";
export const SOP_STAGE_STATS_PATH = "/admin/sop/analytics/stage-stats";
export const SOP_FACTS_PATH = "/admin/sop/analytics/facts";
export const SOP_PLATFORM_TEST_PATH = "/admin/sop/platform/test";

export const SOP_TASK_STATUS_OPTIONS = [
  { value: "all", label: "全部状态" },
  { value: "pending", label: "处理中" },
  { value: "success", label: "成功" },
  { value: "failed", label: "失败" },
  { value: "resent", label: "已补发" },
];

const TASK_STATUS_LABELS = {
  accepted: "处理中",
  dispatched: "处理中",
  pending: "处理中",
  queued: "处理中",
  running: "处理中",
  success: "成功",
  sent: "成功",
  completed: "成功",
  failed: "失败",
  timeout: "超时",
  cancelled: "已取消",
  resent: "已补发",
};

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

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否"].includes(normalized)) return false;
  return fallback;
}

function numberValue(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function compactParams(values = {}) {
  const params = {};
  Object.entries(values).forEach(([key, value]) => {
    const normalized = cleanText(value);
    if (!normalized || normalized === "all") return;
    params[key] = normalized;
  });
  return params;
}

function listFromPayload(payload = {}, key = "items") {
  if (Array.isArray(payload)) return payload;
  if (Array.isArray(payload?.[key])) return payload[key];
  if (Array.isArray(payload?.data?.[key])) return payload.data[key];
  return [];
}

export function defaultSOPDispatchTaskFilters() {
  return {
    date: "",
    flowId: "",
    status: "all",
    keyword: "",
    page: "1",
    pageSize: "30",
  };
}

export function defaultSOPAnalyticsFilters() {
  return {
    date: "",
    flowId: "",
    stageUniqueId: "",
    status: "all",
    keyword: "",
    page: "1",
    pageSize: "30",
  };
}

export function buildSOPDispatchTasksRequest(filters = {}) {
  const params = compactParams({
    date: firstDefined(filters.date, filters.stat_date),
    flow_id: firstDefined(filters.flowId, filters.flow_id),
    status: firstDefined(filters.status, "all"),
    keyword: filters.keyword,
    page: firstDefined(filters.page, 1),
    page_size: firstDefined(filters.pageSize, filters.page_size, 30),
  });
  return {
    ok: true,
    method: "GET",
    path: SOP_DISPATCH_TASKS_PATH,
    params,
  };
}

export function normalizeSOPDispatchTasks(payload = {}) {
  return {
    batches: listFromPayload(payload, "batches").map(normalizeSOPDispatchBatch).filter(Boolean),
    tasks: listFromPayload(payload, "tasks").map(normalizeSOPDispatchTask).filter(Boolean),
    pagination: normalizePagination(payload?.pagination || payload?.data?.pagination),
  };
}

export function normalizeSOPDispatchBatch(record = {}) {
  const taskId = cleanText(firstDefined(record?.task_id, record?.taskId));
  const batchId = cleanText(firstDefined(record?.batch_id, record?.batchId)) || taskId;
  if (!taskId && !batchId) return null;
  const taskStatus = normalizeTaskStatus(firstDefined(record?.task_status, record?.taskStatus));
  const actionPreview = normalizeActionPreview(record?.action_preview || record?.actionPreview);
  return {
    taskId,
    batchId,
    aiTraceId: cleanText(firstDefined(record?.ai_trace_id, record?.aiTraceId)),
    flowId: cleanText(firstDefined(record?.flow_id, record?.flowId)),
    flowName: cleanText(firstDefined(record?.flow_name, record?.flowName)),
    conversationId: cleanText(firstDefined(record?.conversation_id, record?.conversationId)),
    senderName: cleanText(firstDefined(record?.sender_name, record?.senderName, record?.receiver_display_name, record?.receiverDisplayName)),
    assigneeId: cleanText(firstDefined(record?.assignee_id, record?.assigneeId)),
    assigneeName: cleanText(firstDefined(record?.assignee_name, record?.assigneeName)),
    accountId: cleanText(firstDefined(record?.account_id, record?.accountId)),
    deviceId: cleanText(firstDefined(record?.device_id, record?.deviceId)),
    weworkUserId: cleanText(firstDefined(record?.wework_user_id, record?.weworkUserId)),
    entity: cleanText(record?.entity),
    dayStage: cleanText(firstDefined(record?.day_stage, record?.dayStage)),
    customerState: cleanText(firstDefined(record?.customer_state, record?.customerState)),
    stageTag: cleanText(firstDefined(record?.stage_tag, record?.stageTag)),
    stageCount: intValue(firstDefined(record?.stage_count, record?.stageCount), 0),
    recipientCount: intValue(firstDefined(record?.recipient_count, record?.recipientCount), 0),
    actionCount: intValue(firstDefined(record?.action_count, record?.actionCount), actionPreview.length),
    statusCounts: normalizeStatusCounts(record?.status_counts || record?.statusCounts),
    dispatchQueue: cleanText(firstDefined(record?.dispatch_queue, record?.dispatchQueue)) || "slow",
    taskStatus,
    taskStatusLabel: taskStatusLabel(taskStatus),
    taskError: cleanText(firstDefined(record?.task_error, record?.taskError)),
    actionPreview,
    details: Array.isArray(record?.details) ? record.details.map(normalizeSOPDispatchTask).filter(Boolean) : [],
    createdAt: cleanText(firstDefined(record?.created_at, record?.createdAt)),
    completedAt: cleanText(firstDefined(record?.completed_at, record?.completedAt)),
    triggerEvent: cleanText(firstDefined(record?.trigger_event, record?.triggerEvent)),
    canResend: parseBool(firstDefined(record?.can_resend, record?.canResend), false),
    resendBlockReason: cleanText(firstDefined(record?.resend_block_reason, record?.resendBlockReason)),
    originalTaskId: cleanText(firstDefined(record?.original_task_id, record?.originalTaskId)),
    autoResendAttempt: intValue(firstDefined(record?.auto_resend_attempt, record?.autoResendAttempt), 0),
    raw: record,
  };
}

export function normalizeSOPDispatchTask(record = {}) {
  const taskId = cleanText(firstDefined(record?.task_id, record?.taskId));
  if (!taskId) return null;
  return {
    ...normalizeSOPDispatchBatch(record),
    stageUniqueId: cleanText(firstDefined(record?.stage_unique_id, record?.stageUniqueId)),
    stageName: cleanText(firstDefined(record?.stage_name, record?.stageName)),
    messageDetails: Array.isArray(record?.message_details)
      ? record.message_details.map(normalizeSOPMessageDetail).filter(Boolean)
      : [],
    customerReplied: parseBool(firstDefined(record?.customer_replied, record?.customerReplied), false),
    firstCustomerReplyAt: cleanText(firstDefined(record?.first_customer_reply_at, record?.firstCustomerReplyAt)),
    aiReplyStatus: cleanText(firstDefined(record?.ai_reply_status, record?.aiReplyStatus)),
    aiReplyAt: cleanText(firstDefined(record?.ai_reply_at, record?.aiReplyAt)),
  };
}

export function buildSOPDispatchResendMutation(options = {}) {
  const flowId = cleanText(firstDefined(options.flowId, options.flow_id));
  if (!flowId) return { ok: false, error: "flow_id_required" };
  const allFailed = parseBool(firstDefined(options.allFailed, options.all_failed), false);
  const taskIds = cleanStringList(firstDefined(options.taskIds, options.task_ids));
  const singleTaskId = cleanText(firstDefined(options.taskId, options.task_id));
  if (singleTaskId) taskIds.push(singleTaskId);
  const dedupedTaskIds = Array.from(new Set(taskIds));
  if (!allFailed && dedupedTaskIds.length === 0) return { ok: false, error: "task_id_required" };
  const body = {
    flow_id: flowId,
    all_failed: allFailed,
    task_ids: allFailed ? [] : dedupedTaskIds,
    limit: positiveInt(firstDefined(options.limit, 100), 100),
  };
  const date = cleanText(options.date);
  if (date) body.date = date;
  return {
    ok: true,
    method: "POST",
    path: SOP_DISPATCH_RESEND_PATH,
    body,
  };
}

export function normalizeSOPDispatchResendResult(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  return {
    success: source?.success === true,
    date: cleanText(source?.date),
    flowId: cleanText(firstDefined(source?.flow_id, source?.flowId)),
    requested: intValue(source?.requested, 0),
    succeeded: intValue(source?.succeeded, 0),
    failed: intValue(source?.failed, 0),
    results: listFromPayload(source, "results").map((item = {}) => ({
      success: item?.success === true,
      originalTaskId: cleanText(firstDefined(item?.original_task_id, item?.originalTaskId)),
      resendTaskId: cleanText(firstDefined(item?.resend_task_id, item?.resendTaskId)),
      status: cleanText(item?.status),
      error: cleanText(item?.error),
    })),
  };
}

export function buildSOPStageStatsRequest(filters = {}) {
  const params = compactParams({
    date: filters.date,
    flow_id: firstDefined(filters.flowId, filters.flow_id),
  });
  return {
    ok: true,
    method: "GET",
    path: SOP_STAGE_STATS_PATH,
    params,
  };
}

export function normalizeSOPStageStats(payload = {}) {
  return {
    date: cleanText(payload?.date || payload?.data?.date),
    flowId: cleanText(firstDefined(payload?.flow_id, payload?.flowId, payload?.data?.flow_id, payload?.data?.flowId)),
    items: listFromPayload(payload, "items").map(normalizeSOPStageStat).filter(Boolean),
  };
}

export function normalizeSOPStageStat(record = {}) {
  const stageUniqueId = cleanText(firstDefined(record?.stage_unique_id, record?.stageUniqueId));
  if (!stageUniqueId) return null;
  return {
    flowId: cleanText(firstDefined(record?.flow_id, record?.flowId)),
    stageUniqueId,
    stageName: cleanText(firstDefined(record?.stage_name, record?.stageName)),
    stageIndex: intValue(firstDefined(record?.stage_index, record?.stageIndex), 0),
    dayStage: cleanText(firstDefined(record?.day_stage, record?.dayStage)),
    customerState: cleanText(firstDefined(record?.customer_state, record?.customerState)),
    deliveredTaskCount: intValue(firstDefined(record?.delivered_task_count, record?.deliveredTaskCount), 0),
    deliveredCustomerCount: intValue(firstDefined(record?.delivered_customer_count, record?.deliveredCustomerCount), 0),
    deliveredMessageCount: intValue(firstDefined(record?.delivered_message_count, record?.deliveredMessageCount), 0),
    customerOpenCount: intValue(firstDefined(record?.customer_open_count, record?.customerOpenCount), 0),
    customerReplyMessageCount: intValue(firstDefined(record?.customer_reply_message_count, record?.customerReplyMessageCount), 0),
    aiReplyCount: intValue(firstDefined(record?.ai_reply_count, record?.aiReplyCount, record?.ai_reply_customer_count), 0),
    customerOpenRate: numberValue(firstDefined(record?.customer_open_rate, record?.customerOpenRate), 0),
    aiReplyRate: numberValue(firstDefined(record?.ai_reply_rate, record?.aiReplyRate, record?.ai_takeover_rate), 0),
    aiReplyDeliveryRate: numberValue(firstDefined(record?.ai_reply_delivery_rate, record?.aiReplyDeliveryRate), 0),
    raw: record,
  };
}

export function buildSOPFactsRequest(filters = {}) {
  const params = compactParams({
    date: filters.date,
    flow_id: firstDefined(filters.flowId, filters.flow_id),
    stage_unique_id: firstDefined(filters.stageUniqueId, filters.stage_unique_id),
    status: firstDefined(filters.status, "all"),
    keyword: filters.keyword,
    page: firstDefined(filters.page, 1),
    page_size: firstDefined(filters.pageSize, filters.page_size, 30),
  });
  return {
    ok: true,
    method: "GET",
    path: SOP_FACTS_PATH,
    params,
  };
}

export function normalizeSOPFacts(payload = {}) {
  return {
    items: listFromPayload(payload, "items").map(normalizeSOPFact).filter(Boolean),
    pagination: normalizePagination(payload?.pagination || payload?.data?.pagination),
  };
}

export function normalizeSOPFact(record = {}) {
  const factId = cleanText(firstDefined(record?.fact_id, record?.factId));
  if (!factId) return null;
  const deliveryStatus = normalizeTaskStatus(firstDefined(record?.delivery_status, record?.deliveryStatus));
  return {
    factId,
    taskId: cleanText(firstDefined(record?.task_id, record?.taskId)),
    flowId: cleanText(firstDefined(record?.flow_id, record?.flowId)),
    stageUniqueId: cleanText(firstDefined(record?.stage_unique_id, record?.stageUniqueId)),
    stageName: cleanText(firstDefined(record?.stage_name, record?.stageName)),
    dayStage: cleanText(firstDefined(record?.day_stage, record?.dayStage)),
    customerState: cleanText(firstDefined(record?.customer_state, record?.customerState)),
    conversationId: cleanText(firstDefined(record?.conversation_id, record?.conversationId)),
    conversationKey: cleanText(firstDefined(record?.conversation_key, record?.conversationKey)),
    deliveryStatus,
    deliveryStatusLabel: taskStatusLabel(deliveryStatus),
    deliveryError: cleanText(firstDefined(record?.delivery_error, record?.deliveryError)),
    messageCount: intValue(firstDefined(record?.message_count, record?.messageCount), 0),
    queuedAt: cleanText(firstDefined(record?.queued_at, record?.queuedAt)),
    deliveredAt: cleanText(firstDefined(record?.delivered_at, record?.deliveredAt)),
    failedAt: cleanText(firstDefined(record?.failed_at, record?.failedAt)),
    customerReplied: parseBool(firstDefined(record?.customer_replied, record?.customerReplied), false),
    aiReplyStatus: cleanText(firstDefined(record?.ai_reply_status, record?.aiReplyStatus)),
    raw: record,
  };
}

export function buildSOPPlatformTestMutation(options = {}) {
  const taskURL = cleanText(firstDefined(options.taskURL, options.task_url));
  if (!taskURL) return { ok: false, error: "task_url_required" };
  return {
    ok: true,
    method: "POST",
    path: SOP_PLATFORM_TEST_PATH,
    body: { task_url: taskURL },
  };
}

export function normalizeSOPPlatformTestResult(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  return {
    success: source?.success === true,
    message: cleanText(source?.message),
  };
}

export function isSOPDispatchBatchResendable(row = {}) {
  const normalized = normalizeSOPDispatchBatch(row) || row;
  return normalized.canResend === true && normalized.taskStatus === "failed" && Boolean(normalized.taskId);
}

export function formatSOPTaskStatusCounts(row = {}) {
  const counts = normalizeStatusCounts(row?.statusCounts || row?.status_counts || row?.raw?.status_counts);
  const parts = [];
  if (counts.success) parts.push(`成功 ${counts.success}`);
  if (counts.failed) parts.push(`失败 ${counts.failed}`);
  if (counts.resent) parts.push(`已补发 ${counts.resent}`);
  const pending = (counts.pending || 0) + (counts.accepted || 0) + (counts.dispatched || 0) + (counts.running || 0);
  if (pending) parts.push(`处理中 ${pending}`);
  return parts.join(" / ") || "-";
}

export function formatSOPRate(value = 0) {
  const number = numberValue(value, 0);
  return `${(number * 100).toFixed(1)}%`;
}

function normalizeTaskStatus(value = "") {
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return "pending";
  if (normalized === "sent" || normalized === "completed") return "success";
  return normalized;
}

function taskStatusLabel(status = "") {
  const normalized = normalizeTaskStatus(status);
  return TASK_STATUS_LABELS[normalized] || normalized || "处理中";
}

function normalizeStatusCounts(value = {}) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value).map(([key, count]) => [cleanText(key).toLowerCase(), intValue(count, 0)]));
}

function normalizeActionPreview(value = []) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => ({
      type: cleanText(item?.type) || "text",
      contentPreview: cleanText(firstDefined(item?.content_preview, item?.contentPreview, item?.content, item?.text)),
    }))
    .filter((item) => item.contentPreview);
}

function normalizeSOPMessageDetail(value = {}) {
  const content = cleanText(value?.content);
  if (!content) return null;
  return {
    messageIndex: intValue(firstDefined(value?.message_index, value?.messageIndex), 0),
    type: cleanText(value?.type) || "text",
    content,
    stageUniqueId: cleanText(firstDefined(value?.stage_unique_id, value?.stageUniqueId)),
    taskStatus: normalizeTaskStatus(firstDefined(value?.task_status, value?.taskStatus)),
    taskError: cleanText(firstDefined(value?.task_error, value?.taskError)),
  };
}

function normalizePagination(value = {}) {
  const source = value && typeof value === "object" ? value : {};
  return {
    page: positiveInt(source.page, 1),
    pageSize: positiveInt(firstDefined(source.page_size, source.pageSize), 30),
    total: intValue(source.total, 0),
    totalPages: positiveInt(firstDefined(source.total_pages, source.totalPages), 1),
  };
}

function cleanStringList(value = []) {
  const source = Array.isArray(value) ? value : cleanText(value).split(/[\n,，;；]/);
  return source.map(cleanText).filter(Boolean);
}
