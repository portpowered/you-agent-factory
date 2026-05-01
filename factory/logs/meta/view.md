# meta view

## world state

- repository `HEAD` is `d55b57e` on `main` after `git pull` on May 1, 2026, and
  `origin/main` is at the same commit.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- this workspace still has ignored local residue under the inbox surface:
  - `factory/inputs/idea/default/dedupe-dispatcher-throttle-pause-filter.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state materially changed on May 1, 2026:
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#17` `ci-cd`
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
- the narrow throttle dedupe follow-up is no longer pending:
  - `pkg/factory/subsystems/subsystem_dispatcher.go` no longer carries the
    redundant post-scheduler pause filter removed by PR `#21`
  - `pkg/factory/projections/throttle_pause_projection.go` now owns the
    projection path for throttle-pause observability
  - `pkg/service/factory.go` now enriches the rendered world view through the
    throttle-pause projection wrapper rather than the base world-view builder
  - `docs/development/dispatcher-throttle-pause-audit-closeout.md` records the
    closeout evidence and the deliberately deferred broader redesign
- the CI/CD ask has partially landed:
  - `.github/workflows/ci.yml` now exists on `main`
  - `README.md` and `docs/development/development.md` document the repository
    CI workflow and local reproduction commands
- the functional-test cleanup ask also advanced:
  - `docs/development/functional-test-cleanup-closeout.md` records the
    remaining behavioral coverage
  - workstation prompts now explicitly steer away from structural/meta tests
- the checked-in replay evidence now reflects the landed continue-versus-
  rejection contract in the maintainer loop:
  - `process` completions in `factory/logs/agent-fails.replay.json`:
    `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review` completions: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`
  - the embedded `process` workstation topology now includes `onContinue`
  - the embedded `process` prompt now reserves rejection for true review
    send-back and treats `<CONTINUE>` as ordinary executor iteration
  - `review` still uses rejection to send work back through `task:init`
- the broader throttle customer ask remains open at the architecture level:
  - the current system still keeps pause state as dispatcher-owned runtime
    memory keyed by provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is not implemented on `main`
  - that redesign should now be considered separately from the already-landed
    duplicate-filter cleanup

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.

## theory of mind

- the repository is in a materially better state than the prior meta entry
  described:
  - the narrow dispatcher throttle simplification is done
  - the first CI workflow is checked in
  - the functional-test cleanup lane already removed several structural tests
- the highest-signal remaining cleanup lane is no longer workflow semantics;
  that contract cleanup is now landed in both the live workflow and the
  checked-in replay evidence.
- the right follow-up for the throttle customer ask is no longer another narrow
  dispatcher cleanup. The next defensible step is either:
  - a broader throttle-guard design lane, or
  - a later explicitly scoped design lane for `INFERENCE_THROTTLE_GUARD`.

## next best move

- update the checked-in meta world model and progress log when repository
  state changes again.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later dedicated
  lane now that the maintainer loop semantics are cleaner.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but the already-merged PR `#21` closed
  only the narrow duplicate-filter cleanup slice.
- the best next customer-facing cleanup lane is no longer the process/review
  contract alignment because that ambiguity is now resolved in both routing and
  checked-in replay evidence.
