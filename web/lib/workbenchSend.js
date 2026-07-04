const CLIENT_BATCH_WINDOW_MS = 15_000;
const CLIENT_BATCH_MAX_SIZE = 20;
export const MEDIA_MAX_UPLOAD_BYTES = 50 * 1024 * 1024;
export const MESSAGE_REVOKE_WINDOW_MS = 2 * 60 * 1000;
export const AI_SUGGESTION_CONFLICT_MESSAGE = "AI 回复已由其他终端处理";
const RESENDABLE_SEND_STATUSES = new Set(["failed", "timeout", "cancelled"]);
const RESENDABLE_MESSAGE_TYPES = new Set(["text", "image", "video", "file"]);
const RESENDABLE_SIDEBAR_COMMAND_TYPES = new Set(["appointment_billing", "request_money", "send_address"]);
const LOCAL_RETRYABLE_MEDIA_TYPES = new Set(["image", "video", "voice", "file"]);
const PREVIEWABLE_IMAGE_EXTENSIONS = new Set([".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"]);
const EDITABLE_AI_SUGGESTION_STATUSES = new Set(["failed", "timeout", "cancelled"]);
const SIDEBAR_MIXED_MESSAGE_TYPES = new Set(["text", "image", "file", "video"]);

function cleanText(value) {
  return String(value || "").trim();
}

function cleanDisplay(value) {
  const text = cleanText(value);
  if (!text || text.includes("\uFFFD")) return "";
  if (["企微客户", "企微用户", "未知客户", "-", "--", "—", "——"].includes(text)) return "";
  const stripped = text.replace(/[?？\s]/g, "");
  return stripped ? text : "";
}

function stripArchiveUserPrefix(value) {
  const text = cleanText(value);
  if (!text) return "";
  return text.replace(/^archive_user:/i, "");
}

export function resolveSendDeviceId(conversation = {}) {
  return (
    stripArchiveUserPrefix(conversation?.resolved_device_id) ||
    stripArchiveUserPrefix(conversation?.account_device_id) ||
    stripArchiveUserPrefix(conversation?.device_id)
  );
}

export function resolveConversationRemark(conversation = {}) {
  const scoped = conversation?.identity_scoped_profile && typeof conversation.identity_scoped_profile === "object"
    ? conversation.identity_scoped_profile
    : {};
  return (
    cleanDisplay(scoped.remark_name) ||
    cleanDisplay(scoped.display_name) ||
    cleanDisplay(conversation?.sender_remark) ||
    cleanDisplay(conversation?.identity_remark_name) ||
    cleanDisplay(conversation?.customer_name)
  );
}

export function resolveConversationReceiver(conversation = {}) {
  const scoped = conversation?.identity_scoped_profile && typeof conversation.identity_scoped_profile === "object"
    ? conversation.identity_scoped_profile
    : {};
  return (
    resolveConversationRemark(conversation) ||
    cleanDisplay(scoped.nickname) ||
    cleanDisplay(conversation?.identity_nickname) ||
    cleanDisplay(conversation?.sender_name) ||
    cleanDisplay(conversation?.conversation_name) ||
    cleanDisplay(conversation?.send_target_name)
  );
}

export function resolveConversationSenderName(conversation = {}) {
  const scoped = conversation?.identity_scoped_profile && typeof conversation.identity_scoped_profile === "object"
    ? conversation.identity_scoped_profile
    : {};
  return (
    cleanDisplay(scoped.display_name) ||
    cleanDisplay(conversation?.customer_name) ||
    cleanDisplay(conversation?.identity_display_name) ||
    cleanDisplay(conversation?.sender_name) ||
    cleanDisplay(conversation?.send_target_name) ||
    cleanText(conversation?.sender_id)
  );
}

