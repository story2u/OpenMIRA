#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const artifactDir = path.resolve(process.argv[2] ?? "go/tmp/phase1");

const readJSON = (name) => {
  const filePath = path.join(artifactDir, name);
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch {
    return null;
  }
};

const md = (value) => String(value ?? "").replace(/\|/g, "\\|").replace(/\n/g, " ");
const display = (value) => {
  if (value === undefined || value === null || value === "") {
    return "n/a";
  }
  return md(value);
};
const count = (value) => (Array.isArray(value) ? value.length : value ?? 0);
const routeCount = (report, key, countKey) => {
  if (!report) {
    return "n/a";
  }
  if (Number.isFinite(report[countKey])) {
    return report[countKey];
  }
  return count(report[key]);
};

const manifest = readJSON("phase1_gate_manifest.json");
const routeDefault = readJSON("route-diff.json");
const routeCandidate = readJSON("route-diff-candidate.json");
const schemaDrift = readJSON("route-schema-drift.json");
const openAPIDrift = readJSON("route-openapi-drift.json");
const openAPICandidate = readJSON("route-openapi-drift-candidate.json");
const inventoryDiff = readJSON("inventory-diff.json");
const webUnit = readJSON("web-unit-test.json");
const webBuild = readJSON("web-build.json");
const cutover = readCutoverSummary();
const runbookBaseURL = resolveRunbookBaseURL();

const lines = [];
lines.push("# Phase 1 Gate Summary");
lines.push("");
lines.push(`Artifact dir: \`${md(artifactDir)}\``);
lines.push("");

lines.push("## Configuration");
lines.push("");
lines.push("| Key | Value |");
lines.push("| --- | --- |");
lines.push(`| generated_at_utc | ${display(manifest?.generated_at_utc)} |`);
lines.push(`| inventory_diff_profile | ${display(manifest?.inventory_diff_profile)} |`);
lines.push(`| inventory_diff_effective_profile | ${display(manifest?.inventory_diff_effective_profile)} |`);
lines.push(`| inventory_diff_branch | ${display(manifest?.inventory_diff_branch)} |`);
lines.push(`| inventory_baseline_source | ${display(manifest?.inventory_baseline_source)} |`);
lines.push(`| run_reference_gates | ${display(manifest?.run_reference_gates)} |`);
lines.push(`| reference_gates_reason | ${display(manifest?.reference_gates_reason)} |`);
lines.push(`| schema_drift_threshold | ${display(manifest?.schema_drift_mismatch_threshold)} |`);
lines.push(`| openapi_drift_threshold | ${display(manifest?.openapi_drift_mismatch_threshold)} |`);
lines.push(`| route_diff_max_reference_only | ${display(manifest?.route_diff_max_reference_only ?? manifest?.route_diff_max_python_only)} |`);
lines.push(`| route_diff_max_go_only | ${display(manifest?.route_diff_max_go_only)} |`);
lines.push("");

lines.push("## Route Coverage");
lines.push("");
lines.push("| Report | Reference routes | Go routes | Matching | Reference-only | Go-only |");
lines.push("| --- | ---: | ---: | ---: | ---: | ---: |");
for (const [label, report] of [
  ["default", routeDefault],
  ["candidate", routeCandidate],
]) {
  lines.push(
    `| ${label} | ${display(report?.python_route_count)} | ${display(report?.go_route_count)} | ${display(routeCount(report, "matching", "matching_count"))} | ${display(routeCount(report, "python_only", "python_only_count"))} | ${display(routeCount(report, "go_only", "go_only_count"))} |`
  );
}
lines.push("");

lines.push("## Drift Gates");
lines.push("");
lines.push("| Report | Comparable | Matches | Mismatches | Reference spec | Go spec | Top reasons |");
lines.push("| --- | ---: | ---: | ---: | --- | --- | --- |");
lines.push(
  `| schema | ${display(schemaDrift?.schema_comparable_count)} | ${display(schemaDrift?.schema_match_count)} | ${display(schemaDrift?.schema_mismatch_count)} | n/a | n/a | ${display(formatReasons(schemaDrift?.top_drift_reasons))} |`
);
lines.push(
  `| openapi default | ${display(openAPIDrift?.comparable_pairs)} | ${display(openAPIDrift?.match_count)} | ${display(openAPIDrift?.mismatch_count)} | ${display(openAPIDrift?.python_openapi_source_status)} | ${display(openAPIDrift?.go_openapi_source_status)} | ${display(formatReasons(openAPIDrift?.top_drift_reasons))} |`
);
lines.push(
  `| openapi candidate | ${display(openAPICandidate?.comparable_pairs)} | ${display(openAPICandidate?.match_count)} | ${display(openAPICandidate?.mismatch_count)} | ${display(openAPICandidate?.python_openapi_source_status)} | ${display(openAPICandidate?.go_openapi_source_status)} | ${display(formatReasons(openAPICandidate?.top_drift_reasons))} |`
);
lines.push("");

