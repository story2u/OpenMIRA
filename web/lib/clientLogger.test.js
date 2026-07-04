import assert from "node:assert/strict";
import test from "node:test";
import {
  buildClientLogItem,
  createClientLogger,
  sanitizeLogExtra,
  shouldDemoteClientRuntimeLog,
  trimLogText,
} from "./clientLogger.js";

test("sanitizeLogExtra masks sensitive keys recursively", () => {
  const sanitized = sanitizeLogExtra({
    token: "secret-token",
    nested: {
      authorization: "Bearer abc",
      safe: "value",
    },
    list: [{ password: "hidden" }],
  });

  assert.equal(sanitized.token, "***");
  assert.equal(sanitized.nested.authorization, "***");
  assert.equal(sanitized.nested.safe, "value");
  assert.equal(sanitized.list[0].password, "***");
});

test("shouldDemoteClientRuntimeLog recognizes non fatal runtime patterns", () => {
  assert.equal(shouldDemoteClientRuntimeLog("ChunkLoadError: failed to fetch dynamically imported module"), true);
  assert.equal(shouldDemoteClientRuntimeLog("TypeError: cannot read property"), false);
});

test("buildClientLogItem trims fields and reads operator context", () => {
  const item = buildClientLogItem({
    level: "error",
    module: "runtime",
    action: "window.onerror",
    detail: "boom",
    now: new Date("2026-07-01T00:00:00.000Z"),
    readStorage: (key) => {
      if (key === "cloud.assignee_id") return "cs-1";
      if (key === "cloud.tenant_id") return "tenant-1";
      return "";
    },
  });

  assert.equal(item.ts, "2026-07-01T00:00:00.000Z");
  assert.equal(item.level, "ERROR");
  assert.equal(item.operator, "cs-1");
  assert.equal(item.tenant_id, "tenant-1");
  assert.equal(trimLogText("x".repeat(900)).endsWith("...(truncated)"), true);
});

test("createClientLogger flushes JSON batch with bearer token", async () => {
  const calls = [];
  const logger = createClientLogger({
    autoFlush: false,
    now: () => new Date("2026-07-01T00:00:00.000Z"),
    readStorage: (key) => {
      if (key === "wework.adminToken") return "Bearer admin-token";
      return "";
    },
    fetchImpl: () => async (url, init) => {
      calls.push({ url, init });
      return { ok: true };
    },
  });

  assert.equal(logger.error("runtime", "window.onerror", "boom", { token: "secret" }), true);
  const result = await logger.flush();

  assert.deepEqual(result, { accepted: 1, dropped: 0 });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "/api/v1/client-logs");
  assert.equal(calls[0].init.headers.Authorization, "Bearer admin-token");
  const body = JSON.parse(calls[0].init.body);
  assert.equal(body.logs[0].level, "ERROR");
  assert.equal(body.logs[0].extra.token, "***");
});

test("createClientLogger drops duplicate fingerprints inside window", async () => {
  let current = new Date("2026-07-01T00:00:00.000Z");
  const logger = createClientLogger({
    autoFlush: false,
    now: () => current,
    readStorage: () => "",
    fetchImpl: () => async () => ({ ok: true }),
  });

  assert.equal(logger.warn("api", "request_failed", "HTTP 500"), true);
  assert.equal(logger.warn("api", "request_failed", "HTTP 500"), false);
  current = new Date("2026-07-01T00:00:06.000Z");
  assert.equal(logger.warn("api", "request_failed", "HTTP 500"), true);
  assert.equal(logger.queueLength(), 2);
});
