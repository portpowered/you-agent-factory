# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `5736135` on May 2, 2026 in the local maintainer workspace
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
- the previously recommended API/service reserve seam is now solved:
  - merged PR `#53` made `pkg/apisurface/contract.go` embed
    `factory.APIFactory` instead of re-declaring the shared runtime methods
  - the ignored local idea
    `factory/inputs/idea/default/dedupe-api-surface-factory-contract.md`
    is now solved residue rather than pending work
- direct code inspection plus a sidecar explorer found the next smaller
  non-colliding reserve cleanup seam in generated config-boundary alias
  rejection:
  - `pkg/config/factory_config_mapping.go` still walks worker and workstation
    objects twice, once at the top-level object and again for nested
    `definition` objects, while applying the same retired-field inventories
  - the duplication is narrow, entirely inside `pkg/config`, and aligned with
    the checked-in development-guide rule to keep one retired-field inventory
    per boundary type and vary only the caller-owned path text
  - focused proof already exists in `pkg/config/factory_config_mapping_test.go`
    for top-level, nested `definition`, and nested cron alias rejection
  - this seam stays outside the open `tests/functional/**` reorganization lane
    in PR `#30`

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid that tree until it merges
2. the previous checked-in world model was stale again:
   - it still described `HEAD` as `f9c2cf0`
   - it did not include merged PR `#53`
   - it still treated the API/service contract seam as queueable even though
     `#53` already landed it on `main`
   - it did not capture the next reserve seam in
     `pkg/config/factory_config_mapping.go`
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
4. the codebase-memory graph is useful for discovery but can lag immediately
   after a fresh merge, so recent seam validation still needs live file reads
   before the maintainer loop queues work

## theory of mind

- merged PR history and live open PR file sets must keep winning over both the
  checked-in meta view and ignored `factory/inputs/**` residue; this
  repository changes quickly enough that the checked-in world model drifts
  within hours
- the customer throttle outage-prevention ask is complete on `main`; the
  correct maintainer posture is to stop treating throttle cleanup as live
  backlog and to avoid creating new overlapping throttle requests from stale
  local residue
- with PR `#53` merged, the next highest-confidence cleanup is no longer at
  the API/service interface seam; it is duplicated retired-field rejection
  plumbing at the generated config boundary
- when the same retirement policy applies to both a top-level object and its
  nested `definition`, the preferred cleanup direction is to keep one field
  inventory per boundary type and vary only the caller-owned path text
- the only live broad collision surface is still PR `#30` in
  `tests/functional/**`, so safe parallel cleanup should stay package-local
  and outside that tree
- the broad quality and website-quality asks remain important, but they are
  still programs rather than one narrow immediate lane; until they are broken
  down further, the highest-confidence maintainer move is narrow cleanup that
  reduces local complexity without colliding with the active functional-test
  reorganization

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; the checked-in priority order
  is still correct
- do not re-queue the already-landed API/service contract cleanup from ignored
  local residue
- queue one new ignored reserve cleanup idea for deduplicating the generated
  config-boundary retired-field rejection path in
  `pkg/config/factory_config_mapping.go`
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
