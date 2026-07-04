import assert from "node:assert/strict";
import test from "node:test";

import {
  ARCHIVE_CALLBACK_RECEIPTS_PATH,
  ARCHIVE_INTEGRATION_TEST_PATH,
  ARCHIVE_OFFICIAL_CHECK_PATH,
  archiveOperationStatusLabel,
  buildArchiveCallbackReceiptsRequest,
  buildArchiveIntegrationTestMutation,
  buildArchiveOfficialCheckMutation,
  defaultArchiveCallbackReceiptFilters,
  defaultArchiveIntegrationForm,
  normalizeArchiveCallbackReceipts,
  normalizeArchiveIntegrationTestResult,
  normalizeArchiveOfficialCheckResult,
} from "./adminArchiveOperations.js";

test("buildArchiveOfficialCheckMutation validates and serializes enterprise id", () => {
  assert.equal(buildArchiveOfficialCheckMutation({}).error, "enterprise_id_required");

  const mutation = buildArchiveOfficialCheckMutation({ enterpriseId: " ent-1 " });
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ARCHIVE_OFFICIAL_CHECK_PATH);
  assert.deepEqual(mutation.body, { enterprise_id: "ent-1" });
});

test("normalizeArchiveOfficialCheckResult projects checks, wizard and suggestions", () => {
  const result = normalizeArchiveOfficialCheckResult({
    accepted: true,
    enterprise_id: "ent-1",
    checks: {
      has_corp_id: true,
      has_corp_secret: true,
      has_contact_secret: false,
      sdk_available: false,
      sdk_error: "sdk library not found",
      token_ok: false,
      token_error: "invalid secret",
    },
    missing_required: ["archive_pull_url"],
    suggested_bridge_urls: {
      archive_pull_url: "https://example.com/api/v1/archive/sdk/pull",
      event_callback_url: "https://example.com/api/v1/archive/callback/corp-1",
    },
    callback_wizard: {
      ready: false,
      summary: "保持稳定",
      steps: [
        {
          id: "fill_callback_credentials",
          title: "填写回调密钥",
          status: "current",
          field_keys: ["archive_event_callback_token"],
        },
      ],
    },
    next_steps: ["补齐配置"],
  });

  assert.equal(result.accepted, true);
  assert.equal(result.enterpriseId, "ent-1");
  assert.equal(result.checks.find((entry) => entry.key === "has_corp_id").ok, true);
  assert.equal(result.checks.find((entry) => entry.key === "has_contact_secret").failText, "建议补充");
  assert.deepEqual(result.missing, ["archive_pull_url"]);
  assert.equal(result.suggested.length, 2);
  assert.equal(result.callbackWizard.steps[0].statusLabel, "当前步骤");
  assert.equal(result.tokenError, "invalid secret");
  assert.equal(result.sdkError, "sdk library not found");
  assert.deepEqual(result.nextSteps, ["补齐配置"]);
});

test("buildArchiveIntegrationTestMutation mirrors legacy diagnostic defaults", () => {
  assert.equal(buildArchiveIntegrationTestMutation({}).error, "enterprise_id_required");
  assert.equal(defaultArchiveIntegrationForm().source, "self_decrypt");

  const mutation = buildArchiveIntegrationTestMutation({
    enterpriseId: " ent-2 ",
    pullLimit: "25",
    syncLimit: "125",
    contactLimit: "75",
    mediaLimit: "10",
  });
  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ARCHIVE_INTEGRATION_TEST_PATH);
  assert.deepEqual(mutation.body, {
    enterprise_id: "ent-2",
    source: "self_decrypt",
    pull_limit: 25,
    sync_limit: 125,
    contact_limit: 75,
    media_limit: 10,
  });
});

test("normalizeArchiveIntegrationTestResult keeps step status labels", () => {
  const result = normalizeArchiveIntegrationTestResult({
    passed: false,
    enterprise_id: "ent-1",
    steps: [
      { name: "配置检查", status: "passed", detail: "配置完整" },
      { name: "媒体补拉检查", status: "warning", detail: "无新媒体", error: "" },
    ],
  });

  assert.equal(result.enterpriseId, "ent-1");
  assert.equal(result.passed, false);
  assert.equal(result.steps[0].statusLabel, "通过");
  assert.equal(result.steps[1].statusLabel, "告警");
});

test("buildArchiveCallbackReceiptsRequest filters and paginates receipts", () => {
  const request = buildArchiveCallbackReceiptsRequest({
    enterpriseId: " ent-1 ",
    eventName: " change_external_contact ",
    page: "2",
    pageSize: "50",
  });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, ARCHIVE_CALLBACK_RECEIPTS_PATH);
  assert.deepEqual(request.params, {
    enterprise_id: "ent-1",
    event_name: "change_external_contact",
    page: "2",
    limit: "50",
  });
  assert.equal(defaultArchiveCallbackReceiptFilters().pageSize, "20");
});

test("normalizeArchiveCallbackReceipts maps rows and pagination", () => {
  const result = normalizeArchiveCallbackReceipts({
    receipts: [
      {
        receipt_id: "acr-1",
        enterprise_id: "ent-1",
        event_name: "change_external_contact",
        callback_event_key: "cb-1",
        status: "failed",
        duplicate_count: 2,
        last_error: "mismatch",
        updated_at: "2026-07-02T10:00:00+08:00",
      },
    ],
    page: 2,
    page_size: 50,
    total: 51,
    total_pages: 2,
  });

  assert.equal(result.receipts.length, 1);
  assert.equal(result.receipts[0].receiptID, "acr-1");
  assert.equal(result.receipts[0].statusLabel, "失败");
  assert.equal(result.receipts[0].duplicateCount, 2);
  assert.deepEqual(result.pagination, { page: 2, pageSize: 50, total: 51, totalPages: 2 });
  assert.equal(archiveOperationStatusLabel("processed"), "已处理");
});
