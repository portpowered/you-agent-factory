#!/usr/bin/env bash
set -euo pipefail
node --input-type=module -e 'import {runConcurrentCLI} from "./scripts/ci/verification-sequence.mjs"; import {frontendPlan} from "./scripts/ci/verification-plans.mjs"; await runConcurrentCLI(frontendPlan(process.env.FRONTEND_SUITE));'
