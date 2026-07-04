import assert from "node:assert/strict";
import { File } from "node:buffer";
import test from "node:test";
import {
  AI_SUGGESTION_CONFLICT_MESSAGE,
  MEDIA_MAX_UPLOAD_BYTES,
  buildConversationCallPayload,
  buildConversationHangupPayload,
  buildConversationReadMutation,
  buildConversationMediaSendPayload,
  buildConversationResendPayload,
  buildConversationRevokePayload,
  buildConversationReplyPayload,
  buildSidebarMixedMessagesMutation,
  buildAISuggestionEditDraft,
  canRetryLocalMediaMessage,
  canEditAISuggestionMessage,
  canResendConversationMessage,
  canRevokeConversationMessage,
  createLocalMediaRetryMessage,
  createLocalMediaOutgoingMessage,
  createLocalOutgoingMessage,
  createResendOutgoingMessage,
  isAISuggestionConflictError,
  mergeLocalOutgoingMessages,
  nextManualTextClientBatch,
  normalizeSidebarMixedMessages,
  reconcileLocalOutgoingMessage,
  resolveRevokeOccurrenceFromBottom,
  resolveConversationReceiver,
  resolveSendDeviceId,
} from "./workbenchSend.js";

const conversation = {
  conversation_id: "conv-1",
  resolved_device_id: "archive_user:device-1",
  sender_id: "external-1",
  sender_name: "客户原名",
  sender_remark: "客户备注",
  customer_name: "客户展示名",
};

test("resolveSendDeviceId strips archive prefix and uses legacy priority", () => {
  assert.equal(resolveSendDeviceId(conversation), "device-1");
  assert.equal(resolveSendDeviceId({ account_device_id: "device-2", device_id: "device-3" }), "device-2");
});

test("resolveConversationReceiver prefers remark before sender name", () => {
  assert.equal(resolveConversationReceiver(conversation), "客户备注");
  assert.equal(resolveConversationReceiver({ sender_name: "客户原名", conversation_name: "会话名" }), "客户原名");
});

test("buildConversationReplyPayload mirrors legacy conversation reply request", () => {
  const result = buildConversationReplyPayload(conversation, "  hello  ", {
    clientBatch: { client_batch_id: "batch-1", client_batch_index: 2 },
  });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.deepEqual(result.body, {
    device_id: "device-1",
    sender_id: "external-1",
    sender_name: "客户展示名",
    message: "hello",
    source: "cloud-web",
    target_username: "客户备注",
    client_batch_id: "batch-1",
    client_batch_index: 2,
  });
});

test("buildConversationReplyPayload carries AI suggestion id and allows backend-owned text", () => {
  const result = buildConversationReplyPayload(conversation, "", {
    aiSuggestionID: "suggest-1",
    source: "coze-auto-reply",
  });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.equal(result.body.message, "");
  assert.equal(result.body.ai_suggestion_id, "suggest-1");
  assert.equal(result.body.source, "coze-auto-reply");
});

test("buildConversationReplyPayload reports missing send prerequisites", () => {
  assert.equal(buildConversationReplyPayload({}, "hello").error, "conversation_required");
  assert.equal(buildConversationReplyPayload({ conversation_id: "conv-1" }, "hello").error, "device_required");
  assert.equal(buildConversationReplyPayload({ conversation_id: "conv-1", device_id: "device-1" }, "hello").error, "sender_required");
});

test("buildConversationCallPayload mirrors legacy call request body", () => {
  const result = buildConversationCallPayload(conversation, "video", {
    agentID: "sdk:device-override",
    reservationID: "reservation-1",
  });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.equal(result.callType, "video");
  assert.deepEqual(result.body, {
    device_id: "device-1",
    call_type: "video",
    source: "cloud-web",
    agent_id: "sdk:device-override",
    reservation_id: "reservation-1",
  });
});

test("buildConversationCallPayload reports missing call prerequisites", () => {
  assert.equal(buildConversationCallPayload({}, "voice").error, "conversation_required");
  assert.equal(buildConversationCallPayload({ conversation_id: "conv-1" }, "voice").error, "device_required");
  assert.equal(buildConversationCallPayload(conversation, "screen").error, "call_type_required");
});

test("buildConversationHangupPayload mirrors legacy hangup request body", () => {
  const result = buildConversationHangupPayload(conversation, { reservationID: "reservation-1" });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.deepEqual(result.body, {
    device_id: "device-1",
    source: "cloud-web",
    reservation_id: "reservation-1",
  });
});

