export const DEFAULT_API_BASE = "http://localhost:8080/api/v1";

export function apiURL(path, base = DEFAULT_API_BASE) {
  const normalizedBase = String(base || DEFAULT_API_BASE).replace(/\/+$/, "");
  const normalizedPath = String(path || "").replace(/^\/+/, "");
  return `${normalizedBase}/${normalizedPath}`;
}

export async function requestJSON(path, options = {}) {
  const response = await fetch(apiURL(path, options.base), {
    method: options.method || "GET",
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.detail || `HTTP ${response.status}`);
  }
  return payload;
}

export function normalizeMessages(payload = {}) {
  const messages = Array.isArray(payload.messages) ? payload.messages : [];
  return messages.map((message) => ({
    id: text(message.id),
    conversationId: text(message.conversation_id),
    direction: text(message.direction),
    sourceChannel: text(message.source_channel),
    externalMessageId: text(message.external_message_id),
    senderName: text(message.sender_name || message.sender_id || "unknown"),
    content: text(message.content),
    timestamp: text(message.timestamp),
    receivedAt: text(message.received_at),
  }));
}

export function normalizeSOPCollection(payload = {}, key) {
  return Array.isArray(payload[key]) ? payload[key] : [];
}

function text(value) {
  return String(value || "").trim();
}
