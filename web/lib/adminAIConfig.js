export const AI_CONFIG_PATH = "/admin/ai-config";
export const AI_CONFIG_TEST_PATH = "/admin/ai-config/test";
export const AI_CONFIG_TEST_DIALOGUE_PATH = "/admin/ai-config/test-dialogue";
export const DEFAULT_AI_CONFIG_TEST_PROMPT = "请用一句中文回复：AI 配置连接正常。";
export const DEFAULT_KNOWLEDGE_DIALOGUE_QUESTION = "客户咨询退款流程时应该怎么回复？";
export const AI_TARGET_SCOPES = new Set(["none", "assignee", "account", "all"]);

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAdminAIConfig(payload = {}) {
  const config = payload?.config && typeof payload.config === "object"
    ? payload.config
    : payload?.data?.config && typeof payload.data.config === "object"
      ? payload.data.config
      : payload && typeof payload === "object"
        ? payload
        : {};
  return normalizeAIConfigObject(config);
}

export function normalizeAdminAIConfigRecords(records = []) {
  const config = {};
  (Array.isArray(records) ? records : []).forEach((record) => {
    const key = cleanText(record?.key);
    if (key) config[key] = record?.value;
  });
  return normalizeAIConfigObject(config);
}

export function buildAIConfigUpsertMutation(options = {}) {
  const baseUrl = cleanText(options.baseUrl || options.base_url);
  if (!baseUrl) return { ok: false, error: "base_url_required" };
  const model = cleanText(options.model);
  if (!model) return { ok: false, error: "model_required" };
  const timeoutSec = Number(options.timeoutSec ?? options.timeout_sec);
  if (!Number.isFinite(timeoutSec) || timeoutSec <= 0) return { ok: false, error: "timeout_invalid" };
  const temperature = Number(options.temperature);
  if (!Number.isFinite(temperature) || temperature < 0 || temperature > 2) return { ok: false, error: "temperature_invalid" };

  const localTargetScope = normalizeTargetScope(options.localTargetScope || options.local_target_scope);
  const cozeProfiles = parseProfileInput(options.cozeProfiles ?? options.coze_profiles ?? []);
  if (!cozeProfiles.ok) return { ok: false, error: "coze_profiles_invalid" };
  const xiaobeiProfiles = parseProfileInput(options.xiaobeiProfiles ?? options.xiaobei_profiles ?? []);
  if (!xiaobeiProfiles.ok) return { ok: false, error: "xiaobei_profiles_invalid" };

  const body = {
    enabled: parseBool(firstDefined(options.enabled, true), true),
    base_url: baseUrl,
    model,
    timeout_sec: timeoutSec,
    temperature,
    system_prompt: cleanText(options.systemPrompt || options.system_prompt),
    intercept_keywords: cleanText(options.interceptKeywords || options.intercept_keywords),
    default_handoff_reply: cleanText(options.defaultHandoffReply || options.default_handoff_reply),
    local_target_audience: normalizeTargetAudience(options.localTargetAudience || options.local_target_audience),
    local_target_scope: localTargetScope,
    local_target_account_ids: normalizeCSVList(options.localTargetAccountIds || options.local_target_account_ids),
    local_default_ai_enabled: parseBool(firstDefined(options.localDefaultAIEnabled, options.local_default_ai_enabled), false),
    active_coze_profile_id: cleanText(options.activeCozeProfileId || options.active_coze_profile_id),
    coze_profiles: cozeProfiles.value,
    active_xiaobei_profile_id: cleanText(options.activeXiaobeiProfileId || options.active_xiaobei_profile_id),
    xiaobei_profiles: xiaobeiProfiles.value,
  };
  const apiKey = cleanText(options.apiKey || options.api_key);
  if (apiKey) body.api_key = apiKey;

  return {
    ok: true,
    method: "POST",
    path: AI_CONFIG_PATH,
    body,
  };
}

export function buildAIConfigTestMutation(options = {}) {
  const prompt = cleanText(options.prompt);
  if (!prompt) return { ok: false, error: "prompt_required" };
  const baseUrl = cleanText(options.baseUrl || options.base_url);
  if (!baseUrl) return { ok: false, error: "base_url_required" };
  const model = cleanText(options.model);
  if (!model) return { ok: false, error: "model_required" };
  const timeoutSec = Number(options.timeoutSec ?? options.timeout_sec);
  if (!Number.isFinite(timeoutSec) || timeoutSec <= 0) return { ok: false, error: "timeout_invalid" };
  const temperature = Number(options.temperature);
  if (!Number.isFinite(temperature) || temperature < 0 || temperature > 2) return { ok: false, error: "temperature_invalid" };

  const body = {
    prompt,
    base_url: baseUrl,
    model,
    timeout_sec: timeoutSec,
    temperature,
    system_prompt: cleanText(options.systemPrompt || options.system_prompt),
  };
  const apiKey = cleanText(options.apiKey || options.api_key);
  if (apiKey) body.api_key = apiKey;

  return {
    ok: true,
    method: "POST",
    path: AI_CONFIG_TEST_PATH,
    body,
  };
}

