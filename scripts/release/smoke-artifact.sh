#!/usr/bin/env sh
set -eu

if [ "$#" -lt 2 ]; then
  echo "usage: scripts/release/smoke-artifact.sh <binary-path> <fixture-path> [timeout]" >&2
  exit 1
fi

TIMEOUT="${3:-20s}"

go run ./cmd/releasesmoke -binary "$1" -fixture "$2" -timeout "$TIMEOUT"