test("buildConversationReadMutation mirrors legacy mark-read route", () => {
  const result = buildConversationReadMutation({
    conversation_id: "conv/1",
    unread_count: 3,
    last_message_at: "2026-07-02T08:00:00Z",
  });

  assert.equal(result.ok, true);
  assert.equal(result.method, "POST");
  assert.equal(result.conversationId, "conv/1");
  assert.equal(result.path, "/conversations/conv%2F1/read");
  assert.equal(result.dedupeKey, "conv/1:3:2026-07-02T08:00:00Z");
});

test("buildConversationReadMutation skips no-op read states", () => {
  assert.equal(buildConversationReadMutation({}).error, "conversation_required");
  assert.equal(buildConversationReadMutation({ conversation_id: "pending:conv-1", unread_count: 3 }).error, "pending_conversation");
  assert.equal(buildConversationReadMutation({ conversation_id: "conv-1", unread_count: 0 }).error, "already_read");
  assert.equal(buildConversationReadMutation({ conversation_id: "conv-1", unread_count: "bad" }).error, "already_read");
});

test("normalizeSidebarMixedMessages keeps supported text and media entries", () => {
  assert.deepEqual(normalizeSidebarMixedMessages([
    { type: "text", message: " 你好 " },
    { type: "image", content: "https://cdn.example/a.png", filename: " a.png " },
    { type: "voice", content: "skip" },
    { type: "file", content: " " },
    "invalid",
  ]), [
    { type: "text", content: "你好" },
    { type: "image", content: "https://cdn.example/a.png", filename: "a.png" },
  ]);
});

test("buildSidebarMixedMessagesMutation mirrors sidebar-command payload", () => {
  const result = buildSidebarMixedMessagesMutation({
    conversation_id: "fallback-conv",
    conversation_key: "conv/1",
    resolved_device_id: "archive_user:device-1",
    sender_id: "external-1",
    sender_name: "客户原名",
    sender_remark: "客户备注",
    organization_name: "子墨",
    account_agent_id: "sdk:device-1",
  }, [
    { type: "text", content: "你好" },
    { type: "file", content: "https://cdn.example/a.pdf", filename: "a.pdf" },
  ]);

  assert.equal(result.ok, true);
  assert.equal(result.method, "POST");
  assert.equal(result.deviceId, "device-1");
  assert.equal(result.path, "/platform/device/device-1/sidebar-command");
  assert.deepEqual(result.body, {
    type: "send_mixed_messages",
    receiver: "客户备注",
    organization_name: "子墨",
    conversation_id: "conv/1",
    session_id: "conv/1",
    sender_id: "external-1",
    messages: [
      { type: "text", content: "你好" },
      { type: "file", content: "https://cdn.example/a.pdf", filename: "a.pdf" },
    ],
    source: "cloud-web",
    agent_id: "sdk:device-1",
  });
});

test("buildSidebarMixedMessagesMutation reports missing sidebar prerequisites", () => {
  assert.equal(buildSidebarMixedMessagesMutation({}, [{ type: "text", content: "hi" }]).error, "device_required");
  assert.equal(buildSidebarMixedMessagesMutation({ device_id: "device-1" }, [{ type: "text", content: "hi" }]).error, "receiver_required");
  assert.equal(buildSidebarMixedMessagesMutation({ ...conversation, device_id: "device-1" }, []).error, "messages_required");
});

test("nextManualTextClientBatch reuses close sends for the same target", () => {
  const first = nextManualTextClientBatch(null, conversation, {
    nowMs: 1000,
    createId: () => "batch-1",
  });
  const second = nextManualTextClientBatch(first.state, conversation, {
    nowMs: 2000,
    createId: () => "batch-2",
  });

  assert.deepEqual(first.payload, { client_batch_id: "batch-1", client_batch_index: 0 });
  assert.deepEqual(second.payload, { client_batch_id: "batch-1", client_batch_index: 1 });
});

test("create and reconcile local outgoing messages", () => {
  const local = createLocalOutgoingMessage(conversation, "hello", {
    now: new Date("2026-07-01T00:00:00Z"),
    localID: "local-1",
  });
  const reconciled = reconcileLocalOutgoingMessage(local, {
    task: { task_id: "task-1", trace_id: "trace-1", status: "accepted" },
  });

  assert.equal(local.optimistic, true);
  assert.equal(reconciled.task_id, "task-1");
  assert.equal(reconciled.trace_id, "trace-1");
  assert.equal(reconciled.send_status, "accepted");
});

