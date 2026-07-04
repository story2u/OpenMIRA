import assert from "node:assert/strict";
import test from "node:test";

import {
  AI_CONFIG_PATH,
  AI_CONFIG_TEST_PATH,
  AI_CONFIG_TEST_DIALOGUE_PATH,
  buildKnowledgeDialogueMutation,
  buildAIConfigUpsertMutation,
  buildAIConfigTestMutation,
  formatAIProfileJSON,
  normalizeAIConfigTestResult,
  normalizeKnowledgeDialogueResult,
  normalizeAdminAIConfig,
  normalizeAdminAIConfigRecords,
} from "./adminAIConfig.js";

test("normalizeAdminAIConfig keeps legacy config fields", () => {
  const config = normalizeAdminAIConfig({
    config: {
      enabled: "true",
      base_url: " https://ai.example/v1 ",
      model: " deepseek-chat ",
      timeout_sec: "30",
      temperature: "0.3",
      system_prompt: " prompt ",
      local_target_scope: "account",
      local_target_account_ids: [" acc-1 ", "acc-2"],
      local_default_ai_enabled: true,
      api_key_set: true,
      active_coze_profile_id: "coze-a",
      coze_profiles: [{ profile_id: "coze-a", token_set: true }],
      xiaobei_profiles: "[{\"profile_id\":\"xb-a\"}]",
    },
  });

  assert.equal(config.enabled, true);
  assert.equal(config.enabledLabel, "开启");
  assert.equal(config.baseUrl, "https://ai.example/v1");
  assert.equal(config.model, "deepseek-chat");
  assert.equal(config.timeoutSec, 30);
  assert.equal(config.temperature, 0.3);
  assert.equal(config.systemPrompt, "prompt");
  assert.equal(config.localTargetScope, "account");
  assert.deepEqual(config.localTargetAccountIds, ["acc-1", "acc-2"]);
  assert.equal(config.localDefaultAIEnabled, true);
  assert.equal(config.apiKeySet, true);
  assert.equal(config.activeCozeProfileId, "coze-a");
  assert.deepEqual(config.cozeProfiles, [{ profile_id: "coze-a", token_set: true }]);
  assert.deepEqual(config.xiaobeiProfiles, [{ profile_id: "xb-a" }]);
});

test("normalizeAdminAIConfigRecords rebuilds config from dashboard rows", () => {
  const config = normalizeAdminAIConfigRecords([
    { key: "enabled", value: false },
    { key: "base_url", value: "https://ai.example" },
    { key: "model", value: "model-a" },
  ]);

  assert.equal(config.enabled, false);
  assert.equal(config.baseUrl, "https://ai.example");
  assert.equal(config.model, "model-a");
});

