export const SOP_FLOWS_PATH = "/admin/sop/flows";
export const SOP_POLICIES_PATH = "/admin/sop/policies";

export const TARGET_AUDIENCE_ALL = "__ALL__";
export const TARGET_AUDIENCE_NONE = "__NONE__";

export const SOP_FLOW_MODE_OPTIONS = [
  { value: "local_days", label: "本地 Day 规则" },
  { value: "platform_pull", label: "接口拉任务" },
];

export const SOP_PLATFORM_PULL_DRIVER_OPTIONS = [
  { value: "conversation", label: "会话驱动" },
  { value: "platform_task", label: "平台任务驱动" },
];

export const SOP_PLATFORM_QUEUE_OPTIONS = [
  { value: "slow", label: "slow 慢通道" },
  { value: "fast", label: "fast 快通道" },
];

export const SOP_REPLY_MODE_OPTIONS = [
  { value: "sop_only", label: "仅 SOP" },
  { value: "sop_variable_fill", label: "SOP + 变量" },
  { value: "sop_ai_rewrite", label: "SOP + AI 改写" },
  { value: "human_only", label: "仅人工" },
];

export const SOP_CUSTOMER_STATE_OPTIONS = [
  { value: "undecided", label: "未定" },
  { value: "first_add", label: "首次加微" },
  { value: "paid", label: "已定" },
  { value: "booked", label: "已预约" },
  { value: "arrived", label: "到店" },
  { value: "aftersales", label: "术后/售后" },
  { value: "refund", label: "退款/投诉" },
  { value: "pending_pool", label: "待定池" },
];

export const SOP_MEDIA_STRATEGY_OPTIONS = [
  { value: "fixed", label: "固定资源" },
  { value: "tagged", label: "标签匹配" },
];

const FLOW_MODE_LABELS = optionLabelMap(SOP_FLOW_MODE_OPTIONS);
const PULL_DRIVER_LABELS = optionLabelMap(SOP_PLATFORM_PULL_DRIVER_OPTIONS);
const QUEUE_LABELS = optionLabelMap(SOP_PLATFORM_QUEUE_OPTIONS);
const REPLY_MODE_LABELS = optionLabelMap(SOP_REPLY_MODE_OPTIONS);
const CUSTOMER_STATE_LABELS = optionLabelMap(SOP_CUSTOMER_STATE_OPTIONS);
const MEDIA_STRATEGY_LABELS = optionLabelMap(SOP_MEDIA_STRATEGY_OPTIONS);

function optionLabelMap(options) {
  return Object.fromEntries(options.map((item) => [item.value, item.label]));
}

function cleanText(value) {
  return String(value ?? "").trim();
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "启用", "开启"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "停用", "关闭"].includes(normalized)) return false;
  return fallback;
}

function positiveInt(value, fallback = 1) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.max(1, Math.trunc(parsed));
}

function intValue(value, fallback = 0) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.trunc(parsed);
}

function normalizeOption(value, labels, fallback) {
  const normalized = cleanText(value);
  return Object.prototype.hasOwnProperty.call(labels, normalized) ? normalized : fallback;
}

export function defaultSOPFlowForm() {
  return {
    flowId: "",
    flowName: "",
    targetAudienceMode: "none",
    targetAudienceIds: "",
    executionMode: "local_days",
    dayCount: "1",
    platformPullDriver: "conversation",
    platformTaskLimit: "20",
    platformDispatchQueue: "slow",
    platformTaskURL: "",
    executionWindowsText: "",
    enabled: false,
    humanHandoffRule: "",
    riskKeywords: "",
    editing: false,
  };
}

export function defaultSOPPolicyForm(flowId = "default") {
  const resolvedFlowID = cleanText(flowId) || "default";
  return {
    policyId: "",
    flowId: resolvedFlowID,
    name: `${resolvedFlowID}-Day1`,
    dayStage: "day1",
    stageTag: "",
    customerState: "undecided",
    dispatchQueue: "slow",
    triggerEvent: "incoming_message",
    enabled: true,
    priority: "100",
    replyMode: "sop_only",
    promptTemplate: "",
    replyText: "",
    imageURLs: "",
    videoURLs: "",
    messageSequence: "",
    needRAG: false,
    needAIRewrite: false,
    mediaStrategy: "fixed",
    humanHandoffRule: "",
    riskKeywords: "",
    editing: false,
  };
}

