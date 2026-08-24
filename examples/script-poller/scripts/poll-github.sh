#!/usr/bin/env bash
set -euo pipefail

# Replace this deterministic smoke payload with the repository's GitHub query.
# The poller contract is one FACTORY_REQUEST_BATCH JSON document per run.
printf '%s\n' '{"requestId":"github-intake-smoke","type":"FACTORY_REQUEST_BATCH","works":[]}'
