#!/usr/bin/env bash

set -euo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
make_bin="${MAKE_BIN:-make}"
tier="${FUNCTIONAL_TEST_TIER:-pr-short}"
trigger="${FUNCTIONAL_TEST_TRIGGER:-local}"
budget="${FUNCTIONAL_TEST_BUDGET:-35m}"
short="${FUNCTIONAL_SHORT:-true}"
quarantine="${FUNCTIONAL_QUARANTINE:-tests/functional/functional-quarantine.json}"
log_path="$artifact_root/command.log"

mkdir -p "$artifact_root"

printf '%s\n' \
  "Functional CI runner: tier=$tier trigger=$trigger short=$short budget=$budget selection=subtractive quarantine=$quarantine" \
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

printf '%s\n' "Functional CI runner: tier completed within budget=$budget; diagnostics written under $artifact_root." \
  | tee -a "$log_path"
