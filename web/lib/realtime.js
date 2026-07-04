import { buildApiPath, requestJSON } from "./api.js";

const cursorStorageKey = "wework.realtime.cursors";
const maxReplayGap = 100;
const pools = new Map();
const scopeCursors = new Map();

export function normalizeTopics(topics = []) {
  return Array.from(new Set((topics || []).map((item) => String(item || "").trim()).filter(Boolean))).sort();
}

export function resolveRealtimeBaseUrl(explicitBaseUrl = "") {
  const explicit = String(explicitBaseUrl || "").trim();
  if (explicit) return explicit.replace(/\/+$/, "");
  const envBase = String(process.env.NEXT_PUBLIC_REALTIME_BASE_URL || "").trim();
  if (envBase) return envBase.replace(/\/+$/, "");
  if (typeof window !== "undefined" && window.location?.origin) {
    return String(window.location.origin).replace(/\/+$/, "");
  }
  return "";
}

export function createWebSocketUrl(channel, topics = [], { token = "", baseUrl = "" } = {}) {
  const normalizedChannel = encodeURIComponent(String(channel || "").trim());
  const base = resolveRealtimeBaseUrl(baseUrl);
  const wsBase = base ? base.replace(/^http/i, "ws") : "";
  const params = new URLSearchParams();
  const normalizedTopics = normalizeTopics(topics);
  if (normalizedTopics.length > 0) {
    params.set("topics", normalizedTopics.join(","));
  }
  const normalizedToken = String(token || "").trim();
  if (normalizedToken) {
    params.set("token", normalizedToken);
  }
  const query = params.toString();
  return `${wsBase}/ws/${normalizedChannel}${query ? `?${query}` : ""}`;
}

export async function fetchRealtimeEventsReplay({ scope, afterCursor = 0, limit = 100, token = "", signal } = {}) {
  return requestJSON("/realtime/events/replay", {
    params: {
      scope: String(scope || "").trim(),
      after_cursor: Number(afterCursor || 0),
      limit: Number(limit || 100),
    },
    token,
    signal,
  });
}

export async function fetchRealtimeWorkbenchSnapshot({ token = "", signal } = {}) {
  return requestJSON("/realtime/snapshot/workbench", { token, signal });
}

export function getLastSeenCursor(scopeKey) {
  return scopeCursors.get(String(scopeKey || "").trim()) || 0;
}

export function getRealtimeCursorSnapshot() {
  const snapshot = {};
  scopeCursors.forEach((cursor, scopeKey) => {
    snapshot[scopeKey] = cursor;
  });
  return snapshot;
}

export function acknowledgeRealtimeCursor(scopeKey, cursor) {
  const normalizedScope = String(scopeKey || "").trim();
  const nextCursor = Number(cursor || 0);
  if (!normalizedScope || !Number.isFinite(nextCursor) || nextCursor <= 0) {
    return false;
  }
  const current = scopeCursors.get(normalizedScope) || 0;
  if (nextCursor <= current) {
    return false;
  }
  scopeCursors.set(normalizedScope, nextCursor);
  persistCursors();
  return true;
}

export function mergeRealtimeCursorSnapshot(cursors = {}) {
  if (!cursors || typeof cursors !== "object") return;
  Object.entries(cursors).forEach(([scopeKey, cursor]) => {
    acknowledgeRealtimeCursor(scopeKey, cursor);
  });
}

export function detectRealtimeGap(envelope = {}) {
  const scopeKey = String(envelope?.scope_key || "").trim();
  const cursor = Number(envelope?.cursor || 0);
  if (!scopeKey || !Number.isFinite(cursor) || cursor <= 0) {
    return null;
  }
  const lastSeen = getLastSeenCursor(scopeKey);
  acknowledgeRealtimeCursor(scopeKey, cursor);
  if (lastSeen <= 0 || cursor <= lastSeen + 1) {
    return null;
  }
  const gapSize = cursor - lastSeen - 1;
  return {
    scopeKey,
    expectedCursor: lastSeen + 1,
    receivedCursor: cursor,
    gapSize,
    needsResync: gapSize > maxReplayGap,
  };
}

