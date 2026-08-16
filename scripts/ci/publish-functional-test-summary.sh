#!/usr/bin/env bash

set -euo pipefail

markdown_path="${FUNCTIONAL_TEST_VIZ_MARKDOWN:-.artifacts/functional-test-viz/functional-tests.md}"
artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
timing_path="${FUNCTIONAL_TEST_VIZ_TIMING:-$artifact_root/functional-timing-summary.json}"
coverage_path="${FUNCTIONAL_TEST_VIZ_JSON:-$artifact_root/coverage-summary.json}"
profile_path="${FUNCTIONAL_TEST_VIZ_PROFILE:-$artifact_root/coverage.out}"
status_path="$artifact_root/diagnostic-status.txt"

if [ -z "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "GITHUB_STEP_SUMMARY not set; skipping functional test job summary."
  exit 0
fi

if [ ! -f "$markdown_path" ]; then
  if [ -z "${FUNCTIONAL_TEST_VIZ_DIR:-}" ] && [ ! -f "$status_path" ] && [ ! -f "$timing_path" ] && [ ! -f "$coverage_path" ] && [ ! -f "$profile_path" ]; then
    echo "functional-test-viz markdown not found at $markdown_path; skipping job summary (diagnostics unavailable)."
    exit 0
  fi
fi

if [ -f "$markdown_path" ]; then
  cat "$markdown_path" >> "$GITHUB_STEP_SUMMARY" || echo "warning: failed to append functional test markdown to job summary" >&2
else
  printf '%s\n' "## Functional test diagnostics" >> "$GITHUB_STEP_SUMMARY"
  printf '%s\n\n' "The Markdown catalog is unavailable because the tier ended before complete coverage and timing inputs were rendered." >> "$GITHUB_STEP_SUMMARY"
fi

if [ -f "$status_path" ] || [ ! -f "$markdown_path" ]; then
  {
    printf '%s\n\n' "## Functional diagnostics availability"
    if [ -f "$status_path" ]; then
      cat "$status_path"
    else
      if [ -f "$timing_path" ]; then
        printf '%s\n' "available: name=timing path=$timing_path status=incomplete-or-partial"
      else
        printf '%s\n' "missing: name=timing path=$timing_path reason=no timing snapshot was published before the lane ended"
      fi
      if [ -f "$coverage_path" ]; then
        printf '%s\n' "available: name=coverage-summary path=$coverage_path status=incomplete-or-partial"
      else
        printf '%s\n' "missing: name=coverage-summary path=$coverage_path reason=no trustworthy complete or partial coverage measurement was published"
      fi
      if [ -f "$profile_path" ]; then
        printf '%s\n' "available: name=coverage-profile path=$profile_path status=raw-profile"
      else
        printf '%s\n' "missing: name=coverage-profile path=$profile_path reason=go test did not flush a coverage profile"
      fi
      if [ -f "$markdown_path" ]; then
        printf '%s\n' "available: name=markdown path=$markdown_path"
      else
        printf '%s\n' "missing: name=markdown path=$markdown_path reason=the catalog renderer did not receive complete inputs"
      fi
    fi
  } >> "$GITHUB_STEP_SUMMARY" || echo "warning: failed to append functional diagnostics availability" >&2
fi
