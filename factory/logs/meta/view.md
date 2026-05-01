# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` were both at
  `7aa73c7` on May 1, 2026 before this meta refresh commit
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
  - `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/dedupe-replay-factory-merge-helpers.md`
  - `factory/inputs/idea/default/dedupe-worker-event-exit-code-extraction.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#40` `dedupe-worker-event-exit-code-extraction`
  - open PR `#39` `chaining-trace-ids`
  - open PR `#38` `prd-current-factory-default-runtime-support`
  - open PR `#37` `prd-cli-consumer-installation`
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#36` `retire-dispatch-result-hook-syncdispatch-cache`
  - merged PR `#35` `consolidate-dashboard-session-fallback-workitem-collectors`
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
- the public/model and runtime-support asks are now consuming most shared
  boundary surfaces:
  - PR `#33` reaches `api/`, `pkg/config/`, `pkg/interfaces/`, docs,
    examples, replay fixtures, and generated artifacts
  - PR `#38` widens the active lane into named-factory API, generated client,
    service, and functional-test surfaces
  - PR `#37` is broad enough to occupy release, CLI, config, factory,
    projection, and many contract-test file sets at once
- the previously queued narrow cleanup seams have advanced again:
  - PR `#35` is merged, so the dashboard-only fallback-collector
    simplification is complete on `main`
  - PR `#36` is merged, so the `pkg/factory/runtime/dispatch_result_hook.go`
    `syncDispatch` cache retirement is complete on `main`
  - PR `#40` is now open for the worker event `ExitCode` extraction cleanup,
    so that lane is no longer available for re-queueing
- one newer narrow cleanup seam is queueable outside the active PR file sets:
  - `pkg/replay/event_artifact.go` owns both `mergeGeneratedWorkers` and
    `mergeGeneratedWorkstations`
  - both helpers perform the same copy-index-sort-merge flow over generated
    replay factory slices, varying only by concrete type and conversion helper
  - this duplication is internal replay assembly logic, so it is a behavior-safe
    simplification candidate that does not require public contract changes

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is too large
   for a safe single lane
2. open PRs `#40`, `#39`, `#38`, `#37`, `#33`, and `#30` now occupy most of
   the active worker-event, replay-trace, runtime-support, release/CLI/config,
   public-contract, and functional-test file sets
3. the previous checked-in world model was stale in four important ways:
   - it still described upstream `HEAD` as `8c84704`
   - it did not include newly merged PRs `#35` and `#36`
   - it did not include newly open PRs `#37`, `#38`, `#39`, and `#40`
   - it still recommended the worker exit-code cleanup as the next lane even
     though that lane is already in review
4. the broad public/model ask is active in PR `#33`, and the broad
   runtime-support lane is active in PR `#38`, so the next cleanup lane should
   avoid `pkg/api/`, `pkg/config/`, `pkg/interfaces/`, `pkg/service/`, and
   nearby public-contract surfaces until those lanes settle
5. PR `#37` is unusually wide for a nominally focused lane, so any new cleanup
   candidate must be validated against exact changed-file ownership rather than
   package intuition alone

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly on the public/config/lowering side rather than
  on throttle timing reconstruction
- because PRs `#33`, `#38`, and especially `#37` are wide lanes, new
  follow-up work should avoid public boundaries and shared runtime/config
  surfaces rather than trying to parallelize into them
- the website-quality lane advanced materially with merged PR `#32`, so the
  next cleanup task no longer needs to spend the one available sidecar slot on
  shared-component cleanup
- because PR `#40` already picked up the worker event-emission dedupe seam, the
  next sidecar task must move again instead of re-queueing stale local residue
- the best available cleanup work right now is a narrow replay-internal dedupe
  in `pkg/replay/event_artifact.go` that consolidates duplicate generated
  factory merge helper ownership without changing public contracts or replay
  payload shape
- a sidecar candidate that looked valid at the start of the pass can become
  stale during the same pass, so the exact open PR file sets should be
  revalidated after sidecar exploration and before queuing new work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- queue one new ignored cleanup idea for replay artifact assembly:
  preserve current replay factory payload behavior, but consolidate duplicate
  merge-helper ownership across `mergeGeneratedWorkers` and
  `mergeGeneratedWorkstations` in `pkg/replay/event_artifact.go`
- keep the already-open worker exit-code, chaining-trace, runtime-support, and
  public-model lanes as active local context rather than re-queueing them
- avoid re-queuing already-landed lanes and avoid colliding with open PRs
  `#40`, `#39`, `#38`, `#37`, `#33`, and `#30`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the quality and website-quality asks are now partially represented by live
  and freshly merged lanes, but they remain broader than the current narrow
  cleanup queue
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open public-model, runtime-support, or release/CLI umbrella lanes
