import assert from "node:assert/strict";
import test from "node:test";

import {
  CONTACT_SYNC_EXTERNAL_PATH,
  CONTACT_SYNC_FULL_PATH,
  CONTACT_SYNC_REFRESH_STALE_PATH,
  buildContactSyncExternalMutation,
  buildContactSyncFullMutation,
  buildContactSyncRefreshStaleMutation,
  defaultContactSyncForm,
  normalizeContactSyncResult,
} from "./adminContactSync.js";

test("contact sync mutations mirror legacy query routes", () => {
  const external = buildContactSyncExternalMutation({
    enterpriseId: " ent-1 ",
    externalUserID: " wm-1 ",
  });
  const full = buildContactSyncFullMutation({ enterprise_id: " ent-1 " });
  const stale = buildContactSyncRefreshStaleMutation({
    enterpriseId: " ent-1 ",
    refreshLimit: "25",
  });

  assert.equal(external.ok, true);
  assert.equal(external.method, "POST");
  assert.equal(external.path, CONTACT_SYNC_EXTERNAL_PATH);
  assert.deepEqual(external.params, { enterprise_id: "ent-1", external_userid: "wm-1" });
  assert.equal(full.path, CONTACT_SYNC_FULL_PATH);
  assert.deepEqual(full.params, { enterprise_id: "ent-1" });
  assert.equal(stale.path, CONTACT_SYNC_REFRESH_STALE_PATH);
  assert.deepEqual(stale.params, { enterprise_id: "ent-1", limit: "25" });
});

test("contact sync mutations report missing fields and normalize limit", () => {
  assert.equal(buildContactSyncExternalMutation({ externalUserID: "wm-1" }).error, "enterprise_id_required");
  assert.equal(buildContactSyncExternalMutation({ enterpriseId: "ent-1" }).error, "external_userid_required");
  assert.equal(buildContactSyncFullMutation({}).error, "enterprise_id_required");
  assert.equal(buildContactSyncRefreshStaleMutation({}).error, "enterprise_id_required");

  const fallback = buildContactSyncRefreshStaleMutation({ enterpriseId: "ent-1", refreshLimit: "0" });
  assert.deepEqual(fallback.params, { enterprise_id: "ent-1", limit: "50" });
  assert.equal(defaultContactSyncForm().refreshLimit, "50");
});

test("normalizeContactSyncResult keeps single, full and stale counters", () => {
  const result = normalizeContactSyncResult({
    enterprise_id: "ent-1",
    external_userid: "wm-1",
    corp_users_synced: 2,
    external_contacts_synced: 3,
    external_contacts_skipped: 1,
    external_contacts_refreshed: 4,
    corp_users_refreshed: 5,
  });

  assert.equal(result.enterpriseId, "ent-1");
  assert.equal(result.externalUserID, "wm-1");
  assert.equal(result.corpUsersSynced, 2);
  assert.equal(result.externalContactsSynced, 3);
  assert.equal(result.externalContactsSkipped, 1);
  assert.equal(result.externalContactsRefreshed, 4);
  assert.equal(result.corpUsersRefreshed, 5);
});