export function normalizeAdminSOPFlows(payload = {}) {
  const flows = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.flows)
      ? payload.flows
      : Array.isArray(payload?.data?.flows)
        ? payload.data.flows
        : Array.isArray(payload?.records)
          ? payload.records
          : [];
  return flows.map(normalizeAdminSOPFlow).filter(Boolean);
}

export function normalizeAdminSOPFlow(record = {}) {
  const source = record?.flow_config && typeof record.flow_config === "object"
    ? { ...record.flow_config, flow_id: firstDefined(record.flow_config.flow_id, record.flow_id) }
    : record;
  const flowId = cleanText(firstDefined(source?.flow_id, source?.flowId));
  if (!flowId) return null;
  const targetAudience = normalizeSOPTargetAudience(firstDefined(source?.target_audience, source?.targetAudience), true);
  const executionMode = normalizeOption(firstDefined(source?.execution_mode, source?.executionMode), FLOW_MODE_LABELS, "local_days");
  const platformPullDriver = normalizeOption(firstDefined(source?.platform_pull_driver, source?.platformPullDriver), PULL_DRIVER_LABELS, "conversation");
  const platformDispatchQueue = normalizeOption(firstDefined(source?.platform_dispatch_queue, source?.platformDispatchQueue), QUEUE_LABELS, "slow");
  const executionWindows = normalizeSOPExecutionTimeWindows(firstDefined(source?.execution_time_windows, source?.executionWindows));
  return {
    flowId,
    flowName: cleanText(firstDefined(source?.flow_name, source?.flowName)) || flowId,
    targetAudience,
    targetAudienceMode: sopTargetAudienceMode(targetAudience),
    targetAudienceIds: parseSOPTargetAudience(targetAudience).join("\n"),
    targetAudienceLabel: sopTargetAudienceLabel(targetAudience),
    executionMode,
    executionModeLabel: FLOW_MODE_LABELS[executionMode],
    dayCount: positiveInt(firstDefined(source?.day_count, source?.dayCount), 1),
    platformPullDriver,
    platformPullDriverLabel: PULL_DRIVER_LABELS[platformPullDriver],
    platformTaskLimit: positiveInt(firstDefined(source?.platform_task_limit, source?.platformTaskLimit), 20),
    platformDispatchQueue,
    platformDispatchQueueLabel: QUEUE_LABELS[platformDispatchQueue],
    platformTaskURL: cleanText(firstDefined(source?.platform_task_url, source?.platformTaskURL)),
    executionWindows,
    executionWindowsText: formatSOPExecutionTimeWindows(executionWindows),
    enabled: parseBool(source?.enabled, true),
    enabledLabel: parseBool(source?.enabled, true) ? "启用" : "停用",
    humanHandoffRule: cleanText(firstDefined(source?.human_handoff_rule, source?.humanHandoffRule)),
    riskKeywords: cleanText(firstDefined(source?.risk_keywords, source?.riskKeywords)),
    createdAt: cleanText(firstDefined(source?.created_at, source?.createdAt)),
    updatedAt: cleanText(firstDefined(source?.updated_at, source?.updatedAt)),
    raw: record,
  };
}

export function buildSOPFlowForm(record = {}) {
  const flow = normalizeAdminSOPFlow(record) || {};
  return {
    ...defaultSOPFlowForm(),
    flowId: flow.flowId || "",
    flowName: flow.flowName || "",
    targetAudienceMode: flow.targetAudienceMode || "none",
    targetAudienceIds: flow.targetAudienceIds || "",
    executionMode: flow.executionMode || "local_days",
    dayCount: String(flow.dayCount || 1),
    platformPullDriver: flow.platformPullDriver || "conversation",
    platformTaskLimit: String(flow.platformTaskLimit || 20),
    platformDispatchQueue: flow.platformDispatchQueue || "slow",
    platformTaskURL: flow.platformTaskURL || "",
    executionWindowsText: flow.executionWindowsText || "",
    enabled: flow.enabled === true,
    humanHandoffRule: flow.humanHandoffRule || "",
    riskKeywords: flow.riskKeywords || "",
    editing: Boolean(flow.flowId),
  };
}

