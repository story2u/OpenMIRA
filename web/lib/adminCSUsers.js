export const CS_USERS_PATH = "/cs-users";
export const GENERATE_CS_TOKEN_PATH = "/session/admin/generate-cs-token";
export const CONVERSATION_AI_BULK_PATH = "/conversations/ai-auto-reply/bulk";

const VALID_ROLES = new Set(["admin", "supervisor", "cs"]);

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAdminCSUsers(payload = {}) {
  const users = Array.isArray(payload?.users)
    ? payload.users
    : Array.isArray(payload?.data?.users)
      ? payload.data.users
      : Array.isArray(payload?.status)
        ? payload.status
        : [];
  return users.map(normalizeAdminCSUser).filter(Boolean);
}

export function normalizeAdminCSUser(user = {}) {
  const assigneeId = cleanText(user?.assignee_id || user?.assigneeId || user?.id);
  if (!assigneeId) return null;
  const enabled = parseBool(user?.enabled, true);
  const aiEnabled = parseBool(firstDefined(user?.ai_enabled, user?.aiEnabled), false);
  const online = parseBool(firstDefined(user?.is_online, user?.online), false);
  const maxSessions = normalizeNonNegativeInt(firstDefined(user?.max_sessions, user?.maxSessions));
  const currentSessions = normalizeNonNegativeInt(firstDefined(user?.current_sessions, user?.currentSessions));
  const hasPassword = parseBool(firstDefined(user?.has_password, user?.hasPassword), false);
  return {
    assigneeId,
    assigneeName: cleanText(user?.assignee_name || user?.assigneeName || user?.name) || assigneeId,
    role: normalizeRole(user?.role),
    enabled,
    enabledLabel: enabled ? "启用" : "停用",
    aiEnabled,
    aiLabel: aiEnabled ? "开启" : "关闭",
    isOnline: online,
    onlineLabel: online ? "在线" : "离线",
    maxSessions,
    maxSessionsLabel: maxSessions > 0 ? String(maxSessions) : "不限制",
    currentSessions,
    hasPassword,
    passwordLabel: hasPassword ? "已设置" : "未设置",
    lastSeenAt: cleanText(user?.last_seen_at || user?.lastSeenAt),
    createdAt: cleanText(user?.created_at || user?.createdAt),
    updatedAt: cleanText(user?.updated_at || user?.updatedAt),
  };
}

export function buildCSUserUpsertMutation(options = {}) {
  const assigneeId = cleanText(options.assigneeId || options.assignee_id);
  if (!assigneeId) return { ok: false, error: "assignee_id_required" };
  const assigneeName = cleanText(options.assigneeName || options.assignee_name);
  if (!assigneeName) return { ok: false, error: "assignee_name_required" };
  const role = normalizeRole(options.role);
  if (!VALID_ROLES.has(role)) return { ok: false, error: "role_invalid" };
  const password = cleanText(options.password);
  if (password && Array.from(password).length < 6) return { ok: false, error: "password_short" };
  const body = {
    assignee_id: assigneeId,
    assignee_name: assigneeName,
    role,
    enabled: typeof options.enabled === "boolean" ? options.enabled : true,
    ai_enabled: typeof options.aiEnabled === "boolean"
      ? options.aiEnabled
      : typeof options.ai_enabled === "boolean"
        ? options.ai_enabled
        : false,
    max_sessions: normalizeNonNegativeInt(options.maxSessions || options.max_sessions),
    create_only: Boolean(options.createOnly || options.create_only),
  };
  if (password) body.password = password;
  return {
    ok: true,
    method: "POST",
    path: CS_USERS_PATH,
    body,
  };
}

export function buildCSUserDeleteMutation(assigneeId = "") {
  const normalizedAssigneeId = cleanText(assigneeId);
  if (!normalizedAssigneeId) return { ok: false, error: "assignee_id_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${CS_USERS_PATH}/${encodeURIComponent(normalizedAssigneeId)}`,
  };
}

export function buildCSUsersListRequest(keyword = "") {
  const normalizedKeyword = cleanText(keyword);
  return {
    ok: true,
    method: "GET",
    path: CS_USERS_PATH,
    params: normalizedKeyword ? { keyword: normalizedKeyword } : {},
  };
}

