export const CONTACT_SYNC_EXTERNAL_PATH = "/contacts/sync/external-contacts";
export const CONTACT_SYNC_FULL_PATH = "/contacts/sync/full";
export const CONTACT_SYNC_REFRESH_STALE_PATH = "/contacts/sync/refresh-stale";

function cleanText(value) {
  return String(value ?? "").trim();
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function positiveInt(value, fallback = 50) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.floor(parsed);
}

function intValue(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.floor(parsed);
}

function sourceObject(payload = {}) {
  if (payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data)) return payload.data;
  return payload && typeof payload === "object" ? payload : {};
}

export function defaultContactSyncForm() {
  return {
    enterpriseId: "",
    externalUserID: "",
    refreshLimit: "50",
  };
}

export function buildContactSyncExternalMutation(options = {}) {
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  if (!enterpriseId) return { ok: false, error: "enterprise_id_required" };
  const externalUserID = cleanText(firstDefined(options.externalUserID, options.external_userid));
  if (!externalUserID) return { ok: false, error: "external_userid_required" };
  return {
    ok: true,
    method: "POST",
    path: CONTACT_SYNC_EXTERNAL_PATH,
    params: {
      enterprise_id: enterpriseId,
      external_userid: externalUserID,
    },
  };
}

export function buildContactSyncFullMutation(options = {}) {
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  if (!enterpriseId) return { ok: false, error: "enterprise_id_required" };
  return {
    ok: true,
    method: "POST",
    path: CONTACT_SYNC_FULL_PATH,
    params: {
      enterprise_id: enterpriseId,
    },
  };
}

export function buildContactSyncRefreshStaleMutation(options = {}) {
  const enterpriseId = cleanText(firstDefined(options.enterpriseId, options.enterprise_id));
  if (!enterpriseId) return { ok: false, error: "enterprise_id_required" };
  return {
    ok: true,
    method: "POST",
    path: CONTACT_SYNC_REFRESH_STALE_PATH,
    params: {
      enterprise_id: enterpriseId,
      limit: String(positiveInt(firstDefined(options.refreshLimit, options.limit), 50)),
    },
  };
}

export function normalizeContactSyncResult(payload = {}, fallback = {}) {
  const source = sourceObject(payload);
  return {
    enterpriseId: cleanText(firstDefined(source?.enterprise_id, source?.enterpriseId, fallback.enterpriseId, fallback.enterprise_id)),
    externalUserID: cleanText(firstDefined(source?.external_userid, source?.externalUserID, fallback.externalUserID, fallback.external_userid)),
    corpUsersSynced: intValue(firstDefined(source?.corp_users_synced, source?.corpUsersSynced)),
    externalContactsSynced: intValue(firstDefined(source?.external_contacts_synced, source?.externalContactsSynced)),
    externalContactsSkipped: intValue(firstDefined(source?.external_contacts_skipped, source?.externalContactsSkipped)),
    externalContactsRefreshed: intValue(firstDefined(source?.external_contacts_refreshed, source?.externalContactsRefreshed)),
    corpUsersRefreshed: intValue(firstDefined(source?.corp_users_refreshed, source?.corpUsersRefreshed)),
    raw: source,
  };
}
