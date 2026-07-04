#!/usr/bin/env bash
# Phase 1 validation gate for the standalone Go/Next.js IM project.
# Product-local checks run by default. External reference comparison is optional
# evidence and must never be required for a clean standalone checkout.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REFERENCE_ROOT="${REFERENCE_ROOT:-}"
RUN_REFERENCE_GATES="${RUN_REFERENCE_GATES:-0}"
ARTIFACT_DIR="${ARTIFACT_DIR:-$GO_ROOT/tmp/phase1}"
RUN_WEB_E2E="${RUN_WEB_E2E:-0}"
SCHEMA_DRIFT_MISMATCH_THRESHOLD="${SCHEMA_DRIFT_MISMATCH_THRESHOLD:-}"
OPENAPI_DRIFT_MISMATCH_THRESHOLD="${OPENAPI_DRIFT_MISMATCH_THRESHOLD:-}"
REFERENCE_OPENAPI_SPEC="${REFERENCE_OPENAPI_SPEC:-}"
GO_OPENAPI_SPEC="${GO_OPENAPI_SPEC:-}"
INVENTORY_BASELINE_JSON="${INVENTORY_BASELINE_JSON:-}"
DEFAULT_INVENTORY_BASELINE_JSON="$GO_ROOT/testdata/inventory/baseline.json"
INVENTORY_BASELINE_SOURCE="env"
INVENTORY_DIFF_PROFILE="${INVENTORY_DIFF_PROFILE:-auto}"
INVENTORY_DIFF_BRANCH="${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-${BRANCH_NAME:-}}}"
INVENTORY_DIFF_EFFECTIVE_PROFILE="$INVENTORY_DIFF_PROFILE"
SCHEMA_DRIFT_THRESHOLD_TEXT=""
OPENAPI_DRIFT_THRESHOLD_TEXT=""
INVENTORY_DIFF_RUN=0
SCHEMA_DRIFT_RUN=0
OPENAPI_DRIFT_RUN=0
NEXT_REQUIRED_ROUTES="${NEXT_REQUIRED_ROUTES:-/,/admin,/login,/admin-login,/cs-login,/version.txt}"
SKIP_WEB_ROUTE_CHECK="${SKIP_WEB_ROUTE_CHECK:-0}"
ROUTE_DIFF_MAX_REFERENCE_ONLY="${ROUTE_DIFF_MAX_REFERENCE_ONLY:-}"
ROUTE_DIFF_MAX_GO_ONLY="${ROUTE_DIFF_MAX_GO_ONLY:-}"
ROUTE_DIFF_THRESHOLD_TEXT_REFERENCE=""
ROUTE_DIFF_THRESHOLD_TEXT_GO=""
ROUTE_DIFF_THRESHOLD_ARGS=()
if [[ -n "$ROUTE_DIFF_MAX_REFERENCE_ONLY" ]]; then
  ROUTE_DIFF_THRESHOLD_ARGS+=(-max-reference-only "$ROUTE_DIFF_MAX_REFERENCE_ONLY")
  ROUTE_DIFF_THRESHOLD_TEXT_REFERENCE="$ROUTE_DIFF_MAX_REFERENCE_ONLY"
fi
if [[ -n "$ROUTE_DIFF_MAX_GO_ONLY" ]]; then
  ROUTE_DIFF_THRESHOLD_ARGS+=(-max-go-only "$ROUTE_DIFF_MAX_GO_ONLY")
  ROUTE_DIFF_THRESHOLD_TEXT_GO="$ROUTE_DIFF_MAX_GO_ONLY"
fi

REFERENCE_GATES_ENABLED=0
REFERENCE_GATES_REASON=""
case "$RUN_REFERENCE_GATES" in
  auto)
    if [[ -d "$REFERENCE_ROOT" ]]; then
      REFERENCE_GATES_ENABLED=1
      REFERENCE_GATES_REASON="reference root found"
    else
      REFERENCE_GATES_REASON="reference root not found: $REFERENCE_ROOT"
    fi
    ;;
  1 | true | yes)
    if [[ ! -d "$REFERENCE_ROOT" ]]; then
      echo "Reference root not found: $REFERENCE_ROOT" >&2
      exit 1
    fi
    REFERENCE_GATES_ENABLED=1
    REFERENCE_GATES_REASON="explicitly enabled"
    ;;
  0 | false | no)
    REFERENCE_GATES_REASON="explicitly disabled"
    ;;
  *)
    echo "Unsupported RUN_REFERENCE_GATES: $RUN_REFERENCE_GATES" >&2
    exit 1
    ;;
esac

if [[ -z "$INVENTORY_BASELINE_JSON" ]]; then
  if [[ -f "$DEFAULT_INVENTORY_BASELINE_JSON" ]]; then
    INVENTORY_BASELINE_JSON="$DEFAULT_INVENTORY_BASELINE_JSON"
    INVENTORY_BASELINE_SOURCE="default"
  else
    INVENTORY_BASELINE_SOURCE="not_configured"
  fi
fi