export function buildConversationReplyPayload(conversation = {}, text = "", options = {}) {
  const conversationId = cleanText(conversation?.conversation_id);
  const message = cleanText(text);
  const aiSuggestionID = cleanText(options.aiSuggestionID || options.ai_suggestion_id);
  const deviceId = resolveSendDeviceId(conversation);
  const senderID = cleanText(conversation?.sender_id);
  const senderName = resolveConversationSenderName(conversation);
  const receiver = resolveConversationReceiver(conversation);
  const aliases = resolveConversationRemark(conversation);

  if (!conversationId) {
    return { ok: false, error: "conversation_required" };
  }
  if (!message && !aiSuggestionID) {
    return { ok: false, error: "message_required" };
  }
  if (!deviceId) {
    return { ok: false, error: "device_required" };
  }
  if (!senderID) {
    return { ok: false, error: "sender_required" };
  }

  const body = {
    device_id: deviceId,
    sender_id: senderID,
    sender_name: senderName || receiver || senderID,
    message,
    source: cleanText(options.source) || "cloud-web",
  };
  if (receiver) body.target_username = receiver;
  if (aliases && aliases !== receiver) body.aliases = aliases;
  if (aiSuggestionID) body.ai_suggestion_id = aiSuggestionID;
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  const clientBatch = options.clientBatch && typeof options.clientBatch === "object" ? options.clientBatch : {};
  const clientBatchID = cleanText(clientBatch.client_batch_id || clientBatch.clientBatchId);
  if (clientBatchID) {
    body.client_batch_id = clientBatchID;
    const index = Number(clientBatch.client_batch_index ?? clientBatch.clientBatchIndex);
    const total = Number(clientBatch.client_batch_total ?? clientBatch.clientBatchTotal);
    if (Number.isInteger(index) && index >= 0) body.client_batch_index = index;
    if (Number.isInteger(total) && total >= 1) body.client_batch_total = total;
  }

  return { ok: true, conversationId, body };
}

export function buildConversationCallPayload(conversation = {}, callType = "voice", options = {}) {
  const conversationId = cleanText(conversation?.conversation_id);
  const deviceId = resolveSendDeviceId(conversation);
  const normalizedCallType = normalizeCallType(callType);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (!deviceId) return { ok: false, error: "device_required" };
  if (!normalizedCallType) return { ok: false, error: "call_type_required" };

  const body = {
    device_id: deviceId,
    call_type: normalizedCallType,
    source: cleanText(options.source) || "cloud-web",
  };
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  const reservationID = cleanText(options.reservationID || options.reservation_id);
  if (reservationID) body.reservation_id = reservationID;
  return { ok: true, conversationId, callType: normalizedCallType, body };
}

export function buildConversationHangupPayload(conversation = {}, options = {}) {
  const conversationId = cleanText(conversation?.conversation_id);
  const deviceId = resolveSendDeviceId(conversation);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (!deviceId) return { ok: false, error: "device_required" };

  const body = {
    device_id: deviceId,
    source: cleanText(options.source) || "cloud-web",
  };
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  const reservationID = cleanText(options.reservationID || options.reservation_id);
  if (reservationID) body.reservation_id = reservationID;
  return { ok: true, conversationId, body };
}

export function buildConversationReadMutation(conversation = {}) {
  const conversationId = cleanText(conversation?.conversation_id || conversation?.conversationId);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (conversationId.startsWith("pending:")) return { ok: false, error: "pending_conversation" };
  const unreadCount = Math.max(0, Number(conversation?.unread_count ?? conversation?.unreadCount ?? 0));
  if (!Number.isFinite(unreadCount) || unreadCount <= 0) return { ok: false, error: "already_read" };
  const updatedMarker = cleanText(conversation?.last_message_at || conversation?.updated_at || conversation?.lastMessageAt || conversation?.updatedAt);
  return {
    ok: true,
    method: "POST",
    conversationId,
    path: `/conversations/${encodeURIComponent(conversationId)}/read`,
    dedupeKey: `${conversationId}:${unreadCount}:${updatedMarker}`,
  };
}