export function normalizeAIConfigTestResult(payload = {}) {
  const data = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  const reply = cleanText(data?.reply || data?.content || data?.message);
  return {
    success: data?.success !== false,
    reply,
    raw: data && typeof data === "object" ? data : {},
  };
}

export function buildKnowledgeDialogueMutation(options = {}) {
  const question = cleanText(options.question || options.prompt);
  if (!question) return { ok: false, error: "question_required" };
  const topK = Number(options.topK ?? options.top_k ?? 3);
  const body = { question };
  if (Number.isFinite(topK) && topK > 0) {
    body.top_k = Math.floor(topK);
  }
  return {
    ok: true,
    method: "POST",
    path: AI_CONFIG_TEST_DIALOGUE_PATH,
    body,
  };
}

export function normalizeKnowledgeDialogueResult(payload = {}) {
  const data = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  const candidates = Array.isArray(data?.candidates) ? data.candidates : [];
  return {
    reply: cleanText(data?.reply || data?.content || data?.message),
    mode: cleanText(data?.mode),
    matchedQuestion: cleanText(data?.matched_question || data?.matchedQuestion),
    source: cleanText(data?.source),
    confidence: normalizeNumberInRange(data?.confidence, 0, 0, 1),
    candidates,
    raw: data && typeof data === "object" ? data : {},
  };
}

export function formatAIProfileJSON(value = []) {
  try {
    return JSON.stringify(Array.isArray(value) ? value : [], null, 2);
  } catch {
    return "[]";
  }
}

function normalizeAIConfigObject(config = {}) {
  const cozeProfiles = normalizeProfileArray(config.coze_profiles || config.cozeProfiles);
  const xiaobeiProfiles = normalizeProfileArray(config.xiaobei_profiles || config.xiaobeiProfiles);
  return {
    enabled: parseBool(config.enabled, true),
    enabledLabel: parseBool(config.enabled, true) ? "开启" : "关闭",
    baseUrl: cleanText(config.base_url || config.baseUrl),
    model: cleanText(config.model),
    timeoutSec: normalizePositiveNumber(config.timeout_sec || config.timeoutSec, 20),
    temperature: normalizeNumberInRange(config.temperature, 0.7, 0, 2),
    systemPrompt: cleanText(config.system_prompt || config.systemPrompt),
    interceptKeywords: cleanText(config.intercept_keywords || config.interceptKeywords),
    defaultHandoffReply: cleanText(config.default_handoff_reply || config.defaultHandoffReply),
    localTargetAudience: cleanText(config.local_target_audience || config.localTargetAudience) || "__NONE__",
    localTargetScope: normalizeTargetScope(config.local_target_scope || config.localTargetScope),
    localTargetAccountIds: normalizeCSVList(config.local_target_account_ids || config.localTargetAccountIds),
    localDefaultAIEnabled: parseBool(config.local_default_ai_enabled || config.localDefaultAIEnabled, false),
    apiKeySet: parseBool(config.api_key_set || config.apiKeySet, false),
    providerHint: cleanText(config.provider_hint || config.providerHint),
    activeCozeProfileId: cleanText(config.active_coze_profile_id || config.activeCozeProfileId),
    cozeProfiles,
    cozeProfilesJSON: formatAIProfileJSON(cozeProfiles),
    activeXiaobeiProfileId: cleanText(config.active_xiaobei_profile_id || config.activeXiaobeiProfileId),
    xiaobeiProfiles,
    xiaobeiProfilesJSON: formatAIProfileJSON(xiaobeiProfiles),
  };
}

function parseProfileInput(value) {
  if (typeof value === "string") {
    const text = cleanText(value);
    if (!text) return { ok: true, value: [] };
    try {
      return parseProfileInput(JSON.parse(text));
    } catch {
      return { ok: false, value: [] };
    }
  }
  if (!Array.isArray(value)) return { ok: false, value: [] };
  return { ok: true, value: value.filter((item) => item && typeof item === "object") };
}

function normalizeProfileArray(value) {
  if (Array.isArray(value)) return value.filter((item) => item && typeof item === "object");
  if (typeof value === "string") {
    const parsed = parseProfileInput(value);
    return parsed.ok ? parsed.value : [];
  }
  return [];
}

function normalizeTargetScope(value = "") {
  const scope = cleanText(value).toLowerCase();
  return AI_TARGET_SCOPES.has(scope) ? scope : "assignee";
}

function normalizeTargetAudience(value = "") {
  const normalized = cleanText(value);
  return normalized || "__NONE__";
}

function normalizeCSVList(value) {
  if (Array.isArray(value)) {
    return value.map(cleanText).filter(Boolean);
  }
  return String(value || "")
    .split(/[,\n，；;]/)
    .map(cleanText)
    .filter(Boolean);
}

function normalizePositiveNumber(value, fallback) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : fallback;
}

function normalizeNumberInRange(value, fallback, min, max) {
  const number = Number(value);
  return Number.isFinite(number) && number >= min && number <= max ? number : fallback;
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
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
