export const KNOWLEDGE_DOCS_PATH = "/admin/knowledge/documents";
export const KNOWLEDGE_SEARCH_PATH = "/admin/knowledge/search";
export const KNOWLEDGE_FILE_ACCEPT = ".pdf,.txt,.md,.docx,.doc";

const KNOWLEDGE_ALLOWED_EXTENSIONS = new Set([".pdf", ".txt", ".md", ".docx", ".doc"]);

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeKnowledgeDocuments(payload = {}) {
  const documents = Array.isArray(payload?.documents)
    ? payload.documents
    : Array.isArray(payload?.data?.documents)
      ? payload.data.documents
      : [];
  return documents.map(normalizeKnowledgeDocument).filter(Boolean);
}

export function normalizeKnowledgeDocument(document = {}) {
  const docId = cleanText(document?.doc_id || document?.docId || document?.id);
  if (!docId) return null;
  const filename = cleanText(document?.filename || document?.file_name || document?.name) || docId;
  const status = cleanText(document?.status) || "pending";
  return {
    docId,
    filename,
    status,
    statusLabel: knowledgeStatusLabel(status),
    filePath: cleanText(document?.file_path || document?.filePath),
    size: cleanText(document?.size || document?.size_text || document?.sizeBytes || document?.size_bytes),
    createdAt: cleanText(document?.created_at || document?.createdAt),
    updatedAt: cleanText(document?.updated_at || document?.updatedAt),
  };
}

export function normalizeKnowledgeSearchResults(payload = {}) {
  const results = Array.isArray(payload?.results)
    ? payload.results
    : Array.isArray(payload?.data?.results)
      ? payload.data.results
      : [];
  return results.map((result) => {
    const score = Number(result?.score);
    return {
      docId: cleanText(result?.doc_id || result?.docId),
      source: cleanText(result?.source || result?.filename || result?.doc_id || result?.docId) || "unknown",
      content: cleanText(result?.content || result?.snippet || result?.text),
      score: Number.isFinite(score) ? score : null,
      scoreLabel: Number.isFinite(score) ? `${(score * 100).toFixed(1)}%` : "",
    };
  }).filter((result) => result.content || result.source !== "unknown");
}

export function knowledgeStatusLabel(status = "") {
  const normalized = cleanText(status).toLowerCase();
  if (normalized === "indexed") return "已索引";
  if (normalized === "indexing") return "索引中";
  if (normalized === "pending") return "待处理";
  return cleanText(status) || "待处理";
}

export function isKnowledgeFilenameAllowed(filename = "") {
  const lower = cleanText(filename).toLowerCase();
  const dot = lower.lastIndexOf(".");
  if (dot < 0) return false;
  return KNOWLEDGE_ALLOWED_EXTENSIONS.has(lower.slice(dot));
}

export function buildKnowledgeDocumentMutation(action, options = {}) {
  const normalizedAction = cleanText(action).toLowerCase();
  const docId = cleanText(options.docId || options.doc_id);
  if (normalizedAction === "upload" || normalizedAction === "update") {
    const bodyResult = buildKnowledgeUploadFormData(options.file, options);
    if (!bodyResult.ok) return bodyResult;
    if (normalizedAction === "upload") {
      return { ok: true, method: "POST", path: KNOWLEDGE_DOCS_PATH, body: bodyResult.body };
    }
    if (!docId) return { ok: false, error: "doc_required" };
    return {
      ok: true,
      method: "PUT",
      path: `${KNOWLEDGE_DOCS_PATH}/${encodeURIComponent(docId)}`,
      body: bodyResult.body,
    };
  }
  if (normalizedAction === "delete") {
    if (!docId) return { ok: false, error: "doc_required" };
    return { ok: true, method: "DELETE", path: `${KNOWLEDGE_DOCS_PATH}/${encodeURIComponent(docId)}` };
  }
  if (normalizedAction === "reindex") {
    if (!docId) return { ok: false, error: "doc_required" };
    return {
      ok: true,
      method: "POST",
      path: `${KNOWLEDGE_DOCS_PATH}/${encodeURIComponent(docId)}/reindex`,
      body: {},
    };
  }
  return { ok: false, error: "action_required" };
}

export function buildKnowledgeSearchPayload(query = "") {
  const normalizedQuery = cleanText(query);
  if (!normalizedQuery) return { ok: false, error: "query_required" };
  return {
    ok: true,
    method: "POST",
    path: KNOWLEDGE_SEARCH_PATH,
    body: { query: normalizedQuery },
  };
}

function buildKnowledgeUploadFormData(file, options = {}) {
  if (!file) return { ok: false, error: "file_required" };
  const filename = cleanText(file?.name);
  if (filename && !isKnowledgeFilenameAllowed(filename)) {
    return { ok: false, error: "unsupported_file" };
  }
  const FormDataCtor = options.FormDataCtor || globalThis.FormData;
  if (typeof FormDataCtor !== "function") {
    return { ok: false, error: "formdata_unavailable" };
  }
  const formData = new FormDataCtor();
  formData.append("file", file);
  return { ok: true, body: formData };
}
