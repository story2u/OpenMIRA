import assert from "node:assert/strict";
import test from "node:test";

import {
  SOP_DISPATCH_RESEND_PATH,
  SOP_DISPATCH_TASKS_PATH,
  SOP_FACTS_PATH,
  SOP_PLATFORM_TEST_PATH,
  SOP_STAGE_STATS_PATH,
  buildSOPDispatchResendMutation,
  buildSOPDispatchTasksRequest,
  buildSOPFactsRequest,
  buildSOPPlatformTestMutation,
  buildSOPStageStatsRequest,
  defaultSOPAnalyticsFilters,
  defaultSOPDispatchTaskFilters,
  formatSOPRate,
  formatSOPTaskStatusCounts,
  isSOPDispatchBatchResendable,
  normalizeSOPDispatchResendResult,
  normalizeSOPDispatchTasks,
  normalizeSOPFacts,
  normalizeSOPPlatformTestResult,
  normalizeSOPStageStats,
} from "./adminSOPOperations.js";

test("buildSOPDispatchTasksRequest mirrors legacy query filtering", () => {
  const request = buildSOPDispatchTasksRequest({
    date: "2026-07-02",
    flowId: "formal",
    status: "failed",
    keyword: "客户",
    page: "2",
    pageSize: "30",
  });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, SOP_DISPATCH_TASKS_PATH);
  assert.deepEqual(request.params, {
    date: "2026-07-02",
    flow_id: "formal",
    status: "failed",
    keyword: "客户",
    page: "2",
    page_size: "30",
  });
  assert.equal(defaultSOPDispatchTaskFilters().status, "all");
});

test("normalizeSOPDispatchTasks keeps batches, details and pagination", () => {
  const result = normalizeSOPDispatchTasks({
    batches: [
      {
        task_id: "task-1",
        batch_id: "batch-1",
        ai_trace_id: "trace-1",
        flow_id: "formal",
        flow_name: "正式 SOP",
        conversation_id: "conv-1",
        sender_name: "客户 A",
        assignee_name: "客服 A",
        account_id: "acc-1",
        day_stage: "day1",
        status_counts: { failed: 1, success: 2 },
        task_status: "failed",
        task_error: "network timeout",
        action_preview: [{ type: "text", content_preview: "hello" }],
        details: [
          {
            task_id: "task-1",
            stage_unique_id: "stage-1",
            message_details: [{ message_index: 1, type: "text", content: "hello", task_status: "failed" }],
            customer_replied: true,
          },
        ],
        can_resend: true,
        created_at: "2026-07-02T10:00:00+08:00",
      },
    ],
    tasks: [{ task_id: "task-1", task_status: "failed" }],
    pagination: { page: 2, page_size: 30, total: 31, total_pages: 2 },
  });

  assert.equal(result.batches.length, 1);
  assert.equal(result.batches[0].taskId, "task-1");
  assert.equal(result.batches[0].taskStatusLabel, "失败");
  assert.equal(result.batches[0].actionPreview[0].contentPreview, "hello");
  assert.equal(result.batches[0].details[0].messageDetails[0].content, "hello");
  assert.equal(result.tasks[0].taskId, "task-1");
  assert.deepEqual(result.pagination, { page: 2, pageSize: 30, total: 31, totalPages: 2 });
  assert.equal(isSOPDispatchBatchResendable(result.batches[0]), true);
  assert.equal(formatSOPTaskStatusCounts(result.batches[0]), "成功 2 / 失败 1");
});

test("buildSOPDispatchResendMutation validates flow and task ids", () => {
  assert.equal(buildSOPDispatchResendMutation({ taskId: "task-1" }).error, "flow_id_required");
  assert.equal(buildSOPDispatchResendMutation({ flowId: "formal" }).error, "task_id_required");

  const selected = buildSOPDispatchResendMutation({
    flowId: " formal ",
    taskIds: ["task-1", "task-1", " task-2 "],
    date: "2026-07-02",
    limit: "5",
  });
  assert.equal(selected.ok, true);
  assert.equal(selected.method, "POST");
  assert.equal(selected.path, SOP_DISPATCH_RESEND_PATH);
  assert.deepEqual(selected.body, {
    flow_id: "formal",
    all_failed: false,
    task_ids: ["task-1", "task-2"],
    limit: 5,
    date: "2026-07-02",
  });

  const allFailed = buildSOPDispatchResendMutation({ flowId: "formal", allFailed: true });
  assert.deepEqual(allFailed.body.task_ids, []);
  assert.equal(allFailed.body.all_failed, true);
});