test("createLocalOutgoingMessage marks managed AI suggestion echoes", () => {
  const local = createLocalOutgoingMessage(conversation, "AI suggested text", {
    now: new Date("2026-07-01T00:00:00Z"),
    localID: "local-ai-1",
    aiSuggestionID: "suggest-1",
    messageOrigin: "ai_suggestion",
    source: "coze-auto-reply",
  });

  assert.equal(local.local_id, "local-ai-1");
  assert.equal(local.ai_suggestion_id, "suggest-1");
  assert.equal(local.message_origin, "ai_suggestion");
  assert.equal(local.source, "coze-auto-reply");
  assert.equal(local.content, "AI suggested text");
  assert.equal(local.send_status, "pending");
});

test("AI suggestion edit helpers only allow failed local text suggestions", () => {
  const failedSuggestion = createLocalOutgoingMessage(conversation, " AI suggested text ", {
    localID: "ai-suggestion-suggest-1",
    aiSuggestionID: "suggest-1",
    messageOrigin: "ai_suggestion",
    source: "xiaobei-auto-reply",
  });
  failedSuggestion.send_status = "failed";
  failedSuggestion.send_error = "device offline";

  assert.equal(canEditAISuggestionMessage(failedSuggestion), true);
  assert.deepEqual(buildAISuggestionEditDraft(failedSuggestion), {
    conversationId: "conv-1",
    suggestionId: "suggest-1",
    localId: "ai-suggestion-suggest-1",
    source: "xiaobei-auto-reply",
    text: "AI suggested text",
  });
  assert.equal(canEditAISuggestionMessage({ ...failedSuggestion, send_status: "pending" }), false);
  assert.equal(canEditAISuggestionMessage({ ...failedSuggestion, ai_suggestion_id: "" }), false);
  assert.equal(canEditAISuggestionMessage({ ...failedSuggestion, message_origin: "manual_reply", local_id: "local-1" }), false);
  assert.equal(canEditAISuggestionMessage({ ...failedSuggestion, msg_type: "image" }), false);
  assert.equal(buildAISuggestionEditDraft({ ...failedSuggestion, content: " " }), null);
});

test("isAISuggestionConflictError matches legacy conflict detail", () => {
  assert.equal(isAISuggestionConflictError(new Error(AI_SUGGESTION_CONFLICT_MESSAGE)), true);
  assert.equal(isAISuggestionConflictError(new Error("device offline")), false);
});

test("buildConversationMediaSendPayload builds multipart fields for image send", () => {
  const file = new File(["abc"], "photo.png", { type: "image/png" });
  const result = buildConversationMediaSendPayload({
    ...conversation,
    organization_name: "组织",
    account_agent_id: "sdk:device-1",
  }, file);

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.equal(result.kind, "image");
  assert.equal(result.endpoint, "/send/image");
  assert.equal(result.formData.get("file").name, "photo.png");
  assert.equal(result.formData.get("device_id"), "device-1");
  assert.equal(result.formData.get("username"), "客户展示名");
  assert.equal(result.formData.get("target_username"), "客户备注");
  assert.equal(result.formData.get("sender_id"), "external-1");
  assert.equal(result.formData.get("conversation_id"), "conv-1");
  assert.equal(result.formData.get("organization_name"), "组织");
  assert.equal(result.formData.get("agent_id"), "sdk:device-1");
});

test("buildConversationMediaSendPayload dispatches video audio and generic files", () => {
  assert.equal(buildConversationMediaSendPayload(conversation, new File(["v"], "clip.mp4", { type: "video/mp4" })).endpoint, "/send/video");
  assert.equal(buildConversationMediaSendPayload(conversation, new File(["a"], "voice.webm", { type: "audio/webm" }), { voiceDurationSec: 3 }).endpoint, "/send/voice");
  const generic = buildConversationMediaSendPayload(conversation, new File(["x"], "manual.pdf", { type: "application/pdf" }));
  assert.equal(generic.endpoint, "/send/file");
});

test("buildConversationMediaSendPayload reports media prerequisites", () => {
  assert.equal(buildConversationMediaSendPayload({}, new File(["x"], "a.png", { type: "image/png" })).error, "conversation_required");
  assert.equal(buildConversationMediaSendPayload(conversation, null).error, "file_required");
  assert.equal(buildConversationMediaSendPayload({ ...conversation, resolved_device_id: "" }, new File(["x"], "a.png", { type: "image/png" })).error, "device_required");
  assert.equal(buildConversationMediaSendPayload({ ...conversation, sender_id: "" }, new File(["x"], "a.png", { type: "image/png" })).error, "sender_required");
  assert.equal(buildConversationMediaSendPayload(conversation, { name: "big.bin", size: MEDIA_MAX_UPLOAD_BYTES + 1 }).error, "file_too_large");
});

