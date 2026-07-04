import assert from "node:assert/strict";
import test from "node:test";

import {
  SENSITIVE_WORDS_PATH,
  buildSensitiveWordDeleteMutation,
  buildSensitiveWordUpsertMutation,
  normalizeSensitiveWords,
} from "./adminSensitiveWords.js";

test("normalizeSensitiveWords keeps enabled labels and timestamps", () => {
  const words = normalizeSensitiveWords({
    words: [
      { word_id: "sw-1", word: "风险词", enabled: true, updated_at: "2026-07-02T01:02:03Z" },
      { word_id: "sw-2", word: "停用词", enabled: false },
      { word_id: "", word: "missing" },
    ],
  });

  assert.equal(words.length, 2);
  assert.equal(words[0].wordId, "sw-1");
  assert.equal(words[0].enabled, true);
  assert.equal(words[0].enabledLabel, "启用");
  assert.equal(words[1].enabledLabel, "停用");
});

test("buildSensitiveWordUpsertMutation mirrors legacy POST body", () => {
  const create = buildSensitiveWordUpsertMutation({ word: " 风险词 " });
  const update = buildSensitiveWordUpsertMutation({ wordId: "sw/1", word: "停用词", enabled: false });

  assert.equal(create.ok, true);
  assert.equal(create.method, "POST");
  assert.equal(create.path, SENSITIVE_WORDS_PATH);
  assert.deepEqual(create.body, { word: "风险词", enabled: true });
  assert.deepEqual(update.body, { word: "停用词", enabled: false, word_id: "sw/1" });
  assert.equal(buildSensitiveWordUpsertMutation({ word: " " }).error, "word_required");
});

test("buildSensitiveWordDeleteMutation mirrors legacy DELETE path", () => {
  const mutation = buildSensitiveWordDeleteMutation("sw/1");

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${SENSITIVE_WORDS_PATH}/sw%2F1`);
  assert.equal(buildSensitiveWordDeleteMutation("").error, "word_id_required");
});
