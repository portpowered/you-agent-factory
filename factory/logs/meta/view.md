# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at `abcb964`
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
  - merged PR `#30` `prd-functional-test-suite-decomposition`
  - merged PR `#60` `browser-integration-png-export-import-roundtrip`
  - merged PR `#59` `browser-integration-timeline-tick-controls`
  - merged PR `#58` `dedupe-loaded-runtime-definition-lookups`
  - merged PR `#57` `dedupe-portable-bundled-path-containment-validation`
  - merged PR `#56` `inline-workstation-runtime-lookup-key-fallback`
  - merged PR `#55` `dedupe-generated-public-enum-pointer-helpers`
  - merged PR `#54` `dedupe-factory-config-boundary-alias-rejection`
  - merged PR `#53` `dedupe-api-surface-factory-contract`
  - merged PR `#52` `inline-workstation-request-projection-fallback-helpers`
- the worktree is not clean:
  - `factory/logs/meta/asks.md` has a local wording edit from "material design
    style" to "shadcn" in the website-variable ask wording
  - `factory/logs/weird-number-summary.jsonl` is untracked local evidence for
    the dashboard-total bug mentioned in the checked-in ask file
  - ignored local workflow-input residue remains under `factory/inputs/**`
- the broad throttle customer ask remains solved on `main` through merged PRs
  `#46` and `#48`; it should stay treated as complete rather than live backlog
- direct `HEAD` inspection confirmed the previous next-step lane is no longer
  queueable:
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
- direct code inspection against the runtime projection and UI snapshot layers
  validated a distinct live customer bug in the website-quality ask:
  - backend dashboard/session projections count failed totals by
    customer-visible failed dispatch in
    `pkg/factory/projections/world_view_runtime.go` and the matching CLI render
    path in `pkg/cli/dashboardrender/simple_dashboard.go`
  - the UI timeline store currently sets `session.failed_count` from the raw
    size of `failedWorkItemsByID` in `ui/src/state/factoryTimelineStore.ts`
  - `failed_by_work_type` already uses work-item counting semantics, so the UI
    currently mixes dispatch-scoped totals with work-item-scoped groupings
  - this mismatch aligns with the checked-in ask example that
    `factory/logs/weird-number-summary.jsonl` can show one failed total where
    the user expects three failed work items
- direct standards and workflow inspection refined the broader quality posture:
  - `make lint` still runs `go vet ./...` plus `go run ./cmd/deadcodecheck`
  - there is still no checked-in `golangci-lint` configuration or CI lane, so
    broader backend lint automation remains a live follow-up quality seam
  - merged PR `#30` removed the previous `tests/functional/**` collision
    blocker, so new behavioral coverage is no longer constrained by that open
    lane

## current blockers

1. the previous checked-in world model was stale again:
   - it still described `HEAD` as `0ea191c`
   - it still treated PR `#30` as open even though it is now merged at
     `abcb964`
   - it did not include open PR `#61`
   - it still treated the shared-action-primitives website cleanup as the next
     unclaimed move even though that lane is already in flight
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
- the next website-quality lane that had the best fit yesterday is now already
  in flight as PR `#61`, so the correct maintainer move is to stop re-queueing
  that wrapper cleanup and shift to the next distinct user-visible defect
- the strongest distinct live customer-ask slice is now the dashboard
  work-summary count mismatch: backend and CLI session projections use
  customer-dispatch semantics, while the UI timeline store uses raw failed work
  item counts for `failed_count`, so total summaries can drift from both the
  backend view and the user’s expectation
- broader lint automation is still a real quality seam, but it remains a
  secondary follow-up behind the narrower user-visible work-summary bug now
  that PR `#61` occupies the prior website cleanup lane

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; its local wording edit should
  remain intact and the checked-in priority order is still correct
- do not re-queue solved or code-stale cleanup residue from ignored
  `factory/inputs/**`
- queue one new ignored customer-ask-aligned idea for aligning dashboard
  work-summary count semantics across the backend projection and the UI
  timeline snapshot
- keep the new lane focused on observable summary behavior and targeted package
  or component coverage rather than reopening broad functional-test
  restructuring

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling ask is satisfied on `main` through merged PRs `#46` and `#48`
- the website-quality ask remains live, but its previously highest-value narrow
  slice is now already represented by open PR `#61`
- the next unchecked narrow website/customer bug is the incorrect total-work
  summary counting described in `factory/logs/meta/asks.md` and evidenced by
  the local `factory/logs/weird-number-summary.jsonl` trace
