# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `9efe9ca` on May 1, 2026 in the local maintainer workspace
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
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
  - `factory/inputs/task/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/task/default/prd-functional-test-suite-decomposition.md`
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#30` `prd-functional-test-suite-decomposition`
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
- the broad throttle customer ask remains the highest-value architecture ask,
  and the remaining gap is still factory-level guard contract plus lowering:
  - `pkg/factory/internal/throttle/windows.go` owns the pure helper that
    derives active provider/model throttle windows from event history, pause
    duration, and explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` derives current active
    pauses from `snapshot.DispatchHistory` instead of storing mutable pause
    state
  - `api/components/schemas/data-models/Factory.yaml` and generated OpenAPI
    output still have no top-level `guards` property on `Factory`
  - `pkg/interfaces/factory_config.go` still has no factory-level `Guards`
    field, no `INFERENCE_THROTTLE_GUARD` enum value, and no
    `InferenceThrottleGuardConfig`
  - `pkg/config/factory_config_mapping.go`, `pkg/config/openapi_factory.go`,
    and `pkg/config/config_validator.go` still only understand workstation and
    per-input guards
  - `pkg/config/config_mapper.go` is still the next lowering seam if the first
    implementation lane carries the guard all the way into runtime transition
    evaluation
- the best narrow fallback cleanup remains outside recent PR churn:
  - `pkg/logging/logger.go` still owns the exported `VerboseLogger` interface
  - sidecar inspection confirmed it has no callers outside that file, so it is
    still a real dead-code seam if the customer throttle lane blocks again

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid those paths until that lane merges
2. the `INFERENCE_THROTTLE_GUARD` customer ask still spans schema, public
   config, validator, mapping, layout round-trip, and runtime lowering, so it
   should stay one focused customer-ask lane rather than being split into
   unrelated cleanup work
3. the previous checked-in world model was stale again:
   - it still described upstream `HEAD` as `7b006fe`
   - it did not list the local ignored
     `factory/inputs/idea/default/factory-level-inference-throttle-guard.md`
     residue that now exists in this workspace
   - it did not capture the exact authoring file set now known for the
     throttle lane
4. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- the highest-value live customer problem is still global throttling, and the
  remaining gap is now clearly the factory-level guard authoring and lowering
  contract:
  - dispatcher-local mutable throttle state is already gone on `main`
  - the next useful lane is `factory.guards` /
    `INFERENCE_THROTTLE_GUARD`, not another dispatcher cleanup
  - the first cut can safely avoid `tests/functional/**` while `#30` is open
    by leaning on contract, mapping, validator, layout, and targeted runtime
    tests elsewhere
- the current local throttle idea should be refreshed, not duplicated:
  - the ignored file already exists at
    `factory/inputs/idea/default/factory-level-inference-throttle-guard.md`
  - the right maintainer move is to keep that one draft aligned with current
    `main`
- low-risk dead-code cleanups still exist, but they are subordinate to the
  customer throttle ask until that lane blocks again

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- refresh the existing ignored throttle idea file instead of creating a second
  copy of the same lane
- keep the first throttle lane concentrated in:
  - `pkg/interfaces/factory_config.go`
  - `pkg/config/public_factory_enums.go`
  - `api/openapi.yaml`
  - `pkg/api/generated/server.gen.go`
  - `pkg/config/factory_config_mapping.go`
  - `pkg/config/openapi_factory.go`
  - `pkg/config/layout.go`
  - `pkg/config/config_validator.go`
  - the corresponding contract and mapper tests outside `tests/functional/**`
- treat `pkg/config/config_mapper.go` as the next required seam when the lane
  moves from authoring and round-trip support into actual runtime transition
  lowering
- keep the unused `pkg/logging.VerboseLogger` export-retirement seam in
  reserve as the next fallback cleanup if the throttle lane becomes blocked

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, and it remains ready for a first implementation lane because the broad
  API/config contract cleanup is already merged
- the quality and website-quality asks remain broader follow-on programs, but
  they are still subordinate to the throttling outage-prevention ask
- the next throttle follow-up should stay decomposed and should avoid the
  still-open functional-test decomposition lane in `#30`
