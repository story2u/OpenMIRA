export const ARCHIVE_OFFICIAL_CHECK_PATH = "/archive/official/check";
export const ARCHIVE_INTEGRATION_TEST_PATH = "/archive/integration/test";
export const ARCHIVE_CALLBACK_RECEIPTS_PATH = "/archive/callback/receipts";

export const ARCHIVE_CALLBACK_RECEIPT_PAGE_SIZE_OPTIONS = [20, 50, 100, 200];

export const ARCHIVE_INTEGRATION_SOURCE_OPTIONS = [
  { value: "self_decrypt", label: "本地解密/桥接" },
  { value: "official", label: "官方来源" },
];

const OFFICIAL_CHECK_ENTRIES = [
  { key: "has_corp_id", label: "Corp ID" },
  { key: "has_corp_secret", label: "会话存档 Secret" },
  { key: "has_contact_secret", label: "通讯录 Secret", okText: "已填", failText: "建议补充" },
  { key: "has_archive_pull_url", label: "消息补拉 URL" },
  { key: "has_media_pull_url", label: "媒体补拉 URL" },
  { key: "has_callback_token", label: "回调 Token" },
  { key: "has_callback_aes_key", label: "回调 AESKey" },
  { key: "sdk_available", label: "SDK 可用性", okText: "可用", failText: "不可用" },
  { key: "sdk_media_available", label: "SDK 媒体能力", okText: "可用", failText: "不可用" },
  { key: "token_ok", label: "Token 可用性", okText: "通过", failText: "失败" },
];

const SUGGESTED_URL_LABELS = {
  archive_pull_url: "推荐消息桥接 URL",
  media_pull_url: "推荐媒体桥接 URL",
  event_callback_url: "推荐事件回调 URL",
};

const STATUS_LABELS = {
  accepted: "已接受",
  blocked: "前置未完成",
  completed: "已完成",
  current: "当前步骤",
  dispatched: "已分发",
  failed: "失败",
  passed: "通过",
  pending: "待处理",
  processed: "已处理",
  received: "已接收",
  running: "进行中",
  timeout: "超时",
  warning: "告警",
};

function cleanText(value) {
  return String(value ?? "").trim();
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function intValue(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.trunc(parsed) : fallback;
}

function positiveInt(value, fallback = 1) {
  const parsed = intValue(value, fallback);
  return parsed > 0 ? parsed : fallback;
}

function sourceObject(payload = {}) {
  if (payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data)) return payload.data;
  return payload && typeof payload === "object" ? payload : {};
}

function compactParams(values = {}) {
  const params = {};
  Object.entries(values).forEach(([key, value]) => {
    const normalized = cleanText(value);
    if (!normalized || normalized === "all") return;
    params[key] = normalized;
  });
  return params;
}

function listFromPayload(payload = {}, key = "items") {
  if (Array.isArray(payload)) return payload;
  const source = sourceObject(payload);
  if (Array.isArray(source?.[key])) return source[key];
  if (Array.isArray(source?.data?.[key])) return source.data[key];
  return [];
}

export function defaultArchiveIntegrationForm() {
  return {
    enterpriseId: "",
    source: "self_decrypt",
    pullLimit: "20",
    syncLimit: "100",
    contactLimit: "100",
    mediaLimit: "20",
  };
}

export function defaultArchiveCallbackReceiptFilters() {
  return {
    enterpriseId: "",
    eventName: "",
    page: "1",
    pageSize: "20",
  };
}

export function buildArchiveOfficialCheckMutation(options = {}) {
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  if (!enterpriseId) return { ok: false, error: "enterprise_id_required" };
  return {
    ok: true,
    method: "POST",
    path: ARCHIVE_OFFICIAL_CHECK_PATH,
    body: { enterprise_id: enterpriseId },
  };
}

export function normalizeArchiveOfficialCheckResult(payload = {}, fallback = {}) {
  const source = sourceObject(payload);
  const checks = source?.checks && typeof source.checks === "object" ? source.checks : {};
  const suggested = source?.suggested_bridge_urls && typeof source.suggested_bridge_urls === "object"
    ? source.suggested_bridge_urls
    : {};
  const enterpriseId = cleanText(firstDefined(source?.enterprise_id, source?.enterpriseId, fallback.enterpriseId, fallback.enterprise_id));
  return {
    accepted: source?.accepted === true,
    enterpriseId,
    checks: OFFICIAL_CHECK_ENTRIES.map((entry) => ({
      key: entry.key,
      label: entry.label,
      ok: Boolean(checks?.[entry.key]),
      okText: entry.okText || "已配置",
      failText: entry.failText || "未配置",
    })),
    missing: cleanStringList(source?.missing_required),
    suggested: Object.entries(SUGGESTED_URL_LABELS)
      .map(([key, label]) => ({ key, label, value: cleanText(suggested?.[key]) }))
      .filter((entry) => entry.value),
    callbackWizard: normalizeArchiveCallbackWizard(source?.callback_wizard),
    nextSteps: cleanStringList(source?.next_steps),
    tokenError: cleanText(checks?.token_error),
    sdkError: cleanText(firstDefined(checks?.sdk_error, checks?.sdk_media_error)),
    raw: source,
  };
}