export function buildSidebarMixedMessagesMutation(conversation = {}, messages = [], options = {}) {
  const deviceId = resolveSendDeviceId(conversation);
  if (!deviceId) return { ok: false, error: "device_required" };
  const receiver = resolveConversationReceiver(conversation);
  if (!receiver) return { ok: false, error: "receiver_required" };
  const normalizedMessages = normalizeSidebarMixedMessages(messages);
  if (normalizedMessages.length === 0) return { ok: false, error: "messages_required" };
  const conversationId = resolveSidebarConversationID(conversation);
  const organizationName = cleanText(conversation?.organization_name || conversation?.login_organization_name);
  const body = {
    type: "send_mixed_messages",
    receiver,
    organization_name: organizationName,
    conversation_id: conversationId,
    session_id: conversationId,
    sender_id: cleanText(conversation?.sender_id),
    messages: normalizedMessages,
    source: cleanText(options.source) || "cloud-web",
  };
  const aliases = resolveConversationRemark(conversation);
  if (aliases && aliases !== receiver) body.aliases = aliases;
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  return {
    ok: true,
    method: "POST",
    path: `/platform/device/${encodeURIComponent(deviceId)}/sidebar-command`,
    deviceId,
    body,
  };
}

export function normalizeSidebarMixedMessages(messages = []) {
  if (!Array.isArray(messages)) return [];
  return messages.map((item) => {
    if (!item || typeof item !== "object") return null;
    const type = cleanText(item.type).toLowerCase();
    const content = cleanText(item.content || item.message || item.text);
    if (!SIDEBAR_MIXED_MESSAGE_TYPES.has(type) || !content) return null;
    const message = { type, content };
    const filename = cleanText(item.filename || item.file_name || item.name);
    if (filename) message.filename = filename;
    return message;
  }).filter(Boolean);
}

function resolveSidebarConversationID(conversation = {}) {
  return (
    cleanText(conversation?.conversation_key) ||
    cleanText(conversation?.resolved_conversation_id) ||
    cleanText(conversation?.conversation_id)
  );
}

export function nextManualTextClientBatch(previousBatch, conversation = {}, options = {}) {
  const nowMs = Number.isFinite(Number(options.nowMs)) ? Number(options.nowMs) : Date.now();
  const createId = typeof options.createId === "function" ? options.createId : createManualTextClientBatchId;
  const key = buildManualTextClientBatchKey(conversation);
  if (!key) return { state: null, payload: null };

  const previous = previousBatch || {};
  const previousCount = Math.max(0, Number(previous.count || 0));
  const previousClientBatchID = cleanText(previous.clientBatchId);
  const canReuse = cleanText(previous.key) === key &&
    previousClientBatchID &&
    Number(previous.expiresAt || 0) >= nowMs &&
    previousCount < CLIENT_BATCH_MAX_SIZE;
  const clientBatchID = canReuse ? previousClientBatchID : cleanText(createId(nowMs));
  if (!clientBatchID) return { state: null, payload: null };

  const index = canReuse ? previousCount : 0;
  return {
    state: {
      key,
      clientBatchId: clientBatchID,
      count: index + 1,
      expiresAt: canReuse ? Number(previous.expiresAt) : nowMs + CLIENT_BATCH_WINDOW_MS,
    },
    payload: {
      client_batch_id: clientBatchID,
      client_batch_index: index,
    },
  };
}

export function createLocalOutgoingMessage(conversation = {}, text = "", options = {}) {
  const now = options.now instanceof Date ? options.now : new Date();
  const localID = cleanText(options.localID) || createManualTextClientBatchId(now.getTime());
  const message = {
    local_id: localID,
    trace_id: localID,
    conversation_id: cleanText(conversation?.conversation_id),
    direction: "outgoing",
    display_name: "我",
    content: cleanText(text),
    msg_type: "text",
    timestamp: now.toISOString(),
    send_status: "pending",
    send_error: "",
    optimistic: true,
  };
  const aiSuggestionID = cleanText(options.aiSuggestionID || options.ai_suggestion_id);
  if (aiSuggestionID) message.ai_suggestion_id = aiSuggestionID;
  const messageOrigin = cleanText(options.messageOrigin || options.message_origin);
  if (messageOrigin) message.message_origin = messageOrigin;
  const source = cleanText(options.source);
  if (source) message.source = source;
  return message;
}

