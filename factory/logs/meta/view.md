# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `913f007` on May 2, 2026 in the local maintainer workspace
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
  - `factory/inputs/idea/default/inline-workstation-request-projection-fallback-helpers.md`
  - `factory/inputs/idea/default/prd-cli-consumer-installation.md`
  - `factory/inputs/idea/default/prd-current-factory-default-runtime-support.md`
  - `factory/inputs/idea/default/prd-goreleaser-release-pipeline.md`
  - `factory/inputs/idea/default/retire-dispatcher-throttle-pause-map.md`
  - `factory/inputs/idea/default/retire-duplicate-ui-script-copies.md`
  - `factory/inputs/idea/default/retire-legacy-throttle-fallback-after-authored-guard.md`
  - `factory/inputs/idea/default/retire-legacy-workstation-timeout-alias-fallback.md`
  - `factory/inputs/idea/default/retire-runtime-lookup-helper-duplication.md`
  - `factory/inputs/idea/default/retire-verbose-logger-export.md`
  - `factory/inputs/idea/default/unify-token-removal-bookkeeping.md`
  - `factory/inputs/plan/default/retire-dispatch-result-hook-syncdispatch-cache.md`
  - `factory/inputs/task/default/prd-api-model-contract-cleanup.md`
  - `factory/inputs/task/default/prd-functional-test-suite-decomposition.md`
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#51` `retire-legacy-workstation-timeout-alias-fallback`
  - merged PR `#50` `unify-token-removal-bookkeeping`
  - merged PR `#49` `retire-runtime-lookup-helper-duplication`
  - merged PR `#48` `retire-legacy-throttle-fallback-after-authored-guard`
  - merged PR `#47` `retire-verbose-logger-export`
  - merged PR `#46` `factory-level-inference-throttle-guard`
  - merged PR `#45` `retire-duplicate-ui-script-copies`
  - merged PR `#44` `inline-batch-relation-duplicate-rejection`
- the worktree is currently clean even though ignored local workflow-input
  residue remains under `factory/inputs/**`
- the broad throttle customer ask is now implemented on `main` rather than
  merely in-flight:
  - merged PR `#46` added authored factory-level guard support through
    `api/components/schemas/data-models/Factory.yaml`,
    `api/components/schemas/data-models/FactoryGuard.yaml`,
    `api/components/schemas/data-models/GuardType.yaml`,
    `api/openapi.yaml`,
    `pkg/interfaces/factory_config.go`,
    `pkg/config/factory_config_mapping.go`,
    `pkg/config/openapi_factory.go`,
    `pkg/config/config_validator.go`,
    `pkg/config/public_factory_enums.go`,
    `pkg/config/config_mapper.go`,
    `pkg/petri/guard.go`,
    `pkg/petri/inference_throttle_guard.go`,
    `pkg/factory/scheduler/enablement.go`, and
    `pkg/factory/subsystems/subsystem_dispatcher.go`
  - merged PR `#48` retired the remaining implicit provider-throttle fallback
    path after authored guard support landed, simplifying
    `pkg/factory/options.go`,
    `pkg/factory/runtime/factory.go`, and
    `pkg/factory/subsystems/subsystem_dispatcher.go`
  - the next maintainer action on that ask is no longer "queue or merge the
    throttle lane"; it is simply to keep the world model accurate and avoid
    re-queuing solved throttle residue from ignored local inputs
- the previously recommended reserve seam is now solved:
  - merged PR `#51` retired the in-memory top-level workstation timeout alias
    fallback from `pkg/config/workstation_execution_limits.go`
  - the ignored local idea
    `factory/inputs/idea/default/retire-legacy-workstation-timeout-alias-fallback.md`
    is now solved residue rather than pending work
- direct code inspection plus a sidecar explorer found the next smaller
  non-colliding reserve cleanup seam in workstation request projection:
  - `pkg/api/workstation_request_projection.go` still keeps the local helpers
    `inferenceAttemptProviderSessionOrFallback(...)` and
    `inferenceAttemptDiagnosticsOrFallback(...)`
  - each helper has one caller inside
    `workstationDispatchViewFromCompletion(...)`
  - both helpers only express local nil-check fallback that can be inlined at
    the call site without changing any public contract
  - this seam stays outside the open `tests/functional/**` reorganization lane
    in PR `#30`

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid that tree until it merges
2. the previous checked-in world model was stale again:
   - it still described `HEAD` as `d028000`
   - it still treated the throttle follow-up as open even though PR `#48`
     merged
   - it still treated the workstation-timeout reserve seam as queueable even
     though PR `#51` merged
   - it did not capture the next reserve seam in
     `pkg/api/workstation_request_projection.go`
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- the customer throttle outage-prevention ask is now complete on `main`; the
  correct maintainer posture is to stop treating throttle cleanup as live
  backlog and to avoid creating new overlapping throttle requests from stale
  local residue
- with PR `#48` and PR `#51` merged, the only live broad collision surface is
  PR `#30` in `tests/functional/**`
- safe parallel cleanup should therefore stay package-local and outside
  `tests/functional/**`
- local single-caller fallback helpers are good reserve seams when they keep
  duplicate nil-check ownership inside one production function without adding
  public value
- the broad quality and website-quality asks remain important, but they are
  still programs rather than one narrow immediate lane; until they are broken
  down further, the highest-confidence maintainer move is narrow cleanup that
  reduces local complexity without colliding with the active functional-test
  reorganization

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the checked-in priority order
  is still correct
- do not re-queue any throttle or workstation-timeout lane already merged on
  `main`
- queue one new ignored reserve cleanup idea for simplifying the single-caller
  fallback helpers in `pkg/api/workstation_request_projection.go`
- keep any new reserve work out of `tests/functional/**` while PR `#30`
  remains open

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is now satisfied on `main` through merged PRs `#46` and
  `#48`
- the next customer-facing asks are the broader quality, website-quality,
  linting, docs-audit, manual-QA, and systems-quality programs; none of them
  are yet decomposed into a single new checked-in priority item that should
  displace the current backlog ordering during this pass
