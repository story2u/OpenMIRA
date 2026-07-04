#!/usr/bin/env bash
# Refreshes the committed phase-1 inventory baseline used by phase1_gate.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PYTHON_ROOT="${PYTHON_ROOT:-../Python}"
BASELINE_PATH="${INVENTORY_BASELINE_JSON:-$GO_ROOT/testdata/inventory/baseline.json}"

if [[ "$BASELINE_PATH" != /* ]]; then
  BASELINE_PATH="$GO_ROOT/$BASELINE_PATH"
fi

PYTHON_ROOT_CHECK="$PYTHON_ROOT"
if [[ "$PYTHON_ROOT_CHECK" != /* ]]; then
  PYTHON_ROOT_CHECK="$GO_ROOT/$PYTHON_ROOT_CHECK"
fi

if [[ ! -d "$PYTHON_ROOT_CHECK" ]]; then
  echo "Python root not found: $PYTHON_ROOT" >&2
  exit 1
fi

mkdir -p "$(dirname "$BASELINE_PATH")"
TMP_PATH="$(mktemp)"
cleanup() {
  rm -f "$TMP_PATH"
}
trap cleanup EXIT

(cd "$GO_ROOT" && go run ./cmd/inventory -python-root "$PYTHON_ROOT" -pretty > "$TMP_PATH")
mv "$TMP_PATH" "$BASELINE_PATH"
trap - EXIT

echo "Inventory baseline refreshed: $BASELINE_PATH"
