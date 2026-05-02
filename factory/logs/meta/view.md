# meta view

## world state

- after `git pull --ff-only`, repository `main` and `origin/main` are both at
  `0ea191c` on May 2, 2026 in the local maintainer workspace
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
  - `factory/inputs/idea/default/browser-integration-png-export-import-roundtrip.md`
  - `factory/inputs/idea/default/browser-integration-timeline-tick-controls.md`
  - `factory/inputs/idea/default/browser-shared-action-primitives.md`
  - `factory/inputs/idea/default/collapse-dashboard-fallback-work-item-collectors.md`
  - `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
  - `factory/inputs/idea/default/dedupe-api-surface-factory-contract.md`
  - `factory/inputs/idea/default/dedupe-factory-config-boundary-alias-rejection.md`
  - `factory/inputs/idea/default/dedupe-generated-public-enum-pointer-helpers.md`
  - `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
  - `factory/inputs/idea/default/dedupe-loaded-runtime-definition-lookups.md`
  - `factory/inputs/idea/default/dedupe-portable-bundled-path-containment-validation.md`
  - `factory/inputs/idea/default/dedupe-replay-factory-merge-helpers.md`
  - `factory/inputs/idea/default/dedupe-worker-event-exit-code-extraction.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-completed-dispatch-history.md`
  - `factory/inputs/idea/default/derive-throttle-windows-from-event-history.md`
  - `factory/inputs/idea/default/factory-level-inference-throttle-guard.md`
  - `factory/inputs/idea/default/inline-batch-relation-duplicate-rejection.md`
  - `factory/inputs/idea/default/inline-workstation-request-projection-fallback-helpers.md`
  - `factory/inputs/idea/default/inline-workstation-runtime-lookup-key-fallback.md`
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
  - merged PR `#60` `browser-integration-png-export-import-roundtrip`
  - merged PR `#59` `browser-integration-timeline-tick-controls`
  - merged PR `#58` `dedupe-loaded-runtime-definition-lookups`
  - merged PR `#57` `dedupe-portable-bundled-path-containment-validation`
  - merged PR `#56` `inline-workstation-runtime-lookup-key-fallback`
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
- the worktree is not clean:
  - `factory/logs/meta/asks.md` has a local wording edit from "material design
    style" to "shadcn" in the website-variable ask wording
  - ignored local workflow-input residue remains under `factory/inputs/**`
- the broad throttle customer ask is fully implemented on `main` through
  merged PRs `#46` and `#48`; it should remain treated as solved rather than
  live backlog
- direct `HEAD` inspection confirmed that more ignored local workflow-input
  residue is now stale rather than queueable:
  - merged PR `#60` landed browser-level PNG export/import roundtrip coverage
    on `main`, so
    `factory/inputs/idea/default/browser-integration-png-export-import-roundtrip.md`
    is solved residue
  - merged PR `#59` landed browser replay coverage for moving the timeline
    slider and resetting back to `Current`, so
    `factory/inputs/idea/default/browser-integration-timeline-tick-controls.md`
    is solved residue
  - merged PR `#58` landed the runtime lookup ownership cleanup on `main`, so
    `factory/inputs/idea/default/dedupe-loaded-runtime-definition-lookups.md`
    is solved residue
  - merged PR `#57` landed the portable bundled-file containment cleanup on
    `main`, so
    `factory/inputs/idea/default/dedupe-portable-bundled-path-containment-validation.md`
    is solved residue
  - merged PR `#56` landed the workstation runtime lookup-key fallback cleanup
    on `main`, so
    `factory/inputs/idea/default/inline-workstation-runtime-lookup-key-fallback.md`
    is solved residue
  - merged PR `#55` landed the generated enum pointer-helper cleanup on
    `main`, so
    `factory/inputs/idea/default/dedupe-generated-public-enum-pointer-helpers.md`
    is solved residue
  - `pkg/cli/dashboard/dashboard.go` now routes both completed and failed
    fallback collection through `worldViewFallbackWorkItems(...)`, so both
    `factory/inputs/idea/default/collapse-dashboard-fallback-work-item-collectors.md`
    and
    `factory/inputs/idea/default/consolidate-dashboard-session-fallback-workitem-collectors.md`
    no longer match live code
  - `pkg/api/handlers.go` no longer re-parses raw `maxResults`, so
    `factory/inputs/idea/default/dedupe-list-work-legacy-pagination-fallback.md`
    is also stale residue
