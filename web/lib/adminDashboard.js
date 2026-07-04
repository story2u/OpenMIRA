export const adminGroups = [
  {
    key: "operations",
    label: "运营",
    sections: [
      {
        key: "accounts",
        label: "企微账号",
        path: "/accounts",
        params: { limit: 100 },
        columns: ["account_name", "account_id", "device_id", "assignee_name", "status", "ai_enabled"],
      },
      {
        key: "devices",
        label: "设备",
        path: "/devices",
        columns: ["device_id", "agent_id", "online", "wework_logged_in", "model", "version"],
      },
      {
        key: "workloads",
        label: "客服负载",
        path: "/assignments/workloads",
        params: { limit: 100 },
        columns: ["assignee_name", "assignee_id", "current_sessions", "max_sessions", "available_capacity"],
      },
      {
        key: "assignment_config",
        label: "分配规则",
        path: "/admin/assignment-config",
        columns: ["key", "value"],
      },
    ],
  },
  {
    key: "people",
    label: "人员",
    sections: [
      {
        key: "cs_users",
        label: "客服账号",
        path: "/cs-users",
        params: { limit: 100 },
        columns: ["name", "assignee_id", "role", "enabled", "is_online", "current_sessions"],
      },
      {
        key: "assignments",
        label: "当前分配",
        path: "/assignments",
        skipFetch: true,
        params: { limit: 100 },
        columns: ["conversation_id", "assignee_name", "assignee_id", "status", "updated_at"],
      },
      {
        key: "agents",
        label: "账号统计",
        path: "/admin/stats/agents",
        params: { limit: 100 },
        columns: ["agent_id", "account_name", "conversation_count", "pending_reply_count", "ai_reply_count"],
      },
    ],
  },
  {
    key: "content",
    label: "内容",
    sections: [
      {
        key: "scripts",
        label: "快捷话术",
        path: "/admin/scripts",
        params: { limit: 100 },
        columns: ["title", "content", "category", "enabled", "target_audience", "updated_at"],
      },
      {
        key: "sensitive_words",
        label: "敏感词",
        path: "/admin/sensitive-words",
        params: { limit: 100 },
        columns: ["word", "category", "enabled", "updated_at"],
      },
      {
        key: "ai_config",
        label: "AI 配置",
        path: "/admin/ai-config",
        columns: ["key", "value"],
      },
      {
        key: "enterprises",
        label: "企业绑定",
        path: "/admin/enterprises",
        params: { with_secrets: true },
        columns: ["name", "corp_id", "enterprise_id", "incoming_primary_mode", "enabled", "updated_at"],
      },
      {
        key: "contact_sync",
        label: "通讯录同步",
        path: "/contacts/sync/full",
        skipFetch: true,
        columns: ["enterprise_id", "corp_users_synced", "external_contacts_synced", "external_contacts_skipped"],
      },
      {
        key: "sop_config",
        label: "SOP 配置",
        path: "/admin/sop/flows",
        columns: ["flow_name", "flow_id", "execution_mode", "target_audience", "enabled", "updated_at"],
      },
      {
        key: "sop_operations",
        label: "SOP 运维",
        path: "/admin/sop/dispatch-tasks",
        skipFetch: true,
        columns: ["task_id", "flow_id", "task_status", "action_count", "created_at", "task_error"],
      },
      {
        key: "knowledge_docs",
        label: "知识库",
        path: "/admin/knowledge/documents",
        columns: ["filename", "status", "size", "updated_at", "doc_id"],
      },
      {
        key: "sop_media",
        label: "SOP 媒体",
        path: "/admin/sop/media/upload",
        skipFetch: true,
        columns: ["key", "value"],
      },
    ],
  },
  {
    key: "observability",
    label: "观测",
    sections: [
      {
        key: "stats_overview",
        label: "统计概览",
        path: "/admin/stats/overview",
        params: { days: 7 },
        columns: ["key", "value"],
      },
      {
        key: "observability_dashboard",
        label: "运行观测",
        path: "/admin/observability/dashboard",
        skipFetch: true,
        columns: ["name", "status", "value", "observed_at"],
      },
      {
        key: "ai_replies",
        label: "AI 回复",
        path: "/admin/ai-config/reply-logs",
        skipFetch: true,
        columns: ["reply_time", "assignee_name", "account_name", "receiver_name", "status", "failure_type"],
      },
      {
        key: "audit_logs",
        label: "审计日志",
        path: "/admin/audit-logs",
        skipFetch: true,
        params: { page: 1, page_size: 20 },
        columns: ["created_at", "operator", "action_type", "detail"],
      },
      {
        key: "system_logs",
        label: "系统日志",
        path: "/admin/system-logs",
        skipFetch: true,
        params: { limit: 20, offset: 0 },
        columns: ["timestamp", "level", "module", "message"],
      },
      {
        key: "archive_status",
        label: "存档状态",
        path: "/archive/status",
        columns: ["key", "value"],
      },
      {
        key: "archive_operations",
        label: "归档治理",
        path: "/archive/official/check",
        skipFetch: true,
        columns: ["key", "value"],
      },
    ],
  },
];

