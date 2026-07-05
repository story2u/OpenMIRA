export const REPLY_SCRIPTS_ADMIN_PATH = "/admin/scripts";
export const SCRIPT_GENERATE_PATH = "/scripts/generate";
export const TARGET_AUDIENCE_ALL = "__ALL__";
export const TARGET_AUDIENCE_NONE = "__NONE__";
export const DEFAULT_SCRIPT_STYLE = "专业亲和";

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeReplyScripts(payload = {}) {
  const scripts = Array.isArray(payload?.scripts)
    ? payload.scripts
    : Array.isArray(payload?.data?.scripts)
      ? payload.data.scripts
      : [];
  return scripts.map(normalizeReplyScript).filter(Boolean);
}

export function normalizeReplyScript(script = {}) {
  const scriptId = cleanText(script?.script_id || script?.scriptId || script?.id);
  if (!scriptId) return null;
  const title = cleanText(script?.title || script?.name) || scriptId;
  const content = cleanText(script?.content || script?.text || script?.reply);
  const enabled = parseBool(script?.enabled, true);
  const targetAudience = normalizeTargetAudience(script?.target_audience || script?.targetAudience);
  return {
    scriptId,
    title,
    content,
    category: cleanText(script?.category) || "default",
    enabled,
    enabledLabel: enabled ? "启用" : "停用",
    targetAudience,
    targetAudienceLabel: replyScriptAudienceLabel(targetAudience),
    createdAt: cleanText(script?.created_at || script?.createdAt),
    updatedAt: cleanText(script?.updated_at || script?.updatedAt),
  };
}

export function buildReplyScriptUpsertMutation(options = {}) {
  const title = cleanText(options.title);
  if (!title) return { ok: false, error: "title_required" };
  const content = cleanText(options.content);
  if (!content) return { ok: false, error: "content_required" };

  const body = {
    title,
    content,
    category: cleanText(options.category) || "default",
    enabled: typeof options.enabled === "boolean" ? options.enabled : true,
    target_audience: normalizeTargetAudience(options.targetAudience || options.target_audience),
  };
  const scriptId = cleanText(options.scriptId || options.script_id);
  if (scriptId) body.script_id = scriptId;
  return {
    ok: true,
    method: "POST",
    path: REPLY_SCRIPTS_ADMIN_PATH,
    body,
  };
}

export function buildReplyScriptDeleteMutation(scriptId = "") {
  const normalizedScriptId = cleanText(scriptId);
  if (!normalizedScriptId) return { ok: false, error: "script_id_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${REPLY_SCRIPTS_ADMIN_PATH}/${encodeURIComponent(normalizedScriptId)}`,
  };
}

export function buildReplyScriptGenerateMutation(options = {}) {
  const prompt = cleanText(options.prompt);
  if (!prompt) return { ok: false, error: "prompt_required" };
  const body = {
    prompt,
    style: cleanText(options.style) || DEFAULT_SCRIPT_STYLE,
  };
  const systemPrompt = cleanText(options.systemPrompt || options.system_prompt);
  if (systemPrompt) body.system_prompt = systemPrompt;
  return {
    ok: true,
    method: "POST",
    path: SCRIPT_GENERATE_PATH,
    body,
  };
}

export function normalizeGeneratedReplyScript(payload = {}) {
  return cleanText(
    payload?.content
      || payload?.reply
      || payload?.data?.content
      || payload?.data?.reply,
  );
}

export function normalizeTargetAudience(value = "") {
  const normalized = cleanText(value);
  if (!normalized) return TARGET_AUDIENCE_NONE;
  if (normalized === TARGET_AUDIENCE_ALL || normalized === TARGET_AUDIENCE_NONE) return normalized;
  const parts = normalized
    .replaceAll("\n", ",")
    .replaceAll("，", ",")
    .replaceAll("；", ",")
    .split(",");
  const seen = new Set();
  const values = [];
  parts.forEach((part) => {
    const candidate = cleanText(part);
    if (!candidate || candidate === TARGET_AUDIENCE_ALL || candidate === TARGET_AUDIENCE_NONE || seen.has(candidate)) return;
    seen.add(candidate);
    values.push(candidate);
  });
  return values.length > 0 ? values.join(",") : TARGET_AUDIENCE_NONE;
}

export function replyScriptAudienceMode(value = "") {
  const normalized = normalizeTargetAudience(value);
  if (normalized === TARGET_AUDIENCE_ALL) return "all";
  if (normalized === TARGET_AUDIENCE_NONE) return "none";
  return "custom";
}

export function replyScriptAudienceLabel(value = "") {
  const normalized = normalizeTargetAudience(value);
  if (normalized === TARGET_AUDIENCE_ALL) return "全部消息端";
  if (normalized === TARGET_AUDIENCE_NONE) return "未分配";
  const parts = normalized.split(",").filter(Boolean);
  if (parts.length <= 2) return parts.join(", ");
  return `${parts.slice(0, 2).join(", ")} 等 ${parts.length} 人`;
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "启用", "开启"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "停用", "关闭"].includes(normalized)) return false;
  return fallback;
}
