import assert from "node:assert/strict";
import test from "node:test";

import {
  ACCOUNTS_BATCH_PATH,
  ACCOUNTS_PATH,
  ACCOUNT_CSV_ACCEPT,
  buildAccountAIEnabledMutation,
  buildAccountAssignMutation,
  buildAccountBatchImportMutation,
  buildAccountDeleteMutation,
  buildAccountDeviceBindingDraft,
  buildAccountUnassignMutation,
  buildAccountUpsertMutation,
  findAccountForDeviceBinding,
  normalizeAdminAccounts,
} from "./adminAccounts.js";

class FakeFormData {
  constructor() {
    this.entries = [];
  }

  append(key, value) {
    this.entries.push([key, value]);
  }
}

test("normalizeAdminAccounts keeps account rows and management fields", () => {
  const accounts = normalizeAdminAccounts({
    accounts: [
      {
        account_id: "acc-1",
        account_name: "账号一",
        agent_id: "agent-1",
        device_id: "device-1",
        wework_user_id: "DY-1",
        enterprise_id: "ent-1",
        assignee_name: "客服A",
        sop_flow_id: "flow-a",
        sop_enabled: true,
        sop_reply_window_start: "09:00",
        sop_reply_window_end: "18:00",
        status: "online",
        ai_enabled: true,
        ai_model: "coze",
        knowledge_tag: "vip",
      },
      { account_name: "missing id" },
      { account_id: "acc-2", ai_enabled: "off" },
    ],
  });

  assert.equal(accounts.length, 2);
  assert.equal(accounts[0].accountName, "账号一");
  assert.equal(accounts[0].agentId, "agent-1");
  assert.equal(accounts[0].deviceId, "device-1");
  assert.equal(accounts[0].weworkUserId, "DY-1");
  assert.equal(accounts[0].enterpriseId, "ent-1");
  assert.equal(accounts[0].sopFlowId, "flow-a");
  assert.equal(accounts[0].sopEnabled, true);
  assert.equal(accounts[0].sopLabel, "开启");
  assert.equal(accounts[0].sopReplyWindowStart, "09:00");
  assert.equal(accounts[0].sopReplyWindowEnd, "18:00");
  assert.equal(accounts[0].aiEnabled, true);
  assert.equal(accounts[0].aiLabel, "开启");
  assert.equal(accounts[0].aiModel, "coze");
  assert.equal(accounts[0].knowledgeTag, "vip");
  assert.equal(accounts[1].aiEnabled, false);
  assert.equal(accounts[1].aiLabel, "关闭");
});

