#!/usr/bin/env bash

# The supervisor deliberately does not use `set -e`: both child outcomes must
# be observed and reported before the required job returns its combined
# fail-closed verdict.
set -uo pipefail

script_path="${BASH_SOURCE[0]}"
if [[ "$script_path" != /* ]]; then
  script_path="$(cd "$(dirname "$script_path")" && pwd)/$(basename "$script_path")"
fi

timestamp() {
  date -u '+%Y-%m-%dT%H:%M:%S.%3NZ'
}

record_timing() {
  printf '%s\n' "$*" >> "$timing_path" 2>/dev/null || true
}

write_status() {
  local status_path="$1"
  local status="$2"
  if ! printf '%s\n' "$status" > "$status_path"; then
    printf 'Unable to write functional supervisor status: %s\n' "$status_path" >&2
    return 1
  fi
}

run_child() {
  local label="$1"
  local status_path="$2"
  shift 2

  local started_at
  started_at="$(timestamp)"
  record_timing "${label}-start=${started_at}"

  "$@"
  local command_status=$?

  local finished_at
  finished_at="$(timestamp)"
  write_status "$status_path" "$command_status" || true
  record_timing "${label}-end=${finished_at} status=${command_status}"
  exit "$command_status"
}

# The child mode is used to put each command in its own process group. The
# parent can then terminate the complete validator or coverage process tree on
# cancellation instead of leaving a go subprocess behind.
if [[ "${1:-}" == "--child" ]]; then
  if (( $# < 6 )); then
    printf '%s\n' 'child mode requires a label, status path, timing path, and command' >&2
    exit 2
  fi
  child_label="$2"
  child_status_path="$3"
  timing_path="$4"
  shift 4
  if [[ "${1:-}" != "--" ]]; then
    printf '%s\n' 'child mode requires -- before the command' >&2
    exit 2
  fi
  shift
  if (( $# == 0 )); then
    printf '%s\n' 'child mode requires a command' >&2
    exit 2
  fi
  run_child "$child_label" "$child_status_path" "$@"
fi

status_dir="${FUNCTIONAL_CRITICAL_PATH_DIR:-.artifacts/functional-test-viz/c09-critical-path}"
quarantine_status_path="$status_dir/quarantine-status.txt"
coverage_status_path="$status_dir/coverage-status.txt"
timing_path="$status_dir/critical-path-timing.txt"

quarantine_command=(bash scripts/ci/verify-functional-quarantine-packages.sh)
coverage_command=(make functional-test-viz)

# The explicit options are a test-only command seam. CI leaves them unset and
# therefore always uses the fixed validator and make commands above.
while (( $# > 0 )); do
	case "$1" in
		--status-dir)
			if (( $# < 2 )); then
				printf '%s\n' '--status-dir requires a path' >&2
				exit 2
			fi
			status_dir="$2"
			quarantine_status_path="$status_dir/quarantine-status.txt"
			coverage_status_path="$status_dir/coverage-status.txt"
			timing_path="$status_dir/critical-path-timing.txt"
			shift 2
			;;
		--quarantine-command)
			if (( $# < 2 )); then
				printf '%s\n' '--quarantine-command requires an executable path' >&2
				exit 2
			fi
			quarantine_command=("$2")
			shift 2
			;;
		--coverage-command)
			if (( $# < 2 )); then
				printf '%s\n' '--coverage-command requires an executable path' >&2
				exit 2
			fi
			coverage_command=("$2")
			shift 2
			;;
		*)
			printf 'unknown functional supervisor option: %s\n' "$1" >&2
			exit 2
			;;
	esac
done

if ! mkdir -p "$status_dir"; then
  printf 'Unable to create functional supervisor status directory: %s\n' "$status_dir" >&2
  exit 1
fi

: > "$quarantine_status_path"
: > "$coverage_status_path"
: > "$timing_path"

if ! command -v setsid >/dev/null 2>&1; then
  printf '%s\n' 'setsid is required for cancellation-safe functional coverage supervision' >&2
  exit 127
fi

quarantine_pid=""
coverage_pid=""
launched_pid=""
cancelled=0
signal_name=""

terminate_child() {
  local pid="$1"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    return
  fi

  # TERM gives cooperative children a chance to close their own resources;
  # KILL immediately closes the bounded cancellation path if they do not.
  kill -TERM -- "-$pid" 2>/dev/null || true
  kill -KILL -- "-$pid" 2>/dev/null || true
}

cleanup_children() {
  terminate_child "$quarantine_pid"
  terminate_child "$coverage_pid"
}

handle_signal() {
  cancelled=1
  signal_name="$1"
  record_timing "supervisor-signal=${signal_name}"
  cleanup_children
}

trap 'handle_signal TERM' TERM
trap 'handle_signal INT' INT
trap cleanup_children EXIT

launch_child() {
  local label="$1"
  local status_path="$2"
  shift 2

  setsid --wait bash "$script_path" --child "$label" "$status_path" "$timing_path" -- "$@" &
  launched_pid=$!
}

record_timing "supervisor-start=$(timestamp)"
record_timing 'quarantine-dispatch=begin'
launch_child quarantine "$quarantine_status_path" "${quarantine_command[@]}"
quarantine_pid="$launched_pid"
record_timing "quarantine-pid=${quarantine_pid}"

if (( cancelled == 0 )); then
  record_timing 'coverage-dispatch=begin'
  launch_child coverage "$coverage_status_path" "${coverage_command[@]}"
  coverage_pid="$launched_pid"
  record_timing "coverage-pid=${coverage_pid}"
else
  coverage_pid=""
fi

quarantine_status=130
coverage_status=130

if [[ -n "$quarantine_pid" ]]; then
  if wait "$quarantine_pid"; then
    quarantine_status=0
  else
    quarantine_status=$?
  fi
  quarantine_pid=""
fi
record_timing "quarantine-joined=$(timestamp) status=${quarantine_status}"

if [[ -n "$coverage_pid" ]]; then
  if wait "$coverage_pid"; then
    coverage_status=0
  else
    coverage_status=$?
  fi
  coverage_pid=""
fi
record_timing "coverage-joined=$(timestamp) status=${coverage_status}"

if [[ ! -s "$quarantine_status_path" ]]; then
  write_status "$quarantine_status_path" "$quarantine_status" || true
fi
if [[ ! -s "$coverage_status_path" ]]; then
  write_status "$coverage_status_path" "$coverage_status" || true
fi

record_timing "supervisor-end=$(timestamp) quarantine-status=${quarantine_status} coverage-status=${coverage_status}"

if (( cancelled != 0 )); then
  printf 'Functional coverage supervisor cancelled by %s; child processes were terminated.\n' "$signal_name"
  exit 130
fi

if (( quarantine_status != 0 )); then
  printf '%s\n' '::error title=Functional quarantine inventory validation failed::See quarantine-package-evidence.txt'
fi
if (( coverage_status != 0 )); then
  printf '%s\n' '::error title=Backend functional coverage failed::See functional-test-diagnostics artifacts'
fi

if (( quarantine_status != 0 || coverage_status != 0 )); then
  exit 1
fi
