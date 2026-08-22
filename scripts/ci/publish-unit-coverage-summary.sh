#!/usr/bin/env bash

# Publishes the Backend Unit Coverage job summary from the coverage and timing
# artifacts the unit lane writes. This is a reporting-only sibling of
# publish-functional-test-summary.sh and is deliberately fail-open: a missing
# artifact, a malformed document, or a missing node runtime writes an explicit
# "unavailable" section and exits 0. A reporting failure must never turn a
# passing suite red.

set -uo pipefail

artifact_root="${UNIT_COVERAGE_DIR:-.artifacts/unit-coverage}"
coverage_path="${UNIT_COVERAGE_JSON:-$artifact_root/coverage-summary.json}"
timing_path="${UNIT_COVERAGE_TIMING:-$artifact_root/unit-timing-summary.json}"
profile_path="${UNIT_COVERAGE_PROFILE:-$artifact_root/coverage.out}"
summary_path="${UNIT_COVERAGE_SUMMARY_FILE:-$artifact_root/unit-coverage-summary.md}"
node_bin="${NODE_BIN:-node}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -z "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "GITHUB_STEP_SUMMARY not set; skipping the unit coverage job summary."
  exit 0
fi

mkdir -p "$(dirname "$summary_path")" 2>/dev/null || true

# The renderer owns the report and its diagnostics-availability section: it is
# the only party that knows whether an artifact that exists on disk actually
# parsed, so it reports readability rather than mere presence.
if command -v "$node_bin" >/dev/null 2>&1; then
  "$node_bin" "$script_dir/unit-coverage-report.mjs" \
    --coverage "$coverage_path" \
    --timing "$timing_path" \
    --profile "$profile_path" \
    --summary "$summary_path"
  render_status=$?
  if [ "$render_status" -ne 0 ]; then
    echo "unit coverage summary: renderer exited with $render_status; falling back to a presence-only section." >&2
  fi
else
  echo "unit coverage summary: $node_bin is unavailable; falling back to a presence-only section." >&2
fi

if [ -s "$summary_path" ]; then
  cat "$summary_path" >> "$GITHUB_STEP_SUMMARY" || echo "warning: failed to append the unit coverage report to the job summary" >&2
  exit 0
fi

# The renderer produced no body at all, so all this step can still report is
# which inputs are present on disk.
{
  printf '%s\n\n' "## Backend Unit Coverage"
  printf '%s\n\n' "The report is unavailable because the lane ended before the renderer produced a body."
  printf '%s\n\n' "### Unit coverage diagnostics availability"
  for input in "coverage-summary:$coverage_path" "timing:$timing_path" "coverage-profile:$profile_path"; do
    name="${input%%:*}"
    path="${input#*:}"
    if [ -f "$path" ]; then
      printf '%s\n' "present: name=$name path=$path status=unverified — the renderer never read it"
    else
      printf '%s\n' "unavailable: name=$name path=$path reason=the lane published no such file before it ended"
    fi
  done
} >> "$GITHUB_STEP_SUMMARY" || echo "warning: failed to append the unit coverage availability section" >&2

exit 0
