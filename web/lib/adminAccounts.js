export const ACCOUNTS_PATH = "/accounts";
export const ACCOUNTS_BATCH_PATH = "/accounts/batch";
export const ACCOUNT_ASSIGN_PATH_SUFFIX = "/assign";
export const ACCOUNT_AI_ENABLED_PATH_SUFFIX = "/ai-enabled";
export const ACCOUNT_UNASSIGN_PATH_SUFFIX = "/unassign";
export const ACCOUNT_CSV_ACCEPT = ".csv";

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAdminAccounts(payload = {}) {
  const accounts = Array.isArray(payload?.accounts)
    ? payload.accounts
    : Array.isArray(payload?.data?.accounts)
      ? payload.data.accounts
      : [];
  return accounts.map(normalizeAdminAccount).filter(Boolean);
}

export function normalizeAdminAccount(account = {}) {
  const accountId = cleanText(account?.account_id || account?.accountId || account?.id);
  if (!accountId) return null;
  const aiEnabled = parseBool(firstDefined(account?.ai_enabled, account?.aiEnabled), false);
  const sopEnabled = parseBool(firstDefined(account?.sop_enabled, account?.sopEnabled), false);
  return {
    accountId,
    accountName: cleanText(account?.account_name || account?.accountName || account?.name) || accountId,
    agentId: cleanText(account?.agent_id || account?.agentId),
    deviceId: cleanText(account?.device_id || account?.deviceId),
    weworkUserId: cleanText(account?.wework_user_id || account?.weworkUserId),
    enterpriseId: cleanText(account?.enterprise_id || account?.enterpriseId),
    assigneeName: cleanText(account?.assignee_name || account?.assigneeName),
    assigneeId: cleanText(account?.assignee_id || account?.assigneeId),
    sopFlowId: cleanText(account?.sop_flow_id || account?.sopFlowId),
    sopEnabled,
    sopLabel: sopEnabled ? "开启" : "关闭",
    sopReplyWindowStart: cleanText(account?.sop_reply_window_start || account?.sopReplyWindowStart),
    sopReplyWindowEnd: cleanText(account?.sop_reply_window_end || account?.sopReplyWindowEnd),
    status: cleanText(account?.status) || "-",
    aiEnabled,
    aiLabel: aiEnabled ? "开启" : "关闭",
    aiModel: cleanText(account?.ai_model || account?.aiModel),
    knowledgeTag: cleanText(account?.knowledge_tag || account?.knowledgeTag),
    createdAt: cleanText(account?.created_at || account?.createdAt),
    updatedAt: cleanText(account?.updated_at || account?.updatedAt),
  };
}

export function buildAccountUpsertMutation(options = {}) {
  const accountId = cleanText(firstDefined(options.accountId, options.account_id));
  const accountName = cleanText(firstDefined(options.accountName, options.account_name, options.name));
  if (!accountName) return { ok: false, error: "account_name_required" };

  const body = {
    account_id: accountId,
    account_name: accountName,
    agent_id: cleanText(firstDefined(options.agentId, options.agent_id)),
    device_id: cleanText(firstDefined(options.deviceId, options.device_id)),
    wework_user_id: cleanText(firstDefined(options.weworkUserId, options.wework_user_id)),
    enterprise_id: cleanText(firstDefined(options.enterpriseId, options.enterprise_id)),
    sop_flow_id: cleanText(firstDefined(options.sopFlowId, options.sop_flow_id)),
    sop_reply_window_start: cleanText(firstDefined(options.sopReplyWindowStart, options.sop_reply_window_start)),
    sop_reply_window_end: cleanText(firstDefined(options.sopReplyWindowEnd, options.sop_reply_window_end)),
    ai_model: cleanText(firstDefined(options.aiModel, options.ai_model)),
    knowledge_tag: cleanText(firstDefined(options.knowledgeTag, options.knowledge_tag)),
  };
  const sopEnabled = optionalBool(firstDefined(options.sopEnabled, options.sop_enabled));
  const aiEnabled = optionalBool(firstDefined(options.aiEnabled, options.ai_enabled));
  if (sopEnabled !== undefined) body.sop_enabled = sopEnabled;
  if (aiEnabled !== undefined) body.ai_enabled = aiEnabled;

  return {
    ok: true,
    method: "POST",
    path: ACCOUNTS_PATH,
    body,
  };
}

export function findAccountForDeviceBinding(accounts = [], device = {}) {
  const deviceId = cleanText(firstDefined(device?.deviceId, device?.device_id));
  const agentId = cleanText(firstDefined(device?.agentId, device?.agent_id));
  if (!deviceId && !agentId) return null;

  const exact = accounts.find((account) => (
    deviceId
    && agentId
    && cleanText(firstDefined(account?.deviceId, account?.device_id)) === deviceId
    && cleanText(firstDefined(account?.agentId, account?.agent_id)) === agentId
  ));
  if (exact) return exact;

  if (deviceId) {
    const byDevice = accounts.find((account) => cleanText(firstDefined(account?.deviceId, account?.device_id)) === deviceId);
    if (byDevice) return byDevice;
  }
  if (agentId) {
    return accounts.find((account) => cleanText(firstDefined(account?.agentId, account?.agent_id)) === agentId) || null;
  }
  return null;
}