export function buildSOPFlowUpsertMutation(options = {}) {
  const flowId = cleanText(firstDefined(options.flowId, options.flow_id));
  if (!flowId) return { ok: false, error: "flow_id_required" };
  const targetAudience = stringifySOPTargetAudience(
    firstDefined(options.targetAudienceMode, options.target_audience_mode),
    firstDefined(options.targetAudienceIds, options.target_audience_ids, options.targetAudience, options.target_audience),
  );
  const enabled = parseBool(firstDefined(options.enabled, false), false);
  if (enabled && targetAudience === TARGET_AUDIENCE_NONE) return { ok: false, error: "target_audience_required" };

  const body = {
    flow_id: flowId,
    flow_name: cleanText(firstDefined(options.flowName, options.flow_name)) || flowId,
    target_audience: targetAudience,
    execution_mode: normalizeOption(firstDefined(options.executionMode, options.execution_mode), FLOW_MODE_LABELS, "local_days"),
    day_count: positiveInt(firstDefined(options.dayCount, options.day_count), 1),
    platform_pull_driver: normalizeOption(firstDefined(options.platformPullDriver, options.platform_pull_driver), PULL_DRIVER_LABELS, "conversation"),
    platform_task_limit: positiveInt(firstDefined(options.platformTaskLimit, options.platform_task_limit), 20),
    platform_dispatch_queue: normalizeOption(firstDefined(options.platformDispatchQueue, options.platform_dispatch_queue), QUEUE_LABELS, "slow"),
    platform_task_url: cleanText(firstDefined(options.platformTaskURL, options.platform_task_url)),
    execution_time_windows: normalizeSOPExecutionTimeWindows(firstDefined(options.executionWindowsText, options.execution_time_windows, options.executionWindows)),
    enabled,
    human_handoff_rule: cleanText(firstDefined(options.humanHandoffRule, options.human_handoff_rule)),
    risk_keywords: cleanText(firstDefined(options.riskKeywords, options.risk_keywords)),
  };
  return {
    ok: true,
    method: "POST",
    path: SOP_FLOWS_PATH,
    body,
  };
}

export function buildSOPFlowDeleteMutation(flowId = "") {
  const normalizedFlowID = cleanText(flowId);
  if (!normalizedFlowID) return { ok: false, error: "flow_id_required" };
  if (normalizedFlowID === "default") return { ok: false, error: "default_flow_protected" };
  return {
    ok: true,
    method: "DELETE",
    path: `${SOP_FLOWS_PATH}/${encodeURIComponent(normalizedFlowID)}`,
  };
}

export function normalizeAdminSOPPolicies(payload = {}) {
  const directPolicies = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.policies)
      ? payload.policies
      : Array.isArray(payload?.data?.policies)
        ? payload.data.policies
        : Array.isArray(payload?.records)
          ? payload.records
          : [];
  if (directPolicies.length > 0) {
    return directPolicies.map(normalizeAdminSOPPolicy).filter(Boolean);
  }
  const flows = Array.isArray(payload?.flows)
    ? payload.flows
    : Array.isArray(payload?.data?.flows)
      ? payload.data.flows
      : [];
  return flows
    .flatMap((flow) => {
      const flowId = cleanText(firstDefined(flow?.flow_id, flow?.flowId)) || "default";
      const policies = Array.isArray(flow?.policies) ? flow.policies : [];
      return policies.map((policy) => ({ ...policy, flow_id: firstDefined(policy?.flow_id, flowId) }));
    })
    .map(normalizeAdminSOPPolicy)
    .filter(Boolean);
}