test("buildAccountUpsertMutation mirrors legacy POST body", () => {
  const mutation = buildAccountUpsertMutation({
    accountId: " acc-1 ",
    accountName: " 账号一 ",
    agentId: " agent-1 ",
    deviceId: " device-1 ",
    weworkUserId: " DY-1 ",
    enterpriseId: " ent-1 ",
    sopFlowId: " flow-a ",
    sopEnabled: false,
    sopReplyWindowStart: " 09:00 ",
    sopReplyWindowEnd: " 18:00 ",
    aiEnabled: true,
    aiModel: " coze ",
    knowledgeTag: " vip ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ACCOUNTS_PATH);
  assert.deepEqual(mutation.body, {
    account_id: "acc-1",
    account_name: "账号一",
    agent_id: "agent-1",
    device_id: "device-1",
    wework_user_id: "DY-1",
    enterprise_id: "ent-1",
    sop_flow_id: "flow-a",
    sop_reply_window_start: "09:00",
    sop_reply_window_end: "18:00",
    ai_model: "coze",
    knowledge_tag: "vip",
    sop_enabled: false,
    ai_enabled: true,
  });
});

test("buildAccountUpsertMutation supports generated account ids", () => {
  const mutation = buildAccountUpsertMutation({ accountName: "账号一", ai_enabled: "off" });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.body.account_id, "");
  assert.equal(mutation.body.account_name, "账号一");
  assert.equal(mutation.body.ai_enabled, false);
});

test("buildAccountUpsertMutation reports missing fields", () => {
  assert.equal(buildAccountUpsertMutation({}).error, "account_name_required");
});

test("account device binding helpers prefer exact device and agent matches", () => {
  const accounts = normalizeAdminAccounts({
    accounts: [
      { account_id: "acc-1", account_name: "账号一", agent_id: "legacy-agent", device_id: "device-1" },
      { account_id: "acc-2", account_name: "账号二", agent_id: "sdk:device-1", device_id: "device-1", wework_user_id: "DY-2", ai_enabled: true },
    ],
  });
  const matched = findAccountForDeviceBinding(accounts, { agentId: "sdk:device-1", deviceId: "device-1" });
  assert.equal(matched.accountId, "acc-2");

  const draft = buildAccountDeviceBindingDraft({
    agentId: "sdk:device-1",
    deviceId: "device-1",
    loginAccountName: "登录账号",
    loginWeWorkUserId: "LOGIN-DY",
  }, matched);
  assert.deepEqual(draft, {
    accountId: "acc-2",
    accountName: "账号二",
    agentId: "sdk:device-1",
    deviceId: "device-1",
    weworkUserId: "DY-2",
    enterpriseId: "",
    assigneeId: "",
    assigneeName: "",
    sopEnabled: false,
    aiEnabled: true,
    editing: true,
  });
});

test("account device binding draft can start a new account from login identity", () => {
  const draft = buildAccountDeviceBindingDraft({
    agent_id: "sdk:slot-18",
    device_id: "slot-18",
    login_account_name: "子墨",
    login_wework_user_id: "dy1801",
  });

  assert.equal(draft.accountId, "");
  assert.equal(draft.accountName, "子墨");
  assert.equal(draft.agentId, "sdk:slot-18");
  assert.equal(draft.deviceId, "slot-18");
  assert.equal(draft.weworkUserId, "dy1801");
  assert.equal(draft.editing, false);
});

test("buildAccountAIEnabledMutation mirrors legacy toggle route", () => {
  const enabled = buildAccountAIEnabledMutation("acc/1", true);
  const disabled = buildAccountAIEnabledMutation("acc-2", false);

  assert.equal(enabled.ok, true);
  assert.equal(enabled.method, "POST");
  assert.equal(enabled.path, "/accounts/acc%2F1/ai-enabled");
  assert.deepEqual(enabled.body, { enabled: true });
  assert.deepEqual(disabled.body, { enabled: false });
});

test("buildAccountAIEnabledMutation reports missing fields", () => {
  assert.equal(buildAccountAIEnabledMutation("", true).error, "account_required");
  assert.equal(buildAccountAIEnabledMutation("acc-1", "true").error, "enabled_required");
});

test("buildAccountAssignMutation mirrors legacy assign route", () => {
  const mutation = buildAccountAssignMutation("acc/1", {
    assigneeId: " cs-1 ",
    assigneeName: " 客服一 ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, `${ACCOUNTS_PATH}/acc%2F1/assign`);
  assert.deepEqual(mutation.body, {
    assignee_id: "cs-1",
    assignee_name: "客服一",
  });
});

test("buildAccountAssignMutation reports missing fields", () => {
  assert.equal(buildAccountAssignMutation("", { assigneeId: "cs-1" }).error, "account_required");
  assert.equal(buildAccountAssignMutation("acc-1", {}).error, "assignee_id_required");
});

test("buildAccountUnassignMutation mirrors legacy unassign route", () => {
  const mutation = buildAccountUnassignMutation("acc/1");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, `${ACCOUNTS_PATH}/acc%2F1/unassign`);
  assert.equal(buildAccountUnassignMutation("").error, "account_required");
});

test("buildAccountBatchImportMutation mirrors legacy CSV import route", () => {
  const file = { name: "accounts.csv" };
  const mutation = buildAccountBatchImportMutation({ file, FormDataCtor: FakeFormData });

  assert.equal(ACCOUNT_CSV_ACCEPT, ".csv");
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ACCOUNTS_BATCH_PATH);
  assert.deepEqual(mutation.body.entries, [["file", file]]);
});

test("buildAccountBatchImportMutation reports invalid upload prerequisites", () => {
  assert.equal(buildAccountBatchImportMutation({ FormDataCtor: FakeFormData }).error, "file_required");
  assert.equal(buildAccountBatchImportMutation({ file: { name: "accounts.txt" }, FormDataCtor: FakeFormData }).error, "csv_required");
  assert.equal(buildAccountBatchImportMutation({ file: { name: "accounts.csv" }, FormDataCtor: null }).error, "formdata_unavailable");
});

test("buildAccountDeleteMutation mirrors legacy DELETE path", () => {
  const mutation = buildAccountDeleteMutation("acc/1");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${ACCOUNTS_PATH}/acc%2F1`);
  assert.equal(buildAccountDeleteMutation("").error, "account_required");
});
