# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `bd240ae` on May 1, 2026 in the local maintainer workspace
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
- the current GitHub lane state is now materially different from the previous
  checked-in world model:
  - open PR `#33` `prd-api-model-contract-cleanup`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#42` `retire-dispatcher-throttle-pause-map`
  - merged PR `#41` `dedupe-replay-factory-merge-helpers`
  - merged PR `#40` `dedupe-worker-event-exit-code-extraction`
  - merged PR `#39` `chaining-trace-ids`
  - merged PR `#38` `prd-current-factory-default-runtime-support`
  - merged PR `#37` `prd-cli-consumer-installation`
  - merged PR `#36` `retire-dispatch-result-hook-syncdispatch-cache`
  - merged PR `#35` `consolidate-dashboard-session-fallback-workitem-collectors`
  - merged PR `#34` `dedupe-list-work-legacy-pagination-fallback`
  - merged PR `#32` `shadcn-components-for-website`
  - merged PR `#31` `derive-throttle-windows-from-completed-dispatch-history`
  - merged PR `#29` `prd-goreleaser-release-pipeline`
  - merged PR `#28` `derive-throttle-windows-from-event-history`
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask remains the highest-value architecture ask,
  but its implementation posture changed again on `main`:
  - `pkg/factory/internal/throttle/windows.go` still owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` no longer stores the
    mutable `throttlePauses` map after merged PR `#42`; it now derives current
    active pauses from `snapshot.DispatchHistory` per tick and preserves the
    existing `TickResult` pause observability payload
  - there is still no checked-in `factory.guards` lowering path for
    `INFERENCE_THROTTLE_GUARD`
  - the remaining gap is now factory-level config, guard lowering, and
    transition ownership rather than dispatcher-local duplicate pause state
- one new narrow cleanup seam is queueable outside the remaining open PR file
  sets:
  - `pkg/cli/dashboard/dashboard.go` still owns two unexported fallback
    collectors, `worldViewFallbackCompletedWorkItems` and
    `worldViewFallbackFailedWorkItems`
  - both helpers already route through the same collector type and differ only
    in their terminal-work inclusion and missing-input fallback rules
  - that lane can stay inside `pkg/cli/dashboard/dashboard.go` and, if
    necessary, `pkg/cli/dashboard/dashboard_test.go`, so it stays off the
    current `#33` API/config contract lane and the `#30` functional test
    decomposition lane

## current blockers

1. the broad `INFERENCE_THROTTLE_GUARD` customer ask still spans config shape,
   guard lowering, and transition ownership, so it is still too large for a
   safe single lane
2. open PR `#33` occupies most of the active API/config/public-contract file
   sets that a guard-lowering lane would need
3. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new cleanup work should avoid functional test file moves for now
4. the previous checked-in world model was stale in five important ways:
   - it still described upstream `HEAD` as `f7efd5f`
   - it still treated PRs `#38` and `#37` as open even though both are now
     merged
   - it did not account for merged PR `#42`
   - it still treated the dispatcher throttle-pause-map cleanup as the next
     queueable lane even though that seam has already landed
   - it still described the throttle ask as blocked by open runtime-support
     lanes that are no longer open

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, but the
  remaining gap is now clearly the public/config/lowering layer:
  - dispatcher-local duplicate throttle state has already been removed on
    `main`
  - the remaining work is a later `factory.guards` /
    `INFERENCE_THROTTLE_GUARD` lane
- with PRs `#38` and `#37` now merged, the remaining live collision surface is
  much smaller than before:
  - `#33` owns API/config/public guard contract cleanup
  - `#30` owns functional test package decomposition
- because the open PR footprint is now narrow, the best available cleanup work
  is no longer throttle-internal or replay-internal; both of those recent
  narrow lanes already landed
- the best available queueable follow-up is now a low-risk dashboard
  simplification that removes duplicate fallback collector ownership without
  changing public contracts
- when a cleanup lane already exists as ignored local residue under
  `factory/inputs/**`, it may still be the correct next task; the maintainer
  loop should refresh the world model instead of forcing artificial queue churn
  or re-queuing freshly merged work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue already-landed lanes such as `#42`, `#41`, `#40`, `#38`,
  `#37`, `#36`, or `#35`
- queue one new ignored cleanup idea for the CLI dashboard fallback collector
  seam:
  - preserve current completed and failed work-item fallback behavior
  - collapse the duplicate helper ownership behind one parametrized internal
    collector path
  - avoid API/config/public-contract and `tests/functional/**` surfaces

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, but its remaining work is now narrower than the previous checked-in
  model described
- the quality and website-quality asks remain broader follow-on programs
  rather than the next narrow cleanup lane
- the next throttle follow-up should stay decomposed and should not overlap the
  already-open API/config contract lane in `#33`
