import { comparableBuildVersion } from "./appVersion.js";

const VERSION_CHECK_INTERVAL_MS = 60000;
const VERSION_URL = "/version.txt";
const VERSION_RELOAD_FLAG_KEY = "wework.web.version.reload.pending";

let started = false;

function clean(value) {
  return String(value || "").trim();
}

export function readRemoteVersion(text) {
  return clean(text);
}

export function shouldCompareVersion(currentVersion, remoteVersion) {
  const current = clean(currentVersion);
  const remote = clean(remoteVersion);
  return Boolean(
    current &&
      remote &&
      current !== "dev" &&
      current !== "unknown" &&
      remote !== "dev" &&
      remote !== "unknown",
  );
}

export function buildVersionUrl(versionUrl = VERSION_URL, timestamp = Date.now()) {
  const separator = versionUrl.includes("?") ? "&" : "?";
  return `${versionUrl}${separator}t=${encodeURIComponent(String(timestamp))}`;
}

export function reloadForVersionMismatch({
  windowRef = typeof window !== "undefined" ? window : null,
  currentVersion,
  remoteVersion,
  reason = "unknown",
  now = Date.now,
} = {}) {
  if (!windowRef?.location?.href || typeof windowRef.location.replace !== "function") {
    return false;
  }

  const nextUrl = new URL(windowRef.location.href);
  nextUrl.searchParams.set("fresh", "1");
  windowRef.sessionStorage?.setItem(
    VERSION_RELOAD_FLAG_KEY,
    JSON.stringify({
      from: clean(currentVersion),
      to: clean(remoteVersion),
      reason: clean(reason) || "unknown",
      at: now(),
    }),
  );
  windowRef.location.replace(nextUrl.toString());
  return true;
}

export function parseVersionReloadNotice(raw) {
  if (!raw) return null;
  let payload = raw;
  if (typeof raw === "string") {
    try {
      payload = JSON.parse(raw);
    } catch {
      return null;
    }
  }
  if (!payload || typeof payload !== "object") return null;
  const to = clean(payload.to);
  if (!to) return null;
  return {
    from: clean(payload.from),
    to,
    reason: clean(payload.reason) || "unknown",
    at: Number(payload.at || 0) || 0,
  };
}

export function readVersionReloadNotice(storage = browserSessionStorage()) {
  if (!storage || typeof storage.getItem !== "function") return null;
  try {
    return parseVersionReloadNotice(storage.getItem(VERSION_RELOAD_FLAG_KEY));
  } catch {
    return null;
  }
}

export function clearVersionReloadNotice(storage = browserSessionStorage()) {
  if (!storage || typeof storage.removeItem !== "function") return false;
  try {
    storage.removeItem(VERSION_RELOAD_FLAG_KEY);
    return true;
  } catch {
    return false;
  }
}

export function formatVersionReloadNotice(notice) {
  const parsed = parseVersionReloadNotice(notice);
  if (!parsed) return "已加载最新版本";
  if (parsed.from && parsed.from !== parsed.to) {
    return `已从 ${parsed.from} 更新到 ${parsed.to}`;
  }
  return `当前版本 ${parsed.to}`;
}

export async function pollAppVersion({
  currentVersion = comparableBuildVersion(),
  versionUrl = VERSION_URL,
  fetchImpl = typeof fetch === "function" ? fetch : null,
  windowRef = typeof window !== "undefined" ? window : null,
  reason = "interval",
  now = Date.now,
} = {}) {
  if (!fetchImpl) {
    return { checked: false, reloaded: false, reason: "missing_fetch" };
  }

  try {
    const response = await fetchImpl(buildVersionUrl(versionUrl, now()), {
      cache: "no-store",
      headers: { "Cache-Control": "no-cache" },
    });
    if (!response?.ok) {
      return { checked: false, reloaded: false, reason: "http_status" };
    }

    const remoteVersion = readRemoteVersion(await response.text());
    if (!shouldCompareVersion(currentVersion, remoteVersion)) {
      return { checked: true, reloaded: false, remoteVersion };
    }
    if (remoteVersion === clean(currentVersion)) {
      return { checked: true, reloaded: false, remoteVersion };
    }

    const reloaded = reloadForVersionMismatch({
      windowRef,
      currentVersion,
      remoteVersion,
      reason,
      now,
    });
    return { checked: true, reloaded, remoteVersion };
  } catch {
    return { checked: false, reloaded: false, reason: "request_failed" };
  }
}

export function startAppVersionMonitor({
  currentVersion = comparableBuildVersion(),
  versionUrl = VERSION_URL,
  intervalMs = VERSION_CHECK_INTERVAL_MS,
  fetchImpl = typeof fetch === "function" ? fetch : null,
  windowRef = typeof window !== "undefined" ? window : null,
  now = Date.now,
} = {}) {
  if (started || !windowRef || !fetchImpl) {
    return false;
  }
  started = true;

  const poll = (reason) => {
    void pollAppVersion({
      currentVersion,
      versionUrl,
      fetchImpl,
      windowRef,
      reason,
      now,
    });
  };

  windowRef.setTimeout?.(() => poll("startup"), 0);
  windowRef.setInterval?.(() => poll("interval"), intervalMs);
  windowRef.addEventListener?.("visibilitychange", () => {
    const doc = windowRef.document || (typeof document !== "undefined" ? document : null);
    if (doc?.visibilityState && doc.visibilityState !== "visible") return;
    poll("visibility");
  });
  windowRef.addEventListener?.("focus", () => poll("focus"));
  return true;
}

export function resetAppVersionMonitorForTest() {
  started = false;
}

function browserSessionStorage() {
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage || null;
  } catch {
    return null;
  }
}
