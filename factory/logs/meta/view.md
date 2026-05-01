# meta view

## world state

- repository `HEAD` is `24a6463` on `main` on May 1, 2026, and
  `origin/main` is the same commit after `git pull`
- the canonical checked-in maintainer backlog is still
  `factory/logs/meta/asks.md`; no item in that file is marked urgent
- the checked-in workflow inboxes still contain only tracked `.gitkeep`
  sentinels:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- the workspace-local `factory/inputs/**` surface still has ignored residue
  beyond the tracked sentinels, so those files remain local context only and
  not checked-in workflow truth:
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - open PR `#20` `test-cleanup`
  - open PR `#16` `dedupe-root-factory-artifact-contract-entries`
  - open PR `#4` `standardize-contract-guard-skip-policy`
  - merged PR `#34` `dedupe-list-work-legacy-pagination-fallback`
  - merged PR `#32` `shadcn-components-for-website`
  - merged PR `#31` `derive-throttle-windows-from-completed-dispatch-history`
  - merged PR `#29` `prd-goreleaser-release-pipeline`
  - merged PR `#28` `derive-throttle-windows-from-event-history`
  - merged PR `#27` `dedupe-generated-boundary-alias-rejection-coverage`
  - merged PR `#26` `dedupe-retired-boundary-alias-rejection-tables`
  - merged PR `#25` `retire-scriptwrap-build-args-shim`
  - merged PR `#24` `consolidate-public-factory-enum-alias-ownership`
  - merged PR `#23` `centralize-work-request-trace-normalization`
  - merged PR `#22` `align-process-review-loop-contract`
  - merged PR `#21` `dedupe-dispatcher-throttle-pause-filter`
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask remains open, but the repository posture is
  more decomposed than the previous checked-in view captured:
  - `pkg/factory/internal/throttle/windows.go` now owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and an explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` now reconstructs throttle
    failure history from `snapshot.DispatchHistory` using exact
    `interfaces.CompletedDispatch.EndTime` values
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns the mutable
    runtime pause map and still gates scheduling by dispatcher-owned state
  - the ask for config-authored `factory.guards` with
    `INFERENCE_THROTTLE_GUARD` is still not implemented on `main`
- the release-pipeline lane is fully landed on `main`, and the small `/work`
  pagination cleanup lane is now landed too:
  - PR `#29` added `.goreleaser.yml`, release workflows, release smoke
    commands, and release smoke fixtures
  - PR `#34` removed the duplicate tolerant `maxResults` fallback from
    `pkg/api/handlers.go` while preserving the handwritten `/work`
    compatibility wrapper in `pkg/api/server.go`
- the public/model cleanup and website quality asks are now actively in flight
  through open/just-landed lanes:
  - PR `#33` is the current broad API model contract lane and already reaches
    `api/`, `pkg/config/`, `pkg/interfaces/`, `pkg/api/`, docs, examples,
    replay fixtures, and generated artifacts
  - PR `#32` just landed the current website component/system lane and reaches
    shared UI primitives, feature components, stories, tests, styles, and
    built assets
- one narrower cleanup seam is queueable outside the active PR file sets:
  - `pkg/cli/dashboard/dashboard.go` still computes completed and failed
    session work-item fallbacks through two very similar single-caller helpers:
    `worldViewFallbackCompletedWorkItems` and
    `worldViewFallbackFailedWorkItems`
  - both helpers only derive dashboard-facing work refs from
    `session.DispatchHistory`
  - the broader `pkg/factory/runtime/dispatch_result_hook.go` `syncDispatch`
    cleanup seam is still valid, but it touches live runtime dispatch behavior
    and is no longer the best next lane compared with the dashboard-only
    simplification

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is too large
   for a safe single lane
2. open PRs `#33`, `#30`, `#20`, `#16`, and `#4` now occupy most of the
   public contract, functional-test, artifact-contract, and guard-policy file
   sets
3. the previous checked-in world model was stale in three important ways:
   - it still described `HEAD` as `25580f5`
   - it still treated PR `#32` as open even though it is merged
   - it still described the worktree as dirty even though it is clean now
4. the broad public/model ask is active in PR `#33`, so the next cleanup lane
   should avoid `pkg/api/`, `pkg/config/`, `pkg/interfaces/`, and nearby
   public-contract surfaces until that lane settles

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly on the public/config/lowering side rather than
  on throttle timing reconstruction
- because PR `#33` is a wide public-model lane and the functional/artifact
  lanes are also active, new follow-up work should avoid those surfaces rather
  than trying to parallelize into them
- the website-quality lane advanced materially with merged PR `#32`, so the
  next cleanup task no longer needs to spend the one available sidecar slot on
  shared-component cleanup
- the best available cleanup work right now is a narrow dashboard-only
  simplification that reduces duplicate fallback collection logic without
  changing public contracts or scheduler/runtime behavior
- a sidecar candidate that looks package-local can still collide with an open
  umbrella PR, so file-set checks must include all active PRs before queuing
  new work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new ignored cleanup idea for `pkg/cli/dashboard/dashboard.go`:
  preserve current dashboard session output, but consolidate the duplicate
  `DispatchHistory` fallback collectors for completed and failed work items
- keep the older ignored `syncDispatch` runtime idea as historical local
  context rather than the next recommended lane
- avoid re-queuing already-landed lanes and avoid colliding with open PRs
  `#33`, `#30`, `#20`, `#16`, and `#4`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the quality and website-quality asks are now partially represented by live
  and freshly merged lanes, but they remain broader than the current narrow
  cleanup queue
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open public-model contract lane
