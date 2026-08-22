#!/usr/bin/env bash

# Renders the Backend Unit Coverage pull request comment body from the coverage
# and timing artifacts the unit lane publishes. This is a reporting-only sibling
# of publish-unit-coverage-summary.sh and is deliberately fail-open: a missing
# artifact, a malformed document, or a missing node runtime writes a
# degraded-but-present body (or nothing) and exits 0. A reporting failure must
# never turn a passing suite red.

set -uo pipefail

artifact_root="${UNIT_COVERAGE_DIR:-.artifacts/unit-coverage}"
coverage_path="${UNIT_COVERAGE_JSON:-$artifact_root/coverage-summary.json}"
timing_path="${UNIT_COVERAGE_TIMING:-$artifact_root/unit-timing-summary.json}"
comment_path="${UNIT_COVERAGE_COMMENT_FILE:-$artifact_root/unit-coverage-comment.md}"
node_bin="${NODE_BIN:-node}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$(dirname "$comment_path")" 2>/dev/null || true

if ! command -v "$node_bin" >/dev/null 2>&1; then
  echo "unit coverage comment: $node_bin is unavailable; skipping comment body." >&2
  exit 0
fi

"$node_bin" "$script_dir/unit-coverage-report.mjs" \
  --coverage "$coverage_path" \
  --timing "$timing_path" \
  --comment "$comment_path"
status=$?
if [ "$status" -ne 0 ]; then
  echo "unit coverage comment: renderer exited with $status; continuing without a comment body." >&2
fi

exit 0