lines.push("## Release Readiness");
lines.push("");
if (cutover?.profiles?.length > 0) {
  lines.push(`Ready profiles: ${display(cutover.ready_count)}/${display(cutover.total_count)}`);
  lines.push("");
  lines.push("| Surface | Failing checks | Affected profiles | Suggested action |");
  lines.push("| --- | ---: | ---: | --- |");
  for (const row of cutoverBlockerRows(cutover.profiles)) {
    lines.push(`| ${display(row.surface)} | ${row.failures} | ${row.profiles} | ${display(actionForSurface(row.surface))} |`);
  }
  lines.push("");
  lines.push("| Profile | Ready | Pass | Fail | Route fail | Flag fail | Env fail | Service fail | Golden fail | Guide | Suggested action | First failures |");
  lines.push("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |");
  for (const profile of cutover.profiles) {
    lines.push(
      `| ${display(profile.profile)} | ${profile.ready ? "yes" : "no"} | ${checkCount(profile, "pass")} | ${checkCount(profile, "fail")} | ${checkCount(profile, "fail", "route")} | ${checkCount(profile, "fail", "flag")} | ${checkCount(profile, "fail", "env")} | ${checkCount(profile, "fail", "service")} | ${checkCount(profile, "fail", "golden")} | ${runbookLink(profile.profile)} | ${display(suggestedCutoverAction(profile))} | ${display(firstFailures(profile))} |`
    );
  }
} else {
  lines.push("Release readiness artifacts were not generated.");
}
lines.push("");

lines.push("## Inventory Diff");
lines.push("");
if (inventoryDiff) {
  lines.push("| Surface | Baseline | Current | Delta | Threshold |");
  lines.push("| --- | ---: | ---: | ---: | ---: |");
  for (const surface of [
    "routes",
    "contracts",
    "feature_docs",
    "compose_services",
    "ws_events",
    "redis_keys",
    "db_tables",
    "task_types",
  ]) {
    const threshold = inventoryDiff.thresholds?.[surface];
    lines.push(
      `| ${surface} | ${display(inventoryDiff.baseline?.[surface])} | ${display(inventoryDiff.current?.[surface])} | ${display(inventoryDiff.deltas?.[surface])} | ${threshold === undefined || threshold < 0 ? "disabled" : display(threshold)} |`
    );
  }
  lines.push("");
  lines.push(`Threshold violations: ${count(inventoryDiff.failures)}`);
} else {
  lines.push("Inventory diff was not generated.");
}
lines.push("");

lines.push("## Web");
lines.push("");
lines.push("| Check | Status | Detail |");
lines.push("| --- | --- | --- |");
lines.push(`| unit | ${display(webUnit?.status)} | tests=${display(webUnit?.tests)}, fail=${display(webUnit?.fail)} |`);
lines.push(`| build | ${display(webBuild?.status)} | artifact=web-build.out |`);
lines.push("");

console.log(lines.join("\n"));

function formatReasons(reasons) {
  if (!Array.isArray(reasons) || reasons.length === 0) {
    return "";
  }
  return reasons
    .slice(0, 5)
    .map((reason) => `${reason.reason ?? "unknown"}:${reason.count ?? 0}`)
    .join(", ");
}

function readCutoverSummary() {
  const aggregate = readJSON("cutover-all.json");
  if (Array.isArray(aggregate?.profiles)) {
    return normalizeCutoverSummary(aggregate);
  }

  let names = [];
  try {
    names = fs.readdirSync(artifactDir);
  } catch {
    return null;
  }

  const profiles = [];
  const seen = new Set();
  for (const name of names.sort()) {
    if (!name.startsWith("cutover-") || !name.endsWith(".json") || name === "cutover-all.json") {
      continue;
    }
    const report = readJSON(name);
    for (const profile of report?.profiles ?? []) {
      if (!profile?.profile || seen.has(profile.profile)) {
        continue;
      }
      seen.add(profile.profile);
      profiles.push(profile);
    }
  }
  if (profiles.length === 0) {
    return null;
  }
  return normalizeCutoverSummary({ profiles });
}

