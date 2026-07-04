/*
 * API constants for the Next.js rewrite.
 * Business filtering and permission decisions must stay on the backend; this
 * module only centralizes transport paths for future page migrations.
 */
import { clientLogger } from "./clientLogger.js";

export const apiBasePath = "/api/v1";
export const realtimePath = "/ws/{channel}";
const clientLogPath = "/client-logs";
const retryableGatewayStatuses = new Set([502, 503, 504]);
const defaultSafeGetRetryDelaysMs = [350, 900];
const requestBreakerStorageKey = "wework.api.breaker.until";
const externalUpstreamGatewayPathPrefixes = ["/platform/", "/archive/sdk/"];
const globalRequestBreaker = createRequestBreaker();

export function buildApiPath(path, params = {}, basePath = apiBasePath) {
  const searchParams = new URLSearchParams();
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    searchParams.set(key, String(value));
  });
  const query = searchParams.toString();
  return `${String(basePath ?? apiBasePath)}${path}${query ? `?${query}` : ""}`;
}

export async function requestJSON(path, {
  params = {},
  token = "",
  signal,
  method = "GET",
  body,
  basePath = apiBasePath,
  headers: extraHeaders = {},
  logger = clientLogger,
  fetchImpl = globalThis.fetch,
  retryDelaysMs = defaultSafeGetRetryDelaysMs,
  sleepImpl = sleep,
  breaker = globalRequestBreaker,
} = {}) {
  const normalizedMethod = String(method || "GET").trim().toUpperCase() || "GET";
  const breakerWaitMs = requestBreakerRemainingMs(breaker, normalizedMethod, path);
  if (breakerWaitMs > 0) {
    logAPIError(logger, "warn", path, "api request breaker is open", {
      category: "api_breaker",
      method: normalizedMethod,
      api_path: path,
      retry_after_ms: breakerWaitMs,
    });
    throw createRequestBreakerError(breakerWaitMs);
  }
  const headers = { Accept: "application/json", ...extraHeaders };
  const normalizedToken = String(token || "").trim();
  if (normalizedToken) {
    headers.Authorization = `Bearer ${normalizedToken}`;
  }
  const requestBody = normalizeBody(body, headers);
  const retryDelays = normalizeRetryDelays(retryDelaysMs);
  let response;
  for (let attempt = 0; attempt <= retryDelays.length; attempt += 1) {
    try {
      response = await fetchImpl(buildApiPath(path, params, basePath), {
        method: normalizedMethod,
        headers,
        body: requestBody,
        signal,
        cache: "no-store",
      });
    } catch (error) {
      if (!isAbortError(error)) {
        recordRequestBreakerFailure(breaker, normalizedMethod, path);
        logAPIError(logger, "error", path, String(error?.message || error || "network request failed"), {
          category: "network",
          method: normalizedMethod,
          api_path: path,
        });
      }
      throw error;
    }
    if (!response.ok && shouldRetryGatewayResponse(normalizedMethod, response.status) && attempt < retryDelays.length) {
      await sleepImpl(retryDelays[attempt]);
      continue;
    }
    break;
  }
  const text = await response.text();
  let payload = {};
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { detail: text };
    }
  }
  if (!response.ok) {
    const detail = payload?.detail || payload?.message || `HTTP ${response.status}`;
    const message = Array.isArray(detail) ? "request validation failed" : String(detail);
    if (shouldRecordGatewayBreakerFailure(normalizedMethod, path, response.status)) {
      recordRequestBreakerFailure(breaker, normalizedMethod, path);
    }
    logAPIError(logger, resolveAPIErrorLogLevel(path, response.status), path, message, {
      category: "api",
      method: normalizedMethod,
      api_path: path,
      status: response.status,
    });
    const error = new Error(message);
    error.status = response.status;
    error.payload = payload;
    throw error;
  }
  recordRequestBreakerSuccess(breaker, normalizedMethod, path);
  return payload;
}

