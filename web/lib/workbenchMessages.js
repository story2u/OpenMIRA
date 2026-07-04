const MEDIA_MESSAGE_TYPES = new Set(["image", "video", "voice", "audio", "file"]);
const RETRYABLE_VOICE_TRANSCRIPTION_STATUSES = new Set(["failed_retryable", "failed_terminal"]);

function cleanText(value) {
  return String(value || "").trim();
}

function normalizeMessageType(message = {}) {
  const msgType = cleanText(message?.msg_type || message?.message_type || "text").toLowerCase();
  if (msgType === "audio") return "voice";
  return msgType || "text";
}

function firstText(...values) {
  for (const value of values) {
    const text = cleanText(value);
    if (text) return text;
  }
  return "";
}

export function resolveWorkbenchMessagePresentation(message = {}) {
  if (String(message?.revoke_status || "").trim().toLowerCase() === "success") {
    return {
      kind: "text",
      text: "已撤回",
      mediaUrl: "",
      fileName: "",
      extension: "",
      voiceDurationText: "",
      voiceText: "",
      statusText: "",
    };
  }
  const msgType = normalizeMessageType(message);
  const isMedia = MEDIA_MESSAGE_TYPES.has(msgType);
  const mediaUrl = resolveMediaURL(message, msgType);
  const content = cleanText(message?.content);
  const fileName = resolveFileName(message, mediaUrl, content);
  const voiceText = firstText(message?.voice_text, message?.transcript_text, message?.voice_transcription_text);
  const statusText = resolveMediaStatusText(message);

  if (!isMedia) {
    return {
      kind: "text",
      text: content,
      mediaUrl: "",
      fileName: "",
      extension: "",
      voiceDurationText: "",
      voiceText: "",
      statusText,
    };
  }

  const kind = msgType === "audio" ? "voice" : msgType;
  return {
    kind,
    text: resolveMediaFallbackText(kind, content, mediaUrl, fileName, statusText),
    mediaUrl,
    fileName,
    extension: fileExtension(fileName),
    voiceDurationText: kind === "voice" ? formatVoiceDuration(message?.voice_duration_sec) : "",
    voiceText,
    statusText,
  };
}

export function buildWorkbenchMediaPreview(presentation = {}) {
  const kind = cleanText(presentation?.kind).toLowerCase();
  const mediaUrl = cleanText(presentation?.mediaUrl);
  if (!mediaUrl || (kind !== "image" && kind !== "video")) return null;
  return {
    type: kind,
    url: mediaUrl,
    title: cleanText(presentation?.fileName) || (kind === "image" ? "图片预览" : "视频预览"),
  };
}

export function resolveWorkbenchVoiceMediaKind(mediaUrl = "", fallbackType = "") {
  const value = cleanText(mediaUrl).toLowerCase();
  const fallback = cleanText(fallbackType).toLowerCase();
  if (value.includes(".amr") || value.includes("audio/amr")) return "amr";
  if (value.includes(".silk") || value.includes("audio/x-silk")) return "silk";
  if (fallback === "voice" || fallback === "audio") return "audio";
  return fallback || "audio";
}

export function buildWorkbenchArchiveMediaPrepareAction(message = {}, presentation = null) {
  const resolvedPresentation = presentation || resolveWorkbenchMessagePresentation(message);
  const kind = cleanText(resolvedPresentation?.kind).toLowerCase();
  const mediaUrl = cleanText(resolvedPresentation?.mediaUrl);
  const taskID = firstText(message?.media_task_id, message?.archive_media_task_id, message?.mediaTaskId);
  if (!taskID || mediaUrl || !MEDIA_MESSAGE_TYPES.has(kind) || isTruthy(message?.media_ready)) {
    return null;
  }
  return {
    taskId: taskID,
    label: resolvePrepareMediaLabel(kind),
    buttonLabel: "加载媒体",
    loadingLabel: "加载中...",
  };
}

export function resolveWorkbenchVoiceTranscriptionDisplay(message = {}, overrides = {}) {
  const transcript = firstText(
    overrides.voiceText,
    overrides.transcript,
    message?.voice_text,
    message?.transcript_text,
    message?.voice_transcription_text,
  );
  const status = cleanText(overrides.status || message?.voice_transcription_status).toLowerCase();
  const rawError = cleanText(overrides.error || message?.voice_transcription_error).replace(/\s+/g, " ");
  const errorText = rawError.length > 56 ? `${rawError.slice(0, 56)}...` : rawError;
  if (transcript) {
    return { kind: "transcript", tone: "default", text: transcript };
  }
  if (status === "pending") {
    return { kind: "status", tone: "muted", text: "转写排队中" };
  }
  if (status === "running") {
    return { kind: "status", tone: "muted", text: "转写中" };
  }
  if (status === "failed_retryable") {
    return { kind: "status", tone: "warning", text: errorText || "转写稍后自动重试" };
  }
  if (status === "failed_terminal") {
    return { kind: "status", tone: "error", text: errorText || "转写失败" };
  }
  if (status === "success") {
    return { kind: "status", tone: "muted", text: "当前语音暂无转写文本" };
  }
  return null;
}

