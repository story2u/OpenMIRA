const CLIENT_LOG_ENDPOINT = "/api/v1/client-logs";
const FLUSH_INTERVAL_MS = 10_000;
const MAX_BATCH_SIZE = 20;
const MAX_QUEUE_SIZE = 200;
const RECENT_DEDUP_MS = 5_000;
const STALE_API_LOG_MAX_AGE_MS = 60 * 1000;

const nonFatalRuntimePatterns = [
  /requestedscopekey is not defined/i,
  /failed to fetch dynamically imported module/i,
  /chunkloaderror/i,
];

export function shouldDemoteClientRuntimeLog(detail) {
  const normalizedDetail = String(detail || "").trim();
  return normalizedDetail !== "" && nonFatalRuntimePatterns.some((pattern) => pattern.test(normalizedDetail));
}

export function trimLogText(value, maxLength = 800) {
  const text = String(value || "").trim();
  if (!text) return "";
  if (text.length <= maxLength) return text;
  return `${text.slice(0, Math.max(0, maxLength - 15))}...(truncated)`;
}

export function sanitizeLogExtra(value, depth = 0) {
  if (value === null || value === undefined) return undefined;
  if (typeof value === "string") return trimLogText(value, 1000);
  if (typeof value === "number" || typeof value === "boolean") return value;
  if (depth >= 3) return trimLogText(safeJSONStringify(value), 1000);
  if (Array.isArray(value)) return value.slice(0, 20).map((item) => sanitizeLogExtra(item, depth + 1));
  if (typeof value === "object") {
    return Object.entries(value).reduce((acc, [key, item]) => {
      const normalizedKey = String(key || "").trim();
      if (!normalizedKey) return acc;
      if (/(password|token|secret|authorization|cookie|content)/i.test(normalizedKey)) {
        acc[normalizedKey] = "***";
        return acc;
      }
      acc[normalizedKey] = sanitizeLogExtra(item, depth + 1);
      return acc;
    }, {});
  }
  return trimLogText(value, 1000);
}

export function buildClientLogItem({ level, module, action, detail, extra = {}, now = new Date(), readStorage = readBrowserStorage }) {
  const normalizedLevel = trimLogText(level || "INFO", 20).toUpperCase();
  const normalizedModule = trimLogText(module || "web", 120);
  const normalizedAction = trimLogText(action || "client_log", 160);
  const normalizedDetail = trimLogText(detail || "frontend log", 1000);
  if (!normalizedModule || !normalizedAction || !normalizedDetail) return null;
  return {
    ts: toISOString(now),
    level: normalizedLevel,
    module: normalizedModule,
    action: normalizedAction,
    detail: normalizedDetail,
    operator: trimLogText(firstNonEmpty(readStorage("cloud.assignee_id"), readStorage("wework.assignee_id")), 120),
    tenant_id: trimLogText(firstNonEmpty(readStorage("cloud.tenant_id"), readStorage("wework.tenant_id")), 160),
    extra: sanitizeLogExtra(extra || {}),
  };
}

