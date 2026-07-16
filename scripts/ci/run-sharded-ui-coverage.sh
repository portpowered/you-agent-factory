#!/usr/bin/env bash

set -euo pipefail

artifact_root="${ARTIFACT_ROOT:-.artifacts/sharded-ui-coverage}"
make_bin="${MAKE_BIN:-make}"
shard_total="${UI_COVERAGE_SHARD_TOTAL:-10}"

if ! [[ "$shard_total" =~ ^[1-9][0-9]*$ ]]; then
  printf '%s\n' \
    "FAIL: Invalid UI_COVERAGE_SHARD_TOTAL=\"${shard_total}\"; expected a positive integer" \
    "Tune shard count with UI_COVERAGE_SHARD_TOTAL (default: 10)" >&2
  exit 1
fi

reports_dir="ui/.vitest-reports"
timing_reports_dir="ui/.vitest-report-timings"
mkdir -p "$artifact_root"
rm -rf "$reports_dir" "$timing_reports_dir"
mkdir -p "$reports_dir" "$timing_reports_dir"

printf '%s\n' \
  "==> Sharded UI Coverage (${shard_total} main covered Vitest shards + merge)" \
  "==> Default shard total: ${shard_total} (override with UI_COVERAGE_SHARD_TOTAL)" \
  "==> Starting shards 1-${shard_total} in parallel"

run_shard() {
  local index="$1"
  local shard_label="${index}/${shard_total}"
  local log_file="${artifact_root}/shard-${index}.log"

  env UI_COVERAGE_SHARD="${shard_label}" "$make_bin" ui-test-coverage >"$log_file" 2>&1
}

print_shard_log() {
  local index="$1"
  local shard_label="${index}/${shard_total}"
  local lane_label="UI Coverage Shard ${shard_label}"
  local log_file="${artifact_root}/shard-${index}.log"

  while IFS= read -r line || [[ -n "$line" ]]; do
    printf '[%s] %s\n' "$lane_label" "$line"
  done <"$log_file"
}

failed_shards=()
pids=()
indices=()

for index in $(seq 1 "$shard_total"); do
  run_shard "$index" &
  pids+=($!)
  indices+=("$index")
done

for i in "${!pids[@]}"; do
  index="${indices[$i]}"
  pid="${pids[$i]}"
  if ! wait "$pid"; then
    failed_shards+=("$index")
  fi
done

for index in "${indices[@]}"; do
  print_shard_log "$index"
done

if ((${#failed_shards[@]} > 0)); then
  printf '%s\n' "FAIL: Sharded UI Coverage main pass failed."
  for index in "${failed_shards[@]}"; do
    printf '%s\n' \
      "FAIL: UI Coverage Shard ${index}/${shard_total} failed. Rerun with: UI_COVERAGE_SHARD=${index}/${shard_total} make ui-test-coverage" \
      "Shard log: ${artifact_root}/shard-${index}.log"
  done
  printf '%s\n' "Full lane rerun: make run-sharded-ui-coverage"
  exit 1
fi

printf '%s\n' \
  "==> All ${shard_total} main covered Vitest shards passed" \
  "==> UI Coverage merge lane [make test-ui-coverage-merge]"

merge_log="${artifact_root}/merge.log"
if ! "$make_bin" test-ui-coverage-merge 2>&1 | tee "$merge_log"; then
  printf '%s\n' \
    "FAIL: UI Coverage merge lane [make test-ui-coverage-merge] failed. Rerun with: make test-ui-coverage-merge" \
    "Merge log: ${merge_log}" \
    "Full lane rerun: make run-sharded-ui-coverage"
  exit 1
fi
