# meta view

## world state

- after `git pull --ff-only origin main`, repository `main` and `origin/main`
  are both at `7b006fe` on May 1, 2026 in the local maintainer workspace
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
  - `factory/inputs/idea/default/collapse-dashboard-fallback-work-item-collectors.md`
  - `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/dedupe-replay-factory-merge-helpers.md`
  - `factory/inputs/idea/default/dedupe-worker-event-exit-code-extraction.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/inline-batch-relation-duplicate-rejection.md`
  - `factory/inputs/idea/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-functional-test-suite-decomposition.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/retire-duplicate-ui-script-copies.md`
  - `factory/inputs/idea/default/retire-dispatcher-throttle-pause-map.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
- the current GitHub lane state changed again after the previous checked-in
  world model:
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#45` `retire-duplicate-ui-script-copies`
  - merged PR `#33` `prd-api-model-contract-cleanup`
  - merged PR `#44` `inline-batch-relation-duplicate-rejection`
  - merged PR `#43` `collapse-dashboard-fallback-work-item-collectors`
  - merged PR `#42` `retire-dispatcher-throttle-pause-map`
  - merged PR `#41` `dedupe-replay-factory-merge-helpers`
  - merged PR `#40` `dedupe-worker-event-exit-code-extraction`
  - merged PR `#39` `chaining-trace-ids`
  - merged PR `#38` `prd-current-factory-default-runtime-support`
  - merged PR `#37` `prd-cli-consumer-installation`
  - merged PR `#36` `retire-dispatch-result-hook-syncdispatch-cache`
  - merged PR `#35` `consolidate-dashboard-session-fallback-workitem-collectors`
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask remains the highest-value architecture ask,
  and its posture changed materially after merged PR `#33`:
  - `pkg/factory/internal/throttle/windows.go` owns the pure helper that
    derives active provider/model throttle windows from normalized failure
    history, pause duration, and explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` derives current active
    pauses from `snapshot.DispatchHistory` instead of storing mutable pause
    state
  - `api/components/schemas/data-models/Factory.yaml` still has no top-level
    `guards` property
  - `pkg/interfaces/factory_config.go` still has no factory-level guard
    surface
  - `pkg/config/config_validator.go` and `pkg/config/config_mapper.go` still
    only understand workstation-level and per-input guard lowering
  - there is still no checked-in `factory.guards` lowering path for
    `INFERENCE_THROTTLE_GUARD`
- the previous next cleanup lane has already landed on `main`:
  - `pkg/cli/dashboard/dashboard.go` no longer owns separate completed and
    failed fallback wrapper helpers after merged PR `#43`
  - `pkg/factory/work_request.go` no longer owns the standalone
    `rejectDuplicateBatchRelation` helper after merged PR `#44`
- the previous UI dead-code seam is now complete on `main`:
  - merged PR `#45` removed the tracked duplicate `copy` scripts under
    `ui/scripts/`
  - the canonical UI script entrypoints remain the non-`copy` paths referenced
    by `ui/package.json`
- one new customer-ask implementation lane is now queueable outside the only
  remaining open PR file set:
  - add top-level `factory.guards` support for
    `INFERENCE_THROTTLE_GUARD`
  - keep the implementation concentrated in `api/`, `pkg/interfaces/`,
    `pkg/config/`, and pure throttle/guard lowering seams
  - avoid `tests/functional/**` while PR `#30` is open by preferring
    contract, unit, package-integration, and stress coverage for the first cut

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid those paths until that lane merges
2. the broad `INFERENCE_THROTTLE_GUARD` ask still spans schema, public config,
   validation, mapper lowering, and runtime guard evaluation, so it should
   still be dispatched as one narrowly-scoped customer-ask lane rather than
   attempted ad hoc
3. the previous checked-in world model is now stale in five important ways:
   - it still described upstream `HEAD` as `368d182`
   - it still treated PR `#33` as open even though it is merged
   - it did not account for merged PR `#45`
   - it still treated the UI duplicate-script cleanup as merely queueable even
     though that seam has already landed on `main`
   - it did not account for the now-unblocked throttle-guard contract lane
4. workspace-local ignored residue can drift independently of `main`:
   - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
     still exists locally even though PR `#36` already removed the
     `syncDispatch` cache from `pkg/factory/runtime/dispatch_result_hook.go`

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  is changing quickly enough that the checked-in world model can drift within
  hours
- the highest-value live customer problem is still global throttling, and the
  remaining gap is now unambiguously the factory-level guard contract and
  lowering layer:
  - dispatcher-local duplicate throttle state has already been removed on
    `main`
  - merged PR `#33` standardized the guard schema surface into shared `Guard`
    and `GuardType` models
  - the next useful lane is adding `factory.guards` /
    `INFERENCE_THROTTLE_GUARD`, not another dispatcher-local cleanup
- with PR `#45` merged, the best available follow-up is no longer the UI
  duplicate-script cleanup; that dead-code seam is complete on `main`
- the remaining live collision surface is now very narrow:
  - `#30` owns `tests/functional/**` decomposition
  - backend/config/runtime files outside that tree are no longer blocked by an
    open API/config contract PR
- the first throttle-guard lane should be scoped to avoid `tests/functional/**`
  while `#30` is open and should rely on package-level, contract, stress, and
  targeted runtime tests instead
- when a cleanup lane already exists as ignored local residue under
  `factory/inputs/**`, it may already be merged on `main`; the maintainer loop
  should refresh the world model instead of re-queuing recently landed work
- low-risk narrow cleanups still exist outside the customer-ask lane; the
  current best fallback seam is the unused exported `pkg/logging.VerboseLogger`
  surface, which appears local to `pkg/logging/**` and outside recent PR churn

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue already-landed lanes such as `#45`, `#44`, `#43`, `#42`,
  `#41`, `#40`, `#38`, `#37`, `#36`, or `#35`
- queue one new ignored customer-ask idea for the now-unblocked throttle lane:
  - add top-level `factory.guards`
  - add `INFERENCE_THROTTLE_GUARD` plus
    `InferenceThrottleGuardConfig`
  - lower that config into ordinary transition guards backed by event-history
    throttle evaluation instead of dispatcher-owned mutable state
  - avoid `tests/functional/**` while `#30` is open
- keep the `pkg/logging.VerboseLogger` export-retirement seam as the next
  fallback cleanup if the throttle lane becomes blocked again

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, and it is now ready for a first implementation lane because the broad
  API/config contract cleanup has already merged
- the quality and website-quality asks remain broader follow-on programs, but
  they are still subordinate to the throttling outage-prevention ask
- the next throttle follow-up should stay decomposed and should avoid the
  still-open functional-test decomposition lane in `#30`