export function createRequestBreaker(options = {}) {
  const failureLimit = positiveInt(options.failureLimit, 6);
  const windowMs = positiveInt(options.windowMs, 10_000);
  const cooldownMs = positiveInt(options.cooldownMs, 15_000);
  const nowMs = options.nowMs || (() => Date.now());
  const storage = options.storage === undefined ? browserSessionStorage : () => options.storage;
  const storageKey = options.storageKey || requestBreakerStorageKey;
  let failures = [];
  let cooldownUntilMs = 0;

  function now() {
    const value = Number(nowMs());
    return Number.isFinite(value) ? value : Date.now();
  }

  function readCooldownUntil() {
    const current = now();
    const stored = readStorageNumber(storage(), storageKey);
    const until = Math.max(cooldownUntilMs, stored);
    if (until > current) return until;
    cooldownUntilMs = 0;
    writeStorageNumber(storage(), storageKey, 0);
    return 0;
  }

  function remainingMs(request = {}) {
    if (!shouldUseRequestBreaker(request.method)) return 0;
    return Math.max(0, readCooldownUntil() - now());
  }

  function recordFailure(request = {}) {
    if (!shouldUseRequestBreaker(request.method)) return 0;
    const current = now();
    const openUntil = readCooldownUntil();
    if (openUntil > current) return openUntil;
    failures = failures.filter((item) => current - item <= windowMs);
    failures.push(current);
    if (failures.length < failureLimit) return 0;
    failures = [];
    cooldownUntilMs = current + cooldownMs;
    writeStorageNumber(storage(), storageKey, cooldownUntilMs);
    return cooldownUntilMs;
  }

  function recordSuccess(request = {}) {
    if (!shouldUseRequestBreaker(request.method)) return false;
    failures = [];
    if (readCooldownUntil() <= now()) {
      cooldownUntilMs = 0;
      writeStorageNumber(storage(), storageKey, 0);
    }
    return true;
  }

  function reset() {
    failures = [];
    cooldownUntilMs = 0;
    writeStorageNumber(storage(), storageKey, 0);
  }

  return { remainingMs, recordFailure, recordSuccess, reset };
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function normalizeRetryDelays(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => Math.max(0, Number(item) || 0))
    .filter((item) => Number.isFinite(item));
}

function shouldRetryGatewayResponse(method, status) {
  return String(method || "").toUpperCase() === "GET" && retryableGatewayStatuses.has(Number(status || 0));
}

function shouldRecordGatewayBreakerFailure(method, path, status) {
  return shouldRetryGatewayResponse(method, status) && !isExternalUpstreamGatewayPath(path);
}

function isExternalUpstreamGatewayPath(path) {
  const normalizedPath = String(path || "").trim();
  return externalUpstreamGatewayPathPrefixes.some((prefix) => normalizedPath.startsWith(prefix));
}

function resolveAPIErrorLogLevel(path, status) {
  const normalizedStatus = Number(status || 0);
  if (normalizedStatus < 500) return "warn";
  if (retryableGatewayStatuses.has(normalizedStatus) && isExternalUpstreamGatewayPath(path)) return "warn";
  return "error";
}

function shouldUseRequestBreaker(method) {
  const normalizedMethod = String(method || "GET").trim().toUpperCase() || "GET";
  return normalizedMethod === "GET" || normalizedMethod === "HEAD";
}

function requestBreakerRemainingMs(breaker, method, path) {
  if (!breaker || typeof breaker.remainingMs !== "function") return 0;
  return Math.max(0, Number(breaker.remainingMs({ method, path }) || 0));
}

function recordRequestBreakerFailure(breaker, method, path) {
  if (!breaker || typeof breaker.recordFailure !== "function") return 0;
  return breaker.recordFailure({ method, path });
}

function recordRequestBreakerSuccess(breaker, method, path) {
  if (!breaker || typeof breaker.recordSuccess !== "function") return false;
  return breaker.recordSuccess({ method, path });
}

function createRequestBreakerError(retryAfterMs) {
  const error = new Error("API requests are cooling down after repeated gateway failures");
  error.status = 503;
  error.code = "api_request_breaker_open";
  error.retryAfterMs = retryAfterMs;
  return error;
}

function positiveInt(value, fallback) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
}

function browserSessionStorage() {
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage || null;
  } catch {
    return null;
  }
}

function readStorageNumber(storage, key) {
  if (!storage || typeof storage.getItem !== "function") return 0;
  try {
    const value = Number(storage.getItem(key) || 0);
    return Number.isFinite(value) ? value : 0;
  } catch {
    return 0;
  }
}

function writeStorageNumber(storage, key, value) {
  if (!storage) return false;
  try {
    if (!value && typeof storage.removeItem === "function") {
      storage.removeItem(key);
      return true;
    }
    if (typeof storage.setItem === "function") {
      storage.setItem(key, String(value));
      return true;
    }
  } catch {
    return false;
  }
  return false;
}

function normalizeBody(body, headers) {
  if (body === undefined || body === null) return undefined;
  if (typeof FormData !== "undefined" && body instanceof FormData) return body;
  if (typeof Blob !== "undefined" && body instanceof Blob) return body;
  if (typeof body === "string") {
    if (!hasHeader(headers, "Content-Type")) headers["Content-Type"] = "application/json";
    return body;
  }
  if (!hasHeader(headers, "Content-Type")) headers["Content-Type"] = "application/json";
  return JSON.stringify(body);
}

function hasHeader(headers, name) {
  const normalizedName = String(name || "").toLowerCase();
  return Object.keys(headers || {}).some((key) => key.toLowerCase() === normalizedName);
}

function logAPIError(logger, level, path, detail, extra) {
  if (String(path || "").trim() === clientLogPath) return;
  const target = logger?.[level] || logger?.error;
  if (typeof target !== "function") return;
  target.call(logger, "api", path, detail, extra);
}

function isAbortError(error) {
  const text = [error?.name, error?.message, error]
    .map((item) => String(item || "").toLowerCase())
    .join(" ");
  return text.includes("aborterror") || text.includes("aborted");
}
