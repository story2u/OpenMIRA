export const VOICE_RECORDING_MAX_MS = 60_000;

export const SUPPORTED_VOICE_MIME_TYPES = [
  "audio/webm;codecs=opus",
  "audio/webm",
  "audio/ogg;codecs=opus",
  "audio/ogg",
  "audio/mp4",
];

function cleanText(value) {
  return String(value || "").trim();
}

export function selectVoiceRecorderMimeType(mediaRecorder = globalThis.MediaRecorder) {
  const isTypeSupported = mediaRecorder?.isTypeSupported;
  if (typeof isTypeSupported !== "function") return "";
  return SUPPORTED_VOICE_MIME_TYPES.find((mimeType) => {
    try {
      return Boolean(isTypeSupported.call(mediaRecorder, mimeType));
    } catch {
      return false;
    }
  }) || "";
}

export function voiceFileExtension(mimeType = "") {
  const normalized = cleanText(mimeType).toLowerCase();
  if (normalized.includes("ogg")) return "ogg";
  if (normalized.includes("mp4") || normalized.includes("m4a")) return "m4a";
  return "webm";
}

export function formatVoiceRecordingSeconds(seconds = 0) {
  const total = Math.max(0, Math.floor(Number(seconds || 0)));
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  return `${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`;
}

export function createVoiceRecordingFile(chunks = [], options = {}) {
  const FileCtor = options.FileCtor || globalThis.File;
  if (typeof FileCtor !== "function") {
    throw new Error("File constructor is unavailable");
  }
  const mimeType = cleanText(options.mimeType) || "audio/webm";
  const nowMs = Number.isFinite(Number(options.nowMs)) ? Number(options.nowMs) : Date.now();
  const durationSec = Math.max(1, Math.round(Number(options.durationSec || 0)));
  const file = new FileCtor(Array.isArray(chunks) ? chunks : [], `voice-${nowMs}.${voiceFileExtension(mimeType)}`, {
    type: mimeType,
  });
  file.voiceDurationSec = durationSec;
  return file;
}

export function shouldUploadVoiceRecording({ shouldUpload = true, chunks = [] } = {}) {
  return Boolean(shouldUpload && Array.isArray(chunks) && chunks.length > 0);
}

export function voiceRecordingErrorMessage(error, fallback = "录音失败，请检查麦克风后重试") {
  const name = cleanText(error?.name);
  const message = cleanText(error?.message);
  if (name === "NotAllowedError" || name === "PermissionDeniedError" || name === "SecurityError") {
    return "未获得麦克风权限，请在浏览器地址栏允许麦克风后重试";
  }
  if (name === "NotFoundError" || name === "DevicesNotFoundError") {
    return "未检测到可用麦克风，请连接麦克风后重试";
  }
  if (name === "NotReadableError" || name === "TrackStartError") {
    return "麦克风正在被其他应用占用，请关闭通话或录音后重试";
  }
  if (name === "OverconstrainedError" || name === "ConstraintNotSatisfiedError") {
    return "当前麦克风不支持录音参数，请更换设备后重试";
  }
  if (name === "AbortError") {
    return "麦克风启动中断，请重试";
  }
  return message || fallback;
}
