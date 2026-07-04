export const SOP_MEDIA_UPLOAD_PATH = "/admin/sop/media/upload";
export const SOP_MEDIA_LOCAL_PATH = "/admin/sop/media/local";
export const SOP_MEDIA_FILE_ACCEPT = "image/*,video/*";
export const SOP_MEDIA_MAX_BYTES = 50 * 1024 * 1024;

const SOP_MEDIA_TYPES = new Set(["image", "video"]);

function cleanText(value) {
  return String(value || "").trim();
}

export function buildSOPMediaUploadMutation(options = {}) {
  const mediaType = normalizeSOPMediaType(options.mediaType || options.media_type || inferSOPMediaType(options.file));
  if (!mediaType) return { ok: false, error: "media_type_required" };
  const file = options.file || null;
  if (!file) return { ok: false, error: "file_required" };
  const size = Number(file?.size);
  if (Number.isFinite(size) && size > SOP_MEDIA_MAX_BYTES) return { ok: false, error: "file_too_large" };
  const mimeType = cleanText(file?.type).toLowerCase();
  if (mimeType && !mimeType.startsWith(`${mediaType}/`)) return { ok: false, error: "unsupported_mime" };
  const FormDataCtor = Object.prototype.hasOwnProperty.call(options, "FormDataCtor")
    ? options.FormDataCtor
    : globalThis.FormData;
  if (typeof FormDataCtor !== "function") return { ok: false, error: "formdata_unavailable" };

  const body = new FormDataCtor();
  body.append("media_type", mediaType);
  body.append("file", file);
  return {
    ok: true,
    method: "POST",
    path: SOP_MEDIA_UPLOAD_PATH,
    body,
  };
}

export function normalizeSOPMediaUploadResult(payload = {}) {
  const source = payload?.data && typeof payload.data === "object" ? payload.data : payload;
  const objectUrl = cleanText(source?.object_url || source?.objectUrl);
  const accessUrl = cleanText(source?.access_url || source?.accessUrl);
  return {
    success: source?.success !== false,
    mediaType: normalizeSOPMediaType(source?.media_type || source?.mediaType) || inferSOPMediaType({ type: source?.content_type }),
    objectUrl,
    accessUrl,
    filename: cleanText(source?.filename || source?.file_name || source?.name),
    contentType: cleanText(source?.content_type || source?.contentType),
    isLocalObject: objectUrl.toLowerCase().startsWith("local://"),
  };
}

export function buildSOPLocalMediaPath(objectUrl = "") {
  const normalized = cleanText(objectUrl);
  if (!normalized.toLowerCase().startsWith("local://")) return "";
  return `${SOP_MEDIA_LOCAL_PATH}?object_url=${encodeURIComponent(normalized)}`;
}

export function inferSOPMediaType(file = {}) {
  const mimeType = cleanText(file?.type).toLowerCase();
  if (mimeType.startsWith("image/")) return "image";
  if (mimeType.startsWith("video/")) return "video";
  const name = cleanText(file?.name).toLowerCase();
  if (/\.(png|jpe?g|gif|webp|bmp|heic|heif)$/i.test(name)) return "image";
  if (/\.(mp4|mov|m4v|webm|avi|mkv)$/i.test(name)) return "video";
  return "";
}

export function sopMediaTypeLabel(mediaType = "") {
  const normalized = normalizeSOPMediaType(mediaType);
  if (normalized === "image") return "图片";
  if (normalized === "video") return "视频";
  return "媒体";
}

function normalizeSOPMediaType(value = "") {
  const normalized = cleanText(value).toLowerCase();
  return SOP_MEDIA_TYPES.has(normalized) ? normalized : "";
}
