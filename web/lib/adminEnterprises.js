export const ENTERPRISES_PATH = "/admin/enterprises";

function cleanText(value) {
  return String(value || "").trim();
}

export function defaultEnterpriseForm() {
  return {
    enterpriseId: "",
    corpId: "",
    name: "",
    incomingPrimaryMode: "archive_primary",
    enabled: true,
    archivePullURL: "",
    archivePullToken: "",
    mediaPullURL: "",
    mediaPullToken: "",
    corpSecret: "",
    contactSecret: "",
    externalContactSecret: "",
    privateKeyPEM: "",
    privateKeyVersion: "",
    archiveEventCallbackToken: "",
    archiveEventCallbackAESKey: "",
    remark: "",
    editing: false,
  };
}

export function normalizeAdminEnterprises(payload = {}) {
  const enterprises = Array.isArray(payload?.enterprises)
    ? payload.enterprises
    : Array.isArray(payload?.data?.enterprises)
      ? payload.data.enterprises
      : Array.isArray(payload)
        ? payload
        : [];
  return enterprises.map(normalizeAdminEnterprise).filter(Boolean);
}

export function normalizeAdminEnterprise(record = {}) {
  const enterpriseId = cleanText(firstDefined(record?.enterprise_id, record?.enterpriseId));
  const corpId = cleanText(firstDefined(record?.corp_id, record?.corpId));
  if (!enterpriseId && !corpId) return null;
  const incomingPrimaryMode = normalizeIncomingPrimaryMode(firstDefined(record?.incoming_primary_mode, record?.incomingPrimaryMode));
  const archivePullToken = cleanText(firstDefined(record?.archive_pull_token, record?.archivePullToken));
  const mediaPullToken = cleanText(firstDefined(record?.media_pull_token, record?.mediaPullToken));
  const corpSecret = cleanText(firstDefined(record?.corp_secret, record?.corpSecret));
  const contactSecret = cleanText(firstDefined(record?.contact_secret, record?.contactSecret));
  const externalContactSecret = cleanText(firstDefined(record?.external_contact_secret, record?.externalContactSecret));
  const privateKeyPEM = cleanText(firstDefined(record?.private_key_pem, record?.privateKeyPEM));
  const callbackToken = cleanText(firstDefined(record?.archive_event_callback_token, record?.archiveEventCallbackToken));
  const callbackAESKey = cleanText(firstDefined(record?.archive_event_callback_aes_key, record?.archiveEventCallbackAESKey));
  return {
    enterpriseId,
    corpId,
    name: cleanText(record?.name),
    incomingPrimaryMode,
    incomingPrimaryModeLabel: incomingPrimaryMode === "device_primary" ? "设备优先" : "存档优先",
    archiveMode: cleanText(firstDefined(record?.archive_mode, record?.archiveMode)) || "self_decrypt",
    archiveSource: cleanText(firstDefined(record?.archive_source, record?.archiveSource)) || "self_decrypt",
    archivePullURL: cleanText(firstDefined(record?.archive_pull_url, record?.archivePullURL)),
    archivePullToken,
    mediaPullURL: cleanText(firstDefined(record?.media_pull_url, record?.mediaPullURL)),
    mediaPullToken,
    corpSecret,
    contactSecret,
    externalContactSecret,
    privateKeyPEM,
    privateKeyVersion: cleanText(firstDefined(record?.private_key_version, record?.privateKeyVersion)),
    archiveEventCallbackToken: callbackToken,
    archiveEventCallbackAESKey: callbackAESKey,
    hasArchivePullToken: hasSecret(record, "archive_pull_token", archivePullToken),
    hasMediaPullToken: hasSecret(record, "media_pull_token", mediaPullToken),
    hasCorpSecret: hasSecret(record, "corp_secret", corpSecret),
    hasContactSecret: hasSecret(record, "contact_secret", contactSecret),
    hasExternalContactSecret: hasSecret(record, "external_contact_secret", externalContactSecret),
    hasPrivateKeyPEM: hasSecret(record, "private_key_pem", privateKeyPEM),
    hasArchiveEventCallbackToken: hasSecret(record, "archive_event_callback_token", callbackToken),
    hasArchiveEventCallbackAESKey: hasSecret(record, "archive_event_callback_aes_key", callbackAESKey),
    enabled: parseBool(record?.enabled, true),
    enabledLabel: parseBool(record?.enabled, true) ? "启用" : "停用",
    remark: cleanText(record?.remark),
    createdAt: cleanText(firstDefined(record?.created_at, record?.createdAt)),
    updatedAt: cleanText(firstDefined(record?.updated_at, record?.updatedAt)),
    raw: record,
  };
}

