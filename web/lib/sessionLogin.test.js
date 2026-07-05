import assert from "node:assert/strict";
import test from "node:test";
import {
  assigneeIdStorageKey,
  assigneeIdTabStorageKey,
  changeAdminPassword,
  consumeCSURLSession,
  getStoredCSAssigneeID,
  loginAdminWithPassword,
  loginCSWithPassword,
  loginCSWithoutPassword,
  logoutSession,
  sessionLoginErrorMessage,
} from "./sessionLogin.js";
import { getSessionToken, sessionTokenKeys, setSessionToken } from "./sessionToken.js";

test("loginAdminWithPassword posts legacy payload and stores admin token", async () => {
  const storage = new MemoryStorage();
  const calls = [];

  const response = await loginAdminWithPassword(" admin ", " secret ", {
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({
        success: true,
        token: "jwt-admin",
        assignee_id: "admin",
        assignee_name: "管理员",
        role: "admin",
        password_change_required: true,
      });
    },
  });

  assert.equal(calls[0].url, "/api/v1/session/admin-login");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.body, `{"username":"admin","password":"secret"}`);
  assert.equal(response.token, "jwt-admin");
  assert.equal(response.role, "admin");
  assert.equal(response.password_change_required, true);
  assert.equal(getSessionToken("admin", { storage }), "jwt-admin");
});

test("changeAdminPassword posts current and new password then stores final token", async () => {
  const storage = new MemoryStorage();
  const calls = [];
  setSessionToken("admin", "jwt-change", { storage });

  const response = await changeAdminPassword(" 1234567890 ", " new-password-123 ", {
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({
        success: true,
        token: "jwt-admin-final",
        assignee_id: "root",
        role: "admin",
        password_change_required: false,
      });
    },
  });

  assert.equal(calls[0].url, "/api/v1/session/admin/change-password");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers.Authorization, "Bearer jwt-change");
  assert.equal(calls[0].init.body, `{"current_password":"1234567890","new_password":"new-password-123"}`);
  assert.equal(response.assignee_id, "root");
  assert.equal(response.password_change_required, false);
  assert.equal(getSessionToken("admin", { storage }), "jwt-admin-final");
});

test("loginCSWithPassword stores workspace token and legacy assignee id", async () => {
  const storage = new MemoryStorage();
  const calls = [];

  const response = await loginCSWithPassword(" cs-001 ", " secret ", {
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({
        success: true,
        token: "jwt-cs",
        assignee_id: "cs-001",
        assignee_name: "客服一",
        role: "cs",
      });
    },
  });

  assert.equal(calls[0].url, "/api/v1/session/cs-login");
  assert.equal(calls[0].init.body, `{"assignee_id":"cs-001","password":"secret"}`);
  assert.equal(response.assignee_id, "cs-001");
  assert.equal(getSessionToken("cs", { storage }), "jwt-cs");
  assert.equal(storage.getItem(assigneeIdStorageKey), "cs-001");
});

test("loginCSWithoutPassword keeps the legacy public session endpoint available", async () => {
  const storage = new MemoryStorage();
  const calls = [];

  await loginCSWithoutPassword("cs-002", {
    ttlHours: 24,
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({
        success: true,
        token: "jwt-cs-passwordless",
        assignee_id: "cs-002",
      });
    },
  });

  assert.equal(calls[0].url, "/api/v1/session/login");
  assert.equal(calls[0].init.body, `{"assignee_id":"cs-002","ttl_hours":24}`);
  assert.equal(storage.getItem(sessionTokenKeys.cs), "jwt-cs-passwordless");
  assert.equal(storage.getItem(assigneeIdStorageKey), "cs-002");
});

test("logoutSession posts legacy logout and clears local session keys", async () => {
  const storage = new MemoryStorage();
  const calls = [];
  setSessionToken("cs", "jwt-cs", { storage });
  storage.setItem(assigneeIdStorageKey, "cs-001");

  await logoutSession("cs", {
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({ success: true });
    },
  });

  assert.equal(calls[0].url, "/api/v1/session/logout");
  assert.equal(calls[0].init.headers.Authorization, "Bearer jwt-cs");
  assert.equal(getSessionToken("cs", { storage }), "");
  assert.equal(storage.getItem(assigneeIdStorageKey), "");
});

