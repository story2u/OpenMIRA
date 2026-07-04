export const REPLY_OVERDUE_SECONDS = 60;
export const AI_REPLY_ERROR_DISMISS_STORAGE_KEY = "wework:chat:ai_reply_error_dismissed";
export const AI_REPLY_ERROR_DISMISS_STORAGE_LIMIT = 100;

function cleanText(value) {
  return String(value || "").trim();
}

function parseBool(value) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0 || value === null) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return false;
  return ["true", "1", "yes", "on", "是", "开启", "启用"].includes(normalized);
}

function toTimestamp(value) {
  const timestamp = new Date(value || 0).getTime();
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function runtimeState(conversation = {}) {
  return conversation?.sop_runtime_state && typeof conversation.sop_runtime_state === "object"
    ? conversation.sop_runtime_state
    : {};
}

function runtimeText(conversation, key) {
  const runtime = runtimeState(conversation);
  return cleanText(conversation?.[key] ?? runtime?.[key]);
}

function runtimeBool(conversation, key) {
  const runtime = runtimeState(conversation);
  if (conversation?.[key] !== undefined && conversation?.[key] !== null) return parseBool(conversation[key]);
  return parseBool(runtime?.[key]);
}

function normalizeDirection(value) {
  const direction = cleanText(value).toLowerCase();
  return direction === "incoming" || direction === "outgoing" ? direction : "";
}

function normalizeReplyState(value) {
  const replyState = cleanText(value).toLowerCase();
  return replyState === "pending" || replyState === "replied" ? replyState : "";
}

export function resolveConversationLastDirection(conversation = {}) {
  const explicitDirection = normalizeDirection(conversation?.last_direction || conversation?.direction);
  if (explicitDirection) return explicitDirection;
  const lastMessageAt = cleanText(conversation?.last_message_at);
  const lastIncomingAt = cleanText(conversation?.last_incoming_at);
  const lastOutgoingAt = cleanText(conversation?.last_outgoing_at);
  if (lastMessageAt && lastMessageAt === lastIncomingAt) return "incoming";
  if (lastMessageAt && lastMessageAt === lastOutgoingAt) return "outgoing";
  return "";
}

export function resolvePendingReplyStartedAt(conversation = {}) {
  const explicitReplyState = normalizeReplyState(conversation?.reply_state);
  if (explicitReplyState === "replied") return 0;
  const explicitStartedAt = toTimestamp(conversation?.pending_reply_started_at);
  if (explicitStartedAt > 0) return explicitStartedAt;
  const lastDirection = resolveConversationLastDirection(conversation);
  if (lastDirection !== "incoming") return 0;
  return toTimestamp(conversation?.last_incoming_at) || toTimestamp(conversation?.last_message_at);
}

export function resolvePendingReplySeconds(conversation = {}, nowMs = Date.now()) {
  const explicitReplyState = normalizeReplyState(conversation?.reply_state);
  const explicitPendingSeconds = Math.max(0, Number(conversation?.pending_reply_seconds || 0));
  const pendingReplyStartedAt = resolvePendingReplyStartedAt(conversation);
  if (pendingReplyStartedAt > 0) {
    const elapsedSeconds = Math.max(1, Math.floor((nowMs - pendingReplyStartedAt) / 1000));
    return explicitPendingSeconds > 0 ? Math.max(explicitPendingSeconds, elapsedSeconds) : elapsedSeconds;
  }
  if (explicitPendingSeconds > 0) return explicitPendingSeconds;
  if (explicitReplyState === "pending") return 1;
  if (explicitReplyState === "replied") return 0;
  const lastDirection = resolveConversationLastDirection(conversation);
  if (lastDirection === "incoming") {
    const lastAt = toTimestamp(conversation?.last_message_at || conversation?.last_incoming_at);
    return lastAt > 0 ? Math.max(1, Math.floor((nowMs - lastAt) / 1000)) : 1;
  }
  return 0;
}

export function resolveWorkbenchConversationStatus(conversation = {}, nowMs = Date.now()) {
  const pendingReplySeconds = resolvePendingReplySeconds(conversation, nowMs);
  const replyState = pendingReplySeconds > 0 ? "pending" : "replied";
  const aiReplyStatus = runtimeText(conversation, "ai_reply_status").toLowerCase();
  const aiReplyPhase = runtimeText(conversation, "ai_reply_phase").toLowerCase();
  const aiModeSwitchTarget = runtimeText(conversation, "ai_mode_switch_target").toLowerCase();
  const aiAutoReply = parseBool(conversation?.ai_auto_reply);
  const accountAIEnabled = parseBool(conversation?.account_ai_enabled);
  const aiModeOverride = cleanText(conversation?.ai_mode_override).toLowerCase();
  const sensitiveHandoffPending = runtimeBool(conversation, "sensitive_handoff_pending");
  const forceManual = sensitiveHandoffPending || runtimeBool(conversation, "ai_reply_force_manual") || aiModeOverride === "manual";
  const runtimeAiQueued = aiReplyStatus === "queued";
  const activeAiRuntime = !forceManual && ["queued", "processing", "sending"].includes(aiReplyStatus);
  const isAiModeSwitching = runtimeBool(conversation, "ai_mode_switching");
  const effectiveAIEnabled = aiModeOverride === "auto" || (aiModeOverride !== "manual" && (aiAutoReply || accountAIEnabled));
  const modeState = !forceManual && (effectiveAIEnabled || activeAiRuntime) ? "ai" : "manual";
  return {
    replyState,
    modeState,
    pendingReplySeconds,
    isOverdue: pendingReplySeconds >= REPLY_OVERDUE_SECONDS,
    aiReplyStatus,
    aiReplyPhase,
    aiModeSwitchTarget,
    isAiModeSwitching,
    isAiQueued: !forceManual && runtimeAiQueued,
    isAiProcessing: activeAiRuntime,
    sensitiveHandoffPending,
    sensitiveHandoffReason: runtimeText(conversation, "sensitive_handoff_reason"),
  };
}

export function formatPendingReplyDuration(seconds) {
  const value = Math.max(0, Number(seconds || 0));
  if (!value) return "";
  if (value < 60) return `${value}秒未回复`;
  if (value < 3600) return `${Math.floor(value / 60)}分未回复`;
  if (value < 86400) {
    const hours = Math.floor(value / 3600);
    const minutes = Math.floor((value % 3600) / 60);
    return minutes > 0 ? `${hours}小时${minutes}分未回复` : `${hours}小时未回复`;
  }
  return `${Math.floor(value / 86400)}天未回复`;
}

export function resolveWorkbenchConversationBadges(conversation = {}, nowMs = Date.now()) {
  const status = resolveWorkbenchConversationStatus(conversation, nowMs);
  const replyLabel = status.replyState === "pending"
    ? formatPendingReplyDuration(status.pendingReplySeconds)
    : "已回复";
  const runtimeLabel = status.aiReplyStatus === "sending"
    ? "AI发送中"
    : status.isAiQueued
      ? "等待AI回复"
      : status.isAiProcessing
        ? "AI回复中"
        : "";
  const modeLabel = status.isAiModeSwitching
    ? "切换中"
    : status.sensitiveHandoffPending
      ? "敏感转人工"
      : status.modeState === "ai"
        ? "AI处理"
        : "人工处理";
  return {
    status,
    replyLabel,
    runtimeLabel,
    modeLabel,
    modeTitle: status.sensitiveHandoffPending
      ? status.sensitiveHandoffReason || "客户入站消息命中敏感词"
      : modeLabel,
  };
}

export function resolveConversationAIToggleState(conversation = {}) {
  const aiModeOverride = cleanText(conversation?.ai_mode_override).toLowerCase();
  const aiAutoReply = parseBool(conversation?.ai_auto_reply);
  const accountAIEnabled = parseBool(conversation?.account_ai_enabled);
  if (aiModeOverride === "manual") {
    return { enabled: false, nextEnabled: true };
  }
  if (aiModeOverride === "auto") {
    return { enabled: true, nextEnabled: false };
  }
  const enabled = accountAIEnabled || aiAutoReply;
  return { enabled, nextEnabled: !enabled };
}

export function buildConversationAIModeMutation(conversation = {}, enabled) {
  const conversationId = cleanText(conversation?.conversation_id || conversation?.conversationId);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (conversationId.startsWith("pending:")) return { ok: false, error: "pending_conversation" };
  if (typeof enabled !== "boolean") return { ok: false, error: "enabled_required" };
  return {
    ok: true,
    method: "POST",
    conversationId,
    path: `/conversations/${encodeURIComponent(conversationId)}/ai-auto-reply`,
    body: { enabled },
  };
}

export function resolveWorkbenchAIReplyErrorNotice(conversation = {}, options = {}) {
  const error = runtimeText(conversation, "ai_reply_error");
  const conversationId = cleanText(conversation?.conversation_id);
  const jobId = runtimeText(conversation, "ai_reply_job_id") || runtimeText(conversation, "ai_trace_id");
  const errorKey = error ? `${conversationId}|${jobId || "<no-job>"}|${error}` : "";
  const status = runtimeText(conversation, "ai_reply_status").toLowerCase();
  const phase = runtimeText(conversation, "ai_reply_phase").toLowerCase();
  const forceManual = runtimeBool(conversation, "ai_reply_force_manual");
  const activeRuntime = !forceManual && (
    ["queued", "processing", "sending"].includes(status) ||
    ["queued", "preparing", "message_send_started"].includes(phase) ||
    runtimeBool(conversation, "ai_reply_processing")
  );
  const dismissedKey = cleanText(options.dismissedKey);
  const isDismissed = typeof options.isDismissed === "function"
    ? options.isDismissed(errorKey)
    : isAIReplyErrorDismissed(errorKey);
  return {
    visible: Boolean(error && !activeRuntime && dismissedKey !== errorKey && !isDismissed),
    key: errorKey,
    title: forceManual ? "AI已切换为人工处理" : "AI回复未发送",
    error,
  };
}

export function readDismissedAIReplyErrorKeys(storage = browserLocalStorage()) {
  if (!storage || typeof storage.getItem !== "function") return [];
  try {
    const parsed = JSON.parse(storage.getItem(AI_REPLY_ERROR_DISMISS_STORAGE_KEY) || "[]");
    return Array.isArray(parsed) ? parsed.map((item) => cleanText(item)).filter(Boolean) : [];
  } catch {
    return [];
  }
}

export function isAIReplyErrorDismissed(errorKey, storage = browserLocalStorage()) {
  const key = cleanText(errorKey);
  if (!key) return false;
  return readDismissedAIReplyErrorKeys(storage).includes(key);
}

export function rememberDismissedAIReplyError(errorKey, storage = browserLocalStorage()) {
  const key = cleanText(errorKey);
  if (!key || !storage || typeof storage.setItem !== "function") return false;
  try {
    const nextKeys = [key, ...readDismissedAIReplyErrorKeys(storage).filter((item) => item !== key)]
      .slice(0, AI_REPLY_ERROR_DISMISS_STORAGE_LIMIT);
    storage.setItem(AI_REPLY_ERROR_DISMISS_STORAGE_KEY, JSON.stringify(nextKeys));
    return true;
  } catch {
    return false;
  }
}

function browserLocalStorage() {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage || null;
  } catch {
    return null;
  }
}