test("createLocalMediaOutgoingMessage creates pending media echo", () => {
  const file = new File(["abc"], "photo.png", { type: "image/png" });
  const local = createLocalMediaOutgoingMessage(conversation, file, "image", {
    now: new Date("2026-07-01T00:00:00Z"),
    localID: "local-media-1",
  });

  assert.equal(local.local_id, "local-media-1");
  assert.equal(local.conversation_id, "conv-1");
  assert.equal(local.msg_type, "image");
  assert.equal(local.content, "photo.png");
  assert.equal(local.media_filename, "photo.png");
  assert.equal(local.media_size_bytes, 3);
  assert.equal(local.send_status, "pending");
  assert.equal(local.optimistic, true);
  assert.equal(local.local_media_file, file);
  assert.equal(local.local_media_kind, "image");
});

test("local media retry keeps the original browser file only for failed optimistic media", () => {
  const file = new File(["abc"], "voice.webm", { type: "audio/webm" });
  file.voiceDurationSec = 4;
  const local = createLocalMediaOutgoingMessage(conversation, file, "voice", {
    now: new Date("2026-07-01T00:00:00Z"),
    localID: "local-media-voice",
  });
  const failed = { ...local, send_status: "failed", send_error: "network" };

  assert.equal(local.voice_duration_sec, 4);
  assert.equal(canRetryLocalMediaMessage(failed), true);
  assert.equal(canRetryLocalMediaMessage({ ...failed, send_status: "pending" }), false);
  assert.equal(canRetryLocalMediaMessage({ ...failed, optimistic: false }), false);
  assert.equal(canRetryLocalMediaMessage({ ...failed, local_media_file: null }), false);
  assert.equal(canRetryLocalMediaMessage({ ...failed, msg_type: "text", local_media_kind: "text" }), false);

  const retrying = createLocalMediaRetryMessage(failed, { now: new Date("2026-07-01T00:01:00Z") });
  assert.equal(retrying.local_id, "local-media-voice");
  assert.equal(retrying.local_media_file, file);
  assert.equal(retrying.send_status, "pending");
  assert.equal(retrying.send_error, "");
  assert.equal(retrying.retry_count, 1);
  assert.equal(retrying.retry_started_at, "2026-07-01T00:01:00.000Z");
});

test("mergeLocalOutgoingMessages hides local echoes once server rows arrive", () => {
  const local = { conversation_id: "conv-1", trace_id: "trace-1", task_id: "task-1" };
  const merged = mergeLocalOutgoingMessages([{ trace_id: "trace-1" }], [local], "conv-1");
  assert.equal(merged.length, 1);
  assert.deepEqual(mergeLocalOutgoingMessages([], [local], "conv-1"), [local]);
});

test("canResendConversationMessage allows persisted failed text and media", () => {
  const failed = {
    conversation_id: "conv-1",
    trace_id: "trace-failed",
    direction: "outgoing",
    send_status: "failed",
    msg_type: "text",
    content: "您好",
  };

  assert.equal(canResendConversationMessage(failed), true);
  assert.equal(canResendConversationMessage({ ...failed, msg_type: "image", content: "", media_url: "https://cdn.example/a.png" }), true);
  assert.equal(canResendConversationMessage({ ...failed, msg_type: "video", content: "https://cdn.example/a.mp4" }), true);
  assert.equal(canResendConversationMessage({ ...failed, msg_type: "file", content: "https://cdn.example/a.pdf" }), true);
  assert.equal(canResendConversationMessage({ ...failed, trace_id: "local-1" }), false);
  assert.equal(canResendConversationMessage({ ...failed, optimistic: true }), false);
  assert.equal(canResendConversationMessage({ ...failed, send_status: "success" }), false);
  assert.equal(canResendConversationMessage({ ...failed, msg_type: "voice", media_url: "https://cdn.example/a.webm" }), false);
  assert.equal(canResendConversationMessage({ ...failed, msg_type: "image", content: "", media_url: "" }), false);
  assert.equal(canResendConversationMessage({ ...failed, resend_status: "pending" }), false);
});

test("canResendConversationMessage allows legacy failed sidebar commands", () => {
  const failedSidebar = {
    conversation_id: "conv-1",
    trace_id: "trace-sidebar",
    direction: "outgoing",
    send_status: "failed",
    msg_type: "text",
    content: "",
    task_source: "sidebar",
    sidebar_command_payload: { type: "send_address", address: "上海门店" },
  };

  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "send_address" }), true);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "request_money" }), true);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "appointment_billing" }), true);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "transfer_money" }), false);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "send_address", task_source: "", sidebar_command_payload: null }), false);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "send_address", send_status: "success" }), false);
  assert.equal(canResendConversationMessage({ ...failedSidebar, command_type: "send_address", resend_status: "running" }), false);
});

