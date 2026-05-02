# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `a1dd288` on May 2, 2026 in the local maintainer workspace
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
  - `factory/inputs/idea/default/dedupe-api-surface-factory-contract.md`
  - `factory/inputs/idea/default/dedupe-factory-config-boundary-alias-rejection.md`
  - `factory/inputs/idea/default/dedupe-generated-public-enum-pointer-helpers.md`
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
  - merged PR `#54` `dedupe-factory-config-boundary-alias-rejection`
  - merged PR `#53` `dedupe-api-surface-factory-contract`
  - merged PR `#52` `inline-workstation-request-projection-fallback-helpers`
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
- the broad throttle customer ask is fully implemented on `main` through
  merged PRs `#46` and `#48`; it should remain treated as solved rather than
  live backlog
- the previously recommended config-boundary alias-rejection seam is now
  solved:
  - merged PR `#54` replaced the duplicated top-level versus nested
    `definition` alias-rejection walks in
    `pkg/config/factory_config_mapping.go` with shared recursive helpers
  - focused coverage for top-level, nested `definition`, and nested cron alias
    rejection now lives in `pkg/config/factory_config_mapping_test.go` and
    `pkg/config/openapi_factory_test.go`
  - the ignored local idea
    `factory/inputs/idea/default/dedupe-factory-config-boundary-alias-rejection.md`
    is now solved residue rather than pending work
- direct code inspection plus a sidecar explorer found the next smaller
  non-colliding reserve cleanup seam in generated enum pointer wrappers:
  - `pkg/interfaces/public_factory_enums.go` still repeats the same
    trim-check plus `&enumValue` wrapper in
    `GeneratedPublicFactoryWorkerTypePtr`,
    `GeneratedPublicFactoryWorkerModelProviderPtr`,
    `GeneratedPublicFactoryWorkerProviderPtr`, and
    `GeneratedPublicFactoryWorkstationTypePtr`
  - `pkg/interfaces/workstation_kind_public.go` repeats the same wrapper in
    `GeneratedPublicWorkstationKindPtr`
  - the only production call site is `pkg/factory/event_history.go`, so the
    seam is isolated to `pkg/interfaces` with a small consumer surface
  - current package coverage already exercises the enum normalization path, so
    the lane can stay focused and outside `tests/functional/**`

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid that tree until it merges
2. the previous checked-in world model was stale again:
   - it still described `HEAD` as `5736135`
   - it did not include merged PR `#54`
   - it still treated the config-boundary alias-rejection seam as queueable
     even though `#54` already landed it on `main`
   - it did not capture the next reserve seam in generated enum pointer
     wrappers under `pkg/interfaces`
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
4. many ignored local idea files now correspond to already merged PRs, so the
   local workflow-input surface is increasingly a mix of solved residue and one
   still-live PR-backed task file

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this repository
  changes quickly enough that the checked-in world model drifts within hours
- the customer throttle outage-prevention ask is complete on `main`; the
  correct maintainer posture is to stop treating throttle cleanup as live
  backlog and to avoid creating new overlapping throttle requests from stale
  local residue
- with PR `#54` merged, the config-boundary alias-rejection seam is no longer
  a valid next move; the remaining duplicate test coverage there is largely
  intentional two-surface verification rather than a strong cleanup target
- the current highest-confidence reserve cleanup is the duplicated pointer
  wrapper layer for generated public enums in `pkg/interfaces`, because it is
  isolated, behavior-preserving, and has one small production consumer in
  `pkg/factory/event_history.go`
- the only live broad collision surface is still PR `#30` in
  `tests/functional/**`, so safe parallel cleanup should stay package-local
  and outside that tree

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the checked-in priority order is
  still correct
- do not re-queue the already-landed config-boundary alias-rejection cleanup
  from ignored local residue
- queue one new ignored reserve cleanup idea for deduplicating the generated
  public enum pointer-wrapper helpers across `pkg/interfaces`
- keep any new reserve work out of `tests/functional/**` while PR `#30`
  remains open

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is satisfied on `main` through merged PRs `#46` and `#48`
- the next customer-facing asks are the broader quality, website-quality,
  linting, docs-audit, manual-QA, and systems-quality programs; none of them
  are yet decomposed into a single new checked-in priority item that should
  displace the current backlog ordering during this pass