function normalizeCutoverSummary(summary) {
  const profiles = [...(summary.profiles ?? [])].sort((left, right) =>
    String(left.profile ?? "").localeCompare(String(right.profile ?? ""))
  );
  const readyCount = Number.isFinite(summary.ready_count)
    ? summary.ready_count
    : profiles.filter((profile) => profile.ready).length;
  const totalCount = Number.isFinite(summary.total_count) ? summary.total_count : profiles.length;
  return {
    ...summary,
    profiles,
    ready_count: readyCount,
    total_count: totalCount,
  };
}

function checkCount(profile, status, name = "") {
  return (profile?.checks ?? []).filter((check) => {
    if (status && check.status !== status) {
      return false;
    }
    if (name && check.name !== name) {
      return false;
    }
    return true;
  }).length;
}

function firstFailures(profile) {
  return (profile?.checks ?? [])
    .filter((check) => check.status === "fail")
    .slice(0, 3)
    .map((check) => `${check.name}: ${check.detail}`)
    .join("; ");
}

function cutoverBlockerRows(profiles) {
  const rows = [];
  for (const surface of ["route", "golden", "service", "env", "flag"]) {
    const affectedProfiles = new Set();
    let failures = 0;
    for (const profile of profiles ?? []) {
      const surfaceFailures = checkCount(profile, "fail", surface);
      if (surfaceFailures === 0) {
        continue;
      }
      failures += surfaceFailures;
      affectedProfiles.add(profile.profile);
    }
    if (failures > 0) {
      rows.push({ surface, failures, profiles: affectedProfiles.size });
    }
  }
  if (rows.length === 0) {
    rows.push({ surface: "none", failures: 0, profiles: 0 });
  }
  return rows;
}

function suggestedCutoverAction(profile) {
  if (checkCount(profile, "fail") === 0) {
    return "进入 strict readiness gate 或 shadow/canary 验证";
  }
  if (checkCount(profile, "fail", "route") > 0) {
    return actionForSurface("route");
  }
  if (checkCount(profile, "fail", "golden") > 0) {
    return actionForSurface("golden");
  }
  if (checkCount(profile, "fail", "service") > 0) {
    return actionForSurface("service");
  }
  const envFailures = checkCount(profile, "fail", "env");
  const flagFailures = checkCount(profile, "fail", "flag");
  if (envFailures > 0 && flagFailures > 0) {
    return "配置必需 env/secrets，再开启对应 GO_ENABLE_* 候选开关";
  }
  if (envFailures > 0) {
    return actionForSurface("env");
  }
  if (flagFailures > 0) {
    return actionForSurface("flag");
  }
  return "查看 readiness artifact 明细并补齐失败项";
}

function actionForSurface(surface) {
  switch (surface) {
    case "route":
      return "补齐 Go candidate 路由或 route metadata";
    case "golden":
      return "补齐/恢复对应 golden fixture";
    case "service":
      return "同步 go/deploy/cloud compose 服务定义";
    case "env":
      return "配置 profile 必需 env/secrets";
    case "flag":
      return "在 golden/live gate 通过后开启 GO_ENABLE_* 候选开关";
    case "none":
      return "无 readiness blocker";
    default:
      return "查看 readiness artifact 明细";
  }
}

function runbookLink(profileName) {
  if (!profileName) {
    return "n/a";
  }
  return `[guide](${runbookBaseURL}#${anchorForProfile(profileName)})`;
}

function resolveRunbookBaseURL() {
  const docPath = "go/docs/release-readiness.md";
  const serverURL = process.env.GITHUB_SERVER_URL;
  const repository = process.env.GITHUB_REPOSITORY;
  const sha = process.env.GITHUB_SHA;
  if (serverURL && repository && sha) {
    return `${serverURL}/${repository}/blob/${sha}/${docPath}`;
  }
  return docPath;
}

function anchorForProfile(profileName) {
  return String(profileName)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9 -]/g, "")
    .replace(/\s+/g, "-");
}
