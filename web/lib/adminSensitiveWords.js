export const SENSITIVE_WORDS_PATH = "/admin/sensitive-words";

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeSensitiveWords(payload = {}) {
  const words = Array.isArray(payload?.words)
    ? payload.words
    : Array.isArray(payload?.data?.words)
      ? payload.data.words
      : [];
  return words.map(normalizeSensitiveWord).filter(Boolean);
}

export function normalizeSensitiveWord(word = {}) {
  const wordId = cleanText(word?.word_id || word?.wordId || word?.id);
  const text = cleanText(word?.word || word?.text || word?.keyword);
  if (!wordId || !text) return null;
  const enabled = parseBool(word?.enabled, true);
  return {
    wordId,
    word: text,
    enabled,
    enabledLabel: enabled ? "启用" : "停用",
    createdAt: cleanText(word?.created_at || word?.createdAt),
    updatedAt: cleanText(word?.updated_at || word?.updatedAt),
  };
}

export function buildSensitiveWordUpsertMutation(options = {}) {
  const word = cleanText(options.word);
  if (!word) return { ok: false, error: "word_required" };
  const enabled = typeof options.enabled === "boolean" ? options.enabled : true;
  const body = { word, enabled };
  const wordId = cleanText(options.wordId || options.word_id);
  if (wordId) body.word_id = wordId;
  return {
    ok: true,
    method: "POST",
    path: SENSITIVE_WORDS_PATH,
    body,
  };
}

export function buildSensitiveWordDeleteMutation(wordId = "") {
  const normalizedWordId = cleanText(wordId);
  if (!normalizedWordId) return { ok: false, error: "word_id_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${SENSITIVE_WORDS_PATH}/${encodeURIComponent(normalizedWordId)}`,
  };
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
