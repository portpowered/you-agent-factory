# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at
  `f7efd5f` on May 1, 2026
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
  - `factory/inputs/idea/default/retire-dispatcher-throttle-pause-map.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state on May 1, 2026 is:
  - open PR `#38` `prd-current-factory-default-runtime-support`
  - open PR `#37` `prd-cli-consumer-installation`
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#41` `dedupe-replay-factory-merge-helpers`
  - merged PR `#40` `dedupe-worker-event-exit-code-extraction`
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
    provider/model pause state via `throttlePauses`, still gates scheduling by
    dispatcher-owned runtime state, and still reconstructs that state from
    `snapshot.DispatchHistory`
  - there is still no checked-in `factory.guards` lowering path for
    `INFERENCE_THROTTLE_GUARD`
  - current open PR overlap around the ask is still broad:
    - `#37` touches throttle derivation-adjacent config, interfaces, and
      projections
    - `#33` touches guard/config/API contract surfaces
    - `#38` touches service-facing runtime support and API/service tests
    - `#30` touches the functional throttle coverage lane
    - none of those open PRs currently touch
      `pkg/factory/subsystems/subsystem_dispatcher.go`
- PR `#39` is now merged, so main advanced across shared replay, event-history,
  projection, API-contract, and UI timeline surfaces:
  - `pkg/factory/event_history.go` and `pkg/factory/projections/world_state.go`
    now carry chaining-trace context updates on `main`
  - `pkg/replay/event_reducer.go` now participates in the chaining-trace lane
  - `ui/src/state/factoryTimelineStore.ts` and its tests now consume the newer
    trace context shape
- two previously queued narrow cleanup seams have now landed on `main`:
  - PR `#41` merged the replay artifact merge-helper dedupe in
    `pkg/replay/event_artifact.go`
  - PR `#40` merged the worker event exit-code extraction dedupe in
    `pkg/workers/script.go`, `pkg/workers/recording_provider.go`, and related
    tests
- one new narrow cleanup seam is now queueable outside the active PR file
  sets:
  - `pkg/factory/subsystems/subsystem_dispatcher.go` still owns the mutable
    `throttlePauses` map even though active windows are already derivable from
    completed dispatch history plus `pkg/factory/internal/throttle/windows.go`
  - the dispatcher can likely derive current active pauses directly per tick,
    preserve current `TickResult` and snapshot observability payloads, and
    remove duplicate mutable pause ownership without changing public config
    contracts
  - that seam stays off the current open PR file sets if it is kept inside
    `pkg/factory/subsystems/subsystem_dispatcher.go`,
    `pkg/factory/subsystems/dispatcher_test.go`, and nearby internal throttle
    tests only

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, scheduler ownership, and observability, so it is still too
   large for a safe single lane
2. open PRs `#38`, `#37`, `#33`, and `#30` still occupy most of the active
   runtime-support, release/CLI/config, public-contract, and functional-test
   file sets
3. the previous checked-in world model was stale in four important ways:
   - it still described upstream `HEAD` as `f6e5ac6`
   - it did not account for merged PR `#41`
   - it still treated PR `#40` as open even though it merged on May 1, 2026
   - it still treated the replay merge-helper dedupe as the next queueable lane
     even though it already landed
4. open PRs `#37` and `#33` remain unusually wide for nominally focused lanes,
   so any new
   cleanup candidate still needs exact changed-file validation rather than
   package intuition
5. the highest-priority customer ask now has one attractive internal follow-up
   seam, but that lane still has to avoid adjacent config, projection, and
   service surfaces already occupied by `#37`, `#33`, `#38`, and `#30`

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly split into two layers:
  - an internal dispatcher-state simplification seam
  - a broader later config/lowering/guard-ownership lane
- because PRs `#33`, `#38`, and especially `#37` are wide lanes, new follow-up
  work should still avoid public boundaries and shared runtime/config surfaces
  rather than trying to parallelize into them
- the best available cleanup work right now is no longer replay-internal;
  `#41` already landed that seam
- the best available queueable follow-up now is an internal dispatcher cleanup
  that removes the mutable throttle-pause map while preserving the current
  observability payload shape
- when a cleanup lane already exists as ignored local residue under
  `factory/inputs/**`, it may still be the correct next task; the maintainer
  loop should refresh the world model instead of forcing artificial queue churn
  or re-queuing freshly merged work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue already-landed lanes such as `#41`, `#40`, `#36`, or `#35`
- queue one new ignored cleanup idea for the dispatcher throttle follow-up:
  preserve current pause observability behavior, but remove dispatcher-owned
  mutable `throttlePauses` state and derive active pauses directly from
  dispatch history plus the pure throttle-window helper
- avoid colliding with open PRs `#38`, `#37`, `#33`, and `#30`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026
- the throttling ask is still the most important architecture-level customer
  ask
- the quality and website-quality asks are now partially represented by live
  and freshly merged lanes, but they remain broader than the current narrow
  cleanup queue
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open public-model, runtime-support, or functional-test umbrella lanes
