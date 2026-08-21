# BOOT-PLAN-PARALLEL-005

## Identity

- Status: `PASSED_WITH_LIMITATIONS`
- Factory: `@you/plan-parallel`
- Repository base: `b0bf1dd59747ec8e3b13e814bdb929c32f2910d2`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-PLAN-PARALLEL-005`
- Model for all roles: provider `CODEX`, model `gpt-5.6-terra`
- Current generated artifact:
  `packages/packaged-factories/generated/factories/plan-parallel/factory.yaml`
- Accepted recording `BOOT-PLAN-PARALLEL-005-R02.replay.json`:
  `4B192D7CE13E1D3047760A9C9370505B055FB0219345FD2E06EBFF91188E4808`
- Recorded Factory hash:
  `sha256:085855285b46956770ba4d33def901d47ae42fdecb54b4bf6171d7eb72c6b623`
- Elapsed time: 312.5 seconds

## Frozen holdout

The request required exactly two independent, read-only investigations of
`test-improvement.md`, concrete repository and focused-test evidence from each
executor, no generated synthesis task, and one terminal merged recommendation.

## Accepted result

The planner emitted exactly two uniquely named `planned-task:init` Work items
and no `DEPENDS_ON` edges because the investigations were genuinely independent.
The runtime added the two expected parent-child relations. Both executor
dispatches started before either completed, ran for 168.1 and 238.7 seconds,
and received only their own complete Work payload. The merger started after both
executor responses and completed in 25.3 seconds.

The ACP investigator returned source-grounded package decomposition, direct
service-test, and shutdown-branch recommendations after a passing 24.332-second
focused package run. The factory-transformation investigator returned
server-reuse and package-ownership recommendations after a passing
20.986-second focused package run. Both reported unchanged tracked state and
identified the pre-existing `.you-agent-factory/` directory.

The terminal answer preserved the two distinct evidence lanes, converted them
into an ordered implementation plan, included focused future commands, and
explicitly labeled the expected scheduling improvement as an unmeasured
inference. Inspection against `test-improvement.md` found the result consistent
with the frozen benchmark and more implementation-ready than its starting text.

## Rejected preflight

`R01` used `--named` and resolved the operator's older editable global install,
not the generated artifact at the pinned commit. Its planner returned the old
noncanonical `work`/`instruction` shape, zero children were admitted, and the
merger honestly reported missing results. Its recording hash is
`8625E9ABF1689AFA941E9A2B3C5E5F6B663244DDE3144B6015DBC482D9A9E6D8`.
This is invalid-source diagnostic evidence, not a Factory outcome.

## Decision

- Canary status: `PASSED` from prior live and deterministic coverage.
- Representative status: `PASSED` on the current generated artifact.
- Goal status: `MEETS_EXPECTATIONS`.
- Score: 19/20. One efficiency point is withheld because bounded repository
  evidence gathering still took over five minutes.
- Limitation: editable global installs are not silently refreshed. Trials must
  verify the resolved source and use a current generated artifact or disposable
  fresh materialization.
