# meta view

## world state

- after `git pull --ff-only origin main`, repository `main` and `origin/main`
  are both at `9bb148e` on May 2, 2026 in the local maintainer workspace
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
  - `factory/inputs/idea/default/retire-verbose-logger-export.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
  - `factory/inputs/task/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/task/default/prd-functional-test-suite-decomposition.md`
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#30` `prd-functional-test-suite-decomposition`
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
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask has now been implemented through the
  authored-guard path, and the follow-up cleanup lane is on this branch:
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
  - this branch retires the old implicit fallback so authored
    `INFERENCE_THROTTLE_GUARD` definitions are the only supported
    throttle-policy path
  - the branch removes the old runtime seam from `pkg/factory/options.go`
    and `pkg/factory/runtime/factory.go`, removes the dispatcher no-guard
    fallback in `pkg/factory/subsystems/subsystem_dispatcher.go`, and adds
    focused package-owned regression coverage in `pkg/factory/...` and
    `pkg/petri/...`
- the previously queued dead-surface fallback cleanup is now already complete:
  - merged PR `#47` retired the exported `pkg/logging.VerboseLogger` surface
- sidecar exploration found a smaller non-colliding helper cleanup in
  `pkg/factory/workstationconfig/runtime_lookup.go`, but it is lower value than
  retiring the still-live legacy throttle abstraction

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid those paths until that lane merges
2. the previous checked-in world model was stale again:
   - it still described upstream `HEAD` as `79b0552`
   - it still treated the throttle lane as an open PR instead of a merged
     implementation
   - it still treated the exported `VerboseLogger` seam as queueable even
     though merged PR `#47` already removed it
  - it did not capture that the authored-guard-only cleanup is now in
    progress on its own branch
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- after merged PR `#46`, the customer throttle ask moved from "introduce an
  authored guard contract" to "finish the simplification" by retiring the
  implicit fallback path; this branch is that follow-up lane
- the customer explicitly asked to replace the separate global-throttle logic
  with a factory-level guard and reduce special abstractions, so the current
  branch should converge on authored guards as the only supported throttle
  policy without leaving compatibility seams behind
- a safe cleanup lane must still stay outside `tests/functional/**` while
  PR `#30` is open, which makes focused package/runtime coverage the right
  first follow-up proof shape
- the tiny isolated helper cleanup in
  `pkg/factory/workstationconfig/runtime_lookup.go` remains a valid reserve
  option, but it is subordinate to the remaining customer-owned throttle
  simplification

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue the already-merged authored throttle-guard lane
- do not queue a new cleanup idea for the legacy throttle fallback path;
  that work is already in progress on this branch
- keep that follow-up out of `tests/functional/**` while PR `#30` remains open
- treat the tiny `pkg/factory/workstationconfig/runtime_lookup.go` helper seam
  as reserve hygiene only if the throttle cleanup becomes blocked

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, and the first authored-guard implementation lane is now merged on
  `main`
- the quality and website-quality asks remain broader follow-on programs, but
  they are still subordinate to the throttling outage-prevention ask
- the next throttle follow-up after this branch should focus on any residual
  observability or test-decomposition cleanup rather than reintroducing
  implicit fallback abstractions
