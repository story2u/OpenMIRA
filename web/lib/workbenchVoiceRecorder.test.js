import assert from "node:assert/strict";
import { File } from "node:buffer";
import test from "node:test";

import {
  createVoiceRecordingFile,
  formatVoiceRecordingSeconds,
  selectVoiceRecorderMimeType,
  shouldUploadVoiceRecording,
  voiceRecordingErrorMessage,
  voiceFileExtension,
} from "./workbenchVoiceRecorder.js";

test("selectVoiceRecorderMimeType picks the first browser-supported type", () => {
  const recorder = {
    isTypeSupported: (mimeType) => mimeType === "audio/ogg;codecs=opus",
  };

  assert.equal(selectVoiceRecorderMimeType(recorder), "audio/ogg;codecs=opus");
  assert.equal(selectVoiceRecorderMimeType({ isTypeSupported: () => false }), "");
  assert.equal(selectVoiceRecorderMimeType({}), "");
});

test("voiceFileExtension follows recorder MIME type", () => {
  assert.equal(voiceFileExtension("audio/webm;codecs=opus"), "webm");
  assert.equal(voiceFileExtension("audio/ogg"), "ogg");
  assert.equal(voiceFileExtension("audio/mp4"), "m4a");
});

test("formatVoiceRecordingSeconds renders stable timer text", () => {
  assert.equal(formatVoiceRecordingSeconds(0), "00:00");
  assert.equal(formatVoiceRecordingSeconds(7.9), "00:07");
  assert.equal(formatVoiceRecordingSeconds(75), "01:15");
});

test("createVoiceRecordingFile attaches duration for voice upload payload", async () => {
  const file = createVoiceRecordingFile(["abc"], {
    FileCtor: File,
    mimeType: "audio/ogg;codecs=opus",
    nowMs: 1782980000000,
    durationSec: 2.4,
  });

  assert.equal(file.name, "voice-1782980000000.ogg");
  assert.equal(file.type, "audio/ogg;codecs=opus");
  assert.equal(file.voiceDurationSec, 2);
  assert.equal(await file.text(), "abc");
});

test("shouldUploadVoiceRecording separates cancel from upload", () => {
  assert.equal(shouldUploadVoiceRecording({ shouldUpload: true, chunks: ["blob"] }), true);
  assert.equal(shouldUploadVoiceRecording({ shouldUpload: false, chunks: ["blob"] }), false);
  assert.equal(shouldUploadVoiceRecording({ shouldUpload: true, chunks: [] }), false);
});

test("voiceRecordingErrorMessage gives actionable media permission guidance", () => {
  assert.equal(
    voiceRecordingErrorMessage({ name: "NotAllowedError" }),
    "未获得麦克风权限，请在浏览器地址栏允许麦克风后重试",
  );
  assert.equal(
    voiceRecordingErrorMessage({ name: "NotFoundError" }),
    "未检测到可用麦克风，请连接麦克风后重试",
  );
  assert.equal(
    voiceRecordingErrorMessage({ name: "NotReadableError" }),
    "麦克风正在被其他应用占用，请关闭通话或录音后重试",
  );
  assert.equal(
    voiceRecordingErrorMessage({ name: "OverconstrainedError" }),
    "当前麦克风不支持录音参数，请更换设备后重试",
  );
  assert.equal(voiceRecordingErrorMessage({ message: "custom" }), "custom");
  assert.equal(voiceRecordingErrorMessage(null, "fallback"), "fallback");
});
