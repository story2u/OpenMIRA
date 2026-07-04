import assert from "node:assert/strict";
import test from "node:test";

import {
  ASSIGNMENTS_PATH,
  ASSIGNMENT_AUTO_PATH,
  ASSIGNMENT_CLAIM_PATH,
  ASSIGNMENT_PURGE_PATH,
  ASSIGNMENT_RELEASE_PATH,
  CONVERSATION_TRANSFER_PATH_PREFIX,
  buildAssignmentAutoMutation,
  buildAssignmentClaimMutation,
  buildAssignmentPurgeMutation,
  buildAssignmentReleaseMutation,
  buildAssignmentTransferMutation,
  buildAssignmentsListRequest,
  normalizeAssignmentAutoResult,
  normalizeAssignmentTransferResult,
  normalizeAssignments,
} from "./adminAssignments.js";

test("normalizeAssignments keeps legacy assignment rows", () => {
  const assignments = normalizeAssignments({
    assignments: [
      {
        tenant_id: "tenant-a",
        conversation_id: "conv-001",
        assignee_id: "cs-001",
        assignee_name: "客服一",
        assigned_at: "2026-07-02T01:02:03Z",
        updated_at: "2026-07-02T02:03:04Z",
      },
      { assignee_id: "missing conversation" },
    ],
  });

  assert.equal(assignments.length, 1);
  assert.equal(assignments[0].conversationId, "conv-001");
  assert.equal(assignments[0].assigneeName, "客服一");
  assert.equal(assignments[0].tenantId, "tenant-a");
});

test("buildAssignmentsListRequest mirrors legacy assignee query", () => {
  const request = buildAssignmentsListRequest(" cs-001 ", { limit: 1500 });

  assert.equal(request.ok, true);
  assert.equal(request.method, "GET");
  assert.equal(request.path, ASSIGNMENTS_PATH);
  assert.deepEqual(request.params, { assignee_id: "cs-001", limit: 1000 });
  assert.equal(buildAssignmentsListRequest("").error, "assignee_id_required");
});

test("buildAssignmentClaimMutation mirrors legacy claim payload", () => {
  const mutation = buildAssignmentClaimMutation({
    conversationId: " conv-001 ",
    assigneeId: " cs-001 ",
    assigneeName: " 客服一 ",
    force: true,
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ASSIGNMENT_CLAIM_PATH);
  assert.deepEqual(mutation.body, {
    conversation_id: "conv-001",
    assignee_id: "cs-001",
    assignee_name: "客服一",
    force: true,
  });
});

test("buildAssignmentReleaseMutation allows admin release without assignee id", () => {
  const mutation = buildAssignmentReleaseMutation({ conversationId: "conv-001" });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, ASSIGNMENT_RELEASE_PATH);
  assert.deepEqual(mutation.body, {
    conversation_id: "conv-001",
    assignee_id: "",
    force: false,
  });
});

test("buildAssignmentTransferMutation mirrors legacy conversation transfer payload", () => {
  const mutation = buildAssignmentTransferMutation({
    conversationId: " conv/001 ",
    targetAssigneeId: " cs-002 ",
    targetAssigneeName: " 客服二 ",
    fromAssigneeId: " cs-001 ",
    force: true,
  });

  assert.equal(mutation.ok, true);
  assert.equal(mutation.method, "POST");
  assert.equal(mutation.path, `${CONVERSATION_TRANSFER_PATH_PREFIX}/conv%2F001/transfer`);
  assert.deepEqual(mutation.body, {
    target_assignee_id: "cs-002",
    target_assignee_name: "客服二",
    from_assignee_id: "cs-001",
    force: true,
  });
});

test("assignment mutations report missing required fields", () => {
  assert.equal(buildAssignmentClaimMutation({ assigneeId: "cs-001" }).error, "conversation_id_required");
  assert.equal(buildAssignmentClaimMutation({ conversationId: "conv-001" }).error, "assignee_id_required");
  assert.equal(buildAssignmentReleaseMutation({}).error, "conversation_id_required");
  assert.equal(buildAssignmentTransferMutation({ targetAssigneeId: "cs-002" }).error, "conversation_id_required");
  assert.equal(buildAssignmentTransferMutation({ conversationId: "conv-001" }).error, "target_assignee_id_required");
});

test("buildAssignmentPurgeMutation and auto mutation mirror legacy routes", () => {
  const purge = buildAssignmentPurgeMutation();
  const auto = buildAssignmentAutoMutation({ limit: 0 });

  assert.equal(purge.method, "POST");
  assert.equal(purge.path, ASSIGNMENT_PURGE_PATH);
  assert.equal(auto.method, "POST");
  assert.equal(auto.path, ASSIGNMENT_AUTO_PATH);
  assert.deepEqual(auto.body, { limit: 200 });
});

test("normalizeAssignmentAutoResult keeps legacy counters and rows", () => {
  const result = normalizeAssignmentAutoResult({
    success: true,
    assigned_count: 2,
    skipped_count: 1,
    assignments: [{ conversation_id: "conv-001", assignee_id: "cs-001" }],
    skipped: [{ conversation_id: "conv-002", reason: "no assignee capacity" }],
  });

  assert.equal(result.success, true);
  assert.equal(result.assignedCount, 2);
  assert.equal(result.skippedCount, 1);
  assert.equal(result.assignments[0].conversationId, "conv-001");
  assert.equal(result.skipped.length, 1);
});

test("normalizeAssignmentTransferResult keeps legacy transfer fields", () => {
  const result = normalizeAssignmentTransferResult({
    success: true,
    assignment: {
      conversation_id: "conv-001",
      assignee_id: "cs-002",
      assignee_name: "客服二",
    },
    transfer: {
      conversation_id: "conv-001",
      from_assignee_id: "cs-001",
      from_assignee_name: "客服一",
      to_assignee_id: "cs-002",
      to_assignee_name: "客服二",
      assigned_at: "2026-07-02T03:04:05Z",
    },
  });

  assert.equal(result.success, true);
  assert.equal(result.assignment.assigneeId, "cs-002");
  assert.equal(result.transfer.conversationId, "conv-001");
  assert.equal(result.transfer.fromAssigneeName, "客服一");
  assert.equal(result.transfer.toAssigneeId, "cs-002");
  assert.equal(result.transfer.assignedAt, "2026-07-02T03:04:05Z");
});
