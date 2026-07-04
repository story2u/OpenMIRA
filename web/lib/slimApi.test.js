import assert from "node:assert/strict";
import test from "node:test";

import { apiURL, normalizeMessages, normalizeSOPCollection } from "./slimApi.js";

test("apiURL joins base and path predictably", () => {
  assert.equal(apiURL("/messages/incoming", "http://localhost:8080/api/v1/"), "http://localhost:8080/api/v1/messages/incoming");
});

test("normalizeMessages keeps the slim message shape", () => {
  const messages = normalizeMessages({
    messages: [{ id: " msg-1 ", conversation_id: "conv-1", direction: "incoming", sender_name: "客户", content: " hello " }],
  });
  assert.deepEqual(messages, [{
    id: "msg-1",
    conversationId: "conv-1",
    direction: "incoming",
    senderName: "客户",
    content: "hello",
    timestamp: "",
  }]);
});

test("normalizeSOPCollection returns empty arrays for missing keys", () => {
  assert.deepEqual(normalizeSOPCollection({}, "flows"), []);
});
