#!/usr/bin/env bash

set -euo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
make_bin="${MAKE_BIN:-make}"
tier="${FUNCTIONAL_TEST_TIER:-pr-short}"
trigger="${FUNCTIONAL_TEST_TRIGGER:-local}"
budget="${FUNCTIONAL_TEST_BUDGET:-35m}"
short="${FUNCTIONAL_SHORT:-true}"

mkdir -p "$artifact_root"
"$make_bin" functional-test-viz \
  FUNCTIONAL_TEST_VIZ_DIR="$artifact_root" \
  FUNCTIONAL_TEST_TIER="$tier" \
  FUNCTIONAL_TEST_TRIGGER="$trigger" \
  FUNCTIONAL_TEST_BUDGET="$budget" \
  FUNCTIONAL_SHORT="$short" \
  2>&1 | tee "$artifact_root/command.log"
