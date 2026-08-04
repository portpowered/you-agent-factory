#!/usr/bin/env bash

set -euo pipefail

markdown_path="${FUNCTIONAL_TEST_VIZ_MARKDOWN:-.artifacts/functional-test-viz/functional-tests.md}"

if [ -z "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "GITHUB_STEP_SUMMARY not set; skipping functional test job summary."
  exit 0
fi

if [ ! -f "$markdown_path" ]; then
  echo "functional-test-viz markdown not found at $markdown_path; skipping job summary (diagnostics unavailable)."
  exit 0
fi

cat "$markdown_path" >> "$GITHUB_STEP_SUMMARY" || echo "warning: failed to append functional test markdown to job summary" >&2
