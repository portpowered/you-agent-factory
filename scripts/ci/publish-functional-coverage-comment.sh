#!/usr/bin/env bash

# Renders the Backend Functional Coverage pull request comment body from the
# coverage and timing artifacts the functional tier publishes. This is a
# reporting-only publication step and is
# deliberately fail-open: a missing artifact, a malformed document, or a
# missing node runtime writes an explicit "unavailable" body (or nothing) and
# exits 0. A reporting failure must never turn a passing suite red.

set -uo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
timing_path="${FUNCTIONAL_TEST_VIZ_TIMING:-$artifact_root/functional-timing-summary.json}"
coverage_path="${FUNCTIONAL_TEST_VIZ_JSON:-$artifact_root/coverage-summary.json}"
comment_path="${FUNCTIONAL_COVERAGE_COMMENT_FILE:-$artifact_root/functional-coverage-comment.md}"
node_bin="${NODE_BIN:-node}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$(dirname "$comment_path")" 2>/dev/null || true

if ! command -v "$node_bin" >/dev/null 2>&1; then
  echo "functional coverage comment: $node_bin is unavailable; skipping comment body." >&2
  exit 0
fi

"$node_bin" "$script_dir/functional-coverage-comment.mjs" \
  --coverage "$coverage_path" \
  --timing "$timing_path" \
  --comment "$comment_path"
status=$?
if [ "$status" -ne 0 ]; then
  echo "functional coverage comment: renderer exited with $status; continuing without a comment body." >&2
fi

exit 0