export function subscribeRealtimeChannel(channel, topics = [], onEvent, options = {}) {
  const WebSocketImpl = options.WebSocketImpl || globalThis.WebSocket;
  if (!WebSocketImpl || typeof onEvent !== "function") {
    return () => {};
  }
  loadStoredCursors();
  const normalizedTopics = normalizeTopics(topics);
  const key = `${String(channel || "").trim()}|${normalizedTopics.join(",")}`;
  let pool = pools.get(key);
  if (!pool) {
    pool = createPool({ channel, topics: normalizedTopics, options, WebSocketImpl });
    pools.set(key, pool);
  }
  const listener = {
    onEvent,
    onGap: options.onGap,
    onOpen: options.onOpen,
    onReplay: options.onReplay,
    onReplayFailed: options.onReplayFailed,
    onReconnectScheduled: options.onReconnectScheduled,
    onResync: options.onResync,
  };
  pool.listeners.add(listener);
  connectPool(pool);
  return () => {
    pool.listeners.delete(listener);
    if (pool.listeners.size === 0) {
      closePool(pool);
      pools.delete(key);
    }
  };
}

export function resetRealtimeStateForTest() {
  scopeCursors.clear();
  pools.forEach(closePool);
  pools.clear();
}

function createPool({ channel, topics, options, WebSocketImpl }) {
  return {
    channel,
    topics,
    options,
    WebSocketImpl,
    listeners: new Set(),
    socket: null,
    connecting: false,
    delivery: Promise.resolve(),
    reconnectAttempts: 0,
    reconnectTimer: null,
    closed: false,
  };
}

function connectPool(pool) {
  if (pool.socket || pool.closed || pool.connecting || pool.reconnectTimer) return;
  if (typeof pool.options.ensureTokenFresh === "function") {
    connectPoolAfterTokenRefresh(pool);
    return;
  }
  openSocket(pool, resolveToken(pool.options));
}

async function connectPoolAfterTokenRefresh(pool) {
  pool.connecting = true;
  let token = resolveToken(pool.options);
  try {
    const result = await pool.options.ensureTokenFresh({ minTtlMs: Number(pool.options.minTokenTtlMs || 120000) });
    if (typeof result === "string") {
      token = result;
    } else if (!result) {
      pool.options.onAuthRefreshFailed?.();
      return;
    } else {
      token = resolveToken(pool.options);
    }
  } catch {
    pool.options.onAuthRefreshFailed?.();
    return;
  } finally {
    pool.connecting = false;
  }
  if (pool.closed || pool.socket || pool.listeners.size === 0) return;
  openSocket(pool, token);
}

function openSocket(pool, token) {
  if (pool.socket || pool.closed) return;
  const socket = new pool.WebSocketImpl(createWebSocketUrl(pool.channel, pool.topics, { ...pool.options, token }));
  pool.socket = socket;
  socket.onopen = () => {
    pool.reconnectAttempts = 0;
    notifyOpen(pool);
  };
  socket.onmessage = (message) => {
    const envelope = parseEnvelope(message?.data);
    if (!envelope) return;
    enqueuePoolEnvelope(pool, envelope);
  };
  socket.onclose = () => {
    pool.socket = null;
    scheduleReconnect(pool);
  };
}

function scheduleReconnect(pool) {
  if (pool.closed || pool.reconnectTimer || pool.listeners.size === 0 || pool.options.reconnect === false) return;
  const delayMs = nextReconnectDelay(pool);
  const setTimer = pool.options.setTimeout || globalThis.setTimeout;
  if (typeof setTimer !== "function") return;
  notifyReconnectScheduled(pool, delayMs);
  pool.reconnectTimer = setTimer(() => {
    pool.reconnectTimer = null;
    connectPool(pool);
  }, delayMs);
}

function clearReconnect(pool) {
  if (!pool.reconnectTimer) return;
  const clearTimer = pool.options.clearTimeout || globalThis.clearTimeout;
  if (typeof clearTimer === "function") {
    clearTimer(pool.reconnectTimer);
  }
  pool.reconnectTimer = null;
}

function nextReconnectDelay(pool) {
  const baseDelayMs = positiveNumber(pool.options.reconnectBaseDelayMs, 1000);
  const maxDelayMs = positiveNumber(pool.options.reconnectMaxDelayMs, 30000);
  const delay = Math.min(maxDelayMs, baseDelayMs * 2 ** pool.reconnectAttempts);
  pool.reconnectAttempts += 1;
  return delay;
}

function enqueuePoolEnvelope(pool, envelope) {
  pool.delivery = pool.delivery
    .then(() => dispatchPoolEnvelope(pool, envelope))
    .catch((error) => {
      pool.options.onError?.(error);
    });
}

async function dispatchPoolEnvelope(pool, envelope) {
  if (pool.closed) return;
  const gap = detectRealtimeGap(envelope);
  if (gap) {
    notifyGap(pool, gap, envelope);
    if (shouldRecoverGap(pool, gap)) {
      await recoverRealtimeGap(pool, gap);
    }
  }
  if (pool.closed) return;
  notifyEvent(pool, envelope, { gap });
}

