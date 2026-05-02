# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at `c6f37eb`
  on May 2, 2026 in the local maintainer workspace
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
  not checked-in workflow truth
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#61` `browser-shared-action-primitives`
  - merged PR `#62` `align-dashboard-work-summary-count-semantics`
  - merged PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#60` `browser-integration-png-export-import-roundtrip`
  - merged PR `#59` `browser-integration-timeline-tick-controls`
  - merged PR `#58` `dedupe-loaded-runtime-definition-lookups`
  - merged PR `#57` `dedupe-portable-bundled-path-containment-validation`
  - merged PR `#56` `inline-workstation-runtime-lookup-key-fallback`
  - merged PR `#55` `dedupe-generated-public-enum-pointer-helpers`
  - merged PR `#54` `dedupe-factory-config-boundary-alias-rejection`
  - merged PR `#53` `dedupe-api-surface-factory-contract`
- the worktree is not clean:
  - `factory/logs/meta/asks.md` has a local wording edit from "material design
    style" to "shadcn" in the website-variable ask wording
  - `factory/logs/weird-number-summary.jsonl` is untracked local evidence for
    the already-fixed dashboard-total bug mentioned in the checked-in ask file
  - ignored local workflow-input residue remains under `factory/inputs/**`
- the broad throttle customer ask remains solved on `main` through merged PRs
  `#46` and `#48`; it should stay treated as complete rather than live backlog
- direct merged-history and `HEAD` inspection confirmed the previous queued
  customer bug is no longer queueable:
  - PR `#62` already owns the dashboard work-summary count mismatch lane
  - the ignored idea
    `factory/inputs/idea/default/align-dashboard-work-summary-count-semantics.md`
    is now solved residue rather than checked-in workflow truth
- direct `HEAD` inspection also confirmed the previous website cleanup follow-up
  remains unavailable for a second queue:
  - PR `#61` already owns the narrow shared-action-primitives website slice
  - that lane currently touches
    `ui/src/components/dashboard/button.tsx`,
    `ui/src/components/dashboard/mutation-dialog.tsx`,
    `ui/src/components/dashboard/tick-slider-control.tsx`,
    `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx`,
    the related tests, and the generated UI dist artifacts
  - the remaining wrapper semantics are now small adapter behavior rather than
    a broad design-system gap, so a second overlapping website request would
    be wasteful
- direct UI inspection against the checked-in website-quality ask validated the
  next distinct customer-facing website bug outside PR `#61`:
  - the selected work-item surface still renders a separate `Inference attempts`
    section from `ui/src/features/current-selection/execution-details.tsx`
    alongside `Workstation dispatches` from
    `ui/src/features/current-selection/selected-work-dispatch-history.tsx`
  - the same duplication is reinforced by
    `ui/src/features/current-selection/work-item-card.test.tsx`,
    `ui/src/App.test.tsx`, and `ui/src/App.stories.tsx`
  - this conflicts directly with the checked-in ask that the selection
    component should show general work information plus dispatches, without a
    separate current-inference section, and should denote the current dispatch
    inside the dispatch list
- direct standards and workflow inspection refined the broader quality posture:
  - `make lint` still runs `go vet ./...` plus `go run ./cmd/deadcodecheck`
  - there is still no checked-in `golangci-lint` configuration or CI lane, so
    broader backend lint automation remains a live follow-up quality seam
  - merged PR `#30` removed the previous `tests/functional/**` collision
    blocker, so new behavioral coverage is no longer constrained by that old
    lane

## current blockers

1. the previous checked-in world model was stale again:
   - it still described `HEAD` as `abcb964`
   - it did not include merged PR `#62`
   - it still treated the dashboard failed-summary mismatch as the next live
     customer bug even though that lane is already merged on `main`
2. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
3. many ignored local idea and plan files now correspond either to merged PRs
   or to already-simplified code on `HEAD`, so the local workflow-input
   surface is increasingly a stale mix rather than an actionable queue
4. the quality and linting customer asks remain broad, so they only become
   actionable when decomposed into small idea files with explicit file scope
   and observable acceptance criteria
5. the tracked maintainer backlog file itself is currently dirty in the local
   workspace, so meta updates must avoid overwriting `factory/logs/meta/asks.md`
   while still keeping the world model aligned to live code and merged history

## theory of mind

- merged PR history, open PR file sets, and live `HEAD` file reads must keep
  winning over both the checked-in meta view and ignored `factory/inputs/**`
  residue; this repository changes quickly enough that the checked-in world
  model drifts within hours
- the customer throttle outage-prevention ask is complete on `main`; the
  correct maintainer posture is to stop treating throttle cleanup as live
  backlog and to avoid creating overlapping throttle requests from stale local
  residue
- the local workflow-input surface is stale in two different ways: merged lanes
  remain as ignored idea residue, and some older ideas no longer match live
  code because later cleanups already simplified the targeted seam
- the website-testing sub-ask that explicitly named tick controls plus PNG
  export/import is satisfied on `main` through merged PRs `#59` and `#60`
- the previous strongest customer-ask slice, the dashboard work-summary count
  mismatch, is complete on `main` through merged PR `#62`, so the ignored idea
  for that seam must now be treated as stale residue
- the next website-quality lane now is the selected-work current-selection
  duplication: the work-item card already has a dispatch-history surface, but
  it still keeps a second `Inference attempts` section for the same selected
  run rather than denoting the current dispatch inside the dispatch list
- broader lint automation is still a real quality seam, but it remains a
  secondary follow-up behind the narrower user-visible current-selection bug
  now that PR `#61` occupies the shared-action-primitive lane

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; its local wording edit should
  remain intact and the checked-in priority order is still correct
- do not re-queue solved or code-stale cleanup residue from ignored
  `factory/inputs/**`
- queue one new ignored customer-ask-aligned idea for retiring the separate
  current-inference section from selected work-item details and denoting the
  current dispatch inside `Workstation dispatches`
- keep the new lane focused on observable current-selection behavior and UI
  coverage rather than broad current-selection redesign or dashboard-wide
  component churn

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is satisfied on `main` through merged PRs `#46` and `#48`
- the dashboard total-work summary bug described in
  `factory/logs/weird-number-summary.jsonl` is now satisfied on `main` through
  merged PR `#62`
- the website-quality ask remains live, with open PR `#61` covering the
  shared-action-primitive slice and the next unchecked narrow customer bug now
  being the duplicate current-inference presentation in selected work-item
  details