export function normalizeAdminSOPPolicy(record = {}) {
  const policyId = cleanText(firstDefined(record?.policy_id, record?.policyId));
  if (!policyId) return null;
  const flowId = cleanText(firstDefined(record?.flow_id, record?.flowId)) || "default";
  const dayStage = cleanText(firstDefined(record?.day_stage, record?.dayStage)) || "day1";
  const replyMode = normalizeOption(firstDefined(record?.reply_mode, record?.replyMode), REPLY_MODE_LABELS, "sop_only");
  const customerState = normalizeOption(firstDefined(record?.customer_state, record?.customerState), CUSTOMER_STATE_LABELS, "undecided");
  const dispatchQueue = normalizeOption(firstDefined(record?.dispatch_queue, record?.dispatchQueue), QUEUE_LABELS, "slow");
  const mediaStrategy = normalizeOption(firstDefined(record?.media_strategy, record?.mediaStrategy), MEDIA_STRATEGY_LABELS, "fixed");
  const messages = normalizeSOPMessages(
    firstDefined(record?.messages, record?.message_sequence, record?.messageSequence),
    firstDefined(record?.reply_text, record?.replyText),
    firstDefined(record?.image_urls, record?.imageURLs),
    firstDefined(record?.video_urls, record?.videoURLs),
  );
  return {
    policyId,
    flowId,
    name: cleanText(record?.name) || `${flowId}-${dayStage}`,
    dayStage,
    stageTag: cleanText(firstDefined(record?.stage_tag, record?.stageTag)),
    customerState,
    customerStateLabel: CUSTOMER_STATE_LABELS[customerState],
    dispatchQueue,
    dispatchQueueLabel: QUEUE_LABELS[dispatchQueue],
    triggerEvent: cleanText(firstDefined(record?.trigger_event, record?.triggerEvent)) || "incoming_message",
    enabled: parseBool(record?.enabled, true),
    enabledLabel: parseBool(record?.enabled, true) ? "启用" : "停用",
    priority: intValue(record?.priority, 100),
    replyMode,
    replyModeLabel: REPLY_MODE_LABELS[replyMode],
    promptTemplate: cleanText(firstDefined(record?.prompt_template, record?.promptTemplate)),
    replyText: cleanText(firstDefined(record?.reply_text, record?.replyText)),
    imageURLs: cleanText(firstDefined(record?.image_urls, record?.imageURLs)),
    videoURLs: cleanText(firstDefined(record?.video_urls, record?.videoURLs)),
    messageSequence: cleanText(firstDefined(record?.message_sequence, record?.messageSequence)),
    messages,
    needRAG: parseBool(firstDefined(record?.need_rag, record?.needRAG), false),
    needAIRewrite: parseBool(firstDefined(record?.need_ai_rewrite, record?.needAIRewrite), false),
    mediaStrategy,
    mediaStrategyLabel: MEDIA_STRATEGY_LABELS[mediaStrategy],
    humanHandoffRule: cleanText(firstDefined(record?.human_handoff_rule, record?.humanHandoffRule)),
    riskKeywords: cleanText(firstDefined(record?.risk_keywords, record?.riskKeywords)),
    createdAt: cleanText(firstDefined(record?.created_at, record?.createdAt)),
    updatedAt: cleanText(firstDefined(record?.updated_at, record?.updatedAt)),
    raw: record,
  };
}

export function buildSOPPolicyForm(record = {}) {
  const policy = normalizeAdminSOPPolicy(record) || {};
  const messages = Array.isArray(policy.messages) ? policy.messages : [];
  const textMessages = messages.filter((item) => item.type === "text").map((item) => item.content);
  const imageMessages = messages.filter((item) => item.type === "image").map((item) => item.content);
  const videoMessages = messages.filter((item) => item.type === "video").map((item) => item.content);
  return {
    ...defaultSOPPolicyForm(policy.flowId || "default"),
    policyId: policy.policyId || "",
    flowId: policy.flowId || "default",
    name: policy.name || "",
    dayStage: policy.dayStage || "day1",
    stageTag: policy.stageTag || "",
    customerState: policy.customerState || "undecided",
    dispatchQueue: policy.dispatchQueue || "slow",
    triggerEvent: policy.triggerEvent || "incoming_message",
    enabled: policy.enabled !== false,
    priority: String(policy.priority ?? 100),
    replyMode: policy.replyMode || "sop_only",
    promptTemplate: policy.promptTemplate || "",
    replyText: policy.replyText || textMessages.join("\n"),
    imageURLs: policy.imageURLs || imageMessages.join("\n"),
    videoURLs: policy.videoURLs || videoMessages.join("\n"),
    messageSequence: policy.messageSequence || (messages.length > 0 ? JSON.stringify(messages) : ""),
    needRAG: policy.needRAG === true,
    needAIRewrite: policy.needAIRewrite === true,
    mediaStrategy: policy.mediaStrategy || "fixed",
    humanHandoffRule: policy.humanHandoffRule || "",
    riskKeywords: policy.riskKeywords || "",
    editing: Boolean(policy.policyId),
  };
}

export function buildSOPPoliciesListRequest(options = {}) {
  const flowId = cleanText(firstDefined(options.flowId, options.flow_id));
  const dayStage = cleanText(firstDefined(options.dayStage, options.day_stage));
  const params = {};
  if (flowId && flowId !== "all") params.flow_id = flowId;
  if (dayStage && dayStage !== "all") params.day_stage = dayStage;
  return {
    ok: true,
    method: "GET",
    path: SOP_POLICIES_PATH,
    params,
  };
}