export function canEditAISuggestionMessage(message = {}) {
  const suggestionID = cleanText(message?.ai_suggestion_id || message?.aiSuggestionID);
  const origin = cleanText(message?.message_origin || message?.messageOrigin).toLowerCase();
  const localID = cleanText(message?.local_id || message?.localID);
  const status = cleanText(message?.send_status || message?.sendStatus).toLowerCase();
  const type = cleanText(message?.msg_type || message?.message_type || message?.type || "text").toLowerCase() || "text";
  if (!suggestionID) return false;
  if (origin !== "ai_suggestion" && !localID.startsWith("ai-suggestion-")) return false;
  if (!EDITABLE_AI_SUGGESTION_STATUSES.has(status)) return false;
  if (type !== "text") return false;
  return Boolean(cleanText(message?.content || message?.message));
}

export function buildAISuggestionEditDraft(message = {}) {
  if (!canEditAISuggestionMessage(message)) return null;
  return {
    conversationId: cleanText(message?.conversation_id || message?.conversationId),
    suggestionId: cleanText(message?.ai_suggestion_id || message?.aiSuggestionID),
    localId: cleanText(message?.local_id || message?.localID || message?.trace_id),
    source: cleanText(message?.source || message?.sub_source || message?.message_source) || "coze-auto-reply",
    text: cleanText(message?.content || message?.message),
  };
}

export function buildConversationMediaSendPayload(conversation = {}, file, options = {}) {
  const conversationId = cleanText(conversation?.conversation_id);
  const deviceId = resolveSendDeviceId(conversation);
  const senderID = cleanText(conversation?.sender_id);
  const receiver = resolveConversationReceiver(conversation);
  const aliases = resolveConversationRemark(conversation);
  const username = resolveConversationSenderName(conversation) || receiver || senderID;
  const fileName = cleanText(file?.name);
  const fileSize = Number(file?.size || 0);
  const kind = normalizeMediaKind(cleanText(options.kind) || inferMediaKind(file));

  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (!file) return { ok: false, error: "file_required" };
  if (!deviceId) return { ok: false, error: "device_required" };
  if (!senderID) return { ok: false, error: "sender_required" };
  if (!username) return { ok: false, error: "sender_required" };
  if (!kind) return { ok: false, error: "media_kind_required" };
  if (fileSize > MEDIA_MAX_UPLOAD_BYTES) return { ok: false, error: "file_too_large" };
  if (typeof FormData === "undefined") return { ok: false, error: "formdata_unavailable" };

  const formData = new FormData();
  formData.append("file", file);
  formData.append("device_id", deviceId);
  formData.append("username", username);
  formData.append("sender_id", senderID);
  formData.append("conversation_id", conversationId);
  formData.append("source", cleanText(options.source) || "cloud-web");
  if (receiver) formData.append("target_username", receiver);
  if (aliases && aliases !== receiver) formData.append("aliases", aliases);
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) formData.append("agent_id", agentID);
  const organizationName = cleanText(conversation?.organization_name || conversation?.login_organization_name);
  if (organizationName) formData.append("organization_name", organizationName);
  const voiceDuration = Number(options.voiceDurationSec ?? file?.voiceDurationSec);
  if (kind === "voice" && Number.isFinite(voiceDuration) && voiceDuration > 0) {
    formData.append("voice_duration_sec", String(Math.round(voiceDuration)));
  }

  return { ok: true, conversationId, kind, endpoint: `/send/${kind}`, fileName, formData };
}

