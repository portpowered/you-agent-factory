# meta view

## world state

- repository `HEAD` is `05fb827` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull --ff-only`.
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent.
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- the workspace-local `factory/inputs/**` surface still has ignored residue
  beyond the tracked sentinels, so that material is local context only and not
  checked-in workflow truth:
  - `factory/inputs/idea/default/align-process-review-loop-contract.md`
  - `factory/inputs/idea/default/centralize-work-request-trace-normalization.md`
  - `factory/inputs/idea/default/consolidate-public-factory-enum-alias-ownership.md`
  - `factory/inputs/idea/default/dedupe-dispatcher-throttle-pause-filter.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state on May 1, 2026 is now:
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#18` `api-clean`
  - merged PR `#17` `ci-cd`
- the previous enum-alias follow-up lane is no longer pending:
  - PR `#24` merged the shared public-factory enum alias ownership cleanup
  - `pkg/interfaces/public_factory_enums.go` is now the shared owner for
    canonical public enum helpers
  - strict config-side normalization coverage landed in
    `pkg/config/openapi_factory_test.go`
- the broader throttle customer ask remains open at the architecture level:
  - pause state still lives as dispatcher-owned runtime memory keyed by
    provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- there is a new narrow cleanup lane ready outside the active PR set:
  - `pkg/workers/inference_provider.go` still exposes
    `ScriptWrapProvider.buildArgs`
  - `Infer(...)` already delegates CLI argument construction through
    `providerBehaviorFor(...).BuildArgs(...)`
  - `rg -n "buildArgs\\(" pkg/workers` shows the forwarding shim is now only
    exercised from `pkg/workers/inference_provider_test.go`

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.
2. open PRs `#20`, `#16`, and `#4` still occupy their respective file sets, so
   new cleanup work should stay outside those lanes.

## theory of mind

- the checked-in meta model was stale because it still treated the enum-alias
  consolidation as a pending next lane even though PR `#24` is merged.
- `main` now contains the process/review contract alignment, the work-request
  trace-normalization cleanup, and the public-factory enum alias ownership
  cleanup, so those ideas should not be re-queued.
- ignored `factory/inputs/**` residue can outlive the merge status of the lane
  it originally described, so merged PR history must win over local residue
  when deciding what is still pending.
- the best next cleanup is a true dead-code lane: retire the test-only
  `ScriptWrapProvider.buildArgs` forwarding shim so provider CLI argument
  construction has one canonical owner in `pkg/workers/provider_behavior.go`.
- the right follow-up for the broader throttling ask is still a later,
  dedicated `INFERENCE_THROTTLE_GUARD` design lane rather than another narrow
  dispatcher tweak.

## next best move

- update the checked-in meta world model and progress log now.
- do not queue another enum-alias, trace-normalization, or process/review
  cleanup idea, because those lanes already landed on `main`.
- queue one new ignored cleanup idea for retiring the now-test-only
  `ScriptWrapProvider.buildArgs` shim outside the active PR set.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later dedicated
  lane once a narrow cleanup slot is needed for that customer ask.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but no checked-in change has yet moved
  pause enforcement into config-authored guards.
- the best next customer-adjacent cleanup lane is the provider build-args shim
  retirement, because it removes dead code and simplifies the provider
  abstraction without colliding with the open PR set or reopening just-landed
  work.
