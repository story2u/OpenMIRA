export const ASSIGNMENTS_PATH = "/assignments";
export const ASSIGNMENT_CLAIM_PATH = "/assignments/claim";
export const ASSIGNMENT_RELEASE_PATH = "/assignments/release";
export const ASSIGNMENT_PURGE_PATH = "/assignments/purge-all";
export const ASSIGNMENT_AUTO_PATH = "/assignments/auto-assign";
export const CONVERSATION_TRANSFER_PATH_PREFIX = "/conversations";

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAssignments(payload = {}) {
  const rows = Array.isArray(payload?.assignments)
    ? payload.assignments
    : Array.isArray(payload?.data?.assignments)
      ? payload.data.assignments
      : Array.isArray(payload)
        ? payload
        : [];
  return rows.map(normalizeAssignment).filter(Boolean);
}

export function normalizeAssignment(record = {}) {
  const conversationId = cleanText(record?.conversation_id || record?.conversationId);
  if (!conversationId) return null;
  return {
    tenantId: cleanText(record?.tenant_id || record?.tenantId),
    conversationId,
    assigneeId: cleanText(record?.assignee_id || record?.assigneeId),
    assigneeName: cleanText(record?.assignee_name || record?.assigneeName),
    assignedAt: cleanText(record?.assigned_at || record?.assignedAt),
    updatedAt: cleanText(record?.updated_at || record?.updatedAt),
    raw: record,
  };
}

export function buildAssignmentsListRequest(assigneeId = "", options = {}) {
  const normalizedAssigneeId = cleanText(assigneeId);
  if (!normalizedAssigneeId) return { ok: false, error: "assignee_id_required" };
  return {
    ok: true,
    method: "GET",
    path: ASSIGNMENTS_PATH,
    params: {
      assignee_id: normalizedAssigneeId,
      limit: normalizeLimit(options?.limit),
    },
  };
}

export function buildAssignmentClaimMutation(options = {}) {
  const conversationId = cleanText(options.conversationId || options.conversation_id);
  if (!conversationId) return { ok: false, error: "conversation_id_required" };
  const assigneeId = cleanText(options.assigneeId || options.assignee_id);
  if (!assigneeId) return { ok: false, error: "assignee_id_required" };
  return {
    ok: true,
    method: "POST",
    path: ASSIGNMENT_CLAIM_PATH,
    body: {
      conversation_id: conversationId,
      assignee_id: assigneeId,
      assignee_name: cleanText(options.assigneeName || options.assignee_name),
      force: Boolean(options.force),
    },
  };
}

export function buildAssignmentReleaseMutation(options = {}) {
  const conversationId = cleanText(options.conversationId || options.conversation_id);
  if (!conversationId) return { ok: false, error: "conversation_id_required" };
  return {
    ok: true,
    method: "POST",
    path: ASSIGNMENT_RELEASE_PATH,
    body: {
      conversation_id: conversationId,
      assignee_id: cleanText(options.assigneeId || options.assignee_id),
      force: Boolean(options.force),
    },
  };
}

export function buildAssignmentTransferMutation(options = {}) {
  const conversationId = cleanText(options.conversationId || options.conversation_id);
  if (!conversationId) return { ok: false, error: "conversation_id_required" };
  const targetAssigneeId = cleanText(options.targetAssigneeId || options.target_assignee_id);
  if (!targetAssigneeId) return { ok: false, error: "target_assignee_id_required" };
  return {
    ok: true,
    method: "POST",
    path: `${CONVERSATION_TRANSFER_PATH_PREFIX}/${encodeURIComponent(conversationId)}/transfer`,
    body: {
      target_assignee_id: targetAssigneeId,
      target_assignee_name: cleanText(options.targetAssigneeName || options.target_assignee_name),
      from_assignee_id: cleanText(options.fromAssigneeId || options.from_assignee_id),
      force: Boolean(options.force),
    },
  };
}

export function buildAssignmentPurgeMutation() {
  return {
    ok: true,
    method: "POST",
    path: ASSIGNMENT_PURGE_PATH,
  };
}

export function buildAssignmentAutoMutation(options = {}) {
  return {
    ok: true,
    method: "POST",
    path: ASSIGNMENT_AUTO_PATH,
    body: {
      limit: normalizeLimit(options?.limit),
    },
  };
}

export function normalizeAssignmentTransferResult(payload = {}) {
  const transfer = payload?.transfer || {};
  return {
    success: payload?.success !== false,
    assignment: normalizeAssignment(payload?.assignment),
    transfer: {
      conversationId: cleanText(transfer?.conversation_id || transfer?.conversationId),
      fromAssigneeId: cleanText(transfer?.from_assignee_id || transfer?.fromAssigneeId),
      fromAssigneeName: cleanText(transfer?.from_assignee_name || transfer?.fromAssigneeName),
      toAssigneeId: cleanText(transfer?.to_assignee_id || transfer?.toAssigneeId),
      toAssigneeName: cleanText(transfer?.to_assignee_name || transfer?.toAssigneeName),
      assignedAt: cleanText(transfer?.assigned_at || transfer?.assignedAt),
      updatedAt: cleanText(transfer?.updated_at || transfer?.updatedAt),
    },
  };
}

export function normalizeAssignmentAutoResult(payload = {}) {
  const assignedCount = normalizeNonNegativeInt(payload?.assigned_count || payload?.assignedCount);
  const skippedCount = normalizeNonNegativeInt(payload?.skipped_count || payload?.skippedCount);
  return {
    success: payload?.success !== false,
    assignedCount,
    skippedCount,
    assignments: normalizeAssignments({ assignments: payload?.assignments }),
    skipped: Array.isArray(payload?.skipped) ? payload.skipped : [],
  };
}

function normalizeLimit(value) {
  const number = Number(value || 200);
  if (!Number.isFinite(number) || number <= 0) return 200;
  return Math.min(1000, Math.floor(number));
}

function normalizeNonNegativeInt(value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return Math.floor(number);
}
