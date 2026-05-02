# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `79b0552` on May 1, 2026 in the local maintainer workspace
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
  - open PR `#46` `factory-level-inference-throttle-guard`
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
  and it is now actively being implemented in open PR `#46`:
  - PR `#46` touches the root authored contract in
    `api/components/schemas/data-models/Factory.yaml`,
    `api/components/schemas/data-models/FactoryGuard.yaml`,
    `api/components/schemas/data-models/GuardType.yaml`,
    `api/openapi.yaml`, and generated API/UI client output
  - PR `#46` adds the factory-level config/mapping/validation lane through
    `pkg/interfaces/factory_config.go`,
    `pkg/config/factory_config_mapping.go`,
    `pkg/config/openapi_factory.go`,
    `pkg/config/config_validator.go`,
    `pkg/config/public_factory_enums.go`, and
    `pkg/config/config_mapper.go`
  - PR `#46` lowers the authored guard into ordinary runtime guard evaluation
    through `pkg/petri/guard.go`,
    `pkg/petri/inference_throttle_guard.go`,
    `pkg/factory/scheduler/enablement.go`, and
    `pkg/factory/subsystems/subsystem_dispatcher.go`
  - PR `#46` verifies the lane with targeted package/API/UI tests while still
    avoiding `tests/functional/**`
  - the highest-value remaining gap is no longer "start the throttle lane";
    it is "finish or follow up after PR `#46`" because the branch still keeps
    the legacy fallback throttle path for factories that do not author
    `factory.guards`
  - the legacy fallback still lives around `pkg/factory/options.go`,
    `pkg/factory/runtime/factory.go`, and the fallback branch in
    `pkg/factory/subsystems/subsystem_dispatcher.go`
- the best narrow fallback cleanup remains outside recent PR churn:
  - `pkg/logging/logger.go` still owns the exported `VerboseLogger` interface
  - sidecar inspection confirmed it has no callers outside that file, so it is
    still a real dead-code seam and a safe non-colliding queue candidate while
    PR `#46` is open

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid those paths until that lane merges
2. open PR `#46` now occupies the throttle-guard contract, lowering, and
   runtime lane, so new work should avoid its file set until that lane merges
3. the previous checked-in world model was stale again:
   - it still described upstream `HEAD` as `9efe9ca`
   - it still treated the throttle ask as only a queued idea instead of an
     active open PR
   - it did not capture the exact file set now occupied by PR `#46` or the
     remaining legacy fallback risk after that lane
4. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- the highest-value live customer problem is still global throttling, but the
  state of that problem changed materially on May 1, 2026:
  - the authored `factory.guards` /
    `INFERENCE_THROTTLE_GUARD` lane is now active in PR `#46`
  - the maintainer should not queue overlapping follow-up work in the same
    files until that lane lands
  - the likely next throttle follow-up after merge is not contract authoring;
    it is retiring the legacy fallback abstraction so authored guards become
    the only throttle-policy path
- a safe cleanup lane should therefore stay outside PR `#46` and `#30` while
  the customer ask is in flight
- the unused exported `pkg/logging.VerboseLogger` seam is currently the best
  narrow non-colliding cleanup because it removes dead public surface without
  widening into runtime behavior or active PR files

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the priority order is still
  correct
- do not re-queue the throttle-guard implementation lane while PR `#46` is
  open
- keep the remaining throttle follow-up framed as a post-merge cleanup:
  retire the legacy fallback throttle abstraction once the authored guard lane
  lands cleanly
- queue one separate ignored cleanup idea for the unused exported
  `pkg/logging.VerboseLogger` seam so repository hygiene keeps moving without
  colliding with PR `#46` or `#30`

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 1, 2026 in the maintainer workspace
- the throttling ask is still the most important architecture-level customer
  ask, and it is now actively represented by open PR `#46`
- the quality and website-quality asks remain broader follow-on programs, but
  they are still subordinate to the throttling outage-prevention ask
- the next throttle follow-up should wait until PR `#46` merges and should
  then target removal of the legacy fallback abstraction while continuing to
  avoid the still-open functional-test decomposition lane in `#30`
