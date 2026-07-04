import assert from "node:assert/strict";
import test from "node:test";

import {
  SOP_FLOWS_PATH,
  SOP_POLICIES_PATH,
  TARGET_AUDIENCE_ALL,
  TARGET_AUDIENCE_NONE,
  buildSOPFlowDeleteMutation,
  buildSOPFlowForm,
  buildSOPFlowUpsertMutation,
  buildSOPPoliciesListRequest,
  buildSOPPolicyDeleteMutation,
  buildSOPPolicyForm,
  buildSOPPolicyUpsertMutation,
  defaultSOPFlowForm,
  defaultSOPPolicyForm,
  normalizeAdminSOPFlows,
  normalizeAdminSOPPolicies,
  normalizeSOPExecutionTimeWindows,
  normalizeSOPMessages,
  stringifySOPTargetAudience,
} from "./adminSOPConfig.js";

test("normalizeAdminSOPFlows keeps flow config fields and legacy all audience", () => {
  const flows = normalizeAdminSOPFlows({
    flows: [
      {
        flow_id: "default",
        flow_name: "Default Flow",
        target_audience: "",
        execution_mode: "platform_pull",
        day_count: 3,
        platform_pull_driver: "platform_task",
        platform_task_limit: 30,
        platform_dispatch_queue: "fast",
        platform_task_url: "https://ops.example/tasks",
        execution_time_windows: "[{\"start\":\"09:00\",\"end\":\"18:00\"}]",
        enabled: false,
        human_handoff_rule: "risk",
        risk_keywords: "refund",
        updated_at: "2026-07-02T01:02:03Z",
      },
      { flow_name: "missing id" },
    ],
  });

  assert.equal(flows.length, 1);
  assert.equal(flows[0].flowId, "default");
  assert.equal(flows[0].targetAudience, TARGET_AUDIENCE_ALL);
  assert.equal(flows[0].targetAudienceMode, "all");
  assert.equal(flows[0].executionMode, "platform_pull");
  assert.equal(flows[0].platformPullDriver, "platform_task");
  assert.equal(flows[0].platformDispatchQueueLabel, "fast 快通道");
  assert.deepEqual(flows[0].executionWindows, [{ start: "09:00", end: "18:00" }]);
  assert.equal(flows[0].executionWindowsText, "09:00-18:00");
  assert.equal(flows[0].enabled, false);
  assert.equal(flows[0].updatedAt, "2026-07-02T01:02:03Z");
});

test("buildSOPFlowForm maps API rows to editable form state", () => {
  const form = buildSOPFlowForm({
    flow_id: "flow-a",
    flow_name: "Flow A",
    target_audience: "cs-1,cs-2",
    day_count: 2,
    execution_time_windows: [{ start: "10:00", end: "20:00" }],
    enabled: true,
  });

  assert.equal(form.flowId, "flow-a");
  assert.equal(form.targetAudienceMode, "specific");
  assert.equal(form.targetAudienceIds, "cs-1\ncs-2");
  assert.equal(form.executionWindowsText, "10:00-20:00");
  assert.equal(form.editing, true);
  assert.equal(defaultSOPFlowForm().enabled, false);
});