export function createLocalMediaOutgoingMessage(conversation = {}, file, kind = "file", options = {}) {
  const now = options.now instanceof Date ? options.now : new Date();
  const normalizedKind = normalizeMediaKind(kind) || "file";
  const localID = cleanText(options.localID) || createManualTextClientBatchId(now.getTime());
  const fileName = cleanText(file?.name) || mediaKindLabel(normalizedKind);
  const previewURL = cleanText(options.previewURL);
  const message = {
    local_id: localID,
    trace_id: localID,
    conversation_id: cleanText(conversation?.conversation_id),
    direction: "outgoing",
    display_name: "我",
    content: normalizedKind === "voice" ? "[语音消息]" : fileName,
    msg_type: normalizedKind,
    media_url: previewURL,
    media_ready: Boolean(previewURL),
    media_filename: fileName,
    media_size_bytes: Number(file?.size || 0),
    timestamp: now.toISOString(),
    send_status: "pending",
    send_error: "",
    optimistic: true,
    local_media_file: file || null,
    local_media_kind: normalizedKind,
  };
  const voiceDuration = Number(options.voiceDurationSec ?? file?.voiceDurationSec);
  if (normalizedKind === "voice" && Number.isFinite(voiceDuration) && voiceDuration > 0) {
    message.voice_duration_sec = Math.round(voiceDuration);
  }
  return message;
}

export function reconcileLocalOutgoingMessage(localMessage, response = {}) {
  const task = response?.task && typeof response.task === "object" ? response.task : {};
  const message = response?.message && typeof response.message === "object" ? response.message : {};
  return {
    ...localMessage,
    trace_id: cleanText(message.trace_id || task.trace_id || localMessage.trace_id),
    task_id: cleanText(message.task_id || task.task_id || localMessage.task_id),
    send_status: cleanText(message.send_status || task.status || localMessage.send_status || "pending"),
    send_error: cleanText(message.send_error || task.error || ""),
  };
}

export function isAISuggestionConflictError(error) {
  const text = cleanText(error?.message || error);
  return text.includes(AI_SUGGESTION_CONFLICT_MESSAGE);
}

function inferMediaKind(file) {
  const mime = cleanText(file?.type).toLowerCase();
  if (mime.startsWith("video/")) return "video";
  if (mime.startsWith("audio/")) return "voice";
  if (mime.startsWith("image/") && PREVIEWABLE_IMAGE_EXTENSIONS.has(fileExtension(file?.name))) return "image";
  if (mime.startsWith("image/") && !fileExtension(file?.name)) return "image";
  return "file";
}

function normalizeMediaKind(value) {
  const kind = cleanText(value).toLowerCase();
  if (["image", "video", "voice", "file"].includes(kind)) return kind;
  return "";
}

function fileExtension(value) {
  const name = cleanText(value).toLowerCase();
  const index = name.lastIndexOf(".");
  return index >= 0 ? name.slice(index) : "";
}

function mediaKindLabel(kind) {
  switch (kind) {
    case "image":
      return "图片";
    case "video":
      return "视频";
    case "voice":
      return "语音";
    default:
      return "文件";
  }
}

function normalizeCallType(value) {
  const callType = cleanText(value).toLowerCase();
  if (callType === "voice" || callType === "video") return callType;
  return "";
}

export function mergeLocalOutgoingMessages(messages = [], localMessages = [], conversationId = "") {
  const targetConversationID = cleanText(conversationId);
  const serverKeys = new Set();
  for (const message of Array.isArray(messages) ? messages : []) {
    const traceID = cleanText(message?.trace_id);
    const taskID = cleanText(message?.task_id);
    if (traceID) serverKeys.add(`trace:${traceID}`);
    if (taskID) serverKeys.add(`task:${taskID}`);
  }
  const pending = (Array.isArray(localMessages) ? localMessages : []).filter((message) => {
    if (targetConversationID && cleanText(message?.conversation_id) !== targetConversationID) return false;
    const traceID = cleanText(message?.trace_id);
    const taskID = cleanText(message?.task_id);
    return !(traceID && serverKeys.has(`trace:${traceID}`)) && !(taskID && serverKeys.has(`task:${taskID}`));
  });
  return [...(Array.isArray(messages) ? messages : []), ...pending];
}

