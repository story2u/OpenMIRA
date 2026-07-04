import assert from "node:assert/strict";
import test from "node:test";

import {
  buildWorkbenchArchiveMediaPrepareAction,
  buildWorkbenchMediaPreview,
  buildWorkbenchVoiceTranscriptionRetryAction,
  formatWorkbenchVoiceTranscriptionRetryError,
  normalizeWorkbenchVoiceTranscriptionRetryResult,
  resolveWorkbenchMessagePresentation,
  resolveWorkbenchVoiceMediaKind,
  resolveWorkbenchVoiceTranscriptionDisplay,
} from "./workbenchMessages.js";

test("resolveWorkbenchMessagePresentation keeps text messages plain", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "text",
    content: " hello ",
  });

  assert.deepEqual(result, {
    kind: "text",
    text: "hello",
    mediaUrl: "",
    fileName: "",
    extension: "",
    voiceDurationText: "",
    voiceText: "",
    statusText: "",
  });
});

test("resolveWorkbenchMessagePresentation hides revoked content", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "image",
    content: "https://cdn.example.com/original.png",
    media_url: "https://cdn.example.com/original.png",
    revoke_status: "success",
  });

  assert.deepEqual(result, {
    kind: "text",
    text: "已撤回",
    mediaUrl: "",
    fileName: "",
    extension: "",
    voiceDurationText: "",
    voiceText: "",
    statusText: "",
  });
});

test("resolveWorkbenchMessagePresentation renders image media from content URL", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "image",
    content: "/api/v1/archive/media/files/task-1?token=signed",
    media_ready: true,
  });

  assert.equal(result.kind, "image");
  assert.equal(result.mediaUrl, "/api/v1/archive/media/files/task-1?token=signed");
  assert.equal(result.text, "");
  assert.equal(result.fileName, "task-1");
});

test("resolveWorkbenchMessagePresentation keeps local pending media filename", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "image",
    content: "photo.png",
    media_filename: "photo.png",
    send_status: "pending",
  });

  assert.equal(result.kind, "image");
  assert.equal(result.mediaUrl, "");
  assert.equal(result.fileName, "photo.png");
  assert.equal(result.text, "photo.png");
});

test("resolveWorkbenchMessagePresentation derives file name and extension", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "file",
    media_url: "https://cdn.example.com/files/%E6%8A%A5%E4%BB%B7.pdf?x=1",
  });

  assert.equal(result.kind, "file");
  assert.equal(result.mediaUrl, "https://cdn.example.com/files/%E6%8A%A5%E4%BB%B7.pdf?x=1");
  assert.equal(result.fileName, "报价.pdf");
  assert.equal(result.extension, "PDF");
  assert.equal(result.text, "报价.pdf");
});

test("resolveWorkbenchMessagePresentation carries voice duration and transcript", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "voice",
    media_url: "/api/v1/archive/media/files/voice-1",
    voice_duration_sec: 75,
    voice_text: "转写文本",
  });

  assert.equal(result.kind, "voice");
  assert.equal(result.mediaUrl, "/api/v1/archive/media/files/voice-1");
  assert.equal(result.voiceDurationText, "1:15");
  assert.equal(result.voiceText, "转写文本");
});

test("resolveWorkbenchMessagePresentation keeps pending media status", () => {
  const result = resolveWorkbenchMessagePresentation({
    msg_type: "video",
    media_status: "pending",
  });

  assert.equal(result.kind, "video");
  assert.equal(result.mediaUrl, "");
  assert.equal(result.text, "pending");
  assert.equal(result.statusText, "pending");
});

test("buildWorkbenchMediaPreview opens only image and video media", () => {
  assert.deepEqual(buildWorkbenchMediaPreview({
    kind: "image",
    mediaUrl: "https://cdn.example.com/a.png",
    fileName: "a.png",
  }), {
    type: "image",
    url: "https://cdn.example.com/a.png",
    title: "a.png",
  });
  assert.deepEqual(buildWorkbenchMediaPreview({
    kind: "video",
    mediaUrl: "https://cdn.example.com/a.mp4",
  }), {
    type: "video",
    url: "https://cdn.example.com/a.mp4",
    title: "视频预览",
  });
  assert.equal(buildWorkbenchMediaPreview({ kind: "file", mediaUrl: "https://cdn.example.com/a.pdf" }), null);
  assert.equal(buildWorkbenchMediaPreview({ kind: "image" }), null);
});

