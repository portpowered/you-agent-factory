# BOOT-PLAN-PARALLEL-006

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/plan-parallel`
- Repository base: `4019c27122df9238aa99a92aeabf29e8520917c9`
- Worktree: `.artifacts/bootstrap/worktrees/TEST-IMPROVEMENT-001`
- Planner and merger: provider `CODEX`, model `gpt-5.6-sol`
- Executors: provider `cursor-acp`, model `composer-2.5`
- Generated Factory hash:
  `sha256:085855285b46956770ba4d33def901d47ae42fdecb54b4bf6171d7eb72c6b623`
- Recording: `.artifacts/bootstrap/TEST-IMPROVEMENT-001/PLAN-PARALLEL-R01.replay.json`
- Recording SHA-256:
  `F6C0E7CAB634561492121D3B7456899AE5F003A177333497F7D72AC43AF679C2`
- Elapsed time: 1439.5 seconds

## Frozen request

The exact request instructed the Factory to implement the justified changes in
`test-improvement.md`, first read repository instructions and a fresh
100.293-second red baseline, preserve or restore correctness, create a
dependency-aware DAG with disjoint mutation ownership, and finish with one
integrated verifier depending on all mutation tasks. It explicitly prioritized
ACP classification layering, factory-transformation server reuse, correct
test-tier ownership for the separate CLI tail, and repair of the five
deterministic baseline failures. It prohibited increasing `-jobs`, weakening
coverage, committing, or editing campaign reports.

The invocation used the current generated artifact directly:

```powershell
& '.artifacts/bootstrap/bin/you-4019c2712.exe' --json run `
  --factory 'packages/packaged-factories/generated/factories/plan-parallel/factory.yaml' `
  --record '../../TEST-IMPROVEMENT-001/PLAN-PARALLEL-R01.replay.json' `
  --output primary `
  --planner-provider CODEX --planner-model gpt-5.6-sol `
  --executor-provider cursor-acp --executor-model composer-2.5 `
  --merge-provider CODEX --merge-model gpt-5.6-sol `
  --to '<the frozen request above>'
```

The complete unabridged request is retained in the recording's initial Work
payload and first planner model request.

## Observed orchestration

The planner emitted six uniquely named `planned-task` items:

- five independent mutation tasks for ACP classification, transformation
  server reuse, CLI tiering, invocation-result correctness, and mock-runner
  correctness;
- one `integrated-verification` task with five `DEPENDS_ON` relationships.

All five mutation dispatches received their unique complete payload and started
at the same event time, before any completed. The verifier did not dispatch
until all five prerequisites completed. The terminal merger then consumed the
original request and all six child results. This proves the intended parallel
readiness, dependency gating, and all-child fan-in with Cursor Composer used for
every implementation child.

## Result and independent review

The parallel trial found and implemented the three accepted performance
changes. Its verifier also repaired the original five red package families and
honestly reported a remaining red suite rather than claiming success.

The remaining failure exposed a semantic arbitration weakness. One executor
changed JavaScript string results from customer-facing `TEXT` back to JSON
string parts and updated consumers to agree. Independent history and contract
review established that commit `6a49ef49e` intentionally introduced plain-text
projection for CLI-renderable results. The correct repair was to retain that
production behavior and update stale JSON-decoding assertions. The tracked
`coverage` rewrite produced by one child was also rejected.

After independent correction, the accepted implementation passed three full
functional runs at 75.277, 75.884, and 76.178 seconds. The median was 75.884
seconds, 24.34% below the observed 100.293-second red baseline. The accepted
implementation was committed as `a4863d09b` in the experiment branch and
promoted as `3073fd5bf`.
An additional full run on the promoted root, including the packaged
plan-execute fix, passed in 81.394 seconds and was not folded into the benchmark
median.

## Score and decision

- Intended outcome: 3/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 3/4
- Safety and scope: 2/3
- Final result quality: 2/2
- Efficiency: 0/2
- Total: 14/20
- Canary status: `PASSED` from prior live and deterministic coverage.
- Representative orchestration status: `PASSED`.
- Representative implementation status: `NEEDS_INDEPENDENT_CORRECTION`.
- Goal status: `MEETS_EXPECTATIONS` based on the complete evidence set, with
  this complex shared-worktree limitation retained.

This trial proves that the Factory can generate and execute a real mutation DAG
with parallel Cursor workers and a gated verifier. It does not prove that the
merger can reliably arbitrate a cross-cutting public-contract disagreement
between otherwise green child patches.
