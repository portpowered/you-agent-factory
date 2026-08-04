#!/usr/bin/env bash

set -euo pipefail

artifact_root="${FUNCTIONAL_TEST_VIZ_DIR:-.artifacts/functional-test-viz}"
make_bin="${MAKE_BIN:-make}"

mkdir -p "$artifact_root"
"$make_bin" functional-test-viz FUNCTIONAL_TEST_VIZ_DIR="$artifact_root" 2>&1 | tee "$artifact_root/command.log"