export function buildSOPPolicyUpsertMutation(options = {}) {
  const flowId = cleanText(firstDefined(options.flowId, options.flow_id)) || "default";
  const dayStage = cleanText(firstDefined(options.dayStage, options.day_stage));
  if (!dayStage) return { ok: false, error: "day_stage_required" };
  const name = cleanText(options.name);
  if (!name) return { ok: false, error: "name_required" };
  const triggerEvent = cleanText(firstDefined(options.triggerEvent, options.trigger_event));
  if (!triggerEvent) return { ok: false, error: "trigger_event_required" };

  const messages = normalizeSOPMessages(
    firstDefined(options.messageSequence, options.message_sequence),
    firstDefined(options.replyText, options.reply_text),
    firstDefined(options.imageURLs, options.image_urls),
    firstDefined(options.videoURLs, options.video_urls),
  );
  const textMessages = messages.filter((item) => item.type === "text").map((item) => item.content);
  const replyText = cleanText(firstDefined(options.replyText, options.reply_text)) || textMessages[0] || "";
  const promptTemplate = cleanText(firstDefined(options.promptTemplate, options.prompt_template));
  if (!replyText && !promptTemplate) return { ok: false, error: "reply_content_required" };

  const messageSequence = normalizeMessageSequenceInput(
    firstDefined(options.messageSequence, options.message_sequence),
    messages,
  );
  const body = {
    policy_id: cleanText(firstDefined(options.policyId, options.policy_id)),
    flow_id: flowId,
    name,
    day_stage: dayStage,
    stage_tag: cleanText(firstDefined(options.stageTag, options.stage_tag)),
    customer_state: normalizeOption(firstDefined(options.customerState, options.customer_state), CUSTOMER_STATE_LABELS, "undecided"),
    dispatch_queue: normalizeOption(firstDefined(options.dispatchQueue, options.dispatch_queue), QUEUE_LABELS, "slow"),
    trigger_event: triggerEvent,
    enabled: parseBool(firstDefined(options.enabled, true), true),
    priority: intValue(firstDefined(options.priority, 100), 100),
    reply_mode: normalizeOption(firstDefined(options.replyMode, options.reply_mode), REPLY_MODE_LABELS, "sop_only"),
    prompt_template: promptTemplate,
    reply_text: replyText,
    image_urls: cleanText(firstDefined(options.imageURLs, options.image_urls)),
    video_urls: cleanText(firstDefined(options.videoURLs, options.video_urls)),
    message_sequence: messageSequence,
    need_rag: parseBool(firstDefined(options.needRAG, options.need_rag), false),
    need_ai_rewrite: parseBool(firstDefined(options.needAIRewrite, options.need_ai_rewrite), false),
    media_strategy: normalizeOption(firstDefined(options.mediaStrategy, options.media_strategy), MEDIA_STRATEGY_LABELS, "fixed"),
    human_handoff_rule: cleanText(firstDefined(options.humanHandoffRule, options.human_handoff_rule)),
    risk_keywords: cleanText(firstDefined(options.riskKeywords, options.risk_keywords)),
  };
  return {
    ok: true,
    method: "POST",
    path: SOP_POLICIES_PATH,
    body,
  };
}

export function buildSOPPolicyDeleteMutation(policyId = "") {
  const normalizedPolicyID = cleanText(policyId);
  if (!normalizedPolicyID) return { ok: false, error: "policy_id_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${SOP_POLICIES_PATH}/${encodeURIComponent(normalizedPolicyID)}`,
  };
}

export function normalizeSOPTargetAudience(value = "", emptyAsAll = false) {
  const normalized = cleanText(value);
  if (!normalized) return emptyAsAll ? TARGET_AUDIENCE_ALL : TARGET_AUDIENCE_NONE;
  if (normalized === TARGET_AUDIENCE_ALL || normalized === TARGET_AUDIENCE_NONE) return normalized;
  const values = parseSOPTargetAudience(normalized);
  return values.length > 0 ? values.join(",") : TARGET_AUDIENCE_NONE;
}

export function sopTargetAudienceMode(value = "") {
  const normalized = normalizeSOPTargetAudience(value, false);
  if (normalized === TARGET_AUDIENCE_ALL) return "all";
  if (normalized === TARGET_AUDIENCE_NONE) return "none";
  return "specific";
}

