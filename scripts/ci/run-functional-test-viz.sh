#!/usr/bin/env bash

set -euo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
make_bin="${MAKE_BIN:-make}"
node_bin="${NODE_BIN:-node}"
tier="${FUNCTIONAL_TEST_TIER:-pr-short}"
trigger="${FUNCTIONAL_TEST_TRIGGER:-local}"
budget="${FUNCTIONAL_TEST_BUDGET:-35m}"
short="${FUNCTIONAL_SHORT:-true}"
quarantine="${FUNCTIONAL_QUARANTINE:-tests/functional/functional-quarantine.json}"
functional_jobs="${FUNCTIONAL_DEFAULT_JOBS:-make-default}"
functional_test_jobs="${FUNCTIONAL_TEST_JOBS:-make-default}"
log_path="$artifact_root/command.log"
timing_path="${FUNCTIONAL_TEST_VIZ_TIMING:-$artifact_root/functional-timing-summary.json}"
coverage_path="${FUNCTIONAL_TEST_VIZ_JSON:-$artifact_root/coverage-summary.json}"
profile_path="${FUNCTIONAL_TEST_VIZ_PROFILE:-$artifact_root/coverage.out}"
markdown_path="${FUNCTIONAL_TEST_VIZ_MARKDOWN:-$artifact_root/functional-tests.md}"
status_path="$artifact_root/diagnostic-status.txt"
gocoverage_exit_path="${FUNCTIONAL_GOCOVERAGE_EXIT_FILE:-$artifact_root/gocoveragecheck-exit-code.txt}"
verdict_path="${FUNCTIONAL_COVERAGE_VERDICT_FILE:-$artifact_root/functional-coverage-verdict.txt}"

mkdir -p "$artifact_root"
# Each invocation owns these outputs. Remove them before starting so a later
# timeout cannot publish a complete artifact produced by an earlier run.
rm -f "$status_path" "$timing_path" "$coverage_path" "$profile_path" "$markdown_path" "$gocoverage_exit_path" "$verdict_path"

printf '%s\n' \
  "Functional CI runner: tier=$tier trigger=$trigger short=$short budget=$budget selection=subtractive quarantine=$quarantine jobs=$functional_jobs test_jobs=$functional_test_jobs" \
  "Functional CI runner: timeout=$budget; partial diagnostics are retained under $artifact_root on failure." \
  | tee "$log_path"

handle_interrupt() {
  exit_code="$1"
  printf '%s\n' "Functional CI runner: interrupted with exit=$exit_code; partial diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit "$exit_code"
}

trap 'handle_interrupt 130' INT
trap 'handle_interrupt 143' TERM

if ! command -v timeout >/dev/null 2>&1; then
  printf '%s\n' "Functional CI runner: GNU timeout is required to enforce budget=$budget; diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit 127
fi

set +e
timeout --signal=TERM --kill-after=30s "$budget" \
  "$make_bin" functional-test-viz \
    FUNCTIONAL_TEST_VIZ_DIR="$artifact_root" \
    FUNCTIONAL_TEST_TIER="$tier" \
    FUNCTIONAL_TEST_TRIGGER="$trigger" \
    FUNCTIONAL_TEST_BUDGET="$budget" \
    FUNCTIONAL_SHORT="$short" \
    FUNCTIONAL_QUARANTINE="$quarantine" \
    FUNCTIONAL_GOCOVERAGE_EXIT_FILE="$gocoverage_exit_path" \
    2>&1 | tee -a "$log_path"
pipeline_status=("${PIPESTATUS[@]}")
set -e

command_status="${pipeline_status[0]}"
tee_status="${pipeline_status[1]}"
if [ "$tee_status" -ne 0 ]; then
  printf '%s\n' "Functional CI runner: diagnostic log writer failed with exit=$tee_status; tier failed closed." \
    | tee -a "$log_path" >&2
  exit "$tee_status"
fi

if [ "$command_status" -eq 124 ]; then
  if [ -f "$timing_path" ]; then
    if command -v python3 >/dev/null 2>&1; then
      timeout 5s python3 - "$timing_path" <<'PY' | tee -a "$log_path" || true
import json
import sys

path = sys.argv[1]
try:
    with open(path, encoding="utf-8") as stream:
        summary = json.load(stream)
    wall_seconds = float(summary.get("wallSeconds", 0) or 0)
    states = summary.get("packageStates", []) or []
    if not isinstance(states, list):
        raise ValueError("packageStates is not an array")
except Exception as error:
    print(f"Functional timing snapshot unavailable: path={path} reason={error}")
    raise SystemExit(0)

