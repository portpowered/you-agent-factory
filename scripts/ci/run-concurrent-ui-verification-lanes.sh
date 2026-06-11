#!/usr/bin/env bash

set -euo pipefail

artifact_root="${ARTIFACT_ROOT:-.artifacts/concurrent-ui-verification-lanes}"
make_bin="${MAKE_BIN:-make}"

coverage_lane_label="UI Coverage"
browser_lane_label="UI Browser Integration"
coverage_target="test-ui-coverage"
browser_target="ui-integration-test"

coverage_log="${artifact_root}/ui-coverage.log"
browser_log="${artifact_root}/ui-browser-integration.log"

mkdir -p "$artifact_root"

printf '%s\n' \
  "==> Concurrent UI verification lanes (UI Coverage + UI Browser Integration)" \
  "==> ${coverage_lane_label} lane [make ${coverage_target}] (concurrent)" \
  "==> ${browser_lane_label} lane [make ${browser_target}] (concurrent)"

run_lane() {
  local lane_label="$1"
  local make_target="$2"
  local log_file="$3"

  (
    set +e
    "$make_bin" "$make_target" 2>&1 | while IFS= read -r line || [[ -n "$line" ]]; do
      printf '[%s] %s\n' "$lane_label" "$line"
    done
    exit "${PIPESTATUS[0]}"
  ) | tee "$log_file"
}

run_lane "$coverage_lane_label" "$coverage_target" "$coverage_log" &
coverage_pid=$!

run_lane "$browser_lane_label" "$browser_target" "$browser_log" &
browser_pid=$!

coverage_status=0
browser_status=0

wait "$coverage_pid" || coverage_status=$?
wait "$browser_pid" || browser_status=$?

if (( coverage_status != 0 )); then
  printf '%s\n' \
    "FAIL: ${coverage_lane_label} lane [make ${coverage_target}] failed. Rerun with: make ${coverage_target}" \
    "Lane log: ${coverage_log}"
fi

if (( browser_status != 0 )); then
  printf '%s\n' \
    "FAIL: ${browser_lane_label} lane [make ${browser_target}] failed. Rerun with: make ${browser_target}" \
    "Lane log: ${browser_log}"
fi

if (( coverage_status != 0 || browser_status != 0 )); then
  exit 1
fi