test("normalizeSOPDispatchResendResult keeps counters and result rows", () => {
  const result = normalizeSOPDispatchResendResult({
    success: false,
    date: "2026-07-02",
    flow_id: "formal",
    requested: 2,
    succeeded: 1,
    failed: 1,
    results: [
      { success: true, original_task_id: "task-1", resend_task_id: "sop-resend-1", status: "queued" },
      { success: false, original_task_id: "task-2", error: "missing persisted actions" },
    ],
  });

  assert.equal(result.success, false);
  assert.equal(result.flowId, "formal");
  assert.equal(result.succeeded, 1);
  assert.equal(result.failed, 1);
  assert.equal(result.results[0].resendTaskId, "sop-resend-1");
  assert.equal(result.results[1].error, "missing persisted actions");
});

test("SOP analytics request helpers and normalizers keep stage/fact shape", () => {
  const stageRequest = buildSOPStageStatsRequest({ date: "2026-07-02", flowId: "formal" });
  assert.equal(stageRequest.path, SOP_STAGE_STATS_PATH);
  assert.deepEqual(stageRequest.params, { date: "2026-07-02", flow_id: "formal" });

  const stageStats = normalizeSOPStageStats({
    date: "2026-07-02",
    flow_id: "formal",
    items: [
      {
        flow_id: "formal",
        stage_unique_id: "stage-1",
        stage_name: "Day1",
        delivered_customer_count: 10,
        customer_open_rate: 0.3,
        ai_reply_rate: 0.2,
      },
    ],
  });
  assert.equal(stageStats.items.length, 1);
  assert.equal(stageStats.items[0].stageUniqueId, "stage-1");
  assert.equal(stageStats.items[0].customerOpenRate, 0.3);
  assert.equal(formatSOPRate(stageStats.items[0].aiReplyRate), "20.0%");

  const factsRequest = buildSOPFactsRequest({
    date: "2026-07-02",
    flowId: "formal",
    stageUniqueId: "stage-1",
    status: "success",
    keyword: "trace",
    page: "2",
    pageSize: "50",
  });
  assert.equal(factsRequest.path, SOP_FACTS_PATH);
  assert.deepEqual(factsRequest.params, {
    date: "2026-07-02",
    flow_id: "formal",
    stage_unique_id: "stage-1",
    status: "success",
    keyword: "trace",
    page: "2",
    page_size: "50",
  });
  assert.equal(defaultSOPAnalyticsFilters().pageSize, "30");

  const facts = normalizeSOPFacts({
    items: [
      {
        fact_id: "fact-1",
        task_id: "task-1",
        flow_id: "formal",
        stage_unique_id: "stage-1",
        delivery_status: "success",
        message_count: 2,
        customer_replied: true,
      },
    ],
    pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
  });
  assert.equal(facts.items[0].deliveryStatusLabel, "成功");
  assert.equal(facts.items[0].customerReplied, true);
  assert.equal(facts.pagination.pageSize, 50);
});

test("SOP platform test helper validates task URL and result", () => {
  assert.equal(buildSOPPlatformTestMutation({}).error, "task_url_required");

  const mutation = buildSOPPlatformTestMutation({ taskURL: " https://platform.example/tasks " });
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, SOP_PLATFORM_TEST_PATH);
  assert.deepEqual(mutation.body, { task_url: "https://platform.example/tasks" });

  const result = normalizeSOPPlatformTestResult({ success: true, message: "连接成功 (HTTP 200)" });
  assert.equal(result.success, true);
  assert.equal(result.message, "连接成功 (HTTP 200)");
});