test("buildSOPFlowUpsertMutation mirrors Go flow write payload", () => {
  const mutation = buildSOPFlowUpsertMutation({
    flowId: " flow-a ",
    flowName: " Flow A ",
    targetAudienceMode: "specific",
    targetAudienceIds: " cs-1\ncs-2,cs-1 ",
    executionMode: "platform_pull",
    dayCount: "2",
    platformPullDriver: "platform_task",
    platformTaskLimit: "30",
    platformDispatchQueue: "fast",
    platformTaskURL: " https://ops.example/tasks ",
    executionWindowsText: "09:00-18:00\ninvalid\n20:00~22:00",
    enabled: true,
    humanHandoffRule: " human ",
    riskKeywords: " risk ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, SOP_FLOWS_PATH);
  assert.deepEqual(mutation.body, {
    flow_id: "flow-a",
    flow_name: "Flow A",
    target_audience: "cs-1,cs-2",
    execution_mode: "platform_pull",
    day_count: 2,
    platform_pull_driver: "platform_task",
    platform_task_limit: 30,
    platform_dispatch_queue: "fast",
    platform_task_url: "https://ops.example/tasks",
    execution_time_windows: [
      { start: "09:00", end: "18:00" },
      { start: "20:00", end: "22:00" },
    ],
    enabled: true,
    human_handoff_rule: "human",
    risk_keywords: "risk",
  });
});

test("flow helpers validate protected and missing fields", () => {
  assert.equal(buildSOPFlowUpsertMutation({ flowName: "Flow" }).error, "flow_id_required");
  assert.equal(buildSOPFlowUpsertMutation({ flowId: "flow-a", enabled: true }).error, "target_audience_required");
  assert.equal(buildSOPFlowDeleteMutation("").error, "flow_id_required");
  assert.equal(buildSOPFlowDeleteMutation("default").error, "default_flow_protected");

  const mutation = buildSOPFlowDeleteMutation("flow/a");
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${SOP_FLOWS_PATH}/flow%2Fa`);
});

test("normalizeAdminSOPPolicies reads direct and grouped policy payloads", () => {
  const direct = normalizeAdminSOPPolicies({
    policies: [
      {
        policy_id: "policy-1",
        flow_id: "flow-a",
        name: "DAY1",
        day_stage: "day1",
        customer_state: "paid",
        dispatch_queue: "fast",
        trigger_event: "incoming_message",
        reply_mode: "sop_ai_rewrite",
        reply_text: "hello",
        image_urls: "local://welcome.png",
        need_rag: true,
        enabled: false,
      },
    ],
  });

  assert.equal(direct.length, 1);
  assert.equal(direct[0].policyId, "policy-1");
  assert.equal(direct[0].customerStateLabel, "已定");
  assert.equal(direct[0].dispatchQueueLabel, "fast 快通道");
  assert.equal(direct[0].replyModeLabel, "SOP + AI 改写");
  assert.equal(direct[0].enabled, false);
  assert.deepEqual(direct[0].messages.map((item) => item.type), ["text", "image"]);

  const grouped = normalizeAdminSOPPolicies({
    flows: [
      {
        flow_id: "flow-b",
        policies: [{ policy_id: "policy-2", name: "DAY2", day_stage: "day2", trigger_event: "friend_added", prompt_template: "prompt" }],
      },
    ],
  });
  assert.equal(grouped.length, 1);
  assert.equal(grouped[0].flowId, "flow-b");
  assert.equal(grouped[0].policyId, "policy-2");
});

test("buildSOPPolicyForm maps policy rows to editable state", () => {
  const form = buildSOPPolicyForm({
    policy_id: "policy-1",
    flow_id: "flow-a",
    name: "DAY1",
    day_stage: "day1",
    trigger_event: "incoming_message",
    message_sequence: "[{\"type\":\"text\",\"content\":\"hello\"},{\"type\":\"video\",\"content\":\"local://clip.mp4\"}]",
    enabled: true,
    priority: 5,
  });

  assert.equal(form.policyId, "policy-1");
  assert.equal(form.flowId, "flow-a");
  assert.equal(form.replyText, "hello");
  assert.equal(form.videoURLs, "local://clip.mp4");
  assert.equal(form.priority, "5");
  assert.equal(form.editing, true);
  assert.equal(defaultSOPPolicyForm("flow-a").name, "flow-a-Day1");
});

test("buildSOPPoliciesListRequest mirrors query filters", () => {
  const request = buildSOPPoliciesListRequest({ flowId: "flow-a", dayStage: "day2" });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, SOP_POLICIES_PATH);
  assert.deepEqual(request.params, { flow_id: "flow-a", day_stage: "day2" });
  assert.deepEqual(buildSOPPoliciesListRequest({ flowId: "all", dayStage: "all" }).params, {});
});

test("buildSOPPolicyUpsertMutation mirrors Go policy write payload", () => {
  const mutation = buildSOPPolicyUpsertMutation({
    policyId: " policy-1 ",
    flowId: " flow-a ",
    name: " DAY1 ",
    dayStage: " day1 ",
    stageTag: " paid_confirmation ",
    customerState: "paid",
    dispatchQueue: "fast",
    triggerEvent: " incoming_message ",
    enabled: false,
    priority: "5",
    replyMode: "sop_ai_rewrite",
    replyText: " hello ",
    imageURLs: " local://welcome.png ",
    videoURLs: " local://clip.mp4 ",
    needRAG: true,
    needAIRewrite: true,
    mediaStrategy: "tagged",
    humanHandoffRule: " human ",
    riskKeywords: " risk ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, SOP_POLICIES_PATH);
  assert.deepEqual(mutation.body, {
    policy_id: "policy-1",
    flow_id: "flow-a",
    name: "DAY1",
    day_stage: "day1",
    stage_tag: "paid_confirmation",
    customer_state: "paid",
    dispatch_queue: "fast",
    trigger_event: "incoming_message",
    enabled: false,
    priority: 5,
    reply_mode: "sop_ai_rewrite",
    prompt_template: "",
    reply_text: "hello",
    image_urls: "local://welcome.png",
    video_urls: "local://clip.mp4",
    message_sequence: JSON.stringify([
      { type: "text", content: "hello" },
      { type: "image", content: "local://welcome.png" },
      { type: "video", content: "local://clip.mp4" },
    ]),
    need_rag: true,
    need_ai_rewrite: true,
    media_strategy: "tagged",
    human_handoff_rule: "human",
    risk_keywords: "risk",
  });
});

test("policy helpers validate required fields and encode delete path", () => {
  assert.equal(buildSOPPolicyUpsertMutation({ name: "DAY1", triggerEvent: "incoming_message", replyText: "hello" }).error, "day_stage_required");
  assert.equal(buildSOPPolicyUpsertMutation({ dayStage: "day1", triggerEvent: "incoming_message", replyText: "hello" }).error, "name_required");
  assert.equal(buildSOPPolicyUpsertMutation({ name: "DAY1", dayStage: "day1", replyText: "hello" }).error, "trigger_event_required");
  assert.equal(buildSOPPolicyUpsertMutation({ name: "DAY1", dayStage: "day1", triggerEvent: "incoming_message" }).error, "reply_content_required");
  assert.equal(buildSOPPolicyDeleteMutation("").error, "policy_id_required");

  const mutation = buildSOPPolicyDeleteMutation("policy/1");
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${SOP_POLICIES_PATH}/policy%2F1`);
});

test("SOP parsing helpers keep stable compact formats", () => {
  assert.deepEqual(normalizeSOPExecutionTimeWindows("09:00-18:00\nbad\n20:00至22:00"), [
    { start: "09:00", end: "18:00" },
    { start: "20:00", end: "22:00" },
  ]);
  assert.deepEqual(normalizeSOPMessages("", "hello", "local://a.png", "local://b.mp4"), [
    { type: "text", content: "hello", preview_url: "" },
    { type: "image", content: "local://a.png", preview_url: "" },
    { type: "video", content: "local://b.mp4", preview_url: "" },
  ]);
  assert.equal(stringifySOPTargetAudience("all"), TARGET_AUDIENCE_ALL);
  assert.equal(stringifySOPTargetAudience("none"), TARGET_AUDIENCE_NONE);
  assert.equal(stringifySOPTargetAudience("specific", "cs-1,cs-1\ncs-2"), "cs-1,cs-2");
});