export function parseSOPTargetAudience(value = "") {
  const normalized = cleanText(value);
  if (!normalized || normalized === TARGET_AUDIENCE_ALL || normalized === TARGET_AUDIENCE_NONE) return [];
  const values = [];
  const seen = new Set();
  for (const part of normalized.split(/[\n,，;；]/)) {
    const candidate = cleanText(part);
    if (!candidate || candidate === TARGET_AUDIENCE_ALL || candidate === TARGET_AUDIENCE_NONE || seen.has(candidate)) continue;
    seen.add(candidate);
    values.push(candidate);
  }
  return values;
}

export function stringifySOPTargetAudience(mode = "none", rawIds = "") {
  const normalizedMode = cleanText(mode) || "none";
  if (normalizedMode === "all") return TARGET_AUDIENCE_ALL;
  if (normalizedMode === "specific") {
    const ids = parseSOPTargetAudience(rawIds);
    return ids.length > 0 ? ids.join(",") : TARGET_AUDIENCE_NONE;
  }
  return TARGET_AUDIENCE_NONE;
}

export function sopTargetAudienceLabel(value = "") {
  const normalized = normalizeSOPTargetAudience(value, false);
  if (normalized === TARGET_AUDIENCE_ALL) return "全部消息端";
  if (normalized === TARGET_AUDIENCE_NONE) return "未选择";
  return parseSOPTargetAudience(normalized).join(", ");
}

export function normalizeSOPExecutionTimeWindows(value = "") {
  if (Array.isArray(value)) {
    return value.map(normalizeWindow).filter(Boolean);
  }
  const raw = cleanText(value);
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) return parsed.map(normalizeWindow).filter(Boolean);
  } catch {
    // Fall through to textarea format parsing.
  }
  return raw
    .split(/[\n；;]/)
    .map((line) => {
      const [start, end] = cleanText(line).split(/\s*(?:-|~|至|到)\s*/);
      return normalizeWindow({ start, end });
    })
    .filter(Boolean);
}

export function formatSOPExecutionTimeWindows(windows = []) {
  return normalizeSOPExecutionTimeWindows(windows).map((item) => `${item.start}-${item.end}`).join("\n");
}

function normalizeWindow(item = {}) {
  const start = cleanText(item?.start);
  const end = cleanText(item?.end);
  if (!isHHMM(start) || !isHHMM(end)) return null;
  return { start, end };
}

function isHHMM(value = "") {
  return /^([01]\d|2[0-3]):[0-5]\d$/.test(cleanText(value));
}

export function normalizeSOPMessages(raw = "", fallbackText = "", fallbackImages = "", fallbackVideos = "") {
  const parsed = parseMessages(raw);
  if (parsed.length > 0) return parsed;
  const messages = [];
  for (const line of cleanText(fallbackText).split("\n")) {
    const content = cleanText(line);
    if (content) messages.push({ type: "text", content, preview_url: "" });
  }
  for (const line of cleanText(fallbackImages).split("\n")) {
    const content = cleanText(line);
    if (content) messages.push({ type: "image", content, preview_url: "" });
  }
  for (const line of cleanText(fallbackVideos).split("\n")) {
    const content = cleanText(line);
    if (content) messages.push({ type: "video", content, preview_url: "" });
  }
  return messages;
}

function parseMessages(raw = "") {
  const source = Array.isArray(raw) ? raw : parseMessageJSON(raw);
  return source
    .map((item) => ({
      type: normalizeMessageType(item?.type),
      content: cleanText(item?.content),
      preview_url: cleanText(item?.preview_url || item?.previewUrl),
    }))
    .filter((item) => item.content);
}

function parseMessageJSON(raw = "") {
  const text = cleanText(raw);
  if (!text) return [];
  try {
    const parsed = JSON.parse(text);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function normalizeMessageType(value = "") {
  const normalized = cleanText(value).toLowerCase();
  return ["text", "image", "video", "file"].includes(normalized) ? normalized : "text";
}

function normalizeMessageSequenceInput(raw = "", fallbackMessages = []) {
  const parsed = parseMessages(raw);
  const messages = parsed.length > 0 ? parsed : fallbackMessages;
  return messages.length > 0
    ? JSON.stringify(messages.map((item) => ({ type: item.type, content: item.content })))
    : "";
}
