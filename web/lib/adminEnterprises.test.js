import assert from "node:assert/strict";
import test from "node:test";

import {
  ENTERPRISES_PATH,
  buildEnterpriseDeleteMutation,
  buildEnterpriseForm,
  buildEnterpriseUpsertMutation,
  buildEnterprisesListRequest,
  defaultEnterpriseForm,
  normalizeAdminEnterprises,
} from "./adminEnterprises.js";

test("normalizeAdminEnterprises keeps enterprise rows and secret flags", () => {
  const enterprises = normalizeAdminEnterprises({
    enterprises: [
      {
        enterprise_id: "ent-1",
        corp_id: "corp-1",
        name: "Corp One",
        incoming_primary_mode: "device_primary",
        archive_pull_url: "https://archive.example/pull",
        media_pull_url: "https://archive.example/media",
        has_archive_pull_token: true,
        corp_secret: "secret",
        enabled: false,
        remark: "primary",
        updated_at: "2026-07-02T01:02:03Z",
      },
      { name: "missing ids" },
    ],
  });

  assert.equal(enterprises.length, 1);
  assert.equal(enterprises[0].enterpriseId, "ent-1");
  assert.equal(enterprises[0].corpId, "corp-1");
  assert.equal(enterprises[0].incomingPrimaryMode, "device_primary");
  assert.equal(enterprises[0].incomingPrimaryModeLabel, "设备优先");
  assert.equal(enterprises[0].hasArchivePullToken, true);
  assert.equal(enterprises[0].hasCorpSecret, true);
  assert.equal(enterprises[0].enabled, false);
  assert.equal(enterprises[0].enabledLabel, "停用");
  assert.equal(enterprises[0].updatedAt, "2026-07-02T01:02:03Z");
});

test("buildEnterprisesListRequest mirrors legacy with_secrets query", () => {
  const request = buildEnterprisesListRequest({ withSecrets: true });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, ENTERPRISES_PATH);
  assert.deepEqual(request.params, { with_secrets: true });
  assert.deepEqual(buildEnterprisesListRequest().params, {});
});

test("buildEnterpriseForm maps API rows to editable form state", () => {
  const form = buildEnterpriseForm({
    enterprise_id: "ent-1",
    corp_id: "corp-1",
    name: "Corp One",
    incoming_primary_mode: "device_primary",
    archive_pull_token: "pull-token",
    media_pull_token: "media-token",
    corp_secret: "corp-secret",
    contact_secret: "contact-secret",
    external_contact_secret: "external-secret",
    private_key_pem: "pem",
    private_key_version: "v1",
    archive_event_callback_token: "callback-token",
    archive_event_callback_aes_key: "callback-aes",
    enabled: true,
    remark: "remark",
  });

  assert.equal(form.enterpriseId, "ent-1");
  assert.equal(form.corpId, "corp-1");
  assert.equal(form.incomingPrimaryMode, "device_primary");
  assert.equal(form.archivePullToken, "pull-token");
  assert.equal(form.privateKeyPEM, "pem");
  assert.equal(form.editing, true);
  assert.equal(defaultEnterpriseForm().incomingPrimaryMode, "archive_primary");
});

test("buildEnterpriseUpsertMutation mirrors legacy enterprise payload", () => {
  const mutation = buildEnterpriseUpsertMutation({
    enterpriseId: " ent-1 ",
    corpId: " corp-1 ",
    name: " Corp One ",
    incomingPrimaryMode: "device_primary",
    archivePullURL: " https://archive.example/pull ",
    archivePullToken: " pull-token ",
    mediaPullURL: " https://archive.example/media ",
    mediaPullToken: " media-token ",
    corpSecret: " corp-secret ",
    contactSecret: " contact-secret ",
    externalContactSecret: " external-secret ",
    privateKeyPEM: " pem ",
    privateKeyVersion: " v1 ",
    archiveEventCallbackToken: " callback-token ",
    archiveEventCallbackAESKey: " callback-aes ",
    enabled: false,
    remark: " primary ",
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ENTERPRISES_PATH);
  assert.deepEqual(mutation.body, {
    enterprise_id: "ent-1",
    corp_id: "corp-1",
    name: "Corp One",
    incoming_primary_mode: "device_primary",
    archive_mode: "self_decrypt",
    archive_source: "self_decrypt",
    archive_pull_url: "https://archive.example/pull",
    archive_pull_token: "pull-token",
    media_pull_url: "https://archive.example/media",
    media_pull_token: "media-token",
    corp_secret: "corp-secret",
    contact_secret: "contact-secret",
    external_contact_secret: "external-secret",
    private_key_pem: "pem",
    private_key_version: "v1",
    archive_event_callback_token: "callback-token",
    archive_event_callback_aes_key: "callback-aes",
    enabled: false,
    remark: "primary",
  });
});

test("enterprise mutations validate required fields and encode delete path", () => {
  assert.equal(buildEnterpriseUpsertMutation({ name: "Corp One" }).error, "corp_id_required");
  assert.equal(buildEnterpriseUpsertMutation({ corpId: "corp-1" }).error, "name_required");
  assert.equal(buildEnterpriseDeleteMutation("").error, "enterprise_id_required");

  const mutation = buildEnterpriseDeleteMutation("ent/1");
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "DELETE");
  assert.equal(mutation.path, `${ENTERPRISES_PATH}/ent%2F1`);
});