if [[ "$INVENTORY_DIFF_PROFILE" == "auto" ]]; then
  case "$INVENTORY_DIFF_BRANCH" in
    main | master | release/*)
      INVENTORY_DIFF_EFFECTIVE_PROFILE="strict"
      ;;
    *)
      INVENTORY_DIFF_EFFECTIVE_PROFILE="observe"
      ;;
  esac
fi

case "$INVENTORY_DIFF_EFFECTIVE_PROFILE" in
  observe)
    ;;
  strict)
    INVENTORY_DIFF_MAX_ROUTES="${INVENTORY_DIFF_MAX_ROUTES:-0}"
    INVENTORY_DIFF_MAX_CONTRACTS="${INVENTORY_DIFF_MAX_CONTRACTS:-0}"
    INVENTORY_DIFF_MAX_FEATURE_DOCS="${INVENTORY_DIFF_MAX_FEATURE_DOCS:-0}"
    INVENTORY_DIFF_MAX_COMPOSE_SERVICES="${INVENTORY_DIFF_MAX_COMPOSE_SERVICES:-0}"
    INVENTORY_DIFF_MAX_WS_EVENTS="${INVENTORY_DIFF_MAX_WS_EVENTS:-0}"
    INVENTORY_DIFF_MAX_REDIS_KEYS="${INVENTORY_DIFF_MAX_REDIS_KEYS:-0}"
    INVENTORY_DIFF_MAX_DB_TABLES="${INVENTORY_DIFF_MAX_DB_TABLES:-0}"
    INVENTORY_DIFF_MAX_TASK_TYPES="${INVENTORY_DIFF_MAX_TASK_TYPES:-0}"
    ;;
  *)
    echo "Unsupported INVENTORY_DIFF_PROFILE: $INVENTORY_DIFF_PROFILE" >&2
    exit 1
    ;;
esac

mkdir -p "$ARTIFACT_DIR"
ARTIFACT_DIR="$(cd "$ARTIFACT_DIR" && pwd)"

echo "==> go test ./..."
(cd "$GO_ROOT" && go test ./...)

echo "==> go vet ./..."
(cd "$GO_ROOT" && go vet ./...)

if [[ "$REFERENCE_GATES_ENABLED" == "1" ]]; then
  SCHEMA_DRIFT_RUN=1
  OPENAPI_DRIFT_RUN=1

  echo "==> reference inventory JSON"
  (cd "$GO_ROOT" && go run ./cmd/inventory -reference-root "$REFERENCE_ROOT" -pretty > "$ARTIFACT_DIR/inventory-report.json")

  echo "==> reference inventory Markdown"
  (cd "$GO_ROOT" && go run ./cmd/inventory -reference-root "$REFERENCE_ROOT" -format markdown > "$ARTIFACT_DIR/inventory-report.md")

  INVENTORY_DIFF_ARGS=()
  if [[ -n "${INVENTORY_DIFF_MAX_ROUTES:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-routes "$INVENTORY_DIFF_MAX_ROUTES")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_CONTRACTS:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-contracts "$INVENTORY_DIFF_MAX_CONTRACTS")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_FEATURE_DOCS:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-feature-docs "$INVENTORY_DIFF_MAX_FEATURE_DOCS")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_COMPOSE_SERVICES:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-compose-services "$INVENTORY_DIFF_MAX_COMPOSE_SERVICES")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_WS_EVENTS:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-ws-events "$INVENTORY_DIFF_MAX_WS_EVENTS")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_REDIS_KEYS:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-redis-keys "$INVENTORY_DIFF_MAX_REDIS_KEYS")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_DB_TABLES:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-db-tables "$INVENTORY_DIFF_MAX_DB_TABLES")
  fi
  if [[ -n "${INVENTORY_DIFF_MAX_TASK_TYPES:-}" ]]; then
    INVENTORY_DIFF_ARGS+=(-max-task-types "$INVENTORY_DIFF_MAX_TASK_TYPES")
  fi
  if [[ -n "$INVENTORY_BASELINE_JSON" ]]; then
    INVENTORY_DIFF_RUN=1
    echo "==> inventory diff JSON"
    (cd "$GO_ROOT" && go run ./cmd/inventory-diff -baseline "$INVENTORY_BASELINE_JSON" -current "$ARTIFACT_DIR/inventory-report.json" -pretty "${INVENTORY_DIFF_ARGS[@]}" > "$ARTIFACT_DIR/inventory-diff.json")
    echo "==> inventory diff Markdown"
    (cd "$GO_ROOT" && go run ./cmd/inventory-diff -baseline "$INVENTORY_BASELINE_JSON" -current "$ARTIFACT_DIR/inventory-report.json" -format markdown "${INVENTORY_DIFF_ARGS[@]}" > "$ARTIFACT_DIR/inventory-diff.md")
  fi

  echo "==> route diff JSON"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -pretty "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-diff.json")

  echo "==> route diff Markdown"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -format markdown "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-diff.md")

  echo "==> candidate route diff JSON"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -go-routes candidate -pretty "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-diff-candidate.json")

  echo "==> candidate route diff Markdown"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -go-routes candidate -format markdown "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-diff-candidate.md")

  SCHEMA_DRIFT_THRESHOLD_ARGS=()
  if [[ -n "$SCHEMA_DRIFT_MISMATCH_THRESHOLD" ]]; then
    SCHEMA_DRIFT_THRESHOLD_ARGS=(-max-schema-mismatch "$SCHEMA_DRIFT_MISMATCH_THRESHOLD")
    SCHEMA_DRIFT_THRESHOLD_TEXT="$SCHEMA_DRIFT_MISMATCH_THRESHOLD"
  fi

  OPENAPI_DRIFT_THRESHOLD_ARGS=()
  if [[ -n "$OPENAPI_DRIFT_MISMATCH_THRESHOLD" ]]; then
    OPENAPI_DRIFT_THRESHOLD_ARGS=(-max-openapi-mismatch "$OPENAPI_DRIFT_MISMATCH_THRESHOLD")
    OPENAPI_DRIFT_THRESHOLD_TEXT="$OPENAPI_DRIFT_MISMATCH_THRESHOLD"
  fi
  OPENAPI_SPEC_ARGS=()
  if [[ -n "$REFERENCE_OPENAPI_SPEC" ]]; then
    OPENAPI_SPEC_ARGS+=(-reference-openapi "$REFERENCE_OPENAPI_SPEC")
  fi
  if [[ -n "$GO_OPENAPI_SPEC" ]]; then
    OPENAPI_SPEC_ARGS+=(-go-openapi "$GO_OPENAPI_SPEC")
  fi

  echo "==> route schema drift JSON"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -mode schema-drift -pretty "${SCHEMA_DRIFT_THRESHOLD_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-schema-drift.json")

  echo "==> route schema drift Markdown"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -mode schema-drift -format markdown "${SCHEMA_DRIFT_THRESHOLD_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-schema-drift.md")

  echo "==> route OpenAPI drift JSON"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -mode openapi-drift -pretty "${OPENAPI_DRIFT_THRESHOLD_ARGS[@]}" "${OPENAPI_SPEC_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-openapi-drift.json")

  echo "==> route OpenAPI drift Markdown"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -mode openapi-drift -format markdown "${OPENAPI_DRIFT_THRESHOLD_ARGS[@]}" "${OPENAPI_SPEC_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-openapi-drift.md")

  echo "==> candidate route OpenAPI drift JSON"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -go-routes candidate -mode openapi-drift -pretty "${OPENAPI_DRIFT_THRESHOLD_ARGS[@]}" "${OPENAPI_SPEC_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-openapi-drift-candidate.json")

  echo "==> candidate route OpenAPI drift Markdown"
  (cd "$GO_ROOT" && go run ./cmd/route-diff -reference-root "$REFERENCE_ROOT" -go-routes candidate -mode openapi-drift -format markdown "${OPENAPI_DRIFT_THRESHOLD_ARGS[@]}" "${OPENAPI_SPEC_ARGS[@]}" "${ROUTE_DIFF_THRESHOLD_ARGS[@]}" > "$ARTIFACT_DIR/route-openapi-drift-candidate.md")
else
  echo "==> reference comparison skipped: $REFERENCE_GATES_REASON"
  cat <<EOF > "$ARTIFACT_DIR/reference-gates.md"
# Reference Gates

External reference comparison was skipped.

Reason: $REFERENCE_GATES_REASON

Standalone Go/Next.js validation continues without requiring another project checkout.
EOF
fi

echo "==> cloud candidate flag surface"
(cd "$GO_ROOT" && rg -o "GO_ENABLE_[A-Z0-9_]+" internal/config/config.go cmd/api internal/app | sed 's/.*GO_ENABLE_/GO_ENABLE_/' | sort -u > "$ARTIFACT_DIR/candidate-flags-code.txt")
(cd "$GO_ROOT" && rg -o "GO_ENABLE_[A-Z0-9_]+" deploy/cloud/docker-compose.yml | sed 's/.*GO_ENABLE_/GO_ENABLE_/' | sort -u > "$ARTIFACT_DIR/candidate-flags-compose.txt")
(cd "$GO_ROOT" && rg -o "GO_ENABLE_[A-Z0-9_]+" deploy/cloud/.env.example | sed 's/.*GO_ENABLE_/GO_ENABLE_/' | sort -u > "$ARTIFACT_DIR/candidate-flags-env-example.txt")
comm -23 "$ARTIFACT_DIR/candidate-flags-code.txt" "$ARTIFACT_DIR/candidate-flags-compose.txt" > "$ARTIFACT_DIR/candidate-flags-missing-compose.txt"
comm -23 "$ARTIFACT_DIR/candidate-flags-code.txt" "$ARTIFACT_DIR/candidate-flags-env-example.txt" > "$ARTIFACT_DIR/candidate-flags-missing-env-example.txt"
if [[ -s "$ARTIFACT_DIR/candidate-flags-missing-compose.txt" ]]; then
  echo "Missing GO_ENABLE flags in deploy/cloud/docker-compose.yml:" >&2
  cat "$ARTIFACT_DIR/candidate-flags-missing-compose.txt" >&2
  exit 1
fi
run_release_profile() {
  local profile="$1"
  local artifact="$2"
  echo "==> release readiness ${artifact} JSON"
  (cd "$GO_ROOT" && go run ./cmd/release-readiness -profile "$profile" -pretty > "$ARTIFACT_DIR/release-readiness-${artifact}.json")
  echo "==> release readiness ${artifact} Markdown"
  (cd "$GO_ROOT" && go run ./cmd/release-readiness -profile "$profile" -format markdown > "$ARTIFACT_DIR/release-readiness-${artifact}.md")
}

run_live_golden_suite() {
  local suite="$1"
  local artifact="$2"
  echo "==> golden live compare JSON: ${artifact}"
  (cd "$GO_ROOT" && go run ./cmd/golden-http \
    -cases "testdata/golden/${suite}" \
    -reference-url "$REFERENCE_BASE_URL" \
    -go-url "$GO_BASE_URL" \
    -pretty > "$ARTIFACT_DIR/${artifact}.json")
  echo "==> golden live compare Markdown: ${artifact}"
  (cd "$GO_ROOT" && go run ./cmd/golden-http \
    -cases "testdata/golden/${suite}" \
    -reference-url "$REFERENCE_BASE_URL" \
    -go-url "$GO_BASE_URL" \
    -format markdown > "$ARTIFACT_DIR/${artifact}.md")
}

run_replay_suite() {
  local suite="$1"
  local artifact="$2"
  local validate_only="${3:-0}"
  if [[ "${validate_only}" == "1" ]]; then
    echo "==> replay suite validate-only JSON: ${artifact}"
    (cd "$GO_ROOT" && go run ./cmd/replay-http \
      -cases "testdata/replay/${suite}" \
      -validate-only \
      -pretty > "$ARTIFACT_DIR/${artifact}.json")
    echo "==> replay suite validate-only Markdown: ${artifact}"
    (cd "$GO_ROOT" && go run ./cmd/replay-http \
      -cases "testdata/replay/${suite}" \
      -validate-only \
      -format markdown > "$ARTIFACT_DIR/${artifact}.md")
    return
  fi

  echo "==> replay suite compare JSON: ${artifact}"
  (cd "$GO_ROOT" && go run ./cmd/replay-http \
    -cases "testdata/replay/${suite}" \
    -pretty > "$ARTIFACT_DIR/${artifact}.json")
  echo "==> replay suite compare Markdown: ${artifact}"
  (cd "$GO_ROOT" && go run ./cmd/replay-http \
    -cases "testdata/replay/${suite}" \
    -format markdown > "$ARTIFACT_DIR/${artifact}.md")
}

RELEASE_PROFILE_LIST="${RELEASE_PROFILE_LIST:-${CUTOVER_PROFILE_LIST:-}}"
SKIP_RELEASE_AGGREGATE="${SKIP_RELEASE_AGGREGATE:-${SKIP_CUTOVER_AGGREGATE:-0}}"

if [[ -n "$RELEASE_PROFILE_LIST" ]]; then
  IFS=',' read -r -a selected_release_profiles <<< "$RELEASE_PROFILE_LIST"
else
  selected_release_profiles=(
    session-access
    admin-observability
    incoming-ingest
    task-status
    workbench-read
    admin-accounts
    admin-assignments
    admin-config-content
    sop-operations
    admin-diagnostics
    realtime-workbench
    contact-sync
    workbench-actions
    connector-events
    platform-proxy
    ai-outreach
    archive-pipeline
    archive-voice-transcription
    archive-cold-storage
    device-ops
    send-dispatch
  )
fi

sanitize_release_profiles() {
  local raw
  local trimmed
  local seen_profiles="|"

  for raw in "$@"; do
    trimmed="${raw//[[:space:]]/}"
    [[ -z "$trimmed" ]] && continue
    case "$seen_profiles" in
      *"|$trimmed|"*)
        ;;
      *)
        seen_profiles="${seen_profiles}${trimmed}|"
        echo "$trimmed"
        ;;
    esac
  done
}

sanitized_release_profiles=()
while IFS= read -r profile; do
  sanitized_release_profiles+=("$profile")
done < <(sanitize_release_profiles "${selected_release_profiles[@]}")
selected_release_profiles=("${sanitized_release_profiles[@]}")

manifest_profiles() {
  local first=true
  local p
  for p in "${selected_release_profiles[@]}"; do
    if [[ "$first" == "true" ]]; then
      first=false
      printf '    "%s"' "$p"
    else
      printf ',\n    "%s"' "$p"
    fi
  done
}

cat <<EOF > "$ARTIFACT_DIR/phase1_gate_manifest.json"
{
  "generated_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "go_root": "$GO_ROOT",
  "reference_root": "$REFERENCE_ROOT",
  "run_reference_gates": ${REFERENCE_GATES_ENABLED},
  "reference_gates_reason": "$REFERENCE_GATES_REASON",
  "artifact_dir": "$ARTIFACT_DIR",
  "run_web_routes": 1,
  "required_next_routes": "$NEXT_REQUIRED_ROUTES",
  "skip_web_route_check": "${SKIP_WEB_ROUTE_CHECK:-0}",
  "skip_npm_ci": "${SKIP_NPM_CI:-0}",
  "run_schema_drift": ${SCHEMA_DRIFT_RUN},
  "schema_drift_mismatch_threshold": "${SCHEMA_DRIFT_THRESHOLD_TEXT}",
  "run_openapi_drift": ${OPENAPI_DRIFT_RUN},
  "openapi_drift_mismatch_threshold": "${OPENAPI_DRIFT_THRESHOLD_TEXT}",
  "reference_openapi_spec": "${REFERENCE_OPENAPI_SPEC}",
  "go_openapi_spec": "${GO_OPENAPI_SPEC}",
  "run_inventory_diff": ${INVENTORY_DIFF_RUN},
  "inventory_diff_profile": "${INVENTORY_DIFF_PROFILE}",
  "inventory_diff_effective_profile": "${INVENTORY_DIFF_EFFECTIVE_PROFILE}",
  "inventory_diff_branch": "${INVENTORY_DIFF_BRANCH}",
  "inventory_baseline_source": "${INVENTORY_BASELINE_SOURCE}",
  "default_inventory_baseline_json": "${DEFAULT_INVENTORY_BASELINE_JSON}",
  "inventory_baseline_json": "${INVENTORY_BASELINE_JSON}",
  "inventory_diff_thresholds": {
    "routes": "${INVENTORY_DIFF_MAX_ROUTES:-}",
    "contracts": "${INVENTORY_DIFF_MAX_CONTRACTS:-}",
    "feature_docs": "${INVENTORY_DIFF_MAX_FEATURE_DOCS:-}",
    "compose_services": "${INVENTORY_DIFF_MAX_COMPOSE_SERVICES:-}",
    "ws_events": "${INVENTORY_DIFF_MAX_WS_EVENTS:-}",
    "redis_keys": "${INVENTORY_DIFF_MAX_REDIS_KEYS:-}",
    "db_tables": "${INVENTORY_DIFF_MAX_DB_TABLES:-}",
    "task_types": "${INVENTORY_DIFF_MAX_TASK_TYPES:-}"
  },
  "inventory_diff_artifacts": [
    "inventory-diff.json",
    "inventory-diff.md"
  ],
  "route_diff_max_reference_only": "${ROUTE_DIFF_THRESHOLD_TEXT_REFERENCE}",
  "route_diff_max_go_only": "${ROUTE_DIFF_THRESHOLD_TEXT_GO}",
  "schema_drift_artifacts": [
    "route-schema-drift.json",
    "route-schema-drift.md"
  ],
  "openapi_drift_artifacts": [
    "route-openapi-drift.json",
    "route-openapi-drift.md",
    "route-openapi-drift-candidate.json",
    "route-openapi-drift-candidate.md"
  ],
  "web_route_artifacts": [
    "web-routes.json",
    "web-routes.md"
  ],
  "web_unit_artifacts": [
    "web-unit-test.out",
    "web-unit-test.json",
    "web-unit-test.md"
  ],
  "web_build_artifacts": [
    "web-build.out",
    "web-build.json",
    "web-build.md"
  ],
  "run_web_e2e": "${RUN_WEB_E2E}",
  "web_e2e_artifacts": [
    "web-e2e.out",
    "web-e2e.json",
    "web-e2e.md"
  ],
  "run_replay_gating": "${RUN_REPLAY_GATING:-0}",
  "run_replay_compare": "${RUN_REPLAY_COMPARE:-0}",
  "skip_release_aggregate": "${SKIP_RELEASE_AGGREGATE}",
  "release_profile_count": ${#selected_release_profiles[@]},
  "release_profiles": [
$(manifest_profiles)
  ]
}
EOF

for profile in "${selected_release_profiles[@]}"; do
  profile="${profile// /}"
  [[ -z "$profile" ]] && continue
  run_release_profile "$profile" "$profile"
done

if [[ "$SKIP_RELEASE_AGGREGATE" != "1" ]]; then
  echo "==> release readiness all profiles JSON"
  (cd "$GO_ROOT" && go run ./cmd/release-readiness -all -pretty > "$ARTIFACT_DIR/release-readiness-all.json")
  echo "==> release readiness all profiles Markdown"
  (cd "$GO_ROOT" && go run ./cmd/release-readiness -all -format markdown > "$ARTIFACT_DIR/release-readiness-all.md")
fi

echo "==> golden suite validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase1-probes.json -validate-only -pretty > "$ARTIFACT_DIR/golden-cases.json")

echo "==> golden suite validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase1-probes.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-cases.md")

echo "==> phase2 session admin login golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-admin-login.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-admin-login.json")

echo "==> phase2 session admin login golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-admin-login.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-admin-login.md")

echo "==> phase2 session login golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-login.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-login.json")

echo "==> phase2 session login golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-login.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-login.md")

echo "==> phase2 session cs login golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-cs-login.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-cs-login.json")

echo "==> phase2 session cs login golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-cs-login.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-cs-login.md")

echo "==> phase2 session generate cs token golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-generate-cs-token.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-generate-cs-token.json")

echo "==> phase2 session generate cs token golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-generate-cs-token.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-generate-cs-token.md")

echo "==> phase2 session golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-me.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-me.json")

echo "==> phase2 session golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-me.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-me.md")

echo "==> phase2 session refresh golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-refresh.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-refresh.json")

echo "==> phase2 session refresh golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-refresh.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-refresh.md")

echo "==> phase2 session logout golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-logout.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase2-session-logout.json")

echo "==> phase2 session logout golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase2-session-logout.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase2-session-logout.md")

echo "==> phase3 cs bootstrap golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-bootstrap.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-cs-bootstrap.json")

echo "==> phase3 cs bootstrap golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-bootstrap.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-cs-bootstrap.md")

echo "==> phase3 cs summary golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-workbench-summary.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-cs-workbench-summary.json")

echo "==> phase3 cs summary golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-workbench-summary.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-cs-workbench-summary.md")

echo "==> phase3 cs conversations golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-conversations.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-cs-conversations.json")

echo "==> phase3 cs conversations golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-conversations.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-cs-conversations.md")

echo "==> phase3 conversation messages golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-messages.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-conversation-messages.json")

echo "==> phase3 conversation messages golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-messages.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-conversation-messages.md")

echo "==> phase3 cs workbench search golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-workbench-search.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-cs-workbench-search.json")

echo "==> phase3 cs workbench search golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-cs-workbench-search.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-cs-workbench-search.md")

echo "==> phase3 conversation list golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-list.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-conversation-list.json")

echo "==> phase3 conversation list golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-list.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-conversation-list.md")

echo "==> phase3 conversation account stats golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-account-stats.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-conversation-account-stats.json")

echo "==> phase3 conversation account stats golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-account-stats.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-conversation-account-stats.md")

echo "==> phase3 conversation panel bootstrap golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-panel-bootstrap.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-conversation-panel-bootstrap.json")

echo "==> phase3 conversation panel bootstrap golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-panel-bootstrap.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-conversation-panel-bootstrap.md")

echo "==> phase3 conversation panel snapshot golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-panel-snapshot.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase3-conversation-panel-snapshot.json")

echo "==> phase3 conversation panel snapshot golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase3-conversation-panel-snapshot.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase3-conversation-panel-snapshot.md")

echo "==> phase4 accounts list golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-list.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-accounts-list.json")

echo "==> phase4 accounts list golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-list.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-accounts-list.md")

echo "==> phase4 accounts ai enabled write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-ai-enabled-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-accounts-ai-enabled-write.json")

echo "==> phase4 accounts ai enabled write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-ai-enabled-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-accounts-ai-enabled-write.md")

echo "==> phase4 accounts manage write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-manage-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-accounts-manage-write.json")

echo "==> phase4 accounts manage write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-manage-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-accounts-manage-write.md")

echo "==> phase4 accounts batch write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-batch-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-accounts-batch-write.json")

echo "==> phase4 accounts batch write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-batch-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-accounts-batch-write.md")

echo "==> phase4 accounts assign write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-assign-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-accounts-assign-write.json")

echo "==> phase4 accounts assign write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-accounts-assign-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-accounts-assign-write.md")

echo "==> phase4 conversation ai write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-conversation-ai-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-conversation-ai-write.json")

echo "==> phase4 conversation ai write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-conversation-ai-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-conversation-ai-write.md")

echo "==> phase4 conversation read golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-conversation-read.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-conversation-read.json")

echo "==> phase4 conversation read golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-conversation-read.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-conversation-read.md")

echo "==> phase4 cs users list golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-list.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-cs-users-list.json")

echo "==> phase4 cs users list golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-list.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-cs-users-list.md")

echo "==> phase4 cs users status golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-status.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-cs-users-status.json")

echo "==> phase4 cs users status golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-status.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-cs-users-status.md")

echo "==> phase4 cs users write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-cs-users-write.json")

echo "==> phase4 cs users write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-cs-users-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-cs-users-write.md")

echo "==> phase4 assignment config golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-config.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-config.json")

echo "==> phase4 assignment config golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-config.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-config.md")

echo "==> phase4 assignment config write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-config-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-config-write.json")

echo "==> phase4 assignment config write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-config-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-config-write.md")

echo "==> phase4 assignment write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-write.json")

echo "==> phase4 assignment write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-write.md")

echo "==> phase4 assignment purge golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-purge.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-purge.json")

echo "==> phase4 assignment purge golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-purge.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-purge.md")

echo "==> phase4 assignment auto golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-auto.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-auto.json")

echo "==> phase4 assignment auto golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-auto.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-auto.md")

echo "==> phase4 assignment workloads golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-workloads.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-workloads.json")

echo "==> phase4 assignment workloads golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-workloads.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-workloads.md")

echo "==> phase4 assignments list golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignments-list.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignments-list.json")

echo "==> phase4 assignments list golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignments-list.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignments-list.md")

echo "==> phase4 assignment detail golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-detail.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-assignment-detail.json")

echo "==> phase4 assignment detail golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-assignment-detail.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-assignment-detail.md")

echo "==> phase4 audit logs golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-audit-logs.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-audit-logs.json")

echo "==> phase4 audit logs golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-audit-logs.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-audit-logs.md")

echo "==> phase4 system logs golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-system-logs.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-system-logs.json")

echo "==> phase4 system logs golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-system-logs.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-system-logs.md")

echo "==> phase4 client errors golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-client-errors.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-client-errors.json")

echo "==> phase4 client errors golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-client-errors.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-client-errors.md")

echo "==> phase4 observability dashboard golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-observability-dashboard.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-observability-dashboard.json")

echo "==> phase4 observability dashboard golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-observability-dashboard.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-observability-dashboard.md")

echo "==> phase4 stage6 health golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stage6-health.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stage6-health.json")

echo "==> phase4 stage6 health golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stage6-health.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stage6-health.md")

echo "==> phase4 p1 screen golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-p1-screen.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-p1-screen.json")

echo "==> phase4 p1 screen golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-p1-screen.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-p1-screen.md")

echo "==> phase4 devices list golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-devices-list.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-devices-list.json")

echo "==> phase4 devices list golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-devices-list.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-devices-list.md")

echo "==> phase4 device discovery refresh golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-discovery-refresh.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-discovery-refresh.json")

echo "==> phase4 device discovery refresh golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-discovery-refresh.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-discovery-refresh.md")

echo "==> phase4 device discovery probe golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-discovery-probe.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-discovery-probe.json")

echo "==> phase4 device discovery probe golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-discovery-probe.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-discovery-probe.md")

echo "==> phase4 devices manual golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-devices-manual.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-devices-manual.json")

echo "==> phase4 devices manual golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-devices-manual.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-devices-manual.md")

echo "==> phase4 agent retired golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-agent-retired.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-agent-retired.json")

echo "==> phase4 agent retired golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-agent-retired.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-agent-retired.md")

echo "==> phase4 wework login qrcode golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-qrcode.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-login-qrcode.json")

echo "==> phase4 wework login qrcode golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-qrcode.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-login-qrcode.md")

echo "==> phase4 wework login verify golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-verify.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-login-verify.json")

echo "==> phase4 wework login verify golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-verify.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-login-verify.md")

echo "==> phase4 wework logout golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-logout.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-logout.json")

echo "==> phase4 wework logout golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-logout.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-logout.md")

echo "==> phase4 wework login status golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-status.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-login-status.json")

echo "==> phase4 wework login status golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-login-status.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-login-status.md")

echo "==> phase4 wework user info last golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-last.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-user-info-last.json")

echo "==> phase4 wework user info last golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-last.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-user-info-last.md")

echo "==> phase4 wework user info request golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-request.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-user-info-request.json")

echo "==> phase4 wework user info request golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-request.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-user-info-request.md")

echo "==> phase4 wework user info candidates golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-candidates.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-wework-user-info-candidates.json")

echo "==> phase4 wework user info candidates golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-wework-user-info-candidates.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-wework-user-info-candidates.md")

echo "==> phase4 device call audio bridge golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-call-audio-bridge.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-call-audio-bridge.json")

echo "==> phase4 device call audio bridge golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-call-audio-bridge.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-call-audio-bridge.md")

echo "==> phase4 device sdk webrtc golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-webrtc.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-sdk-webrtc.json")

echo "==> phase4 device sdk webrtc golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-webrtc.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-sdk-webrtc.md")

echo "==> phase4 device sdk status golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-status.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-sdk-status.json")

echo "==> phase4 device sdk status golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-status.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-sdk-status.md")

echo "==> phase4 device sdk control golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-control.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-sdk-control.json")

echo "==> phase4 device sdk control golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-control.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-sdk-control.md")

echo "==> phase4 device sdk rtc session golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-rtc-session.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-sdk-rtc-session.json")

echo "==> phase4 device sdk rtc session golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-sdk-rtc-session.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-sdk-rtc-session.md")

echo "==> phase4 device rtc active golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-active.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-rtc-active.json")

echo "==> phase4 device rtc active golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-active.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-rtc-active.md")

echo "==> phase4 device rtc control golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-control.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-rtc-control.json")

echo "==> phase4 device rtc control golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-control.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-rtc-control.md")

echo "==> phase4 device rtc media prepare golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-media-prepare.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-device-rtc-media-prepare.json")

echo "==> phase4 device rtc media prepare golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-device-rtc-media-prepare.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-device-rtc-media-prepare.md")

echo "==> phase4 diagnostic device map golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-device-map.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-device-map.json")

echo "==> phase4 diagnostic device map golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-device-map.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-device-map.md")

echo "==> phase4 diagnostic orphan conversations golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-orphan-conversations.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-orphan-conversations.json")

echo "==> phase4 diagnostic orphan conversations golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-orphan-conversations.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-orphan-conversations.md")

echo "==> phase4 diagnostic forked conversations golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-forked-conversations.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-forked-conversations.json")

echo "==> phase4 diagnostic forked conversations golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-forked-conversations.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-forked-conversations.md")

echo "==> phase4 diagnostic dirty contacts golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-dirty-contacts.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-dirty-contacts.json")

echo "==> phase4 diagnostic dirty contacts golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-dirty-contacts.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-dirty-contacts.md")

echo "==> phase4 diagnostic archive sync status golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-sync-status.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-sync-status.json")

echo "==> phase4 diagnostic archive sync status golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-sync-status.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-sync-status.md")

echo "==> phase4 diagnostic archive missing outbox check golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-missing-outbox-check.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-missing-outbox-check.json")

echo "==> phase4 diagnostic archive missing outbox check golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-missing-outbox-check.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-missing-outbox-check.md")

echo "==> phase4 diagnostic archive missing outbox replay golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-missing-outbox-replay.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-missing-outbox-replay.json")

echo "==> phase4 diagnostic archive missing outbox replay golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-archive-missing-outbox-replay.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-archive-missing-outbox-replay.md")

echo "==> phase4 diagnostic historical timezone cutover golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-historical-timezone-cutover.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-diagnostic-historical-timezone-cutover.json")

echo "==> phase4 diagnostic historical timezone cutover golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-diagnostic-historical-timezone-cutover.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-diagnostic-historical-timezone-cutover.md")

echo "==> phase4 contacts read golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contacts-read.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-contacts-read.json")

echo "==> phase4 contacts read golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contacts-read.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-contacts-read.md")

echo "==> phase4 contact sync external golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-external.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-contact-sync-external.json")

echo "==> phase4 contact sync external golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-external.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-contact-sync-external.md")

echo "==> phase4 contact sync full golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-full.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-contact-sync-full.json")

echo "==> phase4 contact sync full golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-full.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-contact-sync-full.md")

echo "==> phase4 contact sync refresh stale golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-refresh-stale.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-contact-sync-refresh-stale.json")

echo "==> phase4 contact sync refresh stale golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-contact-sync-refresh-stale.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-contact-sync-refresh-stale.md")

echo "==> phase9 archive read golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-read.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-read.json")

echo "==> phase9 archive read golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-read.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-read.md")

echo "==> phase9 archive official check golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-official-check.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-official-check.json")

echo "==> phase9 archive official check golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-official-check.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-official-check.md")

echo "==> phase9 archive integration test golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-integration-test.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-integration-test.json")

echo "==> phase9 archive integration test golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-integration-test.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-integration-test.md")

echo "==> phase9 archive messages batch golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-messages-batch.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-messages-batch.json")

echo "==> phase9 archive messages batch golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-messages-batch.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-messages-batch.md")

echo "==> phase9 archive sync run golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-sync-run.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-sync-run.json")

echo "==> phase9 archive sync run golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-sync-run.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-sync-run.md")

echo "==> phase9 archive contacts sync golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-contacts-sync.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-contacts-sync.json")

echo "==> phase9 archive contacts sync golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-contacts-sync.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-contacts-sync.md")

echo "==> phase9 archive callback golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-callback.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-callback.json")

echo "==> phase9 archive callback golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-callback.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-callback.md")

echo "==> phase9 archive events notify golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-events-notify.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-events-notify.json")

echo "==> phase9 archive events notify golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-events-notify.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-events-notify.md")

echo "==> phase9 archive sdk bridge golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-sdk-bridge.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-sdk-bridge.json")

echo "==> phase9 archive sdk bridge golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-sdk-bridge.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-sdk-bridge.md")

echo "==> phase9 archive media run golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-media-run.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-media-run.json")

echo "==> phase9 archive media run golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-media-run.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-media-run.md")

echo "==> phase9 archive media download golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-media-download.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-media-download.json")

echo "==> phase9 archive media download golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-media-download.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-media-download.md")

echo "==> phase9 archive voice retry golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-voice-retry.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase9-archive-voice-retry.json")

echo "==> phase9 archive voice retry golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase9-archive-voice-retry.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase9-archive-voice-retry.md")

echo "==> phase4 sensitive words golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sensitive-words.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sensitive-words.json")

echo "==> phase4 sensitive words golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sensitive-words.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sensitive-words.md")

echo "==> phase4 sensitive words write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sensitive-words-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sensitive-words-write.json")

echo "==> phase4 sensitive words write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sensitive-words-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sensitive-words-write.md")

echo "==> phase4 admin scripts golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-admin-scripts.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-admin-scripts.json")

echo "==> phase4 admin scripts golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-admin-scripts.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-admin-scripts.md")

echo "==> phase4 admin scripts write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-admin-scripts-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-admin-scripts-write.json")

echo "==> phase4 admin scripts write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-admin-scripts-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-admin-scripts-write.md")

echo "==> phase4 script library golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-script-library.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-script-library.json")

echo "==> phase4 script library golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-script-library.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-script-library.md")

echo "==> phase4 script generate golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-script-generate.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-script-generate.json")

echo "==> phase4 script generate golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-script-generate.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-script-generate.md")

echo "==> phase4 ai config golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-ai-config.json")

echo "==> phase4 ai config golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-ai-config.md")

echo "==> phase4 ai config write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-ai-config-write.json")

echo "==> phase4 ai config write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-ai-config-write.md")

echo "==> phase4 ai config test golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config-test.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-ai-config-test.json")

echo "==> phase4 ai config test golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-config-test.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-ai-config-test.md")

echo "==> phase4 ai reply logs golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-reply-logs.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-ai-reply-logs.json")

echo "==> phase4 ai reply logs golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-ai-reply-logs.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-ai-reply-logs.md")

echo "==> phase4 sop flows golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-flows.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-flows.json")

echo "==> phase4 sop flows golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-flows.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-flows.md")

echo "==> phase4 sop policies golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-policies.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-policies.json")

echo "==> phase4 sop policies golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-policies.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-policies.md")

echo "==> phase4 sop config write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-config-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-config-write.json")

echo "==> phase4 sop config write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-config-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-config-write.md")

echo "==> phase4 sop analytics stage stats golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-analytics-stage-stats.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-analytics-stage-stats.json")

echo "==> phase4 sop analytics stage stats golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-analytics-stage-stats.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-analytics-stage-stats.md")

echo "==> phase4 sop analytics facts golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-analytics-facts.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-analytics-facts.json")

echo "==> phase4 sop analytics facts golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-analytics-facts.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-analytics-facts.md")

echo "==> phase4 sop dispatch tasks golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-dispatch-tasks.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-dispatch-tasks.json")

echo "==> phase4 sop dispatch tasks golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-dispatch-tasks.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-dispatch-tasks.md")

echo "==> phase4 sop dispatch resend golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-dispatch-resend.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-dispatch-resend.json")

echo "==> phase4 sop dispatch resend golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-dispatch-resend.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-dispatch-resend.md")

echo "==> phase4 sop media local golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-media-local.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-media-local.json")

echo "==> phase4 sop media local golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-media-local.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-media-local.md")

echo "==> phase4 sop media upload golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-media-upload.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-media-upload.json")

echo "==> phase4 sop media upload golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-media-upload.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-media-upload.md")

echo "==> phase4 sop platform test golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-platform-test.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-sop-platform-test.json")

echo "==> phase4 sop platform test golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-sop-platform-test.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-sop-platform-test.md")

echo "==> phase4 knowledge docs golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-docs.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-knowledge-docs.json")

echo "==> phase4 knowledge docs golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-docs.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-knowledge-docs.md")

echo "==> phase4 knowledge docs write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-docs-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-knowledge-docs-write.json")

echo "==> phase4 knowledge docs write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-docs-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-knowledge-docs-write.md")

echo "==> phase4 knowledge search golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-search.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-knowledge-search.json")

echo "==> phase4 knowledge search golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-knowledge-search.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-knowledge-search.md")

echo "==> phase4 enterprises golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-enterprises.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-enterprises.json")

echo "==> phase4 enterprises golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-enterprises.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-enterprises.md")

echo "==> phase4 enterprises write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-enterprises-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-enterprises-write.json")

echo "==> phase4 enterprises write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-enterprises-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-enterprises-write.md")

echo "==> phase4 stats overview golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-overview.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-overview.json")

echo "==> phase4 stats overview golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-overview.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-overview.md")

echo "==> phase4 stats trend golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-trend.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-trend.json")

echo "==> phase4 stats trend golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-trend.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-trend.md")

echo "==> phase4 stats agents golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-agents.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-agents.json")

echo "==> phase4 stats agents golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-agents.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-agents.md")

echo "==> phase4 stats ai reply overview golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-overview.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-overview.json")

echo "==> phase4 stats ai reply overview golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-overview.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-overview.md")

echo "==> phase4 stats ai reply trend golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-trend.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-trend.json")

echo "==> phase4 stats ai reply trend golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-trend.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-trend.md")

echo "==> phase4 stats ai reply breakdown golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-breakdown.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-breakdown.json")

echo "==> phase4 stats ai reply breakdown golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase4-stats-ai-reply-breakdown.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase4-stats-ai-reply-breakdown.md")

echo "==> phase5 stream channels golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase5-stream-channels.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase5-stream-channels.json")

echo "==> phase5 stream channels golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase5-stream-channels.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase5-stream-channels.md")

echo "==> phase5 realtime read golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase5-realtime-read.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase5-realtime-read.json")

echo "==> phase5 realtime read golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase5-realtime-read.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase5-realtime-read.md")

echo "==> phase6 task create golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase6-task-create.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase6-task-create.json")

echo "==> phase6 task create golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase6-task-create.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase6-task-create.md")

echo "==> phase6 task status golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase6-task-status.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase6-task-status.json")

echo "==> phase6 task status golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase6-task-status.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase6-task-status.md")

echo "==> phase8 incoming messages golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase8-incoming-messages.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase8-incoming-messages.json")

echo "==> phase8 incoming messages golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase8-incoming-messages.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase8-incoming-messages.md")

echo "==> phase10 ai outreach golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-ai-outreach.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase10-ai-outreach.json")

echo "==> phase10 ai outreach golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-ai-outreach.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase10-ai-outreach.md")

echo "==> phase10 platform proxy read golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-read.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase10-platform-proxy-read.json")

echo "==> phase10 platform proxy read golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-read.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase10-platform-proxy-read.md")

echo "==> phase10 platform proxy write golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-write.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase10-platform-proxy-write.json")

echo "==> phase10 platform proxy write golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-write.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase10-platform-proxy-write.md")

echo "==> phase10 platform proxy sidebar golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-sidebar.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase10-platform-proxy-sidebar.json")

echo "==> phase10 platform proxy sidebar golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase10-platform-proxy-sidebar.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase10-platform-proxy-sidebar.md")

echo "==> phase11 conversation reply golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-reply.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-reply.json")

echo "==> phase11 conversation reply golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-reply.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-reply.md")

echo "==> phase11 send text golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-send-text.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-send-text.json")

echo "==> phase11 send text golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-send-text.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-send-text.md")

echo "==> phase11 group invite golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-group-invite.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-group-invite.json")

echo "==> phase11 group invite golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-group-invite.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-group-invite.md")

echo "==> phase11 send media golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-send-media.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-send-media.json")

echo "==> phase11 send media golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-send-media.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-send-media.md")

echo "==> phase11 conversation call golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-call.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-call.json")

echo "==> phase11 conversation call golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-call.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-call.md")

echo "==> phase11 conversation transfer golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-transfer.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-transfer.json")

echo "==> phase11 conversation transfer golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-transfer.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-transfer.md")

echo "==> phase11 conversation customer profile golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-customer-profile.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-customer-profile.json")

echo "==> phase11 conversation customer profile golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-customer-profile.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-customer-profile.md")

echo "==> phase11 contact profile resolve golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-contact-profile-resolve.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-contact-profile-resolve.json")

echo "==> phase11 contact profile resolve golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-contact-profile-resolve.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-contact-profile-resolve.md")

echo "==> phase11 contact profile refresh golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-contact-profile-refresh.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-contact-profile-refresh.json")

echo "==> phase11 contact profile refresh golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-contact-profile-refresh.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-contact-profile-refresh.md")

echo "==> phase11 friend added golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-friend-added.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-friend-added.json")

echo "==> phase11 friend added golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-friend-added.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-friend-added.md")

echo "==> phase11 wework notify callback golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-wework-notify-callback.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-wework-notify-callback.json")

echo "==> phase11 wework notify callback golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-wework-notify-callback.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-wework-notify-callback.md")

echo "==> phase11 conversation revoke golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-revoke.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-revoke.json")

echo "==> phase11 conversation revoke golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-revoke.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-revoke.md")

echo "==> phase11 conversation resend golden validation JSON"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-resend.json -validate-only -pretty > "$ARTIFACT_DIR/golden-phase11-conversation-resend.json")

echo "==> phase11 conversation resend golden validation Markdown"
(cd "$GO_ROOT" && go run ./cmd/golden-http -cases testdata/golden/phase11-conversation-resend.json -validate-only -format markdown > "$ARTIFACT_DIR/golden-phase11-conversation-resend.md")

if [[ "${RUN_LIVE_GOLDEN:-0}" == "1" ]]; then
  REFERENCE_BASE_URL="${REFERENCE_BASE_URL:-}"
  if [[ -z "${REFERENCE_BASE_URL:-}" || -z "${GO_BASE_URL:-}" ]]; then
    echo "Skip live golden compare: REFERENCE_BASE_URL and GO_BASE_URL are required when RUN_LIVE_GOLDEN=1" >&2
  else
    echo "==> golden live compare (runtime probes)"
    run_live_golden_suite "phase1-runtime-probes.json" "golden-phase1-runtime-probes"
    if [[ "${RUN_PHASE2_AUTH_GOLDEN_LIVE:-0}" == "1" ]]; then
      echo "==> golden live compare (phase2 session auth)"
      run_live_golden_suite "phase2-session-auth-readonly.json" "golden-phase2-session-auth-readonly"
    fi
  fi
fi

if [[ "${RUN_REPLAY_GATING:-0}" == "1" ]]; then
  run_replay_suite "phase5-realtime-read-replay.json" "replay-phase5-realtime-read" 1
  if [[ "${RUN_REPLAY_COMPARE:-0}" == "1" ]]; then
    run_replay_suite "phase5-realtime-read-replay.json" "replay-phase5-realtime-read"
  fi
fi

echo "==> next route JSON"
(cd "$GO_ROOT" && node scripts/next-routes.mjs "$GO_ROOT/web/app" --require="$NEXT_REQUIRED_ROUTES" > "$ARTIFACT_DIR/web-routes.json")

echo "==> next route Markdown"
(cd "$GO_ROOT" && node scripts/next-routes.mjs "$GO_ROOT/web/app" --markdown --require="$NEXT_REQUIRED_ROUTES" > "$ARTIFACT_DIR/web-routes.md")

if [[ "$SKIP_WEB_ROUTE_CHECK" != "1" ]]; then
  echo "==> next required route check"
  (cd "$GO_ROOT" && node scripts/next-routes.mjs "$GO_ROOT/web/app" --check --require="$NEXT_REQUIRED_ROUTES" >/dev/null)
fi

if [[ "${SKIP_NPM_CI:-0}" != "1" ]]; then
  echo "==> npm ci"
  (cd "$GO_ROOT/web" && npm ci)
fi

echo "==> npm test"
if (cd "$GO_ROOT/web" && npm test > "$ARTIFACT_DIR/web-unit-test.out" 2>&1); then
  WEB_UNIT_TEST_STATUS="passed"
else
  WEB_UNIT_TEST_STATUS="failed"
  echo "web unit tests failed; tail of artifact:"
  tail -n 60 "$ARTIFACT_DIR/web-unit-test.out" >&2
  exit 1
fi

WEB_UNIT_TESTS=$(awk '/^# tests / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)
WEB_UNIT_PASS=$(awk '/^# pass / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)
WEB_UNIT_FAIL=$(awk '/^# fail / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)
WEB_UNIT_SKIP=$(awk '/^# skipped / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)
WEB_UNIT_TODO=$(awk '/^# todo / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)
WEB_UNIT_DURATION_MS=$(awk '/^# duration_ms / { print $3 }' "$ARTIFACT_DIR/web-unit-test.out" | tail -n 1)

cat <<EOF > "$ARTIFACT_DIR/web-unit-test.json"
{
  "status": "${WEB_UNIT_TEST_STATUS}",
  "tests": ${WEB_UNIT_TESTS:-0},
  "pass": ${WEB_UNIT_PASS:-0},
  "fail": ${WEB_UNIT_FAIL:-0},
  "skipped": ${WEB_UNIT_SKIP:-0},
  "todo": ${WEB_UNIT_TODO:-0},
  "duration_ms": ${WEB_UNIT_DURATION_MS:-0}
}
EOF

cat <<EOF > "$ARTIFACT_DIR/web-unit-test.md"
# Web Unit Test Report

- status: ${WEB_UNIT_TEST_STATUS}
- tests: ${WEB_UNIT_TESTS:-0}
- pass: ${WEB_UNIT_PASS:-0}
- fail: ${WEB_UNIT_FAIL:-0}
- skipped: ${WEB_UNIT_SKIP:-0}
- todo: ${WEB_UNIT_TODO:-0}
- duration_ms: ${WEB_UNIT_DURATION_MS:-0}
- output: web-unit-test.out
EOF

echo "==> npm run build"
if (cd "$GO_ROOT/web" && npm run build > "$ARTIFACT_DIR/web-build.out" 2>&1); then
  WEB_BUILD_STATUS="passed"
else
  WEB_BUILD_STATUS="failed"
  echo "web build failed; tail of artifact:"
  tail -n 60 "$ARTIFACT_DIR/web-build.out" >&2
  exit 1
fi

cat <<EOF > "$ARTIFACT_DIR/web-build.json"
{
  "status": "${WEB_BUILD_STATUS}"
}
EOF

cat <<EOF > "$ARTIFACT_DIR/web-build.md"
# Web Build Report

- status: ${WEB_BUILD_STATUS}
- output: web-build.out
EOF

if [[ "${RUN_WEB_E2E:-0}" == "1" ]]; then
  echo "==> npm run test:e2e"
  if (cd "$GO_ROOT/web" && npm run test:e2e > "$ARTIFACT_DIR/web-e2e.out" 2>&1); then
    WEB_E2E_STATUS="passed"
  else
    WEB_E2E_STATUS="failed"
    echo "web e2e tests failed; tail of artifact:"
    tail -n 120 "$ARTIFACT_DIR/web-e2e.out" >&2
    exit 1
  fi

  WEB_E2E_TOTAL=$(awk '/^\s*[0-9]+\s+passed\b/ {print $1; exit}' "$ARTIFACT_DIR/web-e2e.out" | tr -d ' ')
  WEB_E2E_FAILED=$(awk '/^\s*[0-9]+\s+failed\b/ {print $1; exit}' "$ARTIFACT_DIR/web-e2e.out" | tr -d ' ')
  WEB_E2E_SKIPPED=$(awk '/^\s*[0-9]+\s+skipped\b/ {print $1; exit}' "$ARTIFACT_DIR/web-e2e.out" | tr -d ' ')

  cat <<EOF > "$ARTIFACT_DIR/web-e2e.json"
{
  "status": "${WEB_E2E_STATUS}",
  "passed": ${WEB_E2E_TOTAL:-0},
  "failed": ${WEB_E2E_FAILED:-0},
  "skipped": ${WEB_E2E_SKIPPED:-0}
}
EOF

  cat <<EOF > "$ARTIFACT_DIR/web-e2e.md"
# Web E2E Test Report

- status: ${WEB_E2E_STATUS}
- passed: ${WEB_E2E_TOTAL:-0}
- failed: ${WEB_E2E_FAILED:-0}
- skipped: ${WEB_E2E_SKIPPED:-0}
- output: web-e2e.out
EOF
else
  echo "skip web e2e tests (RUN_WEB_E2E != 1)"
fi

echo "Phase 1 gate passed. Artifacts: $ARTIFACT_DIR"
