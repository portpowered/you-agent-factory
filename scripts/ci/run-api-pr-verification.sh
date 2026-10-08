#!/usr/bin/env bash
set -euo pipefail
node --input-type=module -e 'import {runCLI} from "./scripts/ci/verification-sequence.mjs"; import {apiPlan} from "./scripts/ci/verification-plans.mjs"; await runCLI(apiPlan());'
