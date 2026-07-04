import assert from "node:assert/strict";
import test from "node:test";

import {
  ASSIGNMENT_CONFIG_PATH,
  buildAssignmentConfigMutation,
  formatAssignmentConfigJSON,
  normalizeAssignmentConfig,
  normalizeAssignmentConfigRecords,
} from "./adminAssignmentConfig.js";

test("normalizeAssignmentConfig keeps top-level legacy rules and pools", () => {
  const config = normalizeAssignmentConfig({
    rules: [
      {
        rule_id: "rule-001",
        name: "VIP",
        target_type: "pool",
        target_value: "pool-001",
      },
    ],
    pools: [
      {
        pool_id: "pool-001",
        pool_name: "默认池",
        strategy_type: "round_robin",
        members: [{ assignee_id: "cs-001", weight: 1 }],
      },
    ],
  });

  assert.equal(config.rules.length, 1);
  assert.equal(config.pools.length, 1);
  assert.equal(config.rules[0].rule_id, "rule-001");
  assert.equal(config.pools[0].pool_id, "pool-001");
  assert.match(config.rulesJSON, /"rule_id": "rule-001"/);
  assert.match(config.poolsJSON, /"pool_id": "pool-001"/);
});

test("normalizeAssignmentConfigRecords rebuilds config from dashboard rows", () => {
  const config = normalizeAssignmentConfigRecords([
    { key: "rules", value: [{ rule_id: "rule-001" }] },
    { key: "pools", value: "[{\"pool_id\":\"pool-001\"}]" },
    { key: "ignored", value: [{ ok: true }] },
  ]);

  assert.deepEqual(config.rules, [{ rule_id: "rule-001" }]);
  assert.deepEqual(config.pools, [{ pool_id: "pool-001" }]);
});

test("buildAssignmentConfigMutation mirrors legacy full replace body", () => {
  const mutation = buildAssignmentConfigMutation({
    rulesJSON: JSON.stringify([
      {
        rule_id: " rule-001 ",
        name: "VIP",
        field_name: "sender_name",
        target_type: "pool",
        target_value: "pool-001",
      },
    ]),
    pools: [
      {
        pool_id: "pool-001",
        pool_name: "默认池",
        strategy_type: "round_robin",
        members: [{ assignee_id: "cs-001", weight: 1 }],
      },
    ],
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ASSIGNMENT_CONFIG_PATH);
  assert.deepEqual(mutation.body, {
    rules: [
      {
        rule_id: " rule-001 ",
        name: "VIP",
        field_name: "sender_name",
        target_type: "pool",
        target_value: "pool-001",
      },
    ],
    pools: [
      {
        pool_id: "pool-001",
        pool_name: "默认池",
        strategy_type: "round_robin",
        members: [{ assignee_id: "cs-001", weight: 1 }],
      },
    ],
  });
});

test("buildAssignmentConfigMutation rejects invalid JSON or non-array rows", () => {
  assert.equal(buildAssignmentConfigMutation({ rulesJSON: "{", poolsJSON: "[]" }).error, "rules_invalid");
  assert.equal(buildAssignmentConfigMutation({ rulesJSON: "{}", poolsJSON: "[]" }).error, "rules_invalid");
  assert.equal(buildAssignmentConfigMutation({ rulesJSON: "[]", poolsJSON: "{}" }).error, "pools_invalid");
});

test("formatAssignmentConfigJSON renders stable arrays", () => {
  assert.equal(formatAssignmentConfigJSON([{ rule_id: "rule-001" }]), "[\n  {\n    \"rule_id\": \"rule-001\"\n  }\n]");
  assert.equal(formatAssignmentConfigJSON({}), "[]");
});
