import assert from "node:assert/strict";
import test from "node:test";
import {
  clearSessionToken,
  ensureSessionTokenFresh,
  getSessionToken,
  getSessionTokenSource,
  isSessionTokenExpired,
  parseSessionTokenPayload,
  refreshSessionToken,
  requestSessionJSON,
  sessionTokenTTL,
  setSessionToken,
  sessionTokenKeys,
} from "./sessionToken.js";

test("parseSessionTokenPayload decodes jwt payload and expiration", () => {
  const token = makeToken({ sub: "cs-1", exp: 2000 });

  assert.deepEqual(parseSessionTokenPayload(token), { sub: "cs-1", exp: 2000 });
  assert.equal(isSessionTokenExpired(token, 1_999_000), false);
  assert.equal(isSessionTokenExpired(token, 2_000_000), true);
  assert.equal(sessionTokenTTL(token, 1_990_000), 10_000);
  assert.equal(parseSessionTokenPayload("bad-token"), null);
});

test("setSessionToken and clearSessionToken use role-specific storage keys", () => {
  const storage = new MemoryStorage();

  setSessionToken("cs", "cs-token", { storage });
  setSessionToken("admin", "admin-token", { storage });

  assert.equal(getSessionToken("cs", { storage }), "cs-token");
  assert.equal(getSessionToken("admin", { storage }), "admin-token");
  assert.equal(storage.getItem(sessionTokenKeys.cs), "cs-token");
  assert.equal(storage.getItem(sessionTokenKeys.admin), "admin-token");

  clearSessionToken("cs", { storage });
  assert.equal(getSessionToken("cs", { storage }), "");
  assert.equal(getSessionToken("admin", { storage }), "admin-token");
});

test("tab scoped CS token overrides local token without replacing it", () => {
  const storage = new MemoryStorage();
  const tabStorage = new MemoryStorage();

  setSessionToken("cs", "local-cs-token", { storage, tabStorage });
  setSessionToken("cs", "tab-cs-token", { storage, tabStorage, scope: "tab" });

  assert.equal(getSessionToken("cs", { storage, tabStorage }), "tab-cs-token");
  assert.equal(getSessionTokenSource("cs", { storage, tabStorage }), "tab");
  assert.equal(storage.getItem(sessionTokenKeys.cs), "local-cs-token");
  assert.equal(tabStorage.getItem(sessionTokenKeys.csTab), "tab-cs-token");

  clearSessionToken("cs", { storage, tabStorage, scope: "tab" });
  assert.equal(getSessionToken("cs", { storage, tabStorage }), "local-cs-token");
  assert.equal(storage.getItem(sessionTokenKeys.cs), "local-cs-token");
});

test("ensureSessionTokenFresh keeps token when ttl is sufficient", async () => {
  const storage = new MemoryStorage();
  const token = makeToken({ exp: 3000 });
  setSessionToken("cs", token, { storage });

  const result = await ensureSessionTokenFresh("cs", {
    storage,
    nowMs: 2_000_000,
    minTtlMs: 30_000,
    fetchImpl: () => {
      throw new Error("refresh should not be called");
    },
  });

  assert.equal(result, token);
});

test("refreshSessionToken posts legacy refresh request and stores next token", async () => {
  const storage = new MemoryStorage();
  const oldToken = makeToken({ exp: 1000 });
  const nextToken = makeToken({ exp: 4000 });
  setSessionToken("admin", oldToken, { storage });
  const calls = [];

  const result = await refreshSessionToken("admin", {
    storage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return {
        ok: true,
        json: async () => ({ token: nextToken }),
      };
    },
  });

  assert.equal(result, nextToken);
  assert.equal(getSessionToken("admin", { storage }), nextToken);
  assert.equal(calls[0].url, "/api/v1/session/refresh");
  assert.equal(calls[0].init.headers.Authorization, `Bearer ${oldToken}`);
});

test("refreshSessionToken updates tab scoped CS token in place", async () => {
  const storage = new MemoryStorage();
  const tabStorage = new MemoryStorage();
  const localToken = makeToken({ exp: 1000, sub: "cs-local" });
  const tabToken = makeToken({ exp: 1000, sub: "cs-tab" });
  const nextToken = makeToken({ exp: 4000, sub: "cs-tab" });
  setSessionToken("cs", localToken, { storage, tabStorage });
  setSessionToken("cs", tabToken, { storage, tabStorage, scope: "tab" });
  const calls = [];

  const result = await refreshSessionToken("cs", {
    storage,
    tabStorage,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return {
        ok: true,
        json: async () => ({ token: nextToken }),
      };
    },
  });

  assert.equal(result, nextToken);
  assert.equal(calls[0].init.headers.Authorization, `Bearer ${tabToken}`);
  assert.equal(getSessionToken("cs", { storage, tabStorage }), nextToken);
  assert.equal(storage.getItem(sessionTokenKeys.cs), localToken);
  assert.equal(tabStorage.getItem(sessionTokenKeys.csTab), nextToken);
});

test("refreshSessionToken clears token on unauthorized refresh", async () => {
  const storage = new MemoryStorage();
  setSessionToken("cs", makeToken({ exp: 1000 }), { storage });

  const result = await refreshSessionToken("cs", {
    storage,
    fetchImpl: async () => ({ ok: false, status: 401 }),
  });

  assert.equal(result, false);
  assert.equal(getSessionToken("cs", { storage }), "");
});

test("requestSessionJSON refreshes token and replays once after unauthorized response", async () => {
  const storage = new MemoryStorage();
  const futureExp = Math.floor(Date.now() / 1000) + 3600;
  const oldToken = makeToken({ exp: futureExp });
  const nextToken = makeToken({ exp: futureExp + 3600 });
  setSessionToken("cs", oldToken, { storage });
  const calls = [];

  const result = await requestSessionJSON("cs", "/accounts", {
    storage,
    minTokenTtlMs: 0,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      if (url === "/api/v1/session/refresh") {
        return {
          ok: true,
          json: async () => ({ token: nextToken }),
        };
      }
      if (calls.filter((call) => call.url === "/api/v1/accounts").length === 1) {
        return {
          ok: false,
          status: 401,
          text: async () => `{"detail":"session invalid or expired"}`,
        };
      }
      return {
        ok: true,
        text: async () => `{"accounts":[]}`,
      };
    },
  });

  assert.deepEqual(result, { accounts: [] });
  assert.equal(calls.length, 3);
  assert.equal(calls[0].url, "/api/v1/accounts");
  assert.equal(calls[0].init.headers.Authorization, `Bearer ${oldToken}`);
  assert.equal(calls[1].url, "/api/v1/session/refresh");
  assert.equal(calls[1].init.headers.Authorization, `Bearer ${oldToken}`);
  assert.equal(calls[2].url, "/api/v1/accounts");
  assert.equal(calls[2].init.headers.Authorization, `Bearer ${nextToken}`);
  assert.equal(getSessionToken("cs", { storage }), nextToken);
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

function makeToken(payload) {
  return `${encode({ alg: "HS256", typ: "JWT" })}.${encode(payload)}.sig`;
}

function encode(value) {
  return Buffer.from(JSON.stringify(value), "utf8")
    .toString("base64")
    .replace(/=/g, "")
    .replace(/\+/g, "-")
    .replace(/\//g, "_");
}
