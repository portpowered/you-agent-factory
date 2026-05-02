# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `73fb57f` on May 2, 2026 in the local maintainer workspace
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
  - merged PR `#55` `dedupe-generated-public-enum-pointer-helpers`
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
- direct `HEAD` inspection confirmed that several ignored local workflow-input
  residues are now stale rather than queueable:
  - merged PR `#55` landed the generated enum pointer-helper cleanup on
    `main`, so
    `factory/inputs/idea/default/dedupe-generated-public-enum-pointer-helpers.md`
    is solved residue
  - `pkg/cli/dashboard/dashboard.go` now routes both completed and failed
    session fallback collection through the shared
    `worldViewFallbackWorkItems(...)` seam, so
    `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
    no longer matches live code
  - `pkg/api/handlers.go` no longer re-parses raw `maxResults`, so
    `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
    is also stale residue
- direct code inspection plus a sidecar explorer found the next smaller
  non-colliding reserve cleanup seam in workstation runtime lookup:
  - `pkg/factory/workstationconfig/runtime_lookup.go` still keeps the private
    wrappers `lookupKey(...)` and `lookupKeys(...)`
  - `Workstation(...)` is already the canonical owner of the Name-then-ID
    runtime workstation lookup fallback
  - the helper layer is local redundancy that can be inlined without changing
    behavior
  - the supporting data-model note
    `docs/development/workstation-runtime-config-data-model.md` already
    documents `Workstation` as the unified runtime lookup seam

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid that tree until it merges
2. the previous checked-in world model was stale again:
   - it still described `HEAD` as `a1dd288`
   - it did not include merged PR `#55`
   - it still treated the generated enum pointer-helper seam as the next move
     even though `#55` already landed it on `main`
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
4. many ignored local idea and plan files now correspond either to merged PRs
   or to already-simplified code on `HEAD`, so the local workflow-input
   surface is increasingly a stale mix rather than an actionable queue

## theory of mind

- merged PR history and live `HEAD` file reads must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- the customer throttle outage-prevention ask is complete on `main`; the
  correct maintainer posture is to stop treating throttle cleanup as live
  backlog and to avoid creating new overlapping throttle requests from stale
  local residue
- the local workflow-input surface is now stale in two different ways:
  merged lanes remain as ignored idea residue, and some older ideas no longer
  match live code because later cleanups already simplified the targeted seam
- the safest reserve cleanup posture remains tiny package-local simplifications
  outside `tests/functional/**`, because PR `#30` is still the only live broad
  collision surface
- the current highest-confidence reserve cleanup is the redundant
  `lookupKey(...)` / `lookupKeys(...)` layer in
  `pkg/factory/workstationconfig/runtime_lookup.go`, because the canonical
  `Workstation(...)` lookup already owns the behavior and the seam is isolated

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the checked-in priority order is
  still correct
- do not re-queue solved or code-stale cleanup residue from ignored
  `factory/inputs/**`
- queue one new ignored reserve cleanup idea for inlining the redundant
  workstation runtime lookup key-selection helpers behind
  `pkg/factory/workstationconfig.Workstation(...)`
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
