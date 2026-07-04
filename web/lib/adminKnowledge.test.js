import assert from "node:assert/strict";
import test from "node:test";

import {
  KNOWLEDGE_DOCS_PATH,
  KNOWLEDGE_SEARCH_PATH,
  buildKnowledgeDocumentMutation,
  buildKnowledgeSearchPayload,
  isKnowledgeFilenameAllowed,
  normalizeKnowledgeDocuments,
  normalizeKnowledgeSearchResults,
} from "./adminKnowledge.js";

class FakeFormData {
  constructor() {
    this.entries = [];
  }

  append(key, value) {
    this.entries.push([key, value]);
  }
}

test("normalizeKnowledgeDocuments keeps legacy document fields", () => {
  const docs = normalizeKnowledgeDocuments({
    documents: [
      {
        doc_id: "doc-1",
        filename: "FAQ.md",
        status: "indexed",
        size: "12KB",
        created_at: "2026-07-02T01:02:03Z",
      },
      { filename: "missing-id.md" },
    ],
  });

  assert.equal(docs.length, 1);
  assert.equal(docs[0].docId, "doc-1");
  assert.equal(docs[0].filename, "FAQ.md");
  assert.equal(docs[0].statusLabel, "已索引");
  assert.equal(docs[0].size, "12KB");
});

test("buildKnowledgeDocumentMutation mirrors admin document routes", () => {
  const file = { name: "faq.md" };
  const upload = buildKnowledgeDocumentMutation("upload", { file, FormDataCtor: FakeFormData });
  const update = buildKnowledgeDocumentMutation("update", { docId: "doc/1", file, FormDataCtor: FakeFormData });
  const remove = buildKnowledgeDocumentMutation("delete", { docId: "doc/1" });
  const reindex = buildKnowledgeDocumentMutation("reindex", { docId: "doc/1" });

  assert.equal(upload.ok, true);
  assert.equal(upload.method, "POST");
  assert.equal(upload.path, KNOWLEDGE_DOCS_PATH);
  assert.deepEqual(upload.body.entries, [["file", file]]);
  assert.equal(update.method, "PUT");
  assert.equal(update.path, `${KNOWLEDGE_DOCS_PATH}/doc%2F1`);
  assert.equal(remove.method, "DELETE");
  assert.equal(remove.path, `${KNOWLEDGE_DOCS_PATH}/doc%2F1`);
  assert.equal(reindex.method, "POST");
  assert.equal(reindex.path, `${KNOWLEDGE_DOCS_PATH}/doc%2F1/reindex`);
});

test("buildKnowledgeDocumentMutation reports invalid upload prerequisites", () => {
  assert.equal(buildKnowledgeDocumentMutation("upload", { FormDataCtor: FakeFormData }).error, "file_required");
  assert.equal(buildKnowledgeDocumentMutation("upload", { file: { name: "bad.exe" }, FormDataCtor: FakeFormData }).error, "unsupported_file");
  assert.equal(buildKnowledgeDocumentMutation("update", { file: { name: "faq.md" }, FormDataCtor: FakeFormData }).error, "doc_required");
  assert.equal(buildKnowledgeDocumentMutation("unknown").error, "action_required");
  assert.equal(isKnowledgeFilenameAllowed("guide.DOCX"), true);
  assert.equal(isKnowledgeFilenameAllowed("guide.exe"), false);
});

test("knowledge search payload and results keep legacy fields", () => {
  const payload = buildKnowledgeSearchPayload("  退款  ");
  const results = normalizeKnowledgeSearchResults({
    results: [
      { doc_id: "doc-1", source: "FAQ.md", content: "退款规则", score: 0.875 },
      { source: "", content: "" },
    ],
  });

  assert.equal(payload.ok, true);
  assert.equal(payload.method, "POST");
  assert.equal(payload.path, KNOWLEDGE_SEARCH_PATH);
  assert.deepEqual(payload.body, { query: "退款" });
  assert.equal(buildKnowledgeSearchPayload("").error, "query_required");
  assert.equal(results.length, 1);
  assert.equal(results[0].source, "FAQ.md");
  assert.equal(results[0].scoreLabel, "87.5%");
});