test("buildConversationResendPayload mirrors legacy resend endpoint body", () => {
  const result = buildConversationResendPayload(conversation, {
    conversation_id: "conv-1",
    trace_id: "trace-failed",
    direction: "outgoing",
    send_status: "timeout",
    msg_type: "text",
    content: "超时消息",
  }, { source: "cloud-web-test", agentID: "sdk:device-override" });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.equal(result.traceId, "trace-failed");
  assert.deepEqual(result.body, {
    source: "cloud-web-test",
    device_id: "device-1",
    agent_id: "sdk:device-override",
  });

  const sidebarResult = buildConversationResendPayload(conversation, {
    conversation_id: "conv-1",
    trace_id: "trace-sidebar",
    direction: "outgoing",
    send_status: "failed",
    msg_type: "text",
    content: "",
    task_source: "sidebar",
    command_type: "request_money",
    sidebar_command_payload: { type: "request_money", money: "88.00" },
  });
  assert.equal(sidebarResult.ok, true);
  assert.equal(sidebarResult.traceId, "trace-sidebar");
  assert.deepEqual(sidebarResult.body, {
    source: "cloud-web",
    device_id: "device-1",
  });
});

test("createResendOutgoingMessage normalizes pending echo from resend response", () => {
  const local = createResendOutgoingMessage({
    task: { task_id: "task-resend-1", trace_id: "trace-resend-1", status: "accepted" },
    message: {
      trace_id: "trace-resend-1",
      conversation_id: "conv-1",
      content: "您好",
      send_status: "pending",
    },
  });

  assert.equal(local.local_id, "resend-trace-resend-1");
  assert.equal(local.trace_id, "trace-resend-1");
  assert.equal(local.task_id, "task-resend-1");
  assert.equal(local.direction, "outgoing");
  assert.equal(local.msg_type, "text");
  assert.equal(local.optimistic, true);
});

test("canRevokeConversationMessage allows fresh accepted outgoing text only", () => {
  const nowMs = new Date("2026-07-01T09:30:00.000Z").getTime();
  const fresh = {
    conversation_id: "conv-1",
    trace_id: "trace-ok",
    direction: "outgoing",
    send_status: "success",
    msg_type: "text",
    content: "可撤回",
    timestamp: "2026-07-01T09:29:00.000Z",
  };

  assert.equal(canRevokeConversationMessage(fresh, nowMs), true);
  assert.equal(canRevokeConversationMessage({ ...fresh, trace_id: "local-1" }, nowMs), false);
  assert.equal(canRevokeConversationMessage({ ...fresh, optimistic: true }, nowMs), false);
  assert.equal(canRevokeConversationMessage({ ...fresh, send_status: "failed" }, nowMs), false);
  assert.equal(canRevokeConversationMessage({ ...fresh, revoke_status: "pending" }, nowMs), false);
  assert.equal(canRevokeConversationMessage({ ...fresh, timestamp: "2026-07-01T09:20:00.000Z" }, nowMs), false);
});

test("resolveRevokeOccurrenceFromBottom counts duplicate text from latest message", () => {
  const rows = [
    { trace_id: "trace-1", direction: "outgoing", msg_type: "text", content: "同文" },
    { trace_id: "trace-2", direction: "incoming", msg_type: "text", content: "同文" },
    { trace_id: "trace-3", direction: "outgoing", msg_type: "text", content: "不同" },
    { trace_id: "trace-4", direction: "outgoing", msg_type: "text", content: "同文" },
  ];

  assert.equal(resolveRevokeOccurrenceFromBottom(rows, rows[3]), 1);
  assert.equal(resolveRevokeOccurrenceFromBottom(rows, rows[0]), 2);
});

test("buildConversationRevokePayload mirrors legacy revoke request body", () => {
  const nowMs = new Date("2026-07-01T09:30:00.000Z").getTime();
  const result = buildConversationRevokePayload(conversation, {
    conversation_id: "conv-1",
    trace_id: "trace-ok",
    direction: "outgoing",
    send_status: "success",
    msg_type: "text",
    content: "可撤回",
    timestamp: "2026-07-01T09:29:00.000Z",
  }, { nowMs, occurrenceFromBottom: 2 });

  assert.equal(result.ok, true);
  assert.equal(result.conversationId, "conv-1");
  assert.equal(result.traceId, "trace-ok");
  assert.deepEqual(result.body, {
    device_id: "device-1",
    source: "cloud-web",
    target_content: "可撤回",
    target_msg_type: "text",
    occurrence_from_bottom: 2,
  });
});