export function buildWorkbenchVoiceTranscriptionRetryAction(message = {}, options = {}) {
  const status = cleanText(options.status || message?.voice_transcription_status).toLowerCase();
  const transcript = firstText(
    options.voiceText,
    options.transcript,
    message?.voice_text,
    message?.transcript_text,
    message?.voice_transcription_text,
  );
  const localStatus = cleanText(options.localStatus).toLowerCase();
  const archiveMsgID = cleanText(message?.archive_msgid);
  const canRetry = RETRYABLE_VOICE_TRANSCRIPTION_STATUSES.has(status)
    || (status === "success" && !transcript)
    || (!status && !transcript && archiveMsgID);
  if (!canRetry || localStatus) return null;
  return {
    path: "/archive/voice-transcriptions/retry",
    method: "POST",
    body: {
      enterprise_id: firstText(message?.enterprise_id, message?.tenant_id),
      archive_msgid: archiveMsgID,
    },
    label: "重新转写语音",
    buttonLabel: "转写",
    loadingLabel: "转写中",
    missingTaskMessage: "缺少语音转写任务信息",
  };
}

export function normalizeWorkbenchVoiceTranscriptionRetryResult(result = {}) {
  return {
    status: firstText(result?.voice_transcription_status, result?.status, "running"),
    voiceText: cleanText(result?.voice_text),
    error: cleanText(result?.voice_transcription_error),
    executeId: cleanText(result?.voice_transcription_execute_id),
  };
}

export function formatWorkbenchVoiceTranscriptionRetryError(error) {
  const message = cleanText(error?.message || error || "重新转写失败");
  const normalized = message.toLowerCase();
  if (normalized.includes("not configured") || normalized.includes("unavailable")) {
    return "语音转写配置未完成，请检查 Coze 配置和私钥";
  }
  if (normalized.includes("task not found")) {
    return "当前语音暂无转写任务，请等待媒体处理完成后重试";
  }
  if (normalized.includes("already succeeded")) {
    return "当前语音已转写完成，请刷新查看";
  }
  return message || "重新转写失败";
}

function resolveMediaURL(message = {}, msgType = "") {
  const mediaUrl = cleanText(message?.media_url || message?.mediaUrl || message?.url);
  if (mediaUrl) return mediaUrl;
  const content = cleanText(message?.content);
  if (MEDIA_MESSAGE_TYPES.has(msgType) && looksLikeMediaURL(content)) return content;
  return "";
}

function resolveFileName(message = {}, mediaUrl = "", content = "") {
  const explicit = firstText(message?.file_name, message?.media_filename, message?.filename, message?.name);
  if (explicit) return explicit;
  if (content && !looksLikeMediaURL(content)) return content;
  return fileNameFromURL(mediaUrl) || "file";
}

function resolveMediaFallbackText(kind, content, mediaUrl, fileName, statusText) {
  const namedFile = fileName && fileName !== "file" ? fileName : "";
  if (kind === "image") return mediaUrl ? "" : (statusText || namedFile || "[图片消息]");
  if (kind === "video") return mediaUrl ? "" : (statusText || namedFile || "[视频消息]");
  if (kind === "voice") return statusText || "[语音消息]";
  if (kind === "file") return fileName || content || statusText || "[文件消息]";
  return content || statusText || "";
}

function resolveMediaStatusText(message = {}) {
  return firstText(message?.media_status, message?.voice_transcription_status);
}

function resolvePrepareMediaLabel(kind) {
  if (kind === "image") return "图片暂不可预览";
  if (kind === "video") return "视频暂不可预览";
  if (kind === "voice") return "语音暂不可播放";
  if (kind === "file") return "文件暂不可下载";
  return "媒体暂不可用";
}

function looksLikeMediaURL(value) {
  const text = cleanText(value);
  if (!text) return false;
  return /^(https?:\/\/|\/api\/|blob:|data:|local:\/\/)/i.test(text);
}

function fileNameFromURL(value) {
  const text = cleanText(value);
  if (!text || !looksLikeMediaURL(text)) return "";
  if (/^local:\/\//i.test(text)) {
    return decodePathPart(text.split("/").pop() || "");
  }
  try {
    const url = new URL(text, "http://localhost");
    return decodePathPart(url.pathname.split("/").pop() || "");
  } catch {
    return decodePathPart(text.split(/[/?#]/).filter(Boolean).pop() || "");
  }
}

function decodePathPart(value) {
  const text = cleanText(value);
  if (!text) return "";
  try {
    return decodeURIComponent(text);
  } catch {
    return text;
  }
}

function fileExtension(value) {
  const fileName = cleanText(value);
  const index = fileName.lastIndexOf(".");
  if (index <= 0 || index === fileName.length - 1) return "";
  return fileName.slice(index + 1).toUpperCase();
}

function formatVoiceDuration(value) {
  const seconds = Math.max(0, Math.round(Number(value || 0)));
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return `${seconds}"`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return `${minutes}:${String(rest).padStart(2, "0")}`;
}

function isTruthy(value) {
  if (typeof value === "boolean") return value;
  const text = cleanText(value).toLowerCase();
  return text === "1" || text === "true" || text === "yes" || text === "on";
}
