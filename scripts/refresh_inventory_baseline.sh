#!/usr/bin/env bash
# Refreshes the optional external-reference inventory baseline used by phase1_gate.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REFERENCE_ROOT="${REFERENCE_ROOT:-}"
BASELINE_PATH="${INVENTORY_BASELINE_JSON:-$GO_ROOT/testdata/inventory/baseline.json}"

if [[ -z "$REFERENCE_ROOT" ]]; then
  echo "REFERENCE_ROOT is required to refresh the optional reference inventory baseline." >&2
  echo "For standalone Go/Next.js validation, run scripts/phase1_gate.sh without refreshing this baseline." >&2
  exit 1
fi

if [[ "$BASELINE_PATH" != /* ]]; then
  BASELINE_PATH="$GO_ROOT/$BASELINE_PATH"
fi

REFERENCE_ROOT_CHECK="$REFERENCE_ROOT"
if [[ "$REFERENCE_ROOT_CHECK" != /* ]]; then
  REFERENCE_ROOT_CHECK="$GO_ROOT/$REFERENCE_ROOT_CHECK"
fi

if [[ ! -d "$REFERENCE_ROOT_CHECK" ]]; then
  echo "Reference root not found: $REFERENCE_ROOT" >&2
  exit 1
fi

mkdir -p "$(dirname "$BASELINE_PATH")"
TMP_PATH="$(mktemp)"
cleanup() {
  rm -f "$TMP_PATH"
}
trap cleanup EXIT

(cd "$GO_ROOT" && go run ./cmd/inventory -reference-root "$REFERENCE_ROOT" -pretty > "$TMP_PATH")
mv "$TMP_PATH" "$BASELINE_PATH"
trap - EXIT

echo "Reference inventory baseline refreshed: $BASELINE_PATH"
