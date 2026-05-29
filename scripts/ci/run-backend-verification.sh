#!/usr/bin/env bash

set -euo pipefail

artifact_root="${ARTIFACT_ROOT:-.artifacts/backend-verification}"
make_bin="${MAKE_BIN:-make}"

mkdir -p "$artifact_root"
"$make_bin" test-backend-verification 2>&1 | tee "$artifact_root/command.log"
