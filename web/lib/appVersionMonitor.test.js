import assert from "node:assert/strict";
import test from "node:test";
import {
  buildVersionUrl,
  clearVersionReloadNotice,
  formatVersionReloadNotice,
  parseVersionReloadNotice,
  pollAppVersion,
  readRemoteVersion,
  readVersionReloadNotice,
  reloadForVersionMismatch,
  resetAppVersionMonitorForTest,
  shouldCompareVersion,
  startAppVersionMonitor,
} from "./appVersionMonitor.js";

function createWindowStub(url = "https://console.example/admin") {
  const storage = new Map();
  const events = new Map();
  const replacements = [];
  return {
    document: { visibilityState: "visible" },
    location: {
      href: url,
      replace(next) {
        replacements.push(next);
      },
    },
    sessionStorage: {
      setItem(key, value) {
        storage.set(key, value);
      },
      getItem(key) {
        return storage.get(key) || null;
      },
      removeItem(key) {
        storage.delete(key);
      },
    },
    setTimeout(fn) {
      fn();
    },
    setInterval() {
      return 1;
    },
    addEventListener(name, fn) {
      events.set(name, fn);
    },
    events,
    replacements,
    storage,
  };
}

test("readRemoteVersion and shouldCompareVersion keep dev and unknown non-actionable", () => {
  assert.equal(readRemoteVersion(" abc\n"), "abc");
  assert.equal(shouldCompareVersion("abc", "abc"), true);
  assert.equal(shouldCompareVersion("abc", "unknown"), false);
  assert.equal(shouldCompareVersion("unknown", "abc"), false);
  assert.equal(shouldCompareVersion("dev", "abc"), false);
});

test("buildVersionUrl appends cache-busting timestamp", () => {
  assert.equal(buildVersionUrl("/version.txt", 123), "/version.txt?t=123");
  assert.equal(buildVersionUrl("/version.txt?scope=web", 123), "/version.txt?scope=web&t=123");
});

test("pollAppVersion reloads on remote version mismatch", async () => {
  const windowRef = createWindowStub("https://console.example/?tab=cs");
  const result = await pollAppVersion({
    currentVersion: "local-1",
    fetchImpl: async () => ({ ok: true, text: async () => "remote-2\n" }),
    windowRef,
    now: () => 1000,
  });

  assert.deepEqual(result, { checked: true, reloaded: true, remoteVersion: "remote-2" });
  assert.equal(windowRef.replacements[0], "https://console.example/?tab=cs&fresh=1");
  const payload = JSON.parse(windowRef.storage.get("wework.web.version.reload.pending"));
  assert.equal(payload.from, "local-1");
  assert.equal(payload.to, "remote-2");
});

test("pollAppVersion stays silent when versions match or request fails", async () => {
  const sameWindow = createWindowStub();
  const same = await pollAppVersion({
    currentVersion: "same",
    fetchImpl: async () => ({ ok: true, text: async () => "same" }),
    windowRef: sameWindow,
  });
  assert.equal(same.reloaded, false);
  assert.equal(sameWindow.replacements.length, 0);

  const failed = await pollAppVersion({
    currentVersion: "same",
    fetchImpl: async () => {
      throw new Error("offline");
    },
    windowRef: createWindowStub(),
  });
  assert.deepEqual(failed, { checked: false, reloaded: false, reason: "request_failed" });
});

test("reloadForVersionMismatch preserves current URL and records reason", () => {
  const windowRef = createWindowStub("https://console.example/admin?mode=ops");
  assert.equal(
    reloadForVersionMismatch({
      windowRef,
      currentVersion: "a",
      remoteVersion: "b",
      reason: "focus",
      now: () => 2000,
    }),
    true,
  );
  assert.equal(windowRef.replacements[0], "https://console.example/admin?mode=ops&fresh=1");
  const payload = JSON.parse(windowRef.storage.get("wework.web.version.reload.pending"));
  assert.equal(payload.reason, "focus");
  assert.equal(payload.at, 2000);
});

test("version reload notice can be read, formatted, and cleared", () => {
  const windowRef = createWindowStub("https://console.example/admin?mode=ops");
  reloadForVersionMismatch({
    windowRef,
    currentVersion: "old-build",
    remoteVersion: "new-build",
    reason: "focus",
    now: () => 3000,
  });

  const notice = readVersionReloadNotice(windowRef.sessionStorage);
  assert.deepEqual(notice, {
    from: "old-build",
    to: "new-build",
    reason: "focus",
    at: 3000,
  });
  assert.equal(formatVersionReloadNotice(notice), "已从 old-build 更新到 new-build");
  assert.equal(parseVersionReloadNotice("not-json"), null);
  assert.equal(clearVersionReloadNotice(windowRef.sessionStorage), true);
  assert.equal(readVersionReloadNotice(windowRef.sessionStorage), null);
});

test("startAppVersionMonitor installs one browser monitor", () => {
  resetAppVersionMonitorForTest();
  const windowRef = createWindowStub();
  let calls = 0;
  const fetchImpl = async () => {
    calls += 1;
    return { ok: true, text: async () => "local" };
  };

  assert.equal(startAppVersionMonitor({ currentVersion: "local", windowRef, fetchImpl }), true);
  assert.equal(startAppVersionMonitor({ currentVersion: "local", windowRef, fetchImpl }), false);
  assert.equal(windowRef.events.has("visibilitychange"), true);
  assert.equal(windowRef.events.has("focus"), true);
  assert.equal(calls, 1);
  resetAppVersionMonitorForTest();
});
