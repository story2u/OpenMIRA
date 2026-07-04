export const workbenchConversationRealtimeTopics = [
  "conversation.assignment",
  "conversation.ai_suggested",
  "conversation.media_ready",
  "conversation.message",
  "conversation.voice_transcription_ready",
  "customer.relation",
  "friend.added",
];

export const workbenchTaskRealtimeTopics = ["task.status"];

const conversationRefreshSignals = new Set([
  "conversation.archive_ingested",
  "conversation.ai_suggested",
  "conversation.assignment",
  "conversation.custom_replied",
  "conversation.media_ready",
  "conversation.message",
  "conversation.message.revoke",
  "conversation.replied",
  "conversation.transferred",
  "conversation.voice_transcription_ready",
  "conversation_assigned",
  "conversation_unread_changed",
  "conversation_updated",
  "customer.relation",
  "customer.relation.changed",
  "friend.added",
  "identity.updated",
  "identity_updated",
  "message_created",
]);

const messageRefreshSignals = new Set([
  "conversation.archive_ingested",
  "conversation.custom_replied",
  "conversation.media_ready",
  "conversation.message",
  "conversation.message.revoke",
  "conversation.replied",
  "conversation.voice_transcription_ready",
  "message_created",
  "task.status",
]);

const taskRefreshSignals = new Set(["task.status"]);

const directConversationIDKeys = [
  "conversation_id",
  "conversation_key",
  "resolved_conversation_id",
];

const listConversationIDKeys = [
  "conversation_ids",
  "conversation_keys",
  "resolved_conversation_ids",
];

const nestedPayloadKeys = [
  "assignment",
  "conversation",
  "message",
  "payload",
];

export function resolveWorkbenchRealtimeIntent(envelope = {}, state = {}) {
  const event = normalizedText(envelope?.event);
  const topic = normalizedText(envelope?.topic);
  const channel = normalizedText(envelope?.channel);
  const payload = objectValue(envelope?.payload);
  const selectedConversationId = normalizedText(state?.selectedConversationId);
  const conversationIds = extractRealtimeConversationIds(payload);
  const selectedConversationMatched = selectedConversationId && conversationIds.includes(selectedConversationId);
  const signals = [topic, event].filter(Boolean);
  const isConversationSignal = channel === "conversations" || signals.some((signal) => conversationRefreshSignals.has(signal));
  const isTaskSignal = channel === "tasks" || signals.some((signal) => taskRefreshSignals.has(signal));
  const refreshMessages = Boolean(
    selectedConversationMatched && signals.some((signal) => messageRefreshSignals.has(signal)),
  );
  const refreshConversations = Boolean(
    isConversationSignal || (isTaskSignal && conversationIds.length > 0),
  );
  const recognized = refreshConversations || refreshMessages || isTaskSignal;
  return {
    recognized,
    refreshConversations,
    refreshMessages,
    conversationIds,
    selectedConversationMatched: Boolean(selectedConversationMatched),
    channel,
    event,
    topic,
    reason: resolveReason({ event, topic, channel }),
  };
}

export function resolveWorkbenchAISuggestion(envelope = {}) {
  const event = normalizedText(envelope?.event);
  const topic = normalizedText(envelope?.topic);
  if (event !== "conversation.ai_suggested" && topic !== "conversation.ai_suggested") return null;

  const payload = objectValue(envelope?.payload);
  const nestedConversation = objectValue(payload.conversation);
  const conversationId = normalizedText(
    payload.conversation_id ||
    payload.resolved_conversation_id ||
    nestedConversation.conversation_id ||
    nestedConversation.resolved_conversation_id,
  );
  const suggestionId = normalizedText(payload.suggestion_id || payload.suggestionId);
  const message = normalizedText(payload.message || payload.text || payload.content);
  if (!conversationId || !suggestionId || !message) return null;

  const { conversation: _nested, ...payloadConversation } = payload;
  return {
    conversationId,
    suggestionId,
    message,
    source: normalizedText(payload.source) || "coze-auto-reply",
    conversation: {
      ...payloadConversation,
      ...nestedConversation,
      conversation_id: conversationId,
    },
  };
}

export function buildWorkbenchConversationLookupRequest(conversationId, options = {}) {
  const normalizedConversationId = normalizedText(conversationId);
  if (!normalizedConversationId) return { ok: false, error: "conversation_required" };
  return {
    ok: true,
    path: "/cs/workbench/conversations",
    params: {
      conversation_id: normalizedConversationId,
      conversation_limit: 1,
      selected_account_id: normalizedText(options.selectedAccountID || options.selected_account_id || "all") || "all",
      mode_filter: "all",
      status_filter: "all",
    },
  };
}

export function resolveWorkbenchConversationLookupResult(payload = {}, conversationId = "") {
  const normalizedConversationId = normalizedText(conversationId);
  if (!normalizedConversationId) return null;
  const rows = Array.isArray(payload?.conversations) ? payload.conversations : [];
  return rows.find((row) => {
    const rowConversationId = normalizedText(row?.conversation_id);
    const rowConversationKey = normalizedText(row?.conversation_key);
    const rowResolvedId = normalizedText(row?.resolved_conversation_id);
    return rowConversationId === normalizedConversationId || rowConversationKey === normalizedConversationId || rowResolvedId === normalizedConversationId;
  }) || null;
}

export function extractRealtimeConversationIds(payload = {}) {
  const ids = new Set();
  collectConversationIds(payload, ids, 0);
  return Array.from(ids).sort();
}

function collectConversationIds(value, ids, depth) {
  if (!value || typeof value !== "object" || depth > 2) return;
  directConversationIDKeys.forEach((key) => addConversationID(ids, value[key]));
  listConversationIDKeys.forEach((key) => {
    const list = value[key];
    if (Array.isArray(list)) {
      list.forEach((item) => addConversationID(ids, item));
    }
  });
  nestedPayloadKeys.forEach((key) => {
    collectConversationIds(value[key], ids, depth + 1);
  });
}

function addConversationID(ids, value) {
  const normalized = normalizedText(value);
  if (normalized) ids.add(normalized);
}

function objectValue(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return value;
}

function normalizedText(value) {
  return String(value || "").trim();
}

function resolveReason({ event, topic, channel }) {
  if (taskRefreshSignals.has(topic) || taskRefreshSignals.has(event) || channel === "tasks") {
    return "task_status";
  }
  if (topic === "conversation.assignment" || event === "conversation.transferred" || event === "conversation_assigned") {
    return "conversation_assignment";
  }
  if (topic === "conversation.media_ready" || event === "conversation.media_ready") {
    return "conversation_media_ready";
  }
  if (topic === "conversation.voice_transcription_ready" || event === "conversation.voice_transcription_ready") {
    return "conversation_voice_ready";
  }
  if (
    topic === "conversation.message" ||
    event === "conversation.message" ||
    event === "conversation.message.revoke" ||
    event === "message_created"
  ) {
    return "conversation_message";
  }
  if (event || topic) return event || topic;
  return "";
}
