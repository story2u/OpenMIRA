export const ASSIGNMENT_CONFIG_PATH = "/admin/assignment-config";

function cleanText(value) {
  return String(value || "").trim();
}

export function normalizeAssignmentConfig(payload = {}) {
  const config = payload?.config && typeof payload.config === "object"
    ? payload.config
    : payload?.data?.config && typeof payload.data.config === "object"
      ? payload.data.config
      : payload && typeof payload === "object"
        ? payload
        : {};
  return normalizeAssignmentConfigObject(config);
}

export function normalizeAssignmentConfigRecords(records = []) {
  const config = {};
  (Array.isArray(records) ? records : []).forEach((record) => {
    const key = cleanText(record?.key);
    if (key === "rules" || key === "pools") config[key] = record?.value;
  });
  return normalizeAssignmentConfigObject(config);
}

export function buildAssignmentConfigMutation(options = {}) {
  const rules = parseConfigRows(firstDefined(options.rules, options.rulesJSON, options.rules_json));
  if (!rules.ok) return { ok: false, error: "rules_invalid" };
  const pools = parseConfigRows(firstDefined(options.pools, options.poolsJSON, options.pools_json));
  if (!pools.ok) return { ok: false, error: "pools_invalid" };
  return {
    ok: true,
    method: "POST",
    path: ASSIGNMENT_CONFIG_PATH,
    body: {
      rules: rules.value,
      pools: pools.value,
    },
  };
}

export function formatAssignmentConfigJSON(value = []) {
  try {
    return JSON.stringify(Array.isArray(value) ? value : [], null, 2);
  } catch {
    return "[]";
  }
}

function normalizeAssignmentConfigObject(config = {}) {
  const rules = normalizeConfigRows(config.rules);
  const pools = normalizeConfigRows(config.pools);
  return {
    rules,
    pools,
    rulesJSON: formatAssignmentConfigJSON(rules),
    poolsJSON: formatAssignmentConfigJSON(pools),
  };
}

function normalizeConfigRows(value) {
  if (Array.isArray(value)) return value.filter((item) => item && typeof item === "object");
  if (typeof value === "string") {
    const parsed = parseConfigRows(value);
    return parsed.ok ? parsed.value : [];
  }
  return [];
}

function parseConfigRows(value) {
  if (typeof value === "string") {
    const text = cleanText(value);
    if (!text) return { ok: true, value: [] };
    try {
      return parseConfigRows(JSON.parse(text));
    } catch {
      return { ok: false, value: [] };
    }
  }
  if (!Array.isArray(value)) return { ok: false, value: [] };
  return { ok: true, value: value.filter((item) => item && typeof item === "object") };
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}
