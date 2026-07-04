import assert from "node:assert/strict";
import test from "node:test";

import {
  AI_REPLY_ERROR_DISMISS_STORAGE_KEY,
  buildConversationAIModeMutation,
  formatPendingReplyDuration,
  isAIReplyErrorDismissed,
  readDismissedAIReplyErrorKeys,
  rememberDismissedAIReplyError,
  resolveConversationAIToggleState,
  resolvePendingReplySeconds,
  resolveWorkbenchAIReplyErrorNotice,
  resolveWorkbenchConversationBadges,
  resolveWorkbenchConversationStatus,
} from "./workbenchConversationState.js";

test("resolveWorkbenchConversationStatus prefers backend pending snapshot over stale outgoing direction", () => {
  const nowMs = Date.parse("2026-04-20T10:00:00.000Z");
  const status = resolveWorkbenchConversationStatus({
    reply_state: "pending",
    pending_reply_seconds: 90,
    last_direction: "outgoing",
    last_message_at: "2026-04-20T09:58:30.000Z",
  }, nowMs);

  assert.equal(status.replyState, "pending");
  assert.equal(status.pendingReplySeconds, 90);
});

test("resolvePendingReplySeconds keeps growing from backend pending_reply_started_at", () => {
  const nowMs = Date.parse("2026-04-20T10:05:00.000Z");
  assert.equal(resolvePendingReplySeconds({
    reply_state: "pending",
    pending_reply_seconds: 30,
    pending_reply_started_at: "2026-04-20T10:00:00.000Z",
    last_direction: "outgoing",
  }, nowMs), 300);
});

test("resolveWorkbenchConversationStatus reads nested AI runtime state", () => {
  const status = resolveWorkbenchConversationStatus({
    ai_auto_reply: false,
    sop_runtime_state: {
      ai_reply_status: "queued",
      ai_reply_phase: "queued",
    },
  });

  assert.equal(status.modeState, "ai");
  assert.equal(status.isAiQueued, true);
  assert.equal(status.isAiProcessing, true);
});

test("resolveWorkbenchConversationStatus treats manual and sensitive handoff as manual", () => {
  const manual = resolveWorkbenchConversationStatus({
    ai_auto_reply: true,
    ai_mode_override: "manual",
    sop_runtime_state: { ai_reply_status: "queued" },
  });
  const sensitive = resolveWorkbenchConversationStatus({
    ai_auto_reply: true,
    sop_runtime_state: {
      sensitive_handoff_pending: true,
      sensitive_handoff_reason: "风险词",
    },
  });

  assert.equal(manual.modeState, "manual");
  assert.equal(manual.isAiProcessing, false);
  assert.equal(sensitive.modeState, "manual");
  assert.equal(sensitive.sensitiveHandoffPending, true);
  assert.equal(sensitive.sensitiveHandoffReason, "风险词");
});

test("resolveWorkbenchConversationStatus follows explicit auto and account inheritance for mode", () => {
  const explicitAuto = resolveWorkbenchConversationStatus({
    ai_auto_reply: false,
    account_ai_enabled: false,
    ai_mode_override: "auto",
  });
  const inheritedAuto = resolveWorkbenchConversationStatus({
    ai_auto_reply: false,
    account_ai_enabled: true,
    ai_mode_override: "inherit",
  });

  assert.equal(explicitAuto.modeState, "ai");
  assert.equal(inheritedAuto.modeState, "ai");
});

test("resolveWorkbenchConversationBadges returns stable list labels", () => {
  const badges = resolveWorkbenchConversationBadges({
    reply_state: "pending",
    pending_reply_seconds: 65,
    ai_auto_reply: true,
    ai_reply_status: "sending",
  });

  assert.equal(badges.replyLabel, "1分未回复");
  assert.equal(badges.runtimeLabel, "AI发送中");
  assert.equal(badges.modeLabel, "AI处理");
});

test("formatPendingReplyDuration renders long waits", () => {
  assert.equal(formatPendingReplyDuration(12), "12秒未回复");
  assert.equal(formatPendingReplyDuration(3600), "1小时未回复");
  assert.equal(formatPendingReplyDuration(3900), "1小时5分未回复");
  assert.equal(formatPendingReplyDuration(172800), "2天未回复");
});

test("resolveConversationAIToggleState follows explicit override before account inheritance", () => {
  assert.deepEqual(resolveConversationAIToggleState({
    ai_auto_reply: true,
    account_ai_enabled: true,
    ai_mode_override: "manual",
  }), { enabled: false, nextEnabled: true });

  assert.deepEqual(resolveConversationAIToggleState({
    ai_auto_reply: false,
    account_ai_enabled: false,
    ai_mode_override: "auto",
  }), { enabled: true, nextEnabled: false });

  assert.deepEqual(resolveConversationAIToggleState({
    ai_auto_reply: false,
    account_ai_enabled: "true",
    ai_mode_override: "inherit",
  }), { enabled: true, nextEnabled: false });
});

test("buildConversationAIModeMutation mirrors legacy single conversation AI route", () => {
  assert.deepEqual(buildConversationAIModeMutation({ conversation_id: "conv:1" }, true), {
    ok: true,
    method: "POST",
    conversationId: "conv:1",
    path: "/conversations/conv%3A1/ai-auto-reply",
    body: { enabled: true },
  });

  assert.equal(buildConversationAIModeMutation({}, true).error, "conversation_required");
  assert.equal(buildConversationAIModeMutation({ conversation_id: "pending:1" }, true).error, "pending_conversation");
  assert.equal(buildConversationAIModeMutation({ conversation_id: "conv-1" }).error, "enabled_required");
});

test("resolveWorkbenchAIReplyErrorNotice mirrors legacy failed AI runtime panel", () => {
  const notice = resolveWorkbenchAIReplyErrorNotice({
    conversation_id: "conv-1",
    ai_reply_status: "failed",
    ai_reply_error: "compose timeout",
    ai_reply_force_manual: true,
    ai_reply_job_id: "job-1",
  }, { isDismissed: () => false });

  assert.equal(notice.visible, true);
  assert.equal(notice.key, "conv-1|job-1|compose timeout");
  assert.equal(notice.title, "AI已切换为人工处理");

  const processing = resolveWorkbenchAIReplyErrorNotice({
    conversation_id: "conv-1",
    ai_reply_status: "processing",
    ai_reply_error: "old error",
  }, { isDismissed: () => false });
  assert.equal(processing.visible, false);

  const dismissed = resolveWorkbenchAIReplyErrorNotice({
    conversation_id: "conv-1",
    ai_reply_error: "old error",
  }, { dismissedKey: "conv-1|<no-job>|old error", isDismissed: () => false });
  assert.equal(dismissed.visible, false);
});

test("AI reply error dismiss storage keeps recent stable keys", () => {
  const storage = memoryStorage();
  storage.setItem(AI_REPLY_ERROR_DISMISS_STORAGE_KEY, JSON.stringify(["old", "duplicate"]));

  assert.equal(rememberDismissedAIReplyError("duplicate", storage), true);
  assert.equal(rememberDismissedAIReplyError("new", storage), true);
  assert.deepEqual(readDismissedAIReplyErrorKeys(storage), ["new", "duplicate", "old"]);
  assert.equal(isAIReplyErrorDismissed("duplicate", storage), true);
  assert.equal(isAIReplyErrorDismissed("", storage), false);
});

function memoryStorage() {
  const values = new Map();
  return {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      values.set(key, String(value));
    },
  };
}