test("buildAIConfigUpsertMutation mirrors legacy POST body", () => {
  const mutation = buildAIConfigUpsertMutation({
    enabled: true,
    baseUrl: " https://ai.example/v1 ",
    model: " deepseek-chat ",
    timeoutSec: "30",
    temperature: "0.3",
    systemPrompt: " prompt ",
    interceptKeywords: "投诉,退款",
    defaultHandoffReply: "转人工",
    localTargetAudience: "cs-1，cs-2",
    localTargetScope: "assignee",
    localTargetAccountIds: "acc-1, acc-2",
    localDefaultAIEnabled: true,
    apiKey: "",
    activeCozeProfileId: "coze-a",
    cozeProfiles: "[{\"profile_id\":\"coze-a\",\"enabled\":false}]",
    activeXiaobeiProfileId: "xb-a",
    xiaobeiProfiles: [{ profile_id: "xb-a", enabled: true }],
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, AI_CONFIG_PATH);
  assert.deepEqual(mutation.body, {
    enabled: true,
    base_url: "https://ai.example/v1",
    model: "deepseek-chat",
    timeout_sec: 30,
    temperature: 0.3,
    system_prompt: "prompt",
    intercept_keywords: "投诉,退款",
    default_handoff_reply: "转人工",
    local_target_audience: "cs-1，cs-2",
    local_target_scope: "assignee",
    local_target_account_ids: ["acc-1", "acc-2"],
    local_default_ai_enabled: true,
    active_coze_profile_id: "coze-a",
    coze_profiles: [{ profile_id: "coze-a", enabled: false }],
    active_xiaobei_profile_id: "xb-a",
    xiaobei_profiles: [{ profile_id: "xb-a", enabled: true }],
  });
});

test("buildAIConfigUpsertMutation reports invalid fields", () => {
  assert.equal(buildAIConfigUpsertMutation({ model: "m", timeoutSec: 1, temperature: 1 }).error, "base_url_required");
  assert.equal(buildAIConfigUpsertMutation({ baseUrl: "https://ai", timeoutSec: 1, temperature: 1 }).error, "model_required");
  assert.equal(buildAIConfigUpsertMutation({ baseUrl: "https://ai", model: "m", timeoutSec: 0, temperature: 1 }).error, "timeout_invalid");
  assert.equal(buildAIConfigUpsertMutation({ baseUrl: "https://ai", model: "m", timeoutSec: 1, temperature: 3 }).error, "temperature_invalid");
  assert.equal(buildAIConfigUpsertMutation({ baseUrl: "https://ai", model: "m", timeoutSec: 1, temperature: 1, cozeProfiles: "{" }).error, "coze_profiles_invalid");
  assert.equal(buildAIConfigUpsertMutation({ baseUrl: "https://ai", model: "m", timeoutSec: 1, temperature: 1, xiaobeiProfiles: "{}" }).error, "xiaobei_profiles_invalid");
});

test("buildAIConfigTestMutation mirrors legacy test endpoint", () => {
  const mutation = buildAIConfigTestMutation({
    prompt: " 请回复 pong ",
    baseUrl: " https://ai.example/v1 ",
    model: " deepseek-chat ",
    timeoutSec: "20",
    temperature: "0.4",
    systemPrompt: " 系统提示 ",
    apiKey: " key ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, AI_CONFIG_TEST_PATH);
  assert.deepEqual(mutation.body, {
    prompt: "请回复 pong",
    base_url: "https://ai.example/v1",
    model: "deepseek-chat",
    timeout_sec: 20,
    temperature: 0.4,
    system_prompt: "系统提示",
    api_key: "key",
  });
});

test("buildAIConfigTestMutation reports invalid fields", () => {
  assert.equal(buildAIConfigTestMutation({ baseUrl: "https://ai", model: "m", timeoutSec: 1, temperature: 1 }).error, "prompt_required");
  assert.equal(buildAIConfigTestMutation({ prompt: "p", model: "m", timeoutSec: 1, temperature: 1 }).error, "base_url_required");
  assert.equal(buildAIConfigTestMutation({ prompt: "p", baseUrl: "https://ai", timeoutSec: 1, temperature: 1 }).error, "model_required");
  assert.equal(buildAIConfigTestMutation({ prompt: "p", baseUrl: "https://ai", model: "m", timeoutSec: 0, temperature: 1 }).error, "timeout_invalid");
  assert.equal(buildAIConfigTestMutation({ prompt: "p", baseUrl: "https://ai", model: "m", timeoutSec: 1, temperature: 3 }).error, "temperature_invalid");
});

test("normalizeAIConfigTestResult reads reply aliases", () => {
  assert.deepEqual(normalizeAIConfigTestResult({ reply: " pong " }), { success: true, reply: "pong", raw: { reply: " pong " } });
  assert.deepEqual(normalizeAIConfigTestResult({ success: false, content: " fail " }), { success: false, reply: "fail", raw: { success: false, content: " fail " } });
});

test("buildKnowledgeDialogueMutation mirrors legacy test-dialogue endpoint", () => {
  const mutation = buildKnowledgeDialogueMutation({ prompt: " 退款怎么处理 ", topK: 2 });
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, AI_CONFIG_TEST_DIALOGUE_PATH);
  assert.deepEqual(mutation.body, {
    question: "退款怎么处理",
    top_k: 2,
  });
});

test("buildKnowledgeDialogueMutation reports missing question", () => {
  assert.equal(buildKnowledgeDialogueMutation({ question: " " }).error, "question_required");
});

test("normalizeKnowledgeDialogueResult keeps knowledge dialogue fields", () => {
  assert.deepEqual(normalizeKnowledgeDialogueResult({
    reply: " 退款将在24小时内处理 ",
    mode: "knowledge_qa",
    matched_question: " 退款流程 ",
    source: " faq.md ",
    confidence: 0.81234,
    candidates: [{ question: "退款流程" }],
  }), {
    reply: "退款将在24小时内处理",
    mode: "knowledge_qa",
    matchedQuestion: "退款流程",
    source: "faq.md",
    confidence: 0.81234,
    candidates: [{ question: "退款流程" }],
    raw: {
      reply: " 退款将在24小时内处理 ",
      mode: "knowledge_qa",
      matched_question: " 退款流程 ",
      source: " faq.md ",
      confidence: 0.81234,
      candidates: [{ question: "退款流程" }],
    },
  });
});

test("formatAIProfileJSON renders stable arrays", () => {
  assert.equal(formatAIProfileJSON([{ profile_id: "coze-a" }]), "[\n  {\n    \"profile_id\": \"coze-a\"\n  }\n]");
  assert.equal(formatAIProfileJSON({}), "[]");
});