- direct code inspection against `Makefile`, `.github/workflows/ci.yml`, the
  standards docs, and the current UI component and integration layers refined
  the next customer-ask-aligned quality seams:
  - the current checked-in lint surface is real but narrow: `make lint` runs
    `go vet ./...` plus `go run ./cmd/deadcodecheck`, and CI invokes that same
    target
  - there is no checked-in `golangci-lint` configuration or CI lane yet, so
    broader backend lint automation remains a live follow-up quality seam
  - merged PRs `#59` and `#60` closed the earlier browser-level website test
    ask for timeline tick controls plus PNG export/import roundtrip coverage
  - the next narrow website-quality gap is the duplicated dashboard action
    primitive layer:
    - shared `ui/src/components/ui/button.tsx` and
      `ui/src/components/ui/dialog.tsx` already exist
    - older dashboard-only `ui/src/components/dashboard/button.tsx` and
      `ui/src/components/dashboard/mutation-dialog.tsx` still keep separate
      action/button/dialog styling contracts
    - live consumers remain in
      `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx`,
      `ui/src/components/dashboard/tick-slider-control.tsx`,
      `ui/src/features/export/export-factory-dialog.tsx`,
      `ui/src/features/submit-work/submit-work-card.tsx`,
      `ui/src/features/trace-drilldown/trace-grid-card.tsx`, and
      `ui/src/features/terminal-work/terminal-work-card.tsx`
  - the narrowest website cleanup slice is to retire the remaining
    dashboard-only action primitives in favor of the shared UI button/dialog
    layer without widening into a broader typography, bento, or token-system
    rewrite

## current blockers

1. open PR `#30` occupies the `tests/functional/**` reorganization lane, so
   new work should avoid that tree until it merges
2. the previous checked-in world model was stale again:
   - it still described `HEAD` as `c1dffe4`
   - it did not include merged PR `#60`
   - it still treated browser PNG roundtrip coverage as the next move even
     though `#60` already landed it on `main`
3. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
4. many ignored local idea and plan files now correspond either to merged PRs
   or to already-simplified code on `HEAD`, so the local workflow-input
   surface is increasingly a stale mix rather than an actionable queue
5. the quality and linting customer asks remain broad, so they only become
   actionable when decomposed into small checked-in or ignored idea files with
   explicit file scope and observable acceptance criteria
6. the tracked maintainer backlog file itself is currently dirty in the local
   workspace, so meta updates must avoid overwriting `factory/logs/meta/asks.md`
   while still keeping the world model aligned to live code and merged history

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
- the safest non-colliding cleanup posture remains small, package-local or
  root-quality-lane work outside `tests/functional/**`, because PR `#30` is
  still the only live broad collision surface
- the website-testing sub-ask that explicitly named tick controls plus PNG
  export/import is now satisfied on `main` through merged PRs `#59` and `#60`
- the next customer-ask-aligned website move is to reduce design-system drift
  by collapsing the remaining dashboard-only action primitives onto the shared
  UI button/dialog layer before attempting a broader token or component rewrite
- broader lint automation is still a real quality seam, but it is now the
  secondary follow-up behind the narrower shared-action-primitive website lane

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; its local wording edit should
  remain intact and the checked-in priority order is still correct
- do not re-queue solved or code-stale cleanup residue from ignored
  `factory/inputs/**`
- queue one new ignored customer-ask-aligned idea for retiring the remaining
  dashboard-only action primitives in favor of shared UI button/dialog
  components
- keep any new reserve or quality work out of `tests/functional/**` while PR
  `#30` remains open

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is satisfied on `main` through merged PRs `#46` and `#48`
- the quality and linting asks remain live, but the next best slice after this
  pass is no longer throttle-related
- the website-quality ask remains live at a broader design-system and
  consistency level, and its clearest narrow unchecked subproblem is the
  duplicated dashboard-only action primitive layer now that both browser replay
  tick controls and browser PNG export/import coverage are merged on `main`
