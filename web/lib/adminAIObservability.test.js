import assert from "node:assert/strict";
import test from "node:test";

import {
  AI_REPLY_BREAKDOWN_PATH,
  AI_REPLY_LOGS_PATH,
  AI_REPLY_OVERVIEW_PATH,
  AI_REPLY_TREND_PATH,
  buildAIReplyBreakdownRequest,
  buildAIReplyLogsRequest,
  buildAIReplyOverviewRequest,
  buildAIReplyTrendRequest,
  defaultAIReplyLogFilters,
  defaultAIReplyStatsFilters,
  formatAIDurationMS,
  formatAIRate,
  normalizeAIReplyBreakdown,
  normalizeAIReplyLogs,
  normalizeAIReplyOverview,
  normalizeAIReplyTrend,
} from "./adminAIObservability.js";

test("buildAIReplyLogsRequest mirrors legacy query filters", () => {
  const request = buildAIReplyLogsRequest({
    scope: " coze-main ",
    status: "failed",
    keyword: " 客户 ",
    date: "2026-07-02",
    page: "2",
    pageSize: "100",
  });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, AI_REPLY_LOGS_PATH);
  assert.deepEqual(request.params, {
    scope: "coze-main",
    keyword: "客户",
    status: "failed",
    date: "2026-07-02",
    page: "2",
    page_size: "100",
  });
  assert.equal(defaultAIReplyLogFilters().scope, "local");
});

test("buildAIReplyStatsRequests keep stats paths and bounds", () => {
  assert.deepEqual(buildAIReplyOverviewRequest({ date: "2026-07-02" }), {
    ok: true,
    method: "GET",
    path: AI_REPLY_OVERVIEW_PATH,
    params: { date: "2026-07-02" },
  });
  assert.deepEqual(buildAIReplyTrendRequest({ days: "120" }), {
    ok: true,
    method: "GET",
    path: AI_REPLY_TREND_PATH,
    params: { days: "90" },
  });
  assert.deepEqual(buildAIReplyBreakdownRequest({}), {
    ok: true,
    method: "GET",
    path: AI_REPLY_BREAKDOWN_PATH,
    params: {},
  });
  assert.equal(defaultAIReplyStatsFilters().days, "7");
});

test("normalizeAIReplyLogs keeps payload fields and pagination", () => {
  const result = normalizeAIReplyLogs({
    scope: "local",
    filters: { keyword: "客户", status: "failed", date: "2026-07-02" },
    logs: [
      {
        attempt_id: "attempt-1",
        trace_id: "trace-1",
        incoming_trace_id: "incoming-1",
        task_id: "task-1",
        workflow_id: "wf-1",
        model: "deepseek-chat",
        trigger_event: "message.created",
        reply_time: "2026-07-02T10:00:00+08:00",
        assignee_id: "cs-1",
        assignee_name: "消息端一",
        account_id: "acc-1",
        account_name: "企微一",
        receiver_name: "客户一",
        conversation_id: "conv-1",
        customer_message: "你好",
        content: "您好",
        status: "failed",
        failure_type: "llm_timeout",
        provider_error: "timeout",
        user_facing_error: "稍后再试",
        message_missing: false,
        customer_message_missing: true,
      },
    ],
    pagination: { page: 2, page_size: 50, total: 51, total_pages: 2 },
  });

  assert.equal(result.scope, "local");
  assert.deepEqual(result.filters, { keyword: "客户", status: "failed", date: "2026-07-02" });
  assert.deepEqual(result.pagination, { page: 2, pageSize: 50, total: 51, totalPages: 2 });
  assert.equal(result.logs[0].attemptId, "attempt-1");
  assert.equal(result.logs[0].statusLabel, "失败");
  assert.equal(result.logs[0].customerMessageMissing, true);
  assert.equal(result.logs[0].providerError, "timeout");
});

test("normalizeAIReplyStats keeps overview, trend and breakdown", () => {
  const overview = normalizeAIReplyOverview({
    date: "2026-07-02",
    attempts: 10,
    success_count: 7,
    sent_count: 6,
    unreplyable_count: 1,
    failed_count: 2,
    send_failed_count: 1,
    avg_ai_call_duration_ms: 123.4,
    avg_total_duration_ms: null,
  });
  assert.equal(overview.date, "2026-07-02");
  assert.equal(overview.successCount, 7);
  assert.equal(overview.sentRate, 0.6);
  assert.equal(formatAIRate(overview.successRate), "70.0%");
  assert.equal(formatAIDurationMS(overview.avgAICallDurationMS), "123ms");
  assert.equal(formatAIDurationMS(overview.avgTotalDurationMS), "-");

  const trend = normalizeAIReplyTrend({
    data: [
      { day: "2026-07-01", date: "07-01", attempts: 0 },
      { day: "2026-07-02", date: "07-02", attempts: 10, sent_count: 6 },
    ],
  });
  assert.equal(trend.length, 2);
  assert.equal(trend[1].day, "2026-07-02");
  assert.equal(trend[1].sentCount, 6);

  const breakdown = normalizeAIReplyBreakdown({
    date: "2026-07-02",
    items: [
      { failure_type: "llm_timeout", count: 3 },
      { failure_type: "", count: 1 },
    ],
  });
  assert.equal(breakdown.date, "2026-07-02");
  assert.deepEqual(breakdown.items.map((item) => [item.failureType, item.count]), [["llm_timeout", 3], ["unknown", 1]]);
});