export function buildCSUserWorkbenchTokenMutation(assigneeId = "") {
  const normalizedAssigneeId = cleanText(assigneeId);
  if (!normalizedAssigneeId) return { ok: false, error: "assignee_id_required" };
  return {
    ok: true,
    method: "POST",
    path: GENERATE_CS_TOKEN_PATH,
    body: { assignee_id: normalizedAssigneeId },
  };
}

export function buildCSUserWorkbenchURL(assigneeId = "", token = "") {
  const normalizedAssigneeId = cleanText(assigneeId);
  const normalizedToken = cleanText(token);
  if (!normalizedAssigneeId) return { ok: false, error: "assignee_id_required" };
  if (!normalizedToken) return { ok: false, error: "token_required" };
  const params = new URLSearchParams({
    fresh: "1",
    cs_id: normalizedAssigneeId,
    token: normalizedToken,
  });
  return {
    ok: true,
    url: `/?${params.toString()}`,
  };
}

export function buildCSUserAIBulkMutation(assigneeId = "", enabled, options = {}) {
  const normalizedAssigneeId = cleanText(assigneeId);
  if (!normalizedAssigneeId) return { ok: false, error: "assignee_id_required" };
  if (typeof enabled !== "boolean") return { ok: false, error: "enabled_required" };
  const syncCSUser = typeof options.syncCSUser === "boolean"
    ? options.syncCSUser
    : typeof options.sync_cs_user === "boolean"
      ? options.sync_cs_user
      : true;
  return {
    ok: true,
    method: "POST",
    path: CONVERSATION_AI_BULK_PATH,
    body: {
      enabled,
      assignee_id: normalizedAssigneeId,
      sync_cs_user: syncCSUser,
    },
  };
}

export function buildGlobalConversationAIBulkMutation(enabled) {
  if (typeof enabled !== "boolean") return { ok: false, error: "enabled_required" };
  return {
    ok: true,
    method: "POST",
    path: CONVERSATION_AI_BULK_PATH,
    body: {
      enabled,
      sync_cs_user: false,
    },
  };
}

export function defaultCSUserForm() {
  return {
    assigneeId: "",
    assigneeName: "",
    role: "cs",
    enabled: true,
    aiEnabled: false,
    maxSessions: 0,
    password: "",
    editing: false,
  };
}

export function buildCSUserFormFromUser(user = {}) {
  const normalized = normalizeAdminCSUser(user) || {};
  return {
    ...defaultCSUserForm(),
    assigneeId: cleanText(normalized.assigneeId),
    assigneeName: cleanText(normalized.assigneeName),
    role: normalizeRole(normalized.role),
    enabled: parseBool(normalized.enabled, true),
    aiEnabled: parseBool(normalized.aiEnabled, false),
    maxSessions: normalizeNonNegativeInt(normalized.maxSessions),
    password: "",
    editing: true,
  };
}

export function isCSUserFormDirty(form = {}, baseline = defaultCSUserForm()) {
  const current = normalizeCSUserFormForCompare(form);
  const original = normalizeCSUserFormForCompare(baseline);
  return Object.keys(original).some((key) => current[key] !== original[key]);
}

export function normalizeRole(value = "") {
  const role = cleanText(value) || "cs";
  return role.toLowerCase();
}

function normalizeCSUserFormForCompare(form = {}) {
  return {
    assigneeId: cleanText(form.assigneeId || form.assignee_id),
    assigneeName: cleanText(form.assigneeName || form.assignee_name),
    role: normalizeRole(form.role),
    enabled: typeof form.enabled === "boolean" ? form.enabled : true,
    aiEnabled: typeof form.aiEnabled === "boolean"
      ? form.aiEnabled
      : typeof form.ai_enabled === "boolean"
        ? form.ai_enabled
        : false,
    maxSessions: normalizeNonNegativeInt(form.maxSessions || form.max_sessions),
    password: cleanText(form.password),
    editing: Boolean(form.editing),
  };
}

function normalizeNonNegativeInt(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return Math.floor(number);
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "启用", "开启", "在线"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "停用", "关闭", "离线"].includes(normalized)) return false;
  return fallback;
}
