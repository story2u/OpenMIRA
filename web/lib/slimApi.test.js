import assert from "node:assert/strict";
import test from "node:test";

import { apiURL, normalizeMessages, normalizeSOPCollection } from "./slimApi.js";

test("apiURL joins base and path predictably", () => {
  assert.equal(apiURL("/messages/incoming", "http://localhost:8080/api/v1/"), "http://localhost:8080/api/v1/messages/incoming");
});

test("normalizeMessages keeps the slim message shape", () => {
  const messages = normalizeMessages({
    messages: [{
      id: " msg-1 ",
      conversation_id: "conv-1",
      direction: "incoming",
      source_channel: " webchat ",
      external_message_id: " ext-1 ",
      sender_name: "客户",
      content: " hello ",
      received_at: "2026-01-01T00:00:00Z",
    }],
  });
  assert.deepEqual(messages, [{
    id: "msg-1",
    conversationId: "conv-1",
    direction: "incoming",
    sourceChannel: "webchat",
    externalMessageId: "ext-1",
    senderName: "客户",
    content: "hello",
    timestamp: "",
    receivedAt: "2026-01-01T00:00:00Z",
  }]);
});

test("normalizeSOPCollection returns empty arrays for missing keys", () => {
  assert.deepEqual(normalizeSOPCollection({}, "flows"), []);
});