export function buildEnterpriseForm(record = {}) {
  const enterprise = normalizeAdminEnterprise(record) || {};
  return {
    ...defaultEnterpriseForm(),
    enterpriseId: enterprise.enterpriseId || "",
    corpId: enterprise.corpId || "",
    name: enterprise.name || "",
    incomingPrimaryMode: enterprise.incomingPrimaryMode || "archive_primary",
    enabled: enterprise.enabled !== false,
    archivePullURL: enterprise.archivePullURL || "",
    archivePullToken: enterprise.archivePullToken || "",
    mediaPullURL: enterprise.mediaPullURL || "",
    mediaPullToken: enterprise.mediaPullToken || "",
    corpSecret: enterprise.corpSecret || "",
    contactSecret: enterprise.contactSecret || "",
    externalContactSecret: enterprise.externalContactSecret || "",
    privateKeyPEM: enterprise.privateKeyPEM || "",
    privateKeyVersion: enterprise.privateKeyVersion || "",
    archiveEventCallbackToken: enterprise.archiveEventCallbackToken || "",
    archiveEventCallbackAESKey: enterprise.archiveEventCallbackAESKey || "",
    remark: enterprise.remark || "",
    editing: Boolean(enterprise.enterpriseId),
  };
}

export function buildEnterprisesListRequest(options = {}) {
  return {
    ok: true,
    method: "GET",
    path: ENTERPRISES_PATH,
    params: options?.withSecrets ? { with_secrets: true } : {},
  };
}

export function buildEnterpriseUpsertMutation(options = {}) {
  const corpId = cleanText(firstDefined(options.corpId, options.corp_id));
  if (!corpId) return { ok: false, error: "corp_id_required" };
  const name = cleanText(options.name);
  if (!name) return { ok: false, error: "name_required" };
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  const body = {
    corp_id: corpId,
    name,
    incoming_primary_mode: normalizeIncomingPrimaryMode(firstDefined(options.incomingPrimaryMode, options.incoming_primary_mode)),
    archive_mode: "self_decrypt",
    archive_source: "self_decrypt",
    archive_pull_url: cleanText(firstDefined(options.archivePullURL, options.archive_pull_url)),
    archive_pull_token: cleanText(firstDefined(options.archivePullToken, options.archive_pull_token)),
    media_pull_url: cleanText(firstDefined(options.mediaPullURL, options.media_pull_url)),
    media_pull_token: cleanText(firstDefined(options.mediaPullToken, options.media_pull_token)),
    corp_secret: cleanText(firstDefined(options.corpSecret, options.corp_secret)),
    contact_secret: cleanText(firstDefined(options.contactSecret, options.contact_secret)),
    external_contact_secret: cleanText(firstDefined(options.externalContactSecret, options.external_contact_secret)),
    private_key_pem: cleanText(firstDefined(options.privateKeyPEM, options.private_key_pem)),
    private_key_version: cleanText(firstDefined(options.privateKeyVersion, options.private_key_version)),
    archive_event_callback_token: cleanText(firstDefined(options.archiveEventCallbackToken, options.archive_event_callback_token)),
    archive_event_callback_aes_key: cleanText(firstDefined(options.archiveEventCallbackAESKey, options.archive_event_callback_aes_key)),
    enabled: parseBool(firstDefined(options.enabled, true), true),
    remark: cleanText(options.remark),
  };
  if (enterpriseId) body.enterprise_id = enterpriseId;
  return {
    ok: true,
    method: "POST",
    path: ENTERPRISES_PATH,
    body,
  };
}

export function buildEnterpriseDeleteMutation(enterpriseId = "") {
  const normalizedEnterpriseId = cleanText(enterpriseId);
  if (!normalizedEnterpriseId) return { ok: false, error: "enterprise_id_required" };
  return {
    ok: true,
    method: "DELETE",
    path: `${ENTERPRISES_PATH}/${encodeURIComponent(normalizedEnterpriseId)}`,
  };
}

function normalizeIncomingPrimaryMode(value) {
  return cleanText(value) === "device_primary" ? "device_primary" : "archive_primary";
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function hasSecret(record, snakeKey, value) {
  const hasKey = `has_${snakeKey}`;
  const camelKey = `has${snakeKey.split("_").map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join("")}`;
  return parseBool(firstDefined(record?.[hasKey], record?.[camelKey]), false) || Boolean(cleanText(value));
}

function parseBool(value, fallback = false) {
  if (value === true || value === 1) return true;
  if (value === false || value === 0) return false;
  const normalized = cleanText(value).toLowerCase();
  if (!normalized) return fallback;
  if (["true", "1", "yes", "on", "是", "启用", "开启"].includes(normalized)) return true;
  if (["false", "0", "no", "off", "否", "停用", "关闭"].includes(normalized)) return false;
  return fallback;
}
