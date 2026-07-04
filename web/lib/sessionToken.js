import { buildApiPath, requestJSON } from "./api.js";

export const sessionTokenKeys = {
  cs: "wework.sessionToken",
  csTab: "wework.sessionToken.tab",
  admin: "wework.adminToken",
};

const refreshPromises = new Map();

export function parseSessionTokenPayload(token) {
  const parts = String(token || "").split(".");
  if (parts.length < 2 || !parts[1]) return null;
  try {
    const payload = JSON.parse(decodeBase64Url(parts[1]));
    return payload && typeof payload === "object" ? payload : null;
  } catch {
    return null;
  }
}

export function isSessionTokenExpired(token, nowMs = Date.now()) {
  const expMs = sessionTokenExpiresAt(token);
  return expMs <= 0 || expMs <= nowMs;
}

export function sessionTokenTTL(token, nowMs = Date.now()) {
  const expMs = sessionTokenExpiresAt(token);
  if (expMs <= 0) return 0;
  return Math.max(0, expMs - nowMs);
}

export function getSessionToken(kind = "cs", options = {}) {
  const storage = resolveStorage(options.storage);
  const tabStorage = resolveTabStorage(options.tabStorage);
  if (String(kind || "cs").trim() === "cs") {
    const tabToken = readStorage(tabStorage, sessionTokenKeys.csTab);
    if (tabToken) return tabToken;
  }
  return readStorage(storage, tokenKey(kind));
}

export function setSessionToken(kind = "cs", token = "", options = {}) {
  const normalizedKind = String(kind || "cs").trim();
  const scope = normalizedKind === "cs" && String(options.scope || "").trim() === "tab" ? "tab" : "local";
  const storage = scope === "tab" ? resolveTabStorage(options.tabStorage) : resolveStorage(options.storage);
  const key = scope === "tab" ? sessionTokenKeys.csTab : tokenKey(kind);
  if (!storage) return;
  const value = String(token || "").trim();
  if (value) {
    storage.setItem(key, value);
  } else {
    storage.removeItem(key);
  }
}

export function clearSessionToken(kind = "cs", options = {}) {
  const storage = resolveStorage(options.storage);
  const tabStorage = resolveTabStorage(options.tabStorage);
  const normalizedKind = String(kind || "cs").trim();
  const scope = String(options.scope || "").trim();
  if (normalizedKind === "cs" && scope !== "local") {
    tabStorage?.removeItem(sessionTokenKeys.csTab);
  }
  if (scope !== "tab") {
    storage?.removeItem(tokenKey(kind));
  }
}

export function getSessionTokenSource(kind = "cs", options = {}) {
  if (String(kind || "cs").trim() !== "cs") return "local";
  const tabStorage = resolveTabStorage(options.tabStorage);
  return readStorage(tabStorage, sessionTokenKeys.csTab) ? "tab" : "local";
}

export async function ensureSessionTokenFresh(kind = "cs", options = {}) {
  const minTtlMs = Math.max(0, Number(options.minTtlMs || 0));
  const nowMs = Number(options.nowMs || Date.now());
  const token = getSessionToken(kind, options);
  if (!token) return false;
  if (sessionTokenTTL(token, nowMs) > minTtlMs) return token;
  return refreshSessionToken(kind, options);
}

export async function refreshSessionToken(kind = "cs", options = {}) {
  const storage = resolveStorage(options.storage);
  const tabStorage = resolveTabStorage(options.tabStorage);
  const source = getSessionTokenSource(kind, { storage, tabStorage });
  const currentToken = String(options.token || getSessionToken(kind, { storage, tabStorage })).trim();
  if (!currentToken) return false;
  const key = tokenKey(kind);
  if (refreshPromises.has(key)) return refreshPromises.get(key);

  const promise = (async () => {
    const fetchFn = options.fetchImpl || globalThis.fetch;
    if (typeof fetchFn !== "function") return false;
    try {
      const response = await fetchFn(buildApiPath("/session/refresh"), {
        method: "POST",
        headers: {
          Authorization: `Bearer ${currentToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({}),
      });
      if (!response || response.ok === false) {
        clearSessionToken(kind, { storage, tabStorage, scope: source });
        return false;
      }
      const data = await response.json();
      const nextToken = String(data?.token || "").trim();
      if (!nextToken) {
        clearSessionToken(kind, { storage, tabStorage, scope: source });
        return false;
      }
      setSessionToken(kind, nextToken, { storage, tabStorage, scope: source });
      return nextToken;
    } catch {
      return false;
    } finally {
      refreshPromises.delete(key);
    }
  })();

  refreshPromises.set(key, promise);
  return promise;
}

export async function requestSessionJSON(kind, path, options = {}) {
  const storage = resolveStorage(options.storage);
  const tabStorage = resolveTabStorage(options.tabStorage);
  const refreshFetchImpl = options.refreshFetchImpl || options.fetchImpl;
  const fresh = await ensureSessionTokenFresh(kind, {
    storage,
    tabStorage,
    fetchImpl: refreshFetchImpl,
    minTtlMs: options.minTokenTtlMs ?? 60_000,
  });
  const token = typeof fresh === "string" ? fresh : getSessionToken(kind, { storage, tabStorage });
  try {
    return await requestJSON(path, { ...options, token });
  } catch (error) {
    if (error?.status !== 401 || options.retryUnauthorized === false) {
      throw error;
    }
    const nextToken = await refreshSessionToken(kind, {
      storage,
      tabStorage,
      fetchImpl: refreshFetchImpl,
    });
    if (!nextToken) {
      throw error;
    }
    return requestJSON(path, { ...options, token: nextToken });
  }
}

function tokenKey(kind) {
  const normalizedKind = String(kind || "cs").trim();
  return sessionTokenKeys[normalizedKind] || sessionTokenKeys.cs;
}

function sessionTokenExpiresAt(token) {
  const payload = parseSessionTokenPayload(token);
  const exp = Number(payload?.exp || 0);
  if (!Number.isFinite(exp) || exp <= 0) return 0;
  return exp * 1000;
}

function decodeBase64Url(segment) {
  const normalized = String(segment || "").replace(/-/g, "+").replace(/_/g, "/");
  const padding = (4 - (normalized.length % 4)) % 4;
  const encoded = `${normalized}${"=".repeat(padding)}`;
  if (typeof globalThis.atob === "function") return globalThis.atob(encoded);
  if (typeof Buffer !== "undefined") return Buffer.from(encoded, "base64").toString("utf-8");
  throw new Error("base64 decoder is unavailable");
}

function resolveStorage(storage) {
  if (storage) return storage;
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage || null;
  } catch {
    return null;
  }
}

function resolveTabStorage(storage) {
  if (storage) return storage;
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage || null;
  } catch {
    return null;
  }
}

function readStorage(storage, key) {
  try {
    return storage.getItem(key) || "";
  } catch {
    return "";
  }
}
