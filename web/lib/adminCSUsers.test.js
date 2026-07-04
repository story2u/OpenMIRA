import assert from "node:assert/strict";
import test from "node:test";

import {
  CONVERSATION_AI_BULK_PATH,
  CS_USERS_PATH,
  GENERATE_CS_TOKEN_PATH,
  buildCSUserAIBulkMutation,
  buildCSUserDeleteMutation,
  buildGlobalConversationAIBulkMutation,
  buildCSUserUpsertMutation,
  buildCSUsersListRequest,
  buildCSUserWorkbenchTokenMutation,
  buildCSUserWorkbenchURL,
  buildCSUserFormFromUser,
  defaultCSUserForm,
  isCSUserFormDirty,
  normalizeAdminCSUsers,
} from "./adminCSUsers.js";

test("normalizeAdminCSUsers keeps legacy roster fields", () => {
  const users = normalizeAdminCSUsers({
    users: [
      {
        assignee_id: "cs-1",
        assignee_name: "客服一",
        role: "supervisor",
        enabled: false,
        ai_enabled: true,
        max_sessions: 8,
        current_sessions: 3,
        has_password: true,
        is_online: true,
        updated_at: "2026-07-02T01:02:03Z",
      },
      { assignee_name: "missing id" },
    ],
  });

  assert.equal(users.length, 1);
  assert.equal(users[0].assigneeId, "cs-1");
  assert.equal(users[0].enabledLabel, "停用");
  assert.equal(users[0].aiLabel, "开启");
  assert.equal(users[0].onlineLabel, "在线");
  assert.equal(users[0].maxSessionsLabel, "8");
  assert.equal(users[0].passwordLabel, "已设置");
});

test("buildCSUserUpsertMutation mirrors legacy POST body", () => {
  const create = buildCSUserUpsertMutation({
    assigneeId: " cs-1 ",
    assigneeName: " 客服一 ",
    password: " secret1 ",
    createOnly: true,
  });
  const update = buildCSUserUpsertMutation({
    assigneeId: "cs-2",
    assigneeName: "客服二",
    role: "supervisor",
    enabled: false,
    aiEnabled: true,
    maxSessions: 9.8,
  });

  assert.equal(create.ok, true);
  assert.equal(create.method, "POST");
  assert.equal(create.path, CS_USERS_PATH);
  assert.deepEqual(create.body, {
    assignee_id: "cs-1",
    assignee_name: "客服一",
    role: "cs",
    enabled: true,
    ai_enabled: false,
    max_sessions: 0,
    create_only: true,
    password: "secret1",
  });
  assert.deepEqual(update.body, {
    assignee_id: "cs-2",
    assignee_name: "客服二",
    role: "supervisor",
    enabled: false,
    ai_enabled: true,
    max_sessions: 9,
    create_only: false,
  });
});

test("buildCSUserUpsertMutation reports invalid fields", () => {
  assert.equal(buildCSUserUpsertMutation({ assigneeName: "客服" }).error, "assignee_id_required");
  assert.equal(buildCSUserUpsertMutation({ assigneeId: "cs-1" }).error, "assignee_name_required");
  assert.equal(buildCSUserUpsertMutation({ assigneeId: "cs-1", assigneeName: "客服", role: "owner" }).error, "role_invalid");
  assert.equal(buildCSUserUpsertMutation({ assigneeId: "cs-1", assigneeName: "客服", password: "12345" }).error, "password_short");
});

