import { requestJSON } from "./api.js";
import { clearSessionToken, getSessionToken, getSessionTokenSource, setSessionToken } from "./sessionToken.js";

export const assigneeIdStorageKey = "cloud.assignee_id";
export const assigneeIdTabStorageKey = "cloud.assignee_id.tab";

export async function loginAdminWithPassword(username, password, options = {}) {
  const payload = await requestJSON("/session/admin-login", {
    method: "POST",
    body: {
      username: String(username || "").trim(),
      password: String(password || "").trim(),
    },
    fetchImpl: options.fetchImpl,
    logger: options.logger,
  });
  return persistLoginResponse("admin", payload, options);
}

export async function changeAdminPassword(currentPassword, newPassword, options = {}) {
  const token = String(options.token || getSessionToken("admin", options)).trim();
  const payload = await requestJSON("/session/admin/change-password", {
    method: "POST",
    token,
    body: {
      current_password: String(currentPassword || "").trim(),
      new_password: String(newPassword || "").trim(),
    },
    fetchImpl: options.fetchImpl,
    logger: options.logger,
  });
  return persistLoginResponse("admin", payload, options);
}

export async function loginCSWithPassword(assigneeID, password, options = {}) {
  const payload = await requestJSON("/session/cs-login", {
    method: "POST",
    body: {
      assignee_id: String(assigneeID || "").trim(),
      password: String(password || "").trim(),
    },
    fetchImpl: options.fetchImpl,
    logger: options.logger,
  });
  return persistLoginResponse("cs", payload, options);
}

export async function loginCSWithoutPassword(assigneeID, options = {}) {
  const payload = await requestJSON("/session/login", {
    method: "POST",
    body: {
      assignee_id: String(assigneeID || "").trim(),
      ttl_hours: Number(options.ttlHours || 168),
    },
    fetchImpl: options.fetchImpl,
    logger: options.logger,
  });
  return persistLoginResponse("cs", payload, options);
}

export async function logoutSession(kind = "cs", options = {}) {
  const scope = getSessionTokenSource(kind, options);
  const token = String(options.token || getSessionToken(kind, options)).trim();
  try {
    if (token) {
      await requestJSON("/session/logout", {
        method: "POST",
        body: {},
        token,
        fetchImpl: options.fetchImpl,
        logger: options.logger,
      });
    }
  } finally {
    clearLoginSession(kind, { ...options, scope });
  }
  return { success: true };
}

export function consumeCSURLSession(options = {}) {
  const search = options.search !== undefined
    ? String(options.search || "")
    : typeof window !== "undefined"
      ? window.location.search
      : "";
  const params = new URLSearchParams(search.startsWith("?") ? search : `?${search}`);
  const token = String(params.get("token") || "").trim();
  const assigneeID = String(params.get("cs_id") || "").trim();
  if (!token) return { consumed: false, token: "", assignee_id: assigneeID };
  setSessionToken("cs", token, { ...options, scope: "tab" });
  const tabStorage = resolveTabStorage(options.tabStorage);
  if (tabStorage && assigneeID) tabStorage.setItem(assigneeIdTabStorageKey, assigneeID);
  return { consumed: true, token, assignee_id: assigneeID };
}

export function clearLoginSession(kind = "cs", options = {}) {
  clearSessionToken(kind, options);
  if (kind === "cs") {
    const storage = resolveStorage(options.storage);
    const tabStorage = resolveTabStorage(options.tabStorage);
    const scope = String(options.scope || "").trim();
    if (scope !== "local") tabStorage?.removeItem(assigneeIdTabStorageKey);
    if (scope !== "tab") storage?.removeItem(assigneeIdStorageKey);
  }
}

export function getStoredCSAssigneeID(options = {}) {
  const tabStorage = resolveTabStorage(options.tabStorage);
  const storage = resolveStorage(options.storage);
  return readStorage(tabStorage, assigneeIdTabStorageKey) || readStorage(storage, assigneeIdStorageKey);
}

export function sessionLoginErrorMessage(kind, error) {
  const message = String(error?.message || error || "").trim();
  const status = Number(error?.status || 0);
  if (status === 429 || message.includes("429") || message.includes("too many") || message.includes("rate limit") || message.includes("登录过于频繁")) {
    return "登录过于频繁，请稍后再试";
  }
  if (!message) return "登录失败";
  if (kind === "admin") {
    if (message.includes("用户名或密码") || message.includes("401")) return "用户名或密码错误";
    if (message.includes("admin login is not configured")) return "当前后端未配置管理员账号";
  }
  if (kind === "cs") {
    if (message.includes("账号或密码") || message.includes("401")) return "账号或密码错误";
    if (message.includes("账号不存在") || message.includes("禁用")) return "账号不存在或已禁用";
    if (message.includes("passwordless login disabled")) return "当前后端未开启无密码登录";
  }
  return message;
}

function persistLoginResponse(kind, payload, options) {
  const token = String(payload?.token || "").trim();
  if (token) setSessionToken(kind, token, options);
  if (kind === "cs") {
    const assigneeID = String(payload?.assignee_id || "").trim();
    const storage = resolveStorage(options.storage);
    if (storage && assigneeID) storage.setItem(assigneeIdStorageKey, assigneeID);
  }
  return {
    success: Boolean(payload?.success ?? token),
    token,
    assignee_id: String(payload?.assignee_id || (kind === "admin" ? "admin" : "")).trim(),
    assignee_name: String(payload?.assignee_name || (kind === "admin" ? "管理员" : "")).trim(),
    role: String(payload?.role || kind).trim(),
    expires_at: String(payload?.expires_at || "").trim(),
    password_change_required: Boolean(payload?.password_change_required),
  };
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
    return storage?.getItem(key) || "";
  } catch {
    return "";
  }
}
