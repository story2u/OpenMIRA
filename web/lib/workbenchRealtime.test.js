import assert from "node:assert/strict";
import test from "node:test";

import {
  buildWorkbenchConversationLookupRequest,
  extractRealtimeConversationIds,
  resolveWorkbenchConversationLookupResult,
  resolveWorkbenchAISuggestion,
  resolveWorkbenchRealtimeIntent,
  workbenchConversationRealtimeTopics,
  workbenchTaskRealtimeTopics,
} from "./workbenchRealtime.js";

test("workbench realtime topics subscribe to legacy conversation and task streams", () => {
  assert.deepEqual(workbenchConversationRealtimeTopics, [
    "conversation.assignment",
    "conversation.ai_suggested",
    "conversation.media_ready",
    "conversation.message",
    "conversation.voice_transcription_ready",
    "customer.relation",
    "friend.added",
  ]);
  assert.deepEqual(workbenchTaskRealtimeTopics, ["task.status"]);
});

test("resolveWorkbenchAISuggestion extracts legacy managed reply suggestions", () => {
  const suggestion = resolveWorkbenchAISuggestion({
    channel: "conversations",
    event: "conversation.ai_suggested",
    topic: "conversation.ai_suggested",
    payload: {
      conversation_id: " conv-1 ",
      suggestion_id: " suggest-1 ",
      message: "  AI suggested text  ",
      source: "coze-auto-reply",
      conversation: {
        device_id: "device-1",
        sender_id: "external-1",
      },
    },
  });

  assert.deepEqual(suggestion, {
    conversationId: "conv-1",
    suggestionId: "suggest-1",
    message: "AI suggested text",
    source: "coze-auto-reply",
    conversation: {
      conversation_id: "conv-1",
      suggestion_id: " suggest-1 ",
      message: "  AI suggested text  ",
      source: "coze-auto-reply",
      device_id: "device-1",
      sender_id: "external-1",
    },
  });
  assert.equal(resolveWorkbenchAISuggestion({ topic: "conversation.message", payload: {} }), null);
});

test("workbench conversation lookup helpers fetch hidden suggestion conversations", () => {
  assert.deepEqual(buildWorkbenchConversationLookupRequest(" conv-1 ", { selectedAccountID: "account:acc-1" }), {
    ok: true,
    path: "/cs/workbench/conversations",
    params: {
      conversation_id: "conv-1",
      conversation_limit: 1,
      selected_account_id: "account:acc-1",
      mode_filter: "all",
      status_filter: "all",
    },
  });
  assert.equal(buildWorkbenchConversationLookupRequest(" ").error, "conversation_required");

  const row = { conversation_id: "conv-2", conversation_key: "conv-key", device_id: "device-1" };
  assert.equal(resolveWorkbenchConversationLookupResult({ conversations: [row] }, "conv-key"), row);
  assert.equal(resolveWorkbenchConversationLookupResult({ conversations: [row] }, "missing"), null);
});

test("resolveWorkbenchRealtimeIntent refreshes list and selected messages for conversation messages", () => {
  const intent = resolveWorkbenchRealtimeIntent(
    {
      channel: "conversations",
      event: "conversation.message",
      topic: "conversation.message",
      payload: {
        conversation_id: " conv-1 ",
        resolved_conversation_id: "conv-resolved",
      },
    },
    { selectedConversationId: "conv-1" },
  );

  assert.equal(intent.recognized, true);
  assert.equal(intent.refreshConversations, true);
  assert.equal(intent.refreshMessages, true);
  assert.equal(intent.selectedConversationMatched, true);
  assert.deepEqual(intent.conversationIds, ["conv-1", "conv-resolved"]);
  assert.equal(intent.reason, "conversation_message");
});

test("resolveWorkbenchRealtimeIntent refreshes selected messages for revoke events", () => {
  const intent = resolveWorkbenchRealtimeIntent(
    {
      channel: "conversations",
      event: "conversation.message.revoke",
      topic: "conversation.message",
      payload: {
        message: {
          conversation_id: "conv-1",
          trace_id: "trace-1",
          revoke_status: "pending",
        },
      },
    },
    { selectedConversationId: "conv-1" },
  );

  assert.equal(intent.recognized, true);
  assert.equal(intent.refreshConversations, true);
  assert.equal(intent.refreshMessages, true);
  assert.equal(intent.selectedConversationMatched, true);
  assert.equal(intent.reason, "conversation_message");
});

test("resolveWorkbenchRealtimeIntent keeps media events from refreshing unrelated message panels", () => {
  const intent = resolveWorkbenchRealtimeIntent(
    {
      channel: "conversations",
      event: "conversation.media_ready",
      topic: "conversation.media_ready",
      payload: { conversation_id: "conv-2" },
    },
    { selectedConversationId: "conv-1" },
  );

  assert.equal(intent.recognized, true);
  assert.equal(intent.refreshConversations, true);
  assert.equal(intent.refreshMessages, false);
  assert.equal(intent.reason, "conversation_media_ready");
});

test("resolveWorkbenchRealtimeIntent refreshes selected messages for task status events with conversation id", () => {
  const intent = resolveWorkbenchRealtimeIntent(
    {
      channel: "tasks",
      event: "task.status",
      topic: "task.status",
      payload: {
        task_id: "task-1",
        conversation_id: "conv-1",
        status: "success",
      },
    },
    { selectedConversationId: "conv-1" },
  );

  assert.equal(intent.recognized, true);
  assert.equal(intent.refreshConversations, true);
  assert.equal(intent.refreshMessages, true);
  assert.equal(intent.reason, "task_status");
});

test("resolveWorkbenchRealtimeIntent refreshes list for assignment aliases without message reload", () => {
  const intent = resolveWorkbenchRealtimeIntent(
    {
      channel: "conversations",
      event: "conversation.transferred",
      topic: "conversation.assignment",
      payload: {
        assignment: {
          conversation_key: "conv-key-1",
          conversation_ids: ["conv-1", " conv-2 "],
        },
      },
    },
    { selectedConversationId: "conv-1" },
  );

  assert.equal(intent.recognized, true);
  assert.equal(intent.refreshConversations, true);
  assert.equal(intent.refreshMessages, false);
  assert.deepEqual(intent.conversationIds, ["conv-1", "conv-2", "conv-key-1"]);
  assert.equal(intent.reason, "conversation_assignment");
});

test("resolveWorkbenchRealtimeIntent ignores unrelated envelopes", () => {
  const intent = resolveWorkbenchRealtimeIntent({
    channel: "devices",
    event: "device.heartbeat",
    topic: "device.heartbeat",
    payload: { device_id: "zimo" },
  });

  assert.equal(intent.recognized, false);
  assert.equal(intent.refreshConversations, false);
  assert.equal(intent.refreshMessages, false);
});

test("extractRealtimeConversationIds reads direct, list, and nested payload shapes", () => {
  assert.deepEqual(
    extractRealtimeConversationIds({
      conversation_id: "conv-1",
      conversation_ids: ["conv-2", "", " conv-3 "],
      message: { resolved_conversation_id: "conv-4" },
      payload: { conversation_key: "conv-key" },
    }),
    ["conv-1", "conv-2", "conv-3", "conv-4", "conv-key"],
  );
});
