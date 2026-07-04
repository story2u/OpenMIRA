import assert from "node:assert/strict";
import test from "node:test";

import {
  SOP_MEDIA_LOCAL_PATH,
  SOP_MEDIA_MAX_BYTES,
  SOP_MEDIA_UPLOAD_PATH,
  buildSOPLocalMediaPath,
  buildSOPMediaUploadMutation,
  inferSOPMediaType,
  normalizeSOPMediaUploadResult,
  sopMediaTypeLabel,
} from "./adminSOPMedia.js";

class FakeFormData {
  constructor() {
    this.entries = [];
  }

  append(key, value) {
    this.entries.push([key, value]);
  }
}

test("buildSOPMediaUploadMutation mirrors legacy multipart upload", () => {
  const file = { name: "welcome.png", size: 128, type: "image/png" };
  const mutation = buildSOPMediaUploadMutation({ file, FormDataCtor: FakeFormData });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, SOP_MEDIA_UPLOAD_PATH);
  assert.deepEqual(mutation.body.entries, [
    ["media_type", "image"],
    ["file", file],
  ]);
});

test("buildSOPMediaUploadMutation validates browser-side prerequisites", () => {
  assert.equal(buildSOPMediaUploadMutation({ mediaType: "image", FormDataCtor: FakeFormData }).error, "file_required");
  assert.equal(buildSOPMediaUploadMutation({ file: { name: "asset.bin" }, FormDataCtor: FakeFormData }).error, "media_type_required");
  assert.equal(buildSOPMediaUploadMutation({ mediaType: "image", file: { name: "clip.mp4", size: 10, type: "video/mp4" }, FormDataCtor: FakeFormData }).error, "unsupported_mime");
  assert.equal(buildSOPMediaUploadMutation({ mediaType: "video", file: { name: "large.mp4", size: SOP_MEDIA_MAX_BYTES + 1, type: "video/mp4" }, FormDataCtor: FakeFormData }).error, "file_too_large");
  assert.equal(buildSOPMediaUploadMutation({ mediaType: "video", file: { name: "clip.mp4", size: 10, type: "video/mp4" }, FormDataCtor: null }).error, "formdata_unavailable");
});

test("normalizeSOPMediaUploadResult keeps preview fields", () => {
  const result = normalizeSOPMediaUploadResult({
    success: true,
    media_type: "video",
    object_url: "local://sop/media/welcome image.mp4",
    access_url: "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fmedia%2Fwelcome%20image.mp4",
    filename: "welcome image.mp4",
    content_type: "video/mp4",
  });

  assert.equal(result.success, true);
  assert.equal(result.mediaType, "video");
  assert.equal(result.isLocalObject, true);
  assert.equal(result.objectUrl, "local://sop/media/welcome image.mp4");
  assert.equal(result.accessUrl, "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fmedia%2Fwelcome%20image.mp4");
  assert.equal(result.filename, "welcome image.mp4");
});

test("SOP media helpers infer types and local preview path", () => {
  assert.equal(inferSOPMediaType({ name: "welcome.JPG" }), "image");
  assert.equal(inferSOPMediaType({ type: "video/webm" }), "video");
  assert.equal(inferSOPMediaType({ name: "asset.txt" }), "");
  assert.equal(sopMediaTypeLabel("image"), "图片");
  assert.equal(buildSOPLocalMediaPath("local://sop/media/welcome image.png"), `${SOP_MEDIA_LOCAL_PATH}?object_url=local%3A%2F%2Fsop%2Fmedia%2Fwelcome%20image.png`);
  assert.equal(buildSOPLocalMediaPath("https://cdn.example/welcome.png"), "");
});
