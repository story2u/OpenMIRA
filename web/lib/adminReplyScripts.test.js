import assert from "node:assert/strict";
import test from "node:test";

import {
  REPLY_SCRIPTS_ADMIN_PATH,
  SCRIPT_GENERATE_PATH,
  TARGET_AUDIENCE_ALL,
  TARGET_AUDIENCE_NONE,
  buildReplyScriptGenerateMutation,
  buildReplyScriptDeleteMutation,
  buildReplyScriptUpsertMutation,
  normalizeGeneratedReplyScript,
  normalizeReplyScripts,
  normalizeTargetAudience,
  replyScriptAudienceLabel,
  replyScriptAudienceMode,
} from "./adminReplyScripts.js";

test("normalizeReplyScripts keeps legacy script fields", () => {
  const scripts = normalizeReplyScripts({
    scripts: [
      {
        script_id: "script-1",
        title: "欢迎语",
        content: "您好",
        category: "",
        enabled: false,
        target_audience: "cs-1,cs-2,cs-3",
        updated_at: "2026-07-02T01:02:03Z",
      },
      { title: "missing id" },
    ],
  });

  assert.equal(scripts.length, 1);
  assert.equal(scripts[0].scriptId, "script-1");
  assert.equal(scripts[0].category, "default");
  assert.equal(scripts[0].enabled, false);
  assert.equal(scripts[0].enabledLabel, "停用");
  assert.equal(scripts[0].targetAudienceLabel, "cs-1, cs-2 等 3 人");
});

test("buildReplyScriptUpsertMutation mirrors legacy POST body", () => {
  const create = buildReplyScriptUpsertMutation({
    title: " 欢迎语 ",
    content: " 您好 ",
  });
  const update = buildReplyScriptUpsertMutation({
    scriptId: "script/1",
    title: "停用话术",
    content: "稍后联系",
    category: "followup",
    enabled: false,
    targetAudience: "cs-1，cs-2\ncs-1",
  });

  assert.equal(create.ok, true);
  assert.equal(create.method, "POST");
  assert.equal(create.path, REPLY_SCRIPTS_ADMIN_PATH);
  assert.deepEqual(create.body, {
    title: "欢迎语",
    content: "您好",
    category: "default",
    enabled: true,
    target_audience: TARGET_AUDIENCE_NONE,
  });
  assert.deepEqual(update.body, {
    script_id: "script/1",
    title: "停用话术",
    content: "稍后联系",
    category: "followup",
    enabled: false,
    target_audience: "cs-1,cs-2",
  });
  assert.equal(buildReplyScriptUpsertMutation({ title: " ", content: "ok" }).error, "title_required");
  assert.equal(buildReplyScriptUpsertMutation({ title: "ok", content: " " }).error, "content_required");
});

test("buildReplyScriptDeleteMutation mirrors legacy DELETE path", () => {
  const mutation = buildReplyScriptDeleteMutation("script/1");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${REPLY_SCRIPTS_ADMIN_PATH}/script%2F1`);
  assert.equal(buildReplyScriptDeleteMutation("").error, "script_id_required");
});

test("buildReplyScriptGenerateMutation mirrors legacy AI generation route", () => {
  const mutation = buildReplyScriptGenerateMutation({
    prompt: " 预约流程 ",
    style: "",
    systemPrompt: "自定义系统提示",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, SCRIPT_GENERATE_PATH);
  assert.deepEqual(mutation.body, {
    prompt: "预约流程",
    style: "专业亲和",
    system_prompt: "自定义系统提示",
  });
  assert.equal(buildReplyScriptGenerateMutation({ prompt: " " }).error, "prompt_required");
});

test("normalizeGeneratedReplyScript accepts legacy content envelopes", () => {
  assert.equal(normalizeGeneratedReplyScript({ content: " 您好 " }), "您好");
  assert.equal(normalizeGeneratedReplyScript({ data: { reply: " 可以的 " } }), "可以的");
});

test("target audience normalization keeps SOP-compatible sentinels", () => {
  assert.equal(normalizeTargetAudience(""), TARGET_AUDIENCE_NONE);
  assert.equal(normalizeTargetAudience(TARGET_AUDIENCE_ALL), TARGET_AUDIENCE_ALL);
  assert.equal(normalizeTargetAudience("__NONE__,cs-1"), "cs-1");
  assert.equal(replyScriptAudienceMode(TARGET_AUDIENCE_ALL), "all");
  assert.equal(replyScriptAudienceMode("cs-1"), "custom");
  assert.equal(replyScriptAudienceLabel(TARGET_AUDIENCE_NONE), "未分配");
});