export function buildArchiveIntegrationTestMutation(options = {}) {
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  if (!enterpriseId) return { ok: false, error: "enterprise_id_required" };
  return {
    ok: true,
    method: "POST",
    path: ARCHIVE_INTEGRATION_TEST_PATH,
    body: {
      enterprise_id: enterpriseId,
      source: cleanText(firstDefined(options.source, "self_decrypt")) || "self_decrypt",
      pull_limit: positiveInt(firstDefined(options.pullLimit, options.pull_limit), 20),
      sync_limit: positiveInt(firstDefined(options.syncLimit, options.sync_limit), 100),
      contact_limit: positiveInt(firstDefined(options.contactLimit, options.contact_limit), 100),
      media_limit: positiveInt(firstDefined(options.mediaLimit, options.media_limit), 20),
    },
  };
}

export function normalizeArchiveIntegrationTestResult(payload = {}, fallback = {}) {
  const source = sourceObject(payload);
  return {
    enterpriseId: cleanText(firstDefined(source?.enterprise_id, source?.enterpriseId, fallback.enterpriseId, fallback.enterprise_id)),
    passed: source?.passed === true,
    steps: listFromPayload(source, "steps").map(normalizeArchiveIntegrationStep).filter(Boolean),
    raw: source,
  };
}

export function buildArchiveCallbackReceiptsRequest(filters = {}) {
  const page = positiveInt(firstDefined(filters.page, 1), 1);
  const pageSize = positiveInt(firstDefined(filters.pageSize, filters.page_size, filters.limit, 20), 20);
  return {
    ok: true,
    method: "GET",
    path: ARCHIVE_CALLBACK_RECEIPTS_PATH,
    params: {
      ...compactParams({
        enterprise_id: firstDefined(filters.enterpriseId, filters.enterprise_id),
        event_name: firstDefined(filters.eventName, filters.event_name),
      }),
      page: String(page),
      limit: String(pageSize),
    },
  };
}

export function normalizeArchiveCallbackReceipts(payload = {}) {
  const source = sourceObject(payload);
  return {
    receipts: listFromPayload(source, "receipts").map(normalizeArchiveCallbackReceipt).filter(Boolean),
    pagination: {
      page: positiveInt(source?.page, 1),
      pageSize: positiveInt(firstDefined(source?.page_size, source?.pageSize, source?.limit), 20),
      total: intValue(source?.total, 0),
      totalPages: positiveInt(firstDefined(source?.total_pages, source?.totalPages), 1),
    },
    raw: source,
  };
}

export function archiveOperationStatusLabel(status) {
  const normalized = cleanText(status);
  return STATUS_LABELS[normalized] || normalized || "-";
}

function normalizeArchiveCallbackWizard(value = {}) {
  const source = value && typeof value === "object" ? value : {};
  return {
    ready: source?.ready === true,
    summary: cleanText(source?.summary),
    steps: listFromPayload(source, "steps").map((step = {}) => ({
      id: cleanText(step?.id),
      title: cleanText(step?.title),
      status: cleanText(step?.status) || "pending",
      statusLabel: archiveOperationStatusLabel(step?.status || "pending"),
      description: cleanText(step?.description),
      fieldKeys: cleanStringList(step?.field_keys || step?.fieldKeys),
      valueLabel: cleanText(firstDefined(step?.value_label, step?.valueLabel)),
      value: cleanText(step?.value),
    })),
  };
}

function normalizeArchiveIntegrationStep(record = {}) {
  const name = cleanText(record?.name);
  if (!name) return null;
  const status = cleanText(record?.status) || "pending";
  return {
    name,
    status,
    statusLabel: archiveOperationStatusLabel(status),
    detail: cleanText(record?.detail),
    error: cleanText(record?.error),
  };
}

function normalizeArchiveCallbackReceipt(record = {}) {
  const receiptID = cleanText(firstDefined(record?.receipt_id, record?.receiptID, record?.id));
  const callbackEventKey = cleanText(firstDefined(record?.callback_event_key, record?.callbackEventKey));
  if (!receiptID && !callbackEventKey) return null;
  const status = cleanText(record?.status) || "pending";
  return {
    receiptID,
    enterpriseID: cleanText(firstDefined(record?.enterprise_id, record?.enterpriseID)),
    source: cleanText(record?.source),
    eventName: cleanText(firstDefined(record?.event_name, record?.eventName)),
    callbackEventKey,
    msgSignature: cleanText(firstDefined(record?.msg_signature, record?.msgSignature)),
    timestamp: cleanText(record?.timestamp),
    nonce: cleanText(record?.nonce),
    encryptHash: cleanText(firstDefined(record?.encrypt_hash, record?.encryptHash)),
    plainPayload: cleanText(firstDefined(record?.plain_payload, record?.plainPayload)),
    status,
    statusLabel: archiveOperationStatusLabel(status),
    duplicateCount: intValue(firstDefined(record?.duplicate_count, record?.duplicateCount), 0),
    triggerRequestedAt: cleanText(firstDefined(record?.trigger_requested_at, record?.triggerRequestedAt)),
    processedAt: cleanText(firstDefined(record?.processed_at, record?.processedAt)),
    lastError: cleanText(firstDefined(record?.last_error, record?.lastError)),
    createdAt: cleanText(firstDefined(record?.created_at, record?.createdAt)),
    updatedAt: cleanText(firstDefined(record?.updated_at, record?.updatedAt)),
    raw: record,
  };
}

function cleanStringList(value = []) {
  const source = Array.isArray(value) ? value : cleanText(value).split(/[\n,，;；]/);
  return source.map(cleanText).filter(Boolean);
}
