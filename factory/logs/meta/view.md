# meta view

## world state

- repository `HEAD` is `161da97` on `main` on May 1, 2026, and
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
  - `factory/inputs/idea/default/dedupe-retired-boundary-alias-rejection-tables.md`
  - `factory/inputs/idea/default/retire-scriptwrap-build-args-shim.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state on May 1, 2026 is now:
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#18` `api-clean`
  - merged PR `#17` `ci-cd`
- the previous worker-provider cleanup lane is no longer pending:
  - PR `#25` merged the `ScriptWrapProvider.buildArgs(...)` shim retirement
    on May 1, 2026
  - `pkg/workers/inference_provider.go` no longer carries that forwarding shim
  - `docs/development/retire-scriptwrap-build-args-shim-closeout.md` records
    the verification bundle for the lane
- the broader throttle customer ask remains open at the architecture level:
  - pause state still lives as dispatcher-owned runtime memory keyed by
    provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- there is a new narrow cleanup lane ready outside the active PR set:
  - `pkg/config/factory_config_mapping.go` still duplicates the same retired
    worker alias rejection list for both the top-level worker object and its
    nested `definition` object
  - the same file also duplicates the retired workstation alias rejection list
    for both the top-level workstation object and its nested `definition`
    object
  - that is redundant legacy-boundary handling in a stable code path, and it
    can be collapsed behind shared field sets or a shared helper without
    changing the rejection behavior

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.
2. open PRs `#20`, `#16`, and `#4` still occupy their respective file sets, so
   new cleanup work should stay outside those lanes.
3. any world model that still points to the worker build-args shim retirement
   is stale now that PR `#25` is merged.

## theory of mind

- the checked-in meta model was stale because it still treated the worker
  build-args shim retirement as the next lane even though PR `#25` is merged.
- `main` now contains the process/review contract alignment, the work-request
  trace-normalization cleanup, the public-factory enum alias ownership
  cleanup, and the worker build-args shim retirement, so those ideas should
  not be re-queued.
- ignored `factory/inputs/**` residue can outlive the merge status of the lane
  it originally described, so merged PR history must win over local residue
  when deciding what is still pending.
- the next best cleanup is to collapse the duplicated retired-boundary alias
  rejection tables in `pkg/config/factory_config_mapping.go`, because that is
  low-risk legacy handling with repeated field inventories in one file.
- the right follow-up for the broader throttling ask is still a later,
  dedicated `INFERENCE_THROTTLE_GUARD` design lane rather than another narrow
  dispatcher tweak.

## next best move

- update the checked-in meta world model and progress log now.
- do not queue another worker build-args, enum-alias, trace-normalization, or
  process/review cleanup idea, because those lanes already landed on `main`.
- queue one new ignored cleanup idea for collapsing the duplicated
  retired-boundary alias rejection tables outside the active PR set.
- leave the broader `INFERENCE_THROTTLE_GUARD` redesign for a later dedicated
  lane once a narrow cleanup slot is needed for that customer ask.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but no checked-in change has yet moved
  pause enforcement into config-authored guards.
- the best next customer-adjacent cleanup lane is the retired-boundary alias
  dedupe in `pkg/config/factory_config_mapping.go`, because it simplifies the
  public config boundary without colliding with the open PR set or reopening
  just-landed worker cleanup.