export function canResendConversationMessage(message = {}) {
  const traceID = cleanText(message?.trace_id);
  if (!traceID || traceID.toLowerCase().startsWith("local-")) return false;
  if (message?.optimistic) return false;
  if (String(message?.direction || "").trim().toLowerCase() !== "outgoing") return false;
  const resendStatus = String(message?.resend_status || "").trim().toLowerCase();
  if (["pending", "queued", "running", "success"].includes(resendStatus)) return false;
  const sendStatus = String(message?.send_status || "").trim().toLowerCase();
  if (!RESENDABLE_SEND_STATUSES.has(sendStatus)) return false;
  if (isResendableSidebarCommandMessage(message)) return true;
  const msgType = String(message?.msg_type || "text").trim().toLowerCase() || "text";
  if (!RESENDABLE_MESSAGE_TYPES.has(msgType)) return false;
  const resendContent = msgType === "text" ? cleanText(message?.content) : cleanText(message?.media_url || message?.content);
  if (!resendContent) return false;
  return true;
}

function isResendableSidebarCommandMessage(message = {}) {
  const commandType = cleanText(message?.command_type || message?.commandType || message?.sidebar_command_payload?.type).toLowerCase();
  if (!RESENDABLE_SIDEBAR_COMMAND_TYPES.has(commandType)) return false;
  const taskSource = cleanText(message?.task_source || message?.source || message?.sub_source || message?.message_source).toLowerCase();
  return taskSource === "sidebar" || (message?.sidebar_command_payload && typeof message.sidebar_command_payload === "object");
}

export function canRetryLocalMediaMessage(message = {}) {
  if (!message?.optimistic) return false;
  if (!message?.local_media_file) return false;
  if (String(message?.direction || "").trim().toLowerCase() !== "outgoing") return false;
  if (String(message?.send_status || "").trim().toLowerCase() !== "failed") return false;
  const msgType = String(message?.local_media_kind || message?.msg_type || "").trim().toLowerCase();
  return LOCAL_RETRYABLE_MEDIA_TYPES.has(msgType);
}

export function createLocalMediaRetryMessage(message = {}, options = {}) {
  const now = options.now instanceof Date ? options.now : new Date();
  return {
    ...message,
    send_status: "pending",
    send_error: "",
    retry_count: Math.max(0, Number(message?.retry_count || 0)) + 1,
    retry_started_at: now.toISOString(),
  };
}

export function buildConversationResendPayload(conversation = {}, message = {}, options = {}) {
  const conversationId = cleanText(message?.conversation_id || conversation?.conversation_id);
  const traceId = cleanText(message?.trace_id);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (!traceId) return { ok: false, error: "trace_required" };
  if (!canResendConversationMessage(message)) return { ok: false, error: "message_not_resendable" };

  const body = {
    source: cleanText(options.source) || "cloud-web",
  };
  const deviceId = resolveSendDeviceId(conversation) || stripArchiveUserPrefix(message?.device_id);
  if (deviceId) body.device_id = deviceId;
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  return { ok: true, conversationId, traceId, body };
}

export function createResendOutgoingMessage(response = {}, options = {}) {
  const message = response?.message && typeof response.message === "object" ? response.message : {};
  const task = response?.task && typeof response.task === "object" ? response.task : {};
  const traceID = cleanText(message.trace_id || task.trace_id);
  if (!traceID) return null;
  return {
    ...message,
    local_id: cleanText(options.localID) || `resend-${traceID}`,
    trace_id: traceID,
    task_id: cleanText(message.task_id || task.task_id),
    direction: cleanText(message.direction) || "outgoing",
    msg_type: cleanText(message.msg_type) || "text",
    send_status: cleanText(message.send_status || task.status || "pending"),
    send_error: cleanText(message.send_error || task.error || ""),
    optimistic: true,
  };
}

