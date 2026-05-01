# meta view

## world state

- repository `HEAD` is `6d20718` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull`.
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
  - `factory/inputs/idea/default/dedupe-generated-boundary-alias-rejection-coverage.md`
  - `factory/inputs/idea/default/dedupe-retired-boundary-alias-rejection-tables.md`
  - `factory/inputs/idea/default/retire-scriptwrap-build-args-shim.md`
  - `factory/inputs/idea/default/systems-cleanup.md`
  - `factory/inputs/idea/default/test-cleanup.md`
  - `factory/inputs/task/default/ci-cd.md`
- the recent GitHub lane state on May 1, 2026 is now:
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#26` `dedupe-retired-boundary-alias-rejection-tables`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
  - merged PR `#19` `systems-cleanup`
  - merged PR `#18` `api-clean`
  - merged PR `#17` `ci-cd`
- the previous generated-boundary alias cleanup lane is no longer pending:
  - PR `#26` merged the retired worker/workstation boundary alias rejection
    dedupe on May 1, 2026
  - `pkg/config/factory_config_mapping.go` now keeps one retired-field
    inventory per boundary type and reuses it for the top-level object and its
    nested `definition` object
  - `pkg/config/factory_config_mapping_test.go` now covers top-level and nested
    alias rejection paths, including `definition.cron`
- the broader throttle customer ask remains open at the architecture level:
  - pause state still lives as dispatcher-owned runtime memory keyed by
    provider/model
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- there is not yet a new checked-in narrow cleanup lane ready outside the
  active PR set:
  - the last queued candidate in `pkg/config/factory_config_mapping.go` is now
    merged
  - the remaining customer-visible throttle redesign is broader than a single
    safe cleanup patch and still needs decomposition before queueing
  - the ignored idea residue under `factory/inputs/**` now contains multiple
    already-landed lanes, so filenames there cannot be used as pending-work
    evidence by themselves
- there is one new workspace-local narrow cleanup lane ready outside the active
  PR set:
  - `factory/inputs/idea/default/dedupe-generated-boundary-alias-rejection-coverage.md`
    queues a test-surface dedupe in `pkg/config`
  - the candidate keeps production behavior unchanged and only reduces
    duplicated retired-alias rejection coverage between
    `factory_config_mapping_test.go` and `openapi_factory_test.go`
  - that file set sits outside PRs `#20`, `#16`, and `#4`

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask is still too large for a
   safe direct jump from the current dispatcher-owned runtime policy.
2. open PRs `#20`, `#16`, and `#4` still occupy their respective file sets, so
   new cleanup work should stay outside those lanes.
3. any world model that still points to the retired-boundary alias dedupe as
   the next lane is stale now that PR `#26` is merged.

## theory of mind

- the checked-in meta model was stale because it still proposed the
  retired-boundary alias dedupe even though PR `#26` has already landed.
- `main` now contains the process/review contract alignment, work-request
  trace normalization, public-factory enum alias consolidation, worker
  build-args shim retirement, and retired-boundary alias dedupe, so none of
  those lanes should be re-queued.
- ignored `factory/inputs/**` residue can outlive the merge status of the lane
  it originally described, so merged PR history must win over both local
  residue and the previous checked-in meta view when deciding what is pending.
- the throttle ask is still the highest-value customer problem, but the live
  implementation spans dispatcher runtime pause state, runtime snapshots,
  dashboard observability, and transition-guard wiring, so it still needs a
  narrower design/decomposition step before queueing executable cleanup work.
- the right immediate maintainer action is to refresh the checked-in world
  model and avoid fabricating another narrow cleanup idea until a distinct,
  non-overlapping seam is confirmed.

## next best move

- update the checked-in meta world model and progress log now.
- do not queue another retired-boundary alias, worker build-args, enum-alias,
  trace-normalization, or process/review cleanup idea, because those lanes
  already landed on `main`.
- queue one new ignored cleanup idea for generated-boundary alias rejection
  coverage dedupe in `pkg/config`, because it is narrow, low-risk, and outside
  the active PR file sets.
- leave `factory/logs/meta/asks.md` unchanged for now; no ask is urgent and the
  broad throttle redesign still needs a narrower executable decomposition.
- use the next meta iteration to identify a fresh cleanup seam outside PRs
  `#20`, `#16`, and `#4` or to decompose the throttle ask into a smaller guard
  design lane.

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface.
- no ask is marked urgent as of May 1, 2026.
- the throttling ask is still active, but no checked-in change has yet moved
  pause enforcement into config-authored guards.
- the next customer-adjacent architectural step is still the future
  `INFERENCE_THROTTLE_GUARD` lane, but it should be queued only after it is
  decomposed into a narrow, non-overlapping request.
