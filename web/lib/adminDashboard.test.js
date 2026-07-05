import assert from "node:assert/strict";
import test from "node:test";
import {
  adminGroups,
  findAdminGroup,
  formatAdminValue,
  normalizeAdminPayload,
  summarizeSection,
} from "./adminDashboard.js";

test("normalizeAdminPayload keeps preferred account columns and metrics", () => {
  const section = adminGroups[0].sections[0];
  const snapshot = normalizeAdminPayload(section, {
    total: 2,
    accounts: [
      {
        account_name: "消息端A",
        account_id: "acc-1",
        device_id: "dev-1",
        assignee_name: "夏南",
        status: "normal",
        ai_enabled: false,
        ignored: "x",
      },
      { account_name: "消息端B", account_id: "acc-2", device_id: "dev-2", ai_enabled: true },
    ],
  });

  assert.equal(snapshot.rowCount, 2);
  assert.equal(snapshot.rawCount, 2);
  assert.deepEqual(snapshot.columns, ["account_name", "account_id", "device_id", "assignee_name", "status", "ai_enabled"]);
  assert.equal(snapshot.rows[0].ai_enabled, "否");
  assert.equal(summarizeSection(snapshot), "2");
});

test("normalizeAdminPayload expands object config into key value rows", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "ai_config");
  const snapshot = normalizeAdminPayload(section, {
    config: {
      provider: "coze",
      enabled: true,
      limits: { daily: 100 },
    },
  });

  assert.equal(snapshot.rowCount, 3);
  assert.deepEqual(snapshot.columns, ["key", "value"]);
  assert.deepEqual(snapshot.rows[0], { key: "provider", value: "coze" });
  assert.equal(snapshot.rows[1].value, "是");
  assert.equal(snapshot.rows[2].value, "{\"daily\":100}");
});

test("normalizeAdminPayload keeps assignment rules and pools as key value rows", () => {
  const section = adminGroups[0].sections.find((item) => item.key === "assignment_config");
  const snapshot = normalizeAdminPayload(section, {
    rules: [{ rule_id: "rule-001" }],
    pools: [{ pool_id: "pool-001" }],
  });

  assert.equal(snapshot.rowCount, 2);
  assert.deepEqual(snapshot.columns, ["key", "value"]);
  assert.deepEqual(snapshot.records[0], { key: "rules", value: [{ rule_id: "rule-001" }] });
  assert.deepEqual(snapshot.records[1], { key: "pools", value: [{ pool_id: "pool-001" }] });
});

test("assignments section is loaded by the specialized panel", () => {
  const section = adminGroups[1].sections.find((item) => item.key === "assignments");

  assert.equal(section.skipFetch, true);
});

test("normalizeAdminPayload adds enabled metric for sensitive words", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "sensitive_words");
  const snapshot = normalizeAdminPayload(section, {
    words: [
      { word: "alpha", enabled: true },
      { word: "beta", enabled: false },
      { word: "gamma" },
    ],
  });

  assert.equal(snapshot.metrics[0].key, "enabled");
  assert.equal(snapshot.metrics[0].value, "2");
  assert.equal(snapshot.rows[1].enabled, "否");
});

test("normalizeAdminPayload keeps knowledge document records for specialized panels", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "knowledge_docs");
  const snapshot = normalizeAdminPayload(section, {
    documents: [
      { doc_id: "doc-1", filename: "FAQ.md", status: "indexed", size: "12KB", updated_at: "2026-07-02T01:02:03Z" },
    ],
  });

  assert.equal(snapshot.rowCount, 1);
  assert.deepEqual(snapshot.columns, ["filename", "status", "size", "updated_at", "doc_id"]);
  assert.equal(snapshot.records[0].doc_id, "doc-1");
  assert.equal(snapshot.rows[0].filename, "FAQ.md");
});

test("normalizeAdminPayload keeps enterprise records for specialized panel", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "enterprises");
  const snapshot = normalizeAdminPayload(section, {
    enterprises: [
      {
        enterprise_id: "ent-1",
        corp_id: "corp-1",
        name: "Corp One",
        incoming_primary_mode: "archive_primary",
        enabled: true,
        updated_at: "2026-07-02T01:02:03Z",
      },
    ],
  });

  assert.equal(section.params.with_secrets, true);
  assert.equal(snapshot.rowCount, 1);
  assert.deepEqual(snapshot.columns, ["name", "corp_id", "enterprise_id", "incoming_primary_mode", "enabled", "updated_at"]);
  assert.equal(snapshot.records[0].enterprise_id, "ent-1");
  assert.equal(snapshot.rows[0].enabled, "是");
});

test("contact sync section is loaded by the specialized panel", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "contact_sync");

  assert.equal(section.skipFetch, true);
  assert.equal(section.path, "/contacts/sync/full");
});

test("normalizeAdminPayload keeps SOP flow records for specialized panel", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "sop_config");
  const snapshot = normalizeAdminPayload(section, {
    flows: [
      {
        flow_id: "default",
        flow_name: "Default Flow",
        execution_mode: "local_days",
        target_audience: "__ALL__",
        enabled: true,
        updated_at: "2026-07-02T01:02:03Z",
      },
    ],
  });

  assert.equal(snapshot.rowCount, 1);
  assert.deepEqual(snapshot.columns, ["flow_name", "flow_id", "execution_mode", "target_audience", "enabled", "updated_at"]);
  assert.equal(snapshot.records[0].flow_id, "default");
  assert.equal(snapshot.rows[0].enabled, "是");
});

test("SOP operations section is loaded by the specialized panel", () => {
  const section = adminGroups[2].sections.find((item) => item.key === "sop_operations");

  assert.equal(section.skipFetch, true);
  assert.equal(section.path, "/admin/sop/dispatch-tasks");
});

test("AI reply observability section is loaded by the specialized panel", () => {
  const section = adminGroups[3].sections.find((item) => item.key === "ai_replies");

  assert.equal(section.skipFetch, true);
  assert.equal(section.path, "/admin/ai-config/reply-logs");
});

test("runtime observability section is loaded by the specialized panel", () => {
  const section = adminGroups[3].sections.find((item) => item.key === "observability_dashboard");

  assert.equal(section.skipFetch, true);
  assert.equal(section.path, "/admin/observability/dashboard");
});

test("audit and system log sections are loaded by specialized panels", () => {
  const audit = adminGroups[3].sections.find((item) => item.key === "audit_logs");
  const system = adminGroups[3].sections.find((item) => item.key === "system_logs");

  assert.equal(audit.skipFetch, true);
  assert.equal(audit.path, "/admin/audit-logs");
  assert.equal(system.skipFetch, true);
  assert.equal(system.path, "/admin/system-logs");
});

test("findAdminGroup falls back to operations group", () => {
  assert.equal(findAdminGroup("missing").key, "operations");
  assert.equal(findAdminGroup("observability").key, "observability");
});

test("formatAdminValue handles empty and nested values", () => {
  assert.equal(formatAdminValue(""), "-");
  assert.equal(formatAdminValue(null), "-");
  assert.equal(formatAdminValue(["a", "b"]), "a, b");
  assert.equal(formatAdminValue({ ok: true }), "{\"ok\":true}");
});
