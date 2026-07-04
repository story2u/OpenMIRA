import assert from "node:assert/strict";
import test from "node:test";
import { buildApiPath, createRequestBreaker, requestJSON } from "./api.js";

test("buildApiPath appends non empty query params", () => {
  assert.equal(
    buildApiPath("/accounts", { limit: 20, q: "夏南", empty: "", nil: null }),
    "/api/v1/accounts?limit=20&q=%E5%A4%8F%E5%8D%97",
  );
  assert.equal(
    buildApiPath("/wework/login/status", { device_id: "device-1" }, ""),
    "/wework/login/status?device_id=device-1",
  );
});

test("requestJSON sends JSON body, bearer token, and custom headers", async () => {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      text: async () => `{"ok":true}`,
    };
  };
  const payload = await requestJSON("/tasks", {
    method: "POST",
    token: "token-1",
    body: { task_id: "task-1" },
    headers: { "X-Trace-ID": "trace-1" },
    fetchImpl,
  });

  assert.deepEqual(payload, { ok: true });
  assert.equal(calls[0].url, "/api/v1/tasks");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers.Authorization, "Bearer token-1");
  assert.equal(calls[0].init.headers["Content-Type"], "application/json");
  assert.equal(calls[0].init.headers["X-Trace-ID"], "trace-1");
  assert.equal(calls[0].init.body, `{"task_id":"task-1"}`);
});

test("requestJSON can call root legacy routes", async () => {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: true,
      text: async () => `{"status":"waiting"}`,
    };
  };

  const payload = await requestJSON("/wework/login/status", {
    basePath: "",
    params: { device_id: "device-1" },
    fetchImpl,
  });

  assert.deepEqual(payload, { status: "waiting" });
  assert.equal(calls[0].url, "/wework/login/status?device_id=device-1");
});

test("requestJSON retries safe GET gateway failures before returning success", async () => {
  const calls = [];
  const delays = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (calls.length === 1) {
      return {
        ok: false,
        status: 502,
        text: async () => `{"detail":"bad gateway"}`,
      };
    }
    return {
      ok: true,
      text: async () => `{"ok":true}`,
    };
  };

  const payload = await requestJSON("/admin/stats/overview", {
    fetchImpl,
    retryDelaysMs: [0],
    sleepImpl: async (ms) => {
      delays.push(ms);
    },
  });

  assert.deepEqual(payload, { ok: true });
  assert.equal(calls.length, 2);
  assert.equal(calls[0].init.method, "GET");
  assert.equal(calls[1].url, "/api/v1/admin/stats/overview");
  assert.deepEqual(delays, [0]);
});

test("requestJSON does not retry unsafe gateway write requests", async () => {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    return {
      ok: false,
      status: 502,
      text: async () => `{"detail":"bad gateway"}`,
    };
  };

  await assert.rejects(
    () => requestJSON("/tasks", {
      method: "POST",
      body: { task_id: "task-1" },
      fetchImpl,
      retryDelaysMs: [0],
      logger: {},
    }),
    /bad gateway/,
  );

  assert.equal(calls.length, 1);
  assert.equal(calls[0].init.method, "POST");
});

test("requestJSON opens a GET breaker after repeated gateway failures", async () => {
  let now = 1_000;
  const breaker = createRequestBreaker({
    failureLimit: 2,
    windowMs: 10_000,
    cooldownMs: 5_000,
    nowMs: () => now,
  });
  let failingCalls = 0;
  const failingFetch = async () => {
    failingCalls += 1;
    return {
      ok: false,
      status: 502,
      text: async () => `{"detail":"bad gateway"}`,
    };
  };

  await assert.rejects(
    () => requestJSON("/accounts", { fetchImpl: failingFetch, breaker, retryDelaysMs: [], logger: {} }),
    /bad gateway/,
  );
  await assert.rejects(
    () => requestJSON("/admin/stats/overview", { fetchImpl: failingFetch, breaker, retryDelaysMs: [], logger: {} }),
    /bad gateway/,
  );
  assert.equal(failingCalls, 2);

  await assert.rejects(
    () => requestJSON("/accounts", {
      fetchImpl: async () => {
        throw new Error("breaker should block before fetch");
      },
      breaker,
      retryDelaysMs: [],
      logger: {},
    }),
    (error) => {
      assert.equal(error?.code, "api_request_breaker_open");
      assert.equal(error?.status, 503);
      assert.equal(error?.retryAfterMs, 5_000);
      return true;
    },
  );

  const writePayload = await requestJSON("/tasks", {
    method: "POST",
    body: { task_id: "task-1" },
    breaker,
    fetchImpl: async () => ({
      ok: true,
      text: async () => `{"accepted":true}`,
    }),
  });
  assert.deepEqual(writePayload, { accepted: true });

  now += 5_001;
  const readPayload = await requestJSON("/accounts", {
    breaker,
    fetchImpl: async () => ({
      ok: true,
      text: async () => `{"accounts":[]}`,
    }),
  });
  assert.deepEqual(readPayload, { accounts: [] });
});

test("requestJSON demotes external upstream gateway failures without opening breaker", async () => {
  const logs = [];
  let breakerFailures = 0;
  const breaker = {
    remainingMs: () => 0,
    recordFailure: () => {
      breakerFailures += 1;
      return 0;
    },
    recordSuccess: () => false,
  };

  await assert.rejects(
    () => requestJSON("/platform/options", {
      fetchImpl: async () => ({
        ok: false,
        status: 502,
        text: async () => `{"detail":"bad gateway"}`,
      }),
      breaker,
      retryDelaysMs: [],
      logger: {
        warn(module, action, detail, extra) {
          logs.push({ level: "warn", module, action, detail, extra });
        },
        error(module, action, detail, extra) {
          logs.push({ level: "error", module, action, detail, extra });
        },
      },
    }),
    /bad gateway/,
  );

  assert.equal(breakerFailures, 0);
  assert.equal(logs.length, 1);
  assert.equal(logs[0].level, "warn");
  assert.equal(logs[0].action, "/platform/options");
  assert.equal(logs[0].extra.status, 502);
});

test("requestJSON logs API errors before throwing", async () => {
  const logs = [];
  const fetchImpl = async () => ({
    ok: false,
    status: 502,
    text: async () => `{"detail":"bad gateway"}`,
  });
  await assert.rejects(
    () => requestJSON("/admin/stats/overview", {
      fetchImpl,
      retryDelaysMs: [],
      logger: {
        error(module, action, detail, extra) {
          logs.push({ level: "error", module, action, detail, extra });
        },
      },
    }),
    /bad gateway/,
  );

  assert.equal(logs.length, 1);
  assert.equal(logs[0].module, "api");
  assert.equal(logs[0].action, "/admin/stats/overview");
  assert.equal(logs[0].extra.status, 502);
});

test("requestJSON skips logging abort errors", async () => {
  const logs = [];
  const fetchImpl = async () => {
    const error = new Error("operation aborted");
    error.name = "AbortError";
    throw error;
  };
  await assert.rejects(
    () => requestJSON("/cs/workbench/bootstrap", {
      fetchImpl,
      logger: {
        error(...args) {
          logs.push(args);
        },
      },
    }),
    /operation aborted/,
  );
  assert.equal(logs.length, 0);
});