export function canRevokeConversationMessage(message = {}, nowMs = Date.now(), windowMs = MESSAGE_REVOKE_WINDOW_MS) {
  const traceID = cleanText(message?.trace_id);
  if (!traceID || traceID.toLowerCase().startsWith("local-")) return false;
  if (message?.optimistic) return false;
  if (String(message?.direction || "").trim().toLowerCase() !== "outgoing") return false;
  if (String(message?.msg_type || "text").trim().toLowerCase() !== "text") return false;
  if (!cleanText(message?.content)) return false;
  const timestampMs = messageTimestampMs(message);
  if (timestampMs <= 0) return false;
  const ageMs = Number(nowMs || 0) - timestampMs;
  if (ageMs < -5000 || ageMs > Number(windowMs || MESSAGE_REVOKE_WINDOW_MS)) return false;
  const sendStatus = String(message?.send_status || "").trim().toLowerCase();
  if (["pending", "queued", "running", "sending", "failed", "timeout", "cancelled"].includes(sendStatus)) return false;
  const revokeStatus = String(message?.revoke_status || "").trim().toLowerCase();
  if (["pending", "queued", "running", "success"].includes(revokeStatus)) return false;
  return true;
}

export function resolveRevokeOccurrenceFromBottom(messages = [], targetMessage = {}) {
  const targetTraceID = cleanText(targetMessage?.trace_id);
  const targetContent = cleanText(targetMessage?.content);
  if (!targetTraceID || !targetContent) return 1;
  let occurrence = 0;
  const rows = Array.isArray(messages) ? messages : [];
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const item = rows[index] || {};
    if (String(item?.direction || "").trim().toLowerCase() !== "outgoing") continue;
    if (String(item?.msg_type || "text").trim().toLowerCase() !== "text") continue;
    if (cleanText(item?.content) !== targetContent) continue;
    occurrence += 1;
    if (cleanText(item?.trace_id) === targetTraceID) return occurrence;
  }
  return 1;
}

export function buildConversationRevokePayload(conversation = {}, message = {}, options = {}) {
  const conversationId = cleanText(message?.conversation_id || conversation?.conversation_id);
  const traceId = cleanText(message?.trace_id);
  if (!conversationId) return { ok: false, error: "conversation_required" };
  if (!traceId) return { ok: false, error: "trace_required" };
  if (!canRevokeConversationMessage(message, options.nowMs, options.windowMs)) {
    return { ok: false, error: "message_not_revocable" };
  }
  const deviceId = resolveSendDeviceId(conversation) || stripArchiveUserPrefix(message?.device_id);
  if (!deviceId) return { ok: false, error: "device_required" };
  const occurrenceFromBottom = Number(options.occurrenceFromBottom || 1);
  const body = {
    device_id: deviceId,
    source: cleanText(options.source) || "cloud-web",
    target_content: cleanText(message?.content),
    target_msg_type: "text",
    occurrence_from_bottom: Number.isInteger(occurrenceFromBottom) && occurrenceFromBottom > 0 ? Math.min(20, occurrenceFromBottom) : 1,
  };
  const agentID = cleanText(options.agentID || conversation?.agent_id || conversation?.account_agent_id);
  if (agentID) body.agent_id = agentID;
  return { ok: true, conversationId, traceId, body };
}

function buildManualTextClientBatchKey(conversation = {}) {
  const conversationId = cleanText(conversation?.conversation_id);
  const deviceId = resolveSendDeviceId(conversation);
  const senderID = cleanText(conversation?.sender_id);
  const receiver = resolveConversationReceiver(conversation);
  if (!conversationId || !deviceId || !senderID || !receiver) return "";
  return [conversationId, deviceId, senderID, receiver].join("|");
}

function createManualTextClientBatchId(nowMs = Date.now()) {
  const randomPart = globalThis.crypto?.randomUUID?.() || Math.random().toString(36).slice(2, 12) || String(nowMs);
  return `manual-text-${nowMs}-${randomPart}`;
}

function messageTimestampMs(message = {}) {
  const raw = cleanText(message?.timestamp || message?.created_at);
  if (!raw) return 0;
  const parsed = new Date(raw).getTime();
  return Number.isFinite(parsed) ? parsed : 0;
}