const preferredArrayKeys = [
  "accounts",
  "users",
  "status",
  "assignments",
  "workloads",
  "agents",
  "scripts",
  "words",
  "logs",
  "items",
  "rows",
  "records",
  "documents",
  "results",
  "devices",
  "enterprises",
  "flows",
  "policies",
  "data",
  "events",
  "tasks",
  "receipts",
];

export function findAdminGroup(groupKey) {
  return adminGroups.find((group) => group.key === groupKey) || adminGroups[0];
}

export function normalizeAdminPayload(section, payload) {
  const source = payload && typeof payload === "object" ? payload : {};
  const records = extractRecords(section.key, source);
  const metrics = extractMetrics(section.key, source, records);
  const columns = resolveColumns(section, records);
  return {
    key: section.key,
    label: section.label,
    path: section.path,
    metrics,
    columns,
    records: records.slice(0, 50),
    rows: records.slice(0, 50).map((record) => normalizeRecord(record, columns)),
    rowCount: records.length,
    rawCount: inferRawCount(source, records.length),
  };
}

export function summarizeSection(snapshot) {
  if (!snapshot) return "0";
  if (Number.isFinite(snapshot.rawCount) && snapshot.rawCount !== snapshot.rowCount) {
    return `${snapshot.rowCount}/${snapshot.rawCount}`;
  }
  if (snapshot.rowCount > 0) return String(snapshot.rowCount);
  return String(snapshot.metrics.length);
}

export function formatAdminValue(value) {
  if (value === null || value === undefined || value === "") return "-";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.map(formatAdminValue).join(", ");
  if (typeof value === "object") return compactJSON(value);
  return String(value);
}

function extractRecords(sectionKey, payload) {
  if (sectionKey === "assignment_config") {
    return objectEntries(payload?.config || payload);
  }
  if (sectionKey === "ai_config" || sectionKey === "stats_overview" || sectionKey === "archive_status") {
    const rows = firstArray(payload);
    return rows.length > 0 ? rows : objectEntries(payload?.config || payload?.overview || payload?.current_metrics || payload);
  }
  const rows = firstArray(payload);
  if (rows.length > 0) return rows;
  if (Array.isArray(payload?.data?.items)) return payload.data.items;
  if (payload?.data && typeof payload.data === "object" && !Array.isArray(payload.data)) {
    const nestedRows = firstArray(payload.data);
    if (nestedRows.length > 0) return nestedRows;
  }
  return [];
}

function firstArray(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  for (const key of preferredArrayKeys) {
    if (Array.isArray(payload[key])) return payload[key];
  }
  for (const value of Object.values(payload)) {
    if (Array.isArray(value)) return value;
  }
  return [];
}

function objectEntries(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.entries(value).map(([key, entryValue]) => ({ key, value: entryValue }));
}

function extractMetrics(sectionKey, payload, records) {
  const metrics = [];
  const source = payload?.summary && typeof payload.summary === "object" ? payload.summary : payload;
  Object.entries(source || {}).forEach(([key, value]) => {
    if (metrics.length >= 6) return;
    if (Array.isArray(value) || value === null || value === undefined) return;
    if (typeof value === "object") return;
    if (key === "page" || key === "page_size" || key === "limit" || key === "offset") return;
    metrics.push({ key, value: formatAdminValue(value) });
  });
  if (records.length > 0 && !metrics.some((metric) => metric.key === "returned")) {
    metrics.unshift({ key: "returned", value: formatAdminValue(records.length) });
  }
  if (sectionKey === "sensitive_words" && records.length > 0) {
    const enabled = records.filter((record) => record?.enabled !== false).length;
    metrics.unshift({ key: "enabled", value: formatAdminValue(enabled) });
  }
  return metrics.slice(0, 6);
}

function resolveColumns(section, records) {
  const preferred = Array.isArray(section.columns) ? section.columns : [];
  const observed = [];
  records.slice(0, 8).forEach((record) => {
    if (!record || typeof record !== "object" || Array.isArray(record)) return;
    Object.keys(record).forEach((key) => {
      if (!observed.includes(key)) observed.push(key);
    });
  });
  const columns = [...preferred.filter((key) => observed.includes(key) || key === "key" || key === "value")];
  observed.forEach((key) => {
    if (columns.length < 6 && !columns.includes(key)) columns.push(key);
  });
  return columns.length > 0 ? columns.slice(0, 6) : ["key", "value"];
}

function normalizeRecord(record, columns) {
  if (!record || typeof record !== "object" || Array.isArray(record)) {
    return { key: "value", value: formatAdminValue(record) };
  }
  const normalized = {};
  columns.forEach((column) => {
    normalized[column] = formatAdminValue(record[column]);
  });
  return normalized;
}

function inferRawCount(payload, fallback) {
  const countCandidates = [
    payload?.total,
    payload?.total_count,
    payload?.count,
    payload?.pagination?.total,
    payload?.page?.total,
  ];
  const found = countCandidates.find((value) => Number.isFinite(Number(value)));
  return found === undefined ? fallback : Number(found);
}

function compactJSON(value) {
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