async function recoverRealtimeGap(pool, gap) {
  if (gap.needsResync) {
    await refreshRealtimeSnapshot(pool, gap, "gap_too_large");
    return;
  }
  try {
    const fetchReplay = pool.options.fetchReplay || fetchRealtimeEventsReplay;
    const response = await fetchReplay({
      scope: gap.scopeKey,
      afterCursor: gap.expectedCursor - 1,
      limit: gap.gapSize,
      token: resolveToken(pool.options),
    });
    const replayed = normalizeReplayEvents(response?.events, gap);
    replayed.forEach((event) => notifyEvent(pool, event, { gap, replayed: true }));
    notifyReplay(pool, gap, replayed, response);
    if (replayed.length < gap.gapSize || response?.has_more) {
      await refreshRealtimeSnapshot(pool, gap, "replay_incomplete");
    }
  } catch (error) {
    notifyReplayFailed(pool, gap, error);
    await refreshRealtimeSnapshot(pool, gap, "replay_failed");
  }
}

async function refreshRealtimeSnapshot(pool, gap, reason) {
  try {
    const fetchSnapshot = pool.options.fetchSnapshot || fetchRealtimeWorkbenchSnapshot;
    const snapshot = await fetchSnapshot({ token: resolveToken(pool.options) });
    mergeRealtimeCursorSnapshot(snapshot?.cursors);
    notifyResync(pool, gap, reason, snapshot);
  } catch (error) {
    pool.options.onError?.(error);
  }
}

function shouldRecoverGap(pool, gap) {
  if (!gap || pool.options.replayOnGap === false) return false;
  const hasToken = Boolean(resolveToken(pool.options));
  if (gap.needsResync) {
    return hasToken || typeof pool.options.fetchSnapshot === "function";
  }
  return hasToken || typeof pool.options.fetchReplay === "function";
}

function normalizeReplayEvents(events = [], gap = {}) {
  if (!Array.isArray(events)) return [];
  return events
    .filter((event) => {
      const scopeKey = String(event?.scope_key || "").trim();
      const cursor = Number(event?.cursor || 0);
      return scopeKey === gap.scopeKey && cursor >= gap.expectedCursor && cursor < gap.receivedCursor;
    })
    .sort((left, right) => Number(left?.cursor || 0) - Number(right?.cursor || 0));
}

function notifyEvent(pool, envelope, meta) {
  pool.listeners.forEach((listener) => {
    listener.onEvent(envelope, meta);
  });
}

function notifyOpen(pool) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onOpen === "function") {
      listener.onOpen();
    }
  });
}

function notifyGap(pool, gap, envelope) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onGap === "function") {
      listener.onGap(gap, envelope);
    }
  });
}

function notifyReplay(pool, gap, events, response) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onReplay === "function") {
      listener.onReplay({ gap, events, response });
    }
  });
}

function notifyReplayFailed(pool, gap, error) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onReplayFailed === "function") {
      listener.onReplayFailed(gap, error);
    }
  });
}

function notifyReconnectScheduled(pool, delayMs) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onReconnectScheduled === "function") {
      listener.onReconnectScheduled({ delayMs });
    }
  });
}

function notifyResync(pool, gap, reason, snapshot) {
  pool.listeners.forEach((listener) => {
    if (typeof listener.onResync === "function") {
      listener.onResync({ gap, reason, snapshot });
    }
  });
}

function resolveToken(options = {}) {
  if (typeof options.getToken === "function") {
    return String(options.getToken() || "").trim();
  }
  return String(options.token || "").trim();
}

function closePool(pool) {
  pool.closed = true;
  clearReconnect(pool);
  if (pool.socket && typeof pool.socket.close === "function") {
    pool.socket.close();
  }
  pool.socket = null;
}

function parseEnvelope(raw) {
  if (!raw) return null;
  if (typeof raw === "object") return raw;
  try {
    return JSON.parse(String(raw));
  } catch {
    return null;
  }
}

function positiveNumber(value, fallback) {
  const number = Number(value);
  if (Number.isFinite(number) && number > 0) return number;
  return fallback;
}

function loadStoredCursors() {
  if (typeof window === "undefined") return;
  try {
    const raw = window.sessionStorage?.getItem(cursorStorageKey);
    if (!raw) return;
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return;
    Object.entries(parsed).forEach(([scopeKey, cursor]) => {
      const value = Number(cursor || 0);
      if (String(scopeKey || "").trim() && value > 0) {
        scopeCursors.set(scopeKey, value);
      }
    });
  } catch {
    // ignore storage failures
  }
}

function persistCursors() {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage?.setItem(cursorStorageKey, JSON.stringify(getRealtimeCursorSnapshot()));
  } catch {
    // ignore storage failures
  }
}

export { buildApiPath };
