# meta view

## world state

- after `git pull --ff-only origin main`, repository `main` and
  `origin/main` are both at `314d889` on May 2, 2026 in the local maintainer
  workspace
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
  - `factory/inputs/idea/default/factory-level-inference-throttle-guard.md`
  - `factory/inputs/idea/default/inline-batch-relation-duplicate-rejection.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/retire-dispatcher-throttle-pause-map.md`
  - `factory/inputs/idea/default/retire-duplicate-ui-script-copies.md`
  - `factory/inputs/idea/default/retire-legacy-throttle-fallback-after-authored-guard.md`
  - `factory/inputs/idea/default/retire-runtime-lookup-helper-duplication.md`
  - `factory/inputs/idea/default/retire-verbose-logger-export.md`
  - `factory/inputs/idea/default/unify-token-removal-bookkeeping.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
  - `factory/inputs/task/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/task/default/prd-functional-test-suite-decomposition.md`
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#48` `retire-legacy-throttle-fallback-after-authored-guard`
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#49` `retire-runtime-lookup-helper-duplication`
  - merged PR `#47` `retire-verbose-logger-export`
  - merged PR `#46` `factory-level-inference-throttle-guard`
  - merged PR `#45` `retire-duplicate-ui-script-copies`
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
  - merged PR `#33` `prd-api-model-contract-cleanup`
- the broad throttle customer ask remains the highest-value architecture ask,
  with authored-guard support already merged on `main` and the remaining
  authored-guard-only cleanup active in open PR `#48`:
  - merged PR `#46` added the root authored contract in
    `api/components/schemas/data-models/Factory.yaml`,
    `api/components/schemas/data-models/FactoryGuard.yaml`,
    `api/components/schemas/data-models/GuardType.yaml`,
    `api/openapi.yaml`, and generated API/UI client output
  - merged PR `#46` added the factory-level config/mapping/validation lane
    through `pkg/interfaces/factory_config.go`,
    `pkg/config/factory_config_mapping.go`,
    `pkg/config/openapi_factory.go`,
    `pkg/config/config_validator.go`,
    `pkg/config/public_factory_enums.go`, and
    `pkg/config/config_mapper.go`
  - merged PR `#46` lowered the authored guard into ordinary runtime guard
    evaluation through `pkg/petri/guard.go`,
    `pkg/petri/inference_throttle_guard.go`,
    `pkg/factory/scheduler/enablement.go`, and
    `pkg/factory/subsystems/subsystem_dispatcher.go`
  - merged PR `#46` verified the lane with targeted package/API/UI tests while
    still avoiding `tests/functional/**`
  - open PR `#48` now owns the explicit post-merge simplification follow-up:
    retire the legacy implicit throttle-policy fallback for factories that do
    not author `factory.guards`
  - the live fallback on `main` still lives in:
    - `pkg/factory/options.go` via `WithProviderThrottlePauseDuration`
    - `pkg/factory/runtime/factory.go` via dispatcher wiring from
      `ProviderThrottlePauseDuration`
    - `pkg/factory/subsystems/subsystem_dispatcher.go` via the
      `activeThrottlePauses` branch that falls back to
      `factorythrottle.DeriveActiveThrottlePauses(...)` when no authored
      inference-throttle guard exists
  - PR `#48` covers those files plus focused regression proof in
    `pkg/factory/subsystems/dispatcher_test.go`,
    `pkg/petri/inference_throttle_guard_test.go`,
    `pkg/testutil/testutil.go`, and
    `tests/functional_test/provider_error_smoke_test.go`; that last file sits
    outside the still-open `tests/functional/**` reorganization lane in `#30`
- the previously queued reserve cleanup changed again while `main` advanced:
  - merged PR `#49` already retired the duplicated first-non-nil helper loops
    in `pkg/interfaces/runtime_lookup.go`
  - the ignored local idea
    `factory/inputs/idea/default/retire-runtime-lookup-helper-duplication.md`
    is now solved residue rather than pending work
- direct code inspection also invalidated another ignored local reserve idea:
  - `pkg/api/server.go` still owns the handwritten tolerant `/work` wrapper
  - `pkg/api/handlers.go` on `main` no longer re-parses raw `maxResults`
    query text inside `ListWork`
  - the ignored local idea
    `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
    is stale because its problem statement is no longer true on `main`
- sidecar exploration plus direct validation found the next smaller
  non-colliding reserve cleanup in runtime token bookkeeping:
  - `pkg/petri/marking.go` owns `(*Marking).RemoveToken`
  - `pkg/factory/projections/world_state.go` owns
    `(*factoryWorldReducer).removeToken`
  - both implement the same "remove token and clean the place index when it
    becomes empty" responsibility in different runtime state models
  - this seam stays outside open PR `#48` and open PR `#30`

## current blockers

1. open PR `#48` occupies the exact remaining customer throttle cleanup lane,
   so that ask should not be re-queued while review is pending
2. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid those paths until that lane merges
3. the previous checked-in world model was stale again:
   - it still described `HEAD` as `45cefd2`
   - it still treated the runtime-lookup reserve seam as queueable even though
     merged PR `#49` already landed it on `main`
   - it did not capture that the local
     `dedupe-list-work-legacy-pagination-fallback` idea is stale because the
     duplicate parsing seam is already gone on `main`
4. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- after merged PR `#46`, the customer throttle ask moved from "introduce an
  authored guard contract" to "finish the simplification" by retiring the
  implicit fallback path; open PR `#48` is that follow-up lane
- the customer explicitly asked to replace the separate global-throttle logic
  with a factory-level guard and reduce special abstractions, so the correct
  maintainer action today is to track `#48` as the active lane rather than
  queueing another competing throttle request
- a safe parallel cleanup lane must still stay outside both `#48` and
  `tests/functional/**` while PR `#30` is open
- ignored local idea files are even less trustworthy than their filenames
  suggest; they can be stale because the seam already landed or because later
  mainline cleanup removed the underlying duplication before the idea was ever
  dispatched
- the smallest currently validated reserve hygiene lane is token-removal
  bookkeeping dedupe across `pkg/petri/marking.go` and
  `pkg/factory/projections/world_state.go`; it is still lower value than the
  active customer-owned throttle follow-up but remains safe background work

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue the already-open throttle fallback cleanup lane in `#48`
- queue one new ignored reserve cleanup idea for token-removal bookkeeping
  dedupe across `pkg/petri/marking.go` and
  `pkg/factory/projections/world_state.go`
- keep any new reserve work out of both the `#48` file set and
  `tests/functional/**` while PR `#30` remains open
- treat the active customer throttle follow-up as review/merge work now, not
  as a fresh backlog item

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, with authored-guard support merged on `main` and the remaining fallback
  retirement now active in open PR `#48`
- the quality and website-quality asks remain broader follow-on programs, but
  they are still subordinate to the throttling outage-prevention ask
- the next maintainer action on that ask is to review and merge `#48` rather
  than creating another throttle request, while reserve hygiene can continue in
  non-colliding seams such as token-removal bookkeeping dedupe in
  `pkg/petri/marking.go` and `pkg/factory/projections/world_state.go`
