#!/usr/bin/env bash

set -euo pipefail

run_url="${FUNCTIONAL_QUARANTINE_EVIDENCE_RUN_URL:?FUNCTIONAL_QUARANTINE_EVIDENCE_RUN_URL is required}"
head_sha="${FUNCTIONAL_QUARANTINE_EVIDENCE_SHA:?FUNCTIONAL_QUARANTINE_EVIDENCE_SHA is required}"
output_path="${FUNCTIONAL_QUARANTINE_EVIDENCE_OUTPUT:-.artifacts/functional-test-viz/quarantine-package-evidence.txt}"
test_timeout="${FUNCTIONAL_QUARANTINE_EVIDENCE_TIMEOUT:-15m}"

if ! command -v jq >/dev/null 2>&1; then
  printf '%s\n' 'jq is required to verify the structured go test package events' >&2
  exit 127
fi

mkdir -p "$(dirname "$output_path")"
: > "$output_path"

packages=(
  github.com/portpowered/infinite-you/tests/functional/factory_runtime
  github.com/portpowered/infinite-you/tests/functional/operator_settings
  github.com/portpowered/infinite-you/tests/functional/recordings
  github.com/portpowered/infinite-you/tests/functional/work
  github.com/portpowered/infinite-you/tests/functional/providers/contract
  github.com/portpowered/infinite-you/tests/functional/providers/mock_workers
  github.com/portpowered/infinite-you/tests/functional/providers/observability
  github.com/portpowered/infinite-you/tests/functional/providers/script
  github.com/portpowered/infinite-you/tests/functional/transport/terminalportlock
)

record() {
  printf '%s\n' "$*" | tee -a "$output_path"
}

record "functional-quarantine-evidence-run=$run_url"
record "functional-quarantine-evidence-head-sha=$head_sha"
record "functional-quarantine-evidence-command=go test -list=^Test -json -count=1 -short=false [-tags=functionallong] -timeout=$test_timeout <exact-package>"

temporary_root="$(mktemp -d)"
trap 'rm -rf "$temporary_root"' EXIT

measurement_failed=0
for tag_set in default functionallong; do
  tag_args=()
  if [[ "$tag_set" == "functionallong" ]]; then
    tag_args=(-tags=functionallong)
  fi

  record "tag-set=$tag_set"
  for index in "${!packages[@]}"; do
    package="${packages[$index]}"
    event_path="$temporary_root/${tag_set}-${index}.json"
    command_line=(go test "${tag_args[@]}" -list='^Test' -json -count=1 -short=false "-timeout=$test_timeout" "$package")

    set +e
    "${command_line[@]}" > "$event_path" 2>&1
    command_status=$?
    set -e

    terminal_action=''
    if ! terminal_action="$(jq -r --arg package "$package" 'select(.Package == $package and (.Action == "pass" or .Action == "fail" or .Action == "skip")) | .Action' "$event_path" 2>/dev/null | tail -n 1)"; then
      terminal_action=''
    fi

    if [[ "$command_status" -ne 0 || -z "$terminal_action" ]]; then
      measurement_failed=1
      record "package=$package terminal=${terminal_action:-missing} result=measurement-failed exit=$command_status"
      tail -n 20 "$event_path" | sed 's/^/diagnostic=/' | tee -a "$output_path"
      continue
    fi

    tests=''
    if ! tests="$(jq -r --arg package "$package" 'select(.Package == $package and .Action == "output") | .Output' "$event_path" 2>/dev/null | tr '\r' '\n' | awk '/^Test[A-Za-z0-9_]+$/ {print}' | sort -u | paste -sd, -)"; then
      measurement_failed=1
      record "package=$package terminal=$terminal_action result=measurement-failed exit=$command_status detail=unable-to-parse-top-level-tests"
      continue
    fi
    if [[ -z "$tests" ]]; then
      tests='[]'
      result='empty'
    else
      tests="[$tests]"
      result='runnable'
    fi
    record "package=$package terminal=$terminal_action result=$result exit=$command_status top-level-tests=$tests"
  done
done

if [[ "$measurement_failed" -eq 0 ]]; then
  record 'measurement-status=pass'
else
  record 'measurement-status=measurement-failed'
fi
exit "$measurement_failed"
