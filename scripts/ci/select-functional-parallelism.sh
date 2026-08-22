#!/usr/bin/env bash

set -euo pipefail

# The functional coverage packages launch subprocesses and spend part of their
# lifetime waiting on I/O. A 3x test-window multiplier is a bounded follow-up
# hypothesis to the failed 2x experiment; discovery remains at runner-derived
# concurrency. The cap keeps a large runner from turning this lane into an
# unbounded process fan-out.
runner_cpus="$("${NPROC_BIN:-nproc}" 2>/dev/null || true)"
case "$runner_cpus" in
  ''|*[!0-9]*|0|0[0-9]*)
    echo "Functional CI runner: nproc returned an unusable logical CPU count: '$runner_cpus'" >&2
    exit 1
    ;;
esac

max_functional_logical_cpus=64
if [ "${#runner_cpus}" -gt 2 ] || [ "$runner_cpus" -gt "$max_functional_logical_cpus" ]; then
  echo "Functional CI runner: logical CPU count '$runner_cpus' exceeds the bounded logical-CPU input limit of $max_functional_logical_cpus" >&2
  exit 1
fi

if [ "$runner_cpus" -lt 2 ]; then
  functional_jobs=2
else
  functional_jobs="$runner_cpus"
fi

functional_test_job_multiplier=3
max_functional_test_jobs=16
functional_test_jobs=$((runner_cpus * functional_test_job_multiplier))
if [ "$functional_test_jobs" -gt "$max_functional_test_jobs" ]; then
  functional_test_jobs="$max_functional_test_jobs"
fi

echo "Functional CI runner: logical_cpus=$runner_cpus jobs=$functional_jobs test_jobs=$functional_test_jobs test_job_policy=${functional_test_job_multiplier}x-capped-${max_functional_test_jobs}"
echo "jobs=$functional_jobs" >> "${GITHUB_OUTPUT:?GITHUB_OUTPUT must name the workflow output file}"
echo "test_jobs=$functional_test_jobs" >> "${GITHUB_OUTPUT:?GITHUB_OUTPUT must name the workflow output file}"