print(
    "Functional timing snapshot: "
    f"complete={summary.get('complete', False)} "
    f"packages={summary.get('packageCount', 0)}/{summary.get('expectedPackageCount', 0)} "
    f"wall={wall_seconds:.3f}s reason={summary.get('captureReason', 'tier budget expired')}"
)
for state in states:
    if not isinstance(state, dict):
        continue
    if state.get("state") == "completed":
        continue
    try:
        elapsed = float(state.get("seconds", 0) or 0)
    except (TypeError, ValueError):
        elapsed = 0
    print(
        "Functional package state: "
        f"package={state.get('package', '<unknown>')} "
        f"state={state.get('state', 'unobserved')} "
        f"elapsed={elapsed:.3f}s"
    )
PY
    else
      printf '%s\n' "Functional timing snapshot could not be rendered: python3 is unavailable; timing artifact remains at $timing_path." | tee -a "$log_path" >&2
    fi
  else
    printf '%s\n' "Functional timing summary missing: path=$timing_path reason=no incremental timing snapshot was written before the tier budget expired." | tee -a "$log_path" >&2
  fi

  {
    printf '%s\n' "Functional diagnostics availability: outcome=tier-timeout budget=$budget"
    if [ -f "$timing_path" ]; then
      printf '%s\n' "available: name=timing path=$timing_path status=incomplete-or-partial"
    else
      printf '%s\n' "missing: name=timing path=$timing_path reason=no incremental timing snapshot was written before interruption"
    fi
    if [ -f "$coverage_path" ]; then
      printf '%s\n' "available: name=coverage-summary path=$coverage_path status=incomplete-or-partial"
    else
      printf '%s\n' "missing: name=coverage-summary path=$coverage_path reason=no trustworthy partial coverage summary was available before interruption"
    fi
    if [ -f "$profile_path" ]; then
      printf '%s\n' "available: name=coverage-profile path=$profile_path status=raw-profile"
    else
      printf '%s\n' "missing: name=coverage-profile path=$profile_path reason=go test did not flush a coverage profile before interruption"
    fi
    if [ -f "$markdown_path" ]; then
      printf '%s\n' "available: name=markdown path=$markdown_path"
    else
      printf '%s\n' "missing: name=markdown path=$markdown_path reason=catalog rendering did not run because the tier stopped before complete inputs were available"
    fi
  } > "$status_path" || true
  if [ -f "$status_path" ]; then
    tee -a "$log_path" < "$status_path"
  else
    printf '%s\n' "Functional diagnostics status unavailable: path=$status_path reason=status file could not be written." | tee -a "$log_path" >&2
  fi
  printf '%s\n' "Functional CI runner: tier timed out after budget=$budget; partial diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit 124
fi

if [ "$command_status" -eq 125 ]; then
  printf '%s\n' "Functional CI runner: timeout could not enforce budget=$budget; tier failed closed and diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit 125
fi

if [ "$command_status" -ne 0 ]; then
  printf '%s\n' "Functional CI runner: tier failed with exit=$command_status; diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit "$command_status"
fi

# The Make target deliberately records only gocoveragecheck's ordinary exit 1
# so its complete stream can remain in this successful run step. Classify that
# handoff from the compact terminal markers and write the extract consumed by
# the final failing verdict step. Any unrecognized or missing handoff is an
# infrastructure failure and keeps this full-stream step red.
set +e
"$node_bin" scripts/ci/functional-coverage-verdict.mjs \
  --log "$log_path" \
  --exit-code-file "$gocoverage_exit_path" \
  --output "$verdict_path" \
  2>&1 | tee -a "$log_path"
verdict_pipeline_status=("${PIPESTATUS[@]}")
set -e

verdict_status="${verdict_pipeline_status[0]}"
verdict_tee_status="${verdict_pipeline_status[1]}"
if [ "$verdict_tee_status" -ne 0 ]; then
  printf '%s\n' "Functional CI runner: verdict log writer failed with exit=$verdict_tee_status; tier failed closed." \
    | tee -a "$log_path" >&2
  exit "$verdict_tee_status"
fi
if [ "$verdict_status" -ne 0 ]; then
  printf '%s\n' "Functional CI runner: compact verdict could not classify the recorded coverage outcome; diagnostics retained under $artifact_root." \
    | tee -a "$log_path" >&2
  exit "$verdict_status"
fi

printf '%s\n' "Functional CI runner: tier completed within budget=$budget; diagnostics written under $artifact_root." \
  | tee -a "$log_path"