test("logoutSession clears only tab scoped generated workspace token", async () => {
  const storage = new MemoryStorage();
  const tabStorage = new MemoryStorage();
  setSessionToken("cs", "jwt-local", { storage, tabStorage });
  setSessionToken("cs", "jwt-tab", { storage, tabStorage, scope: "tab" });
  storage.setItem(assigneeIdStorageKey, "cs-local");
  tabStorage.setItem(assigneeIdTabStorageKey, "cs-tab");
  const calls = [];

  await logoutSession("cs", {
    storage,
    tabStorage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return jsonResponse({ success: true });
    },
  });

  assert.equal(calls[0].init.headers.Authorization, "Bearer jwt-tab");
  assert.equal(getSessionToken("cs", { storage, tabStorage }), "jwt-local");
  assert.equal(storage.getItem(assigneeIdStorageKey), "cs-local");
  assert.equal(tabStorage.getItem(assigneeIdTabStorageKey), "");
});

test("consumeCSURLSession stores admin-generated workspace token", () => {
  const storage = new MemoryStorage();
  const tabStorage = new MemoryStorage();
  setSessionToken("cs", "jwt-local", { storage, tabStorage });
  storage.setItem(assigneeIdStorageKey, "cs-local");

  const result = consumeCSURLSession({
    storage,
    tabStorage,
    search: "?fresh=1&cs_id=cs-003&token=jwt-generated",
  });

  assert.equal(result.consumed, true);
  assert.equal(result.token, "jwt-generated");
  assert.equal(result.assignee_id, "cs-003");
  assert.equal(getSessionToken("cs", { storage, tabStorage }), "jwt-generated");
  assert.equal(storage.getItem(sessionTokenKeys.cs), "jwt-local");
  assert.equal(tabStorage.getItem(sessionTokenKeys.csTab), "jwt-generated");
  assert.equal(storage.getItem(assigneeIdStorageKey), "cs-local");
  assert.equal(tabStorage.getItem(assigneeIdTabStorageKey), "cs-003");
  assert.equal(getStoredCSAssigneeID({ storage, tabStorage }), "cs-003");
  assert.deepEqual(consumeCSURLSession({ storage, tabStorage, search: "?cs_id=cs-004" }), {
    consumed: false,
    token: "",
    assignee_id: "cs-004",
  });
});

test("sessionLoginErrorMessage maps legacy backend details", () => {
  assert.equal(sessionLoginErrorMessage("admin", new Error("用户名或密码错误")), "用户名或密码错误");
  assert.equal(sessionLoginErrorMessage("admin", new Error("admin login is not configured")), "当前后端未配置管理员账号");
  assert.equal(sessionLoginErrorMessage("cs", new Error("账号不存在或已禁用")), "账号不存在或已禁用");
  assert.equal(sessionLoginErrorMessage("cs", new Error("passwordless login disabled")), "当前后端未开启无密码登录");
  assert.equal(sessionLoginErrorMessage("cs", { status: 429, message: "too many attempts" }), "登录过于频繁，请稍后再试");
  assert.equal(sessionLoginErrorMessage("admin", new Error("rate limit exceeded")), "登录过于频繁，请稍后再试");
  assert.equal(sessionLoginErrorMessage("cs", new Error("登录过于频繁")), "登录过于频繁，请稍后再试");
});

class MemoryStorage {
  constructor() {
    this.values = new Map();
  }

  getItem(key) {
    return this.values.get(key) || "";
  }

  setItem(key, value) {
    this.values.set(key, String(value));
  }

  removeItem(key) {
    this.values.delete(key);
  }
}

function jsonResponse(payload) {
  return {
    ok: true,
    text: async () => JSON.stringify(payload),
  };
}