test("resolveWorkbenchVoiceMediaKind detects AMR and SILK urls", () => {
  assert.equal(resolveWorkbenchVoiceMediaKind("https://cdn.example.com/a.amr?token=1", "voice"), "amr");
  assert.equal(resolveWorkbenchVoiceMediaKind("https://cdn.example.com/a.silk", "voice"), "silk");
  assert.equal(resolveWorkbenchVoiceMediaKind("https://cdn.example.com/a.mp3", "voice"), "audio");
});

test("buildWorkbenchArchiveMediaPrepareAction exposes pending media task action", () => {
  assert.deepEqual(buildWorkbenchArchiveMediaPrepareAction({
    msg_type: "image",
    media_task_id: " task-1 ",
    media_status: "failed",
    media_ready: false,
  }), {
    taskId: "task-1",
    label: "图片暂不可预览",
    buttonLabel: "加载媒体",
    loadingLabel: "加载中...",
  });

  assert.deepEqual(buildWorkbenchArchiveMediaPrepareAction({
    msg_type: "voice",
    archive_media_task_id: "voice-task",
    media_ready: "false",
  }), {
    taskId: "voice-task",
    label: "语音暂不可播放",
    buttonLabel: "加载媒体",
    loadingLabel: "加载中...",
  });
});

test("buildWorkbenchArchiveMediaPrepareAction hides ready or loaded media", () => {
  assert.equal(buildWorkbenchArchiveMediaPrepareAction({
    msg_type: "image",
    media_task_id: "task-1",
    media_url: "/api/v1/archive/media/files/task-1?token=signed",
    media_ready: false,
  }), null);

  assert.equal(buildWorkbenchArchiveMediaPrepareAction({
    msg_type: "file",
    media_task_id: "task-2",
    media_ready: true,
  }), null);

  assert.equal(buildWorkbenchArchiveMediaPrepareAction({
    msg_type: "text",
    media_task_id: "task-3",
    media_ready: false,
  }), null);
});

test("resolveWorkbenchVoiceTranscriptionDisplay maps transcript statuses", () => {
  assert.deepEqual(resolveWorkbenchVoiceTranscriptionDisplay({
    voice_text: "你好",
    voice_transcription_status: "failed_terminal",
  }), {
    kind: "transcript",
    tone: "default",
    text: "你好",
  });

  assert.deepEqual(resolveWorkbenchVoiceTranscriptionDisplay({
    voice_transcription_status: "pending",
  }), {
    kind: "status",
    tone: "muted",
    text: "转写排队中",
  });

  assert.deepEqual(resolveWorkbenchVoiceTranscriptionDisplay({
    voice_transcription_status: "failed_retryable",
    voice_transcription_error: "coze temporarily unavailable",
  }), {
    kind: "status",
    tone: "warning",
    text: "coze temporarily unavailable",
  });
});

test("buildWorkbenchVoiceTranscriptionRetryAction mirrors legacy retry payload", () => {
  assert.deepEqual(buildWorkbenchVoiceTranscriptionRetryAction({
    tenant_id: "ent-1",
    archive_msgid: "am-1",
    voice_transcription_status: "failed_terminal",
  }), {
    path: "/archive/voice-transcriptions/retry",
    method: "POST",
    body: {
      enterprise_id: "ent-1",
      archive_msgid: "am-1",
    },
    label: "重新转写语音",
    buttonLabel: "转写",
    loadingLabel: "转写中",
    missingTaskMessage: "缺少语音转写任务信息",
  });

  assert.equal(buildWorkbenchVoiceTranscriptionRetryAction({
    archive_msgid: "am-2",
    voice_transcription_status: "success",
    voice_text: "已完成",
  }), null);

  assert.equal(buildWorkbenchVoiceTranscriptionRetryAction({
    archive_msgid: "am-3",
    voice_transcription_status: "failed_retryable",
  }, { localStatus: "running" }), null);
});

test("voice transcription retry result and errors normalize legacy payloads", () => {
  assert.deepEqual(normalizeWorkbenchVoiceTranscriptionRetryResult({
    status: "failed_retryable",
    voice_transcription_error: "coze down",
    voice_transcription_execute_id: "exec-1",
  }), {
    status: "failed_retryable",
    voiceText: "",
    error: "coze down",
    executeId: "exec-1",
  });

  assert.equal(
    formatWorkbenchVoiceTranscriptionRetryError(new Error("voice transcription is not configured")),
    "语音转写配置未完成，请检查 Coze 配置和私钥",
  );
  assert.equal(
    formatWorkbenchVoiceTranscriptionRetryError(new Error("voice transcription task not found")),
    "当前语音暂无转写任务，请等待媒体处理完成后重试",
  );
  assert.equal(
    formatWorkbenchVoiceTranscriptionRetryError(new Error("voice transcription already succeeded")),
    "当前语音已转写完成，请刷新查看",
  );
});
