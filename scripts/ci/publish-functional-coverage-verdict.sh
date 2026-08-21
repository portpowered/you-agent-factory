#!/usr/bin/env bash

set -euo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
verdict_path="${FUNCTIONAL_COVERAGE_VERDICT_FILE:-$artifact_root/functional-coverage-verdict.txt}"
exit_path="${FUNCTIONAL_GOCOVERAGE_EXIT_FILE:-$artifact_root/gocoveragecheck-exit-code.txt}"

# A timeout, missing tool, or other infrastructure failure already makes the
# full-stream run step red and has no compact gocoverage verdict to replay.
# Keep this always-run reporting step successful so it cannot hide that
# original diagnostic outcome.
if [ ! -f "$verdict_path" ] || [ ! -f "$exit_path" ]; then
	printf '%s\n' "Functional coverage verdict unavailable: run step did not produce a classified verdict."
	exit 0
fi

exit_code="$(tr -d '[:space:]' < "$exit_path")"
if ! [[ "$exit_code" =~ ^[0-9]+$ ]] || [ "$exit_code" -gt 255 ]; then
	printf '%s\n' "Functional coverage verdict unavailable: recorded gocoveragecheck exit code is invalid: $exit_code" >&2
	exit 1
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
	{
		printf '%s\n\n' "## Functional coverage verdict"
		cat "$verdict_path"
	} | tee -a "$GITHUB_STEP_SUMMARY"
else
	cat "$verdict_path"
fi

exit "$exit_code"