test("buildCSUserDeleteMutation mirrors legacy DELETE path", () => {
  const mutation = buildCSUserDeleteMutation("cs/1");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${CS_USERS_PATH}/cs%2F1`);
  assert.equal(buildCSUserDeleteMutation("").error, "assignee_id_required");
});

test("buildCSUsersListRequest mirrors legacy keyword filtering", () => {
  const empty = buildCSUsersListRequest("");
  const filtered = buildCSUsersListRequest(" 客服一 ");

  assert.equal(empty.ok, true);
  assert.equal(empty.method, "GET");
  assert.equal(empty.path, CS_USERS_PATH);
  assert.deepEqual(empty.params, {});
  assert.deepEqual(filtered.params, { keyword: "客服一" });
});

test("workbench token helpers mirror admin generated CS token flow", () => {
  const mutation = buildCSUserWorkbenchTokenMutation(" cs/1 ");
  const url = buildCSUserWorkbenchURL("cs/1", "jwt-token");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, GENERATE_CS_TOKEN_PATH);
  assert.deepEqual(mutation.body, { assignee_id: "cs/1" });
  assert.equal(url.ok, true);
  assert.equal(url.url, "/?fresh=1&cs_id=cs%2F1&token=jwt-token");
  assert.equal(buildCSUserWorkbenchTokenMutation("").error, "assignee_id_required");
  assert.equal(buildCSUserWorkbenchURL("cs-1", "").error, "token_required");
});

test("buildCSUserAIBulkMutation mirrors scoped conversation AI bulk route", () => {
  const enabled = buildCSUserAIBulkMutation(" cs/1 ", true);
  const disabled = buildCSUserAIBulkMutation("cs-2", false, { syncCSUser: false });

  assert.equal(enabled.ok, true);
  assert.equal(enabled.method, "POST");
  assert.equal(enabled.path, CONVERSATION_AI_BULK_PATH);
  assert.deepEqual(enabled.body, {
    enabled: true,
    assignee_id: "cs/1",
    sync_cs_user: true,
  });
  assert.deepEqual(disabled.body, {
    enabled: false,
    assignee_id: "cs-2",
    sync_cs_user: false,
  });
});

test("buildCSUserAIBulkMutation reports invalid fields", () => {
  assert.equal(buildCSUserAIBulkMutation("", true).error, "assignee_id_required");
  assert.equal(buildCSUserAIBulkMutation("cs-1", "true").error, "enabled_required");
});

test("buildGlobalConversationAIBulkMutation mirrors global AI bulk route", () => {
  const mutation = buildGlobalConversationAIBulkMutation(false);

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, CONVERSATION_AI_BULK_PATH);
  assert.deepEqual(mutation.body, {
    enabled: false,
    sync_cs_user: false,
  });
  assert.equal(buildGlobalConversationAIBulkMutation("false").error, "enabled_required");
});

test("isCSUserFormDirty detects create form changes", () => {
  const baseline = defaultCSUserForm();

  assert.equal(isCSUserFormDirty(defaultCSUserForm(), baseline), false);
  assert.equal(isCSUserFormDirty({ ...baseline, assigneeId: " " }, baseline), false);
  assert.equal(isCSUserFormDirty({ ...baseline, role: "supervisor" }, baseline), true);
  assert.equal(isCSUserFormDirty({ ...baseline, maxSessions: "3" }, baseline), true);
  assert.equal(isCSUserFormDirty({ ...baseline, password: " secret1 " }, baseline), true);
  assert.equal(isCSUserFormDirty({ ...baseline, enabled: false }, baseline), true);
});

test("buildCSUserFormFromUser creates stable edit baseline", () => {
  const baseline = buildCSUserFormFromUser({
    assignee_id: " cs-1 ",
    assignee_name: " 客服一 ",
    role: "supervisor",
    enabled: false,
    ai_enabled: true,
    max_sessions: 8,
  });

  assert.deepEqual(baseline, {
    assigneeId: "cs-1",
    assigneeName: "客服一",
    role: "supervisor",
    enabled: false,
    aiEnabled: true,
    maxSessions: 8,
    password: "",
    editing: true,
  });
  assert.equal(isCSUserFormDirty({ ...baseline, assigneeName: "客服一 " }, baseline), false);
  assert.equal(isCSUserFormDirty({ ...baseline, password: "secret1" }, baseline), true);
  assert.equal(isCSUserFormDirty({ ...baseline, enabled: true }, baseline), true);
});
