# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at
  `f6e5ac6` on May 1, 2026
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
  - open PR `#38` `prd-current-factory-default-runtime-support`
  - open PR `#37` `prd-cli-consumer-installation`
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#39` `chaining-trace-ids`
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
- the broad throttle customer ask remains open, and its posture is unchanged in
  the ways that matter for follow-up decomposition:
  - `pkg/factory/internal/throttle/windows.go` owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns mutable
    provider/model pause state and still gates scheduling by dispatcher-owned
    runtime state
  - there is still no checked-in `factory.guards` lowering path for
    `INFERENCE_THROTTLE_GUARD`
- PR `#39` is now merged, so main advanced across shared replay, event-history,
  projection, API-contract, and UI timeline surfaces:
  - `pkg/factory/event_history.go` and `pkg/factory/projections/world_state.go`
    now carry chaining-trace context updates on `main`
  - `pkg/replay/event_reducer.go` now participates in the chaining-trace lane
  - `ui/src/state/factoryTimelineStore.ts` and its tests now consume the newer
    trace context shape
- one narrow cleanup seam remains queueable outside the active PR file sets:
  - `pkg/replay/event_artifact.go` still owns both `mergeGeneratedWorkers` and
    `mergeGeneratedWorkstations`
  - both helpers still perform the same copy-index-sort-merge flow over
    generated replay factory slices, varying only by concrete type and
    conversion helper
  - open PRs `#40`, `#38`, `#37`, `#33`, and `#30` do not currently touch
    `pkg/replay/event_artifact.go`, `pkg/replay/artifact_test.go`, or
    `pkg/replay/event_artifact_test.go`
- a sidecar explorer also found a smaller cleanup seam in
  `pkg/replay/event_reducer.go`:
  - `replayMetadataValue` is a single-use wrapper around `stringValue`
  - however PR `#39` just merged into that same reducer file, so that seam is
    lower priority than the untouched replay artifact helper dedupe

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is too large
   for a safe single lane
2. open PRs `#40`, `#38`, `#37`, `#33`, and `#30` still occupy most of the
   active worker-event, runtime-support, release/CLI/config, public-contract,
   and functional-test file sets
3. the previous checked-in world model was stale in three important ways:
   - it still described upstream `HEAD` as `7aa73c7`
   - it still treated PR `#39` as open even though it merged on May 1, 2026
   - it did not account for the newly landed chaining-trace changes on replay,
     event-history, projection, and UI timeline surfaces
4. PR `#37` remains unusually wide for a nominally focused lane, so any new
   cleanup candidate still needs exact changed-file validation rather than
   package intuition
5. although PR `#39` is merged, it raised the cost of touching nearby replay
   reducer and trace-projection files immediately afterward, so the next narrow
   lane should stay inside replay artifact assembly rather than widening into
   fresh trace consumers

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is still on the public/config/lowering side rather than on
  throttle timing reconstruction
- because PRs `#33`, `#38`, and especially `#37` are wide lanes, new
  follow-up work should avoid public boundaries and shared runtime/config
  surfaces rather than trying to parallelize into them
- PR `#39` landing removed one active lane but also advanced main across shared
  replay/event-history/world-state surfaces, so the next cleanup lane should
  avoid replay reducer and trace-projection ownership even though those files
  are no longer in an open PR
- the best available cleanup work right now is still a narrow replay-internal
  dedupe in `pkg/replay/event_artifact.go` that consolidates duplicate
  generated factory merge helper ownership without changing public contracts or
  replay payload shape
- when a cleanup lane already exists as ignored local residue under
  `factory/inputs/**`, it may still be the correct next task; the maintainer
  loop should refresh the world model instead of forcing artificial queue churn

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- keep the existing ignored cleanup idea for replay artifact assembly:
  preserve current replay factory payload behavior, but consolidate duplicate
  merge-helper ownership across `mergeGeneratedWorkers` and
  `mergeGeneratedWorkstations` in `pkg/replay/event_artifact.go`
- avoid re-queuing already-landed lanes and avoid colliding with open PRs
  `#40`, `#38`, `#37`, `#33`, and `#30`

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
