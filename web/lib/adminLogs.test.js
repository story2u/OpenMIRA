import assert from "node:assert/strict";
import test from "node:test";

import {
  AUDIT_LOGS_PATH,
  SYSTEM_LOGS_PATH,
  buildAuditLogsRequest,
  buildSystemLogsRequest,
  defaultAuditLogFilters,
  defaultSystemLogFilters,
  normalizeAuditLogs,
  normalizeSystemLogs,
} from "./adminLogs.js";

test("buildAuditLogsRequest mirrors legacy filters and clamps page size", () => {
  assert.deepEqual(defaultAuditLogFilters(), {
    operator: "",
    actionType: "all",
    date: "",
    page: "1",
    pageSize: "20",
  });

  const request = buildAuditLogsRequest({
    operator: " admin ",
    actionType: " config ",
    date: "2026-07-02",
    page: "0",
    pageSize: "500",
  });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, AUDIT_LOGS_PATH);
  assert.deepEqual(request.params, {
    page: 1,
    page_size: 100,
    operator: "admin",
    action_type: "config",
    date: "2026-07-02",
  });
});

test("normalizeAuditLogs keeps records and pagination", () => {
  const result = normalizeAuditLogs({
    logs: [{
      log_id: "log-1",
      operator: "admin",
      action_type: "config",
      detail: "更新配置",
      ip: "127.0.0.1",
      created_at: "2026-07-02T10:00:00Z",
    }],
    pagination: { page: 2, page_size: 20, total: 21, total_pages: 2 },
  });

  assert.equal(result.logs.length, 1);
  assert.equal(result.logs[0].actionType, "config");
  assert.equal(result.logs[0].createdAt, "2026-07-02T10:00:00Z");
  assert.equal(result.pagination.page, 2);
  assert.equal(result.pagination.totalPages, 2);
});

test("buildSystemLogsRequest mirrors legacy query filters and offset bounds", () => {
  assert.deepEqual(defaultSystemLogFilters(), {
    date: "",
    level: "all",
    module: "",
    keyword: "",
    limit: "200",
    offset: "0",
  });

  const request = buildSystemLogsRequest({
    date: "2026-07-02",
    level: "warn,error",
    module: " api ",
    keyword: " timeout ",
    limit: "999",
    offset: "-1",
  });

  assert.equal(request.ok, true);
  assert.equal(request.path, SYSTEM_LOGS_PATH);
  assert.deepEqual(request.params, {
    limit: 500,
    offset: 0,
    date: "2026-07-02",
    level: "warn,error",
    module: "api",
    keyword: "timeout",
  });
});

test("normalizeSystemLogs keeps jsonl fields and pagination hints", () => {
  const result = normalizeSystemLogs({
    items: [{
      ts: "2026-07-02T10:03:04+08:00",
      level: "ERROR",
      module: "client.runtime",
      action: "/admin",
      detail: "timeout",
      operator: "admin-001",
      extra: { visible: true },
    }],
    total: 3,
    date: "2026-07-02",
  }, { limit: 1, offset: 1 });

  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].timestamp, "2026-07-02T10:03:04+08:00");
  assert.equal(result.items[0].detail, "timeout");
  assert.equal(result.items[0].extra.visible, true);
  assert.equal(result.hasPrevious, true);
  assert.equal(result.hasNext, true);
});