export function buildAccountDeviceBindingDraft(device = {}, account = {}) {
  const deviceId = cleanText(firstDefined(device?.deviceId, device?.device_id, account?.deviceId, account?.device_id));
  const agentId = cleanText(firstDefined(device?.agentId, device?.agent_id, account?.agentId, account?.agent_id));
  const accountId = cleanText(firstDefined(account?.accountId, account?.account_id));
  const loginAccountName = cleanText(firstDefined(device?.loginAccountName, device?.login_account_name));
  const loginWeWorkUserId = cleanText(firstDefined(device?.loginWeWorkUserId, device?.login_wework_user_id));
  const accountName = cleanText(firstDefined(account?.accountName, account?.account_name, loginAccountName, deviceId, accountId));
  return {
    accountId,
    accountName,
    agentId,
    deviceId,
    weworkUserId: cleanText(firstDefined(account?.weworkUserId, account?.wework_user_id, loginWeWorkUserId)),
    enterpriseId: cleanText(firstDefined(account?.enterpriseId, account?.enterprise_id)),
    assigneeId: cleanText(firstDefined(account?.assigneeId, account?.assignee_id)),
    assigneeName: cleanText(firstDefined(account?.assigneeName, account?.assignee_name)),
    sopEnabled: parseBool(firstDefined(account?.sopEnabled, account?.sop_enabled), false),
    aiEnabled: parseBool(firstDefined(account?.aiEnabled, account?.ai_enabled), false),
    editing: Boolean(accountId),
  };
}

export function buildAccountAIEnabledMutation(accountId = "", enabled) {
  const normalizedAccountId = cleanText(accountId);
  if (!normalizedAccountId) return { ok: false, error: "account_required" };
  if (typeof enabled !== "boolean") return { ok: false, error: "enabled_required" };
  return {
    ok: true,
    method: "POST",
    path: `/accounts/${encodeURIComponent(normalizedAccountId)}${ACCOUNT_AI_ENABLED_PATH_SUFFIX}`,
    body: { enabled },
  };
}

export function buildAccountAssignMutation(accountId = "", options = {}) {
  const normalizedAccountId = cleanText(accountId);
  if (!normalizedAccountId) return { ok: false, error: "account_required" };
  const assigneeId = cleanText(firstDefined(options.assigneeId, options.assignee_id));
  if (!assigneeId) return { ok: false, error: "assignee_id_required" };
  return {
    ok: true,
    method: "POST",
    path: `${ACCOUNTS_PATH}/${encodeURIComponent(normalizedAccountId)}${ACCOUNT_ASSIGN_PATH_SUFFIX}`,
    body: {
      assignee_id: assigneeId,
      assignee_name: cleanText(firstDefined(options.assigneeName, options.assignee_name)),
    },
  };
}

export function buildAccountBatchImportMutation(options = {}) {
  const file = options.file;
  if (!file) return { ok: false, error: "file_required" };
  const filename = cleanText(file?.name);
  if (filename && !filename.toLowerCase().endsWith(".csv")) {
    return { ok: false, error: "csv_required" };
  }
  const FormDataCtor = Object.prototype.hasOwnProperty.call(options, "FormDataCtor")
    ? options.FormDataCtor
    : globalThis.FormData;
  if (typeof FormDataCtor !== "function") {
    return { ok: false, error: "formdata_unavailable" };
  }
  const formData = new FormDataCtor();
  formData.append("file", file);
  return {
    ok: true,
    method: "POST",
    path: ACCOUNTS_BATCH_PATH,
    body: formData,
  };
}

export function buildAccountUnassignMutation(accountId = "") {
  const normalizedAccountId = cleanText(accountId);
  if (!normalizedAccountId) return { ok: false, error: "account_required" };
  return {
    ok: true,
    method: "POST",
    path: `${ACCOUNTS_PATH}/${encodeURIComponent(normalizedAccountId)}${ACCOUNT_UNASSIGN_PATH_SUFFIX}`,
  };
}

export function buildAccountDeleteMutation(accountId = "") {
  const normalizedAccountId = cleanText(accountId);
  if (!normalizedAccountId) return { ok: false, error: "account_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${ACCOUNTS_PATH}/${encodeURIComponent(normalizedAccountId)}`,
  };
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function optionalBool(value) {
  if (value === undefined || value === null) return undefined;
  if (typeof value === "boolean") return value;
  if (cleanText(value) === "") return undefined;
  return parseBool(value, false);
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "开启", "启用"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "关闭", "停用"].includes(normalized)) return false;
  return fallback;
}