export function createClientLogger(options = {}) {
  const queue = [];
  const recentFingerprints = new Map();
  const endpoint = options.endpoint || CLIENT_LOG_ENDPOINT;
  const maxBatchSize = positiveInt(options.maxBatchSize, MAX_BATCH_SIZE);
  const maxQueueSize = positiveInt(options.maxQueueSize, MAX_QUEUE_SIZE);
  const dedupMs = positiveInt(options.dedupMs, RECENT_DEDUP_MS);
  const autoFlush = options.autoFlush !== false;
  const readStorage = options.readStorage || readBrowserStorage;
  const now = options.now || (() => new Date());
  const fetchImpl = options.fetchImpl || (() => globalThis.fetch);
  const windowRef = options.windowRef || (() => (typeof window === "undefined" ? undefined : window));
  const navigatorRef = options.navigatorRef || (() => (typeof navigator === "undefined" ? undefined : navigator));
  let flushTimer = null;
  let handlersInstalled = false;

  function enqueue(level, module, action, detail, extra = {}) {
    const item = buildClientLogItem({ level, module, action, detail, extra, now: now(), readStorage });
    if (!item) return false;
    if (shouldDropDuplicate(recentFingerprints, dedupMs, now(), item)) return false;
    while (queue.length >= maxQueueSize) queue.shift();
    queue.push(item);
    if (!autoFlush) return true;
    if (queue.length >= maxBatchSize) {
      void flush();
      return true;
    }
    scheduleFlush();
    return true;
  }

  function scheduleFlush() {
    const win = windowRef();
    if (!win || flushTimer) return;
    flushTimer = win.setTimeout(() => {
      flushTimer = null;
      void flush();
    }, FLUSH_INTERVAL_MS);
  }

  function dequeueBatch() {
    if (!queue.length) return [];
    const batch = [];
    while (queue.length && batch.length < maxBatchSize) {
      const item = queue.shift();
      if (!item || shouldDropStaleQueuedLog(item, now())) continue;
      batch.push(item);
    }
    return batch;
  }

  async function flush() {
    const batch = dequeueBatch();
    if (!batch.length) {
      if (queue.length) scheduleFlush();
      return { accepted: 0, dropped: 0 };
    }
    const fetchFn = fetchImpl();
    if (typeof fetchFn !== "function") {
      queue.unshift(...batch);
      return { accepted: 0, dropped: 0 };
    }
    try {
      const headers = { "Content-Type": "application/json" };
      const token = normalizeBearerToken(firstNonEmpty(readStorage("wework.adminToken"), readStorage("wework.sessionToken")));
      if (token) headers.Authorization = `Bearer ${token}`;
      const response = await fetchFn(endpoint, {
        method: "POST",
        headers,
        body: JSON.stringify({ logs: batch }),
        keepalive: true,
      });
      if (response && response.ok === false) {
        queue.unshift(...batch);
        scheduleFlush();
        return { accepted: 0, dropped: batch.length };
      }
    } catch (_) {
      queue.unshift(...batch);
      scheduleFlush();
      return { accepted: 0, dropped: batch.length };
    }
    if (queue.length) scheduleFlush();
    return { accepted: batch.length, dropped: 0 };
  }

  function flushByBeacon() {
    const nav = navigatorRef();
    if (!nav || typeof nav.sendBeacon !== "function" || !queue.length) return false;
    const batch = dequeueBatch();
    if (!batch.length) return false;
    const blob = new Blob([JSON.stringify({ logs: batch })], { type: "application/json" });
    if (!nav.sendBeacon(endpoint, blob)) {
      queue.unshift(...batch);
      return false;
    }
    return true;
  }

  function install() {
    const win = windowRef();
    if (!win || handlersInstalled) return false;
    handlersInstalled = true;
    win.addEventListener("error", (event) => {
      const runtimeDetail = String(event?.message || event?.error?.message || "").trim() || "frontend runtime error";
      enqueue(shouldDemoteClientRuntimeLog(runtimeDetail) ? "WARN" : "ERROR", "runtime", "window.onerror", runtimeDetail, {
        source: trimLogText(event?.filename || "", 400),
        line: Number(event?.lineno || 0),
        column: Number(event?.colno || 0),
        stack: trimLogText(event?.error?.stack || "", 2000),
      });
    });
    win.addEventListener("unhandledrejection", (event) => {
      const runtimeDetail = String(event?.reason?.message || event?.reason || "").trim() || "unhandled promise rejection";
      enqueue(shouldDemoteClientRuntimeLog(runtimeDetail) ? "WARN" : "ERROR", "runtime", "unhandledrejection", runtimeDetail, {
        stack: trimLogText(event?.reason?.stack || "", 2000),
      });
    });
    const flushOnUnload = () => {
      if (flushTimer) {
        win.clearTimeout(flushTimer);
        flushTimer = null;
      }
      flushByBeacon();
    };
    win.addEventListener("beforeunload", flushOnUnload);
    win.addEventListener("pagehide", flushOnUnload);
    return true;
  }

  return {
    log: enqueue,
    info(module, action, detail, extra = {}) {
      return enqueue("INFO", module, action, detail, extra);
    },
    warn(module, action, detail, extra = {}) {
      return enqueue("WARN", module, action, detail, extra);
    },
    error(module, action, detail, extra = {}) {
      return enqueue("ERROR", module, action, detail, extra);
    },
    flush,
    install,
    queueLength() {
      return queue.length;
    },
  };
}

export const clientLogger = createClientLogger();

function shouldDropDuplicate(fingerprints, dedupMs, capturedAt, item) {
  const nowMs = toTimeMs(capturedAt);
  for (const [key, value] of fingerprints.entries()) {
    if (nowMs - value > dedupMs) fingerprints.delete(key);
  }
  const fingerprint = [item.level, item.module, item.action, item.detail].join("|");
  const previousAt = fingerprints.get(fingerprint) || 0;
  if (nowMs - previousAt < dedupMs) return true;
  fingerprints.set(fingerprint, nowMs);
  return false;
}

function shouldDropStaleQueuedLog(item, capturedAt) {
  const moduleName = String(item?.module || "").trim().toLowerCase();
  if (moduleName !== "api") return false;
  const category = String(item?.extra?.category || "").trim().toLowerCase();
  if (category !== "network" && category !== "api") return false;
  const createdAt = Date.parse(item?.ts || "");
  if (!Number.isFinite(createdAt)) return false;
  return toTimeMs(capturedAt) - createdAt > STALE_API_LOG_MAX_AGE_MS;
}

function readBrowserStorage(key) {
  if (typeof window === "undefined" || !window.localStorage) return "";
  try {
    return window.localStorage.getItem(key) || "";
  } catch {
    return "";
  }
}

function normalizeBearerToken(value) {
  const token = String(value || "").trim();
  if (!token) return "";
  return token.replace(/^Bearer\s+/i, "").trim();
}

function positiveInt(value, fallback) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.floor(parsed);
}

function firstNonEmpty(...values) {
  for (const value of values) {
    const text = String(value || "").trim();
    if (text) return text;
  }
  return "";
}

function toISOString(value) {
  if (value instanceof Date && Number.isFinite(value.getTime())) return value.toISOString();
  const date = new Date(value);
  if (Number.isFinite(date.getTime())) return date.toISOString();
  return new Date().toISOString();
}

function toTimeMs(value) {
  if (value instanceof Date) return value.getTime();
  const parsed = Number(value);
  if (Number.isFinite(parsed)) return parsed;
  return Date.parse(value);
}

function safeJSONStringify(value) {
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
