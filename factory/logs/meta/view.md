# meta view

## world state

- after `git pull`, repository `main` and `origin/main` are both at `e87f9d3`
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
- the worktree is still locally dirty for non-canonical workflow evidence:
  - `factory/logs/meta/asks.md` keeps the pre-existing local wording edit from
    "material design style" to "shadcn"
  - `factory/logs/weird-number-summary.jsonl` remains untracked local evidence
    for the already-merged dashboard-summary bug
- the current GitHub lane state in the maintainer workspace is:
  - open PR `#61` `browser-shared-action-primitives`
  - merged PR `#64` `retire-dashboard-bento-layout-ownership`
  - merged PR `#63` `retire-current-selection-inference-duplication`
  - merged PR `#62` `align-dashboard-work-summary-count-semantics`
  - merged PR `#60` `browser-integration-png-export-import-roundtrip`
  - merged PR `#59` `browser-integration-timeline-tick-controls`
  - merged PR `#58` `dedupe-loaded-runtime-definition-lookups`
  - merged PR `#57` `dedupe-portable-bundled-path-containment-validation`
  - merged PR `#56` `inline-workstation-runtime-lookup-key-fallback`
  - merged PR `#55` `dedupe-generated-public-enum-pointer-helpers`
  - merged PR `#54` `dedupe-factory-config-boundary-alias-rejection`
  - merged PR `#53` `dedupe-api-surface-factory-contract`
- direct merged-history and live-file inspection confirmed the previously
  queued current-selection bug is complete on `main`:
  - PR `#63` merged on May 2, 2026 and now owns the selected-work
    current-inference-duplication lane
  - the previous local idea
    `factory/inputs/idea/default/retire-current-selection-inference-duplication.md`
    is now solved residue rather than pending work
- direct merged-history and live backend inspection also confirmed the top
  throttle/system-deficit ask is no longer the next active lane on `main`:
  - PR `#42` `retire-dispatcher-throttle-pause-map` merged on May 2, 2026
  - PR `#46` `factory-level-inference-throttle-guard` merged on May 2, 2026
  - PR `#48` `retire-legacy-throttle-fallback-after-authored-guard` merged on
    May 2, 2026
  - `pkg/config/config_mapper.go` now lowers authored `factory.guards` entries
    into `petri.InferenceThrottleGuard`
  - `pkg/petri/inference_throttle_guard.go` derives active pauses from
    dispatch history plus explicit clock time
  - `pkg/factory/subsystems/subsystem_dispatcher.go` no longer synthesizes
    provider/model throttle pauses when no authored inference-throttle guard is
    present
- direct UI inspection against the checked-in website-quality ask confirmed
  merged PR `#64` completed the previously queued dashboard-layout lane on
  `main`:
  - `ui/src/components/dashboard/bento.tsx`,
    `ui/src/components/dashboard/widget-board.tsx`, and
    `ui/src/components/dashboard/typography.ts` are now compatibility shims
    that re-export from `ui/src/components/ui/`
  - `ui/src/App.tsx` now imports `AgentBentoLayout` from
    `ui/src/components/ui`
  - `docs/processes/development-guide-relevant-files.md` now documents the
    shared ownership seam and explicitly prefers thin dashboard re-export
    shims over keeping the primary implementation in `components/dashboard/`
- direct UI inspection also validated the next distinct customer-facing website
  cleanup seam outside open PR `#61`:
  - `ui/src/components/dashboard/formatters.ts` and
    `ui/src/components/dashboard/place-labels.ts` remain real implementations
    rather than thin compatibility shims
  - those helpers are imported across multiple feature surfaces, including
    `ui/src/features/current-selection/**`,
    `ui/src/features/flowchart/**`,
    `ui/src/features/trace-drilldown/**`,
    `ui/src/features/terminal-work/**`, and
    `ui/src/features/work-outcome/**`
  - the open PR `#61` file set is confined to
    `button.tsx`, `mutation-dialog.tsx`, `tick-slider-control.tsx`, and
    `react-flow-current-activity-card.tsx`, so formatter and place-label
    ownership remains a separate non-colliding website-quality slice
- a smaller reserve backend simplification seam is still live in
  `pkg/interfaces/runtime_lookup.go`, where `FirstRuntimeDefinitionLookup(...)`
  and `FirstRuntimeWorkstationLookup(...)` remain thin duplicate first-non-nil
  wrappers, but that seam is secondary to the still-live website-quality ask
- direct standards and workflow inspection refined the broader quality posture:
  - `make lint` still runs `go vet ./...` plus `go run ./cmd/deadcodecheck`
  - there is still no checked-in `golangci-lint` configuration or CI lane, so
    broader backend lint automation remains a live follow-up quality seam
  - the checked-in workflow contract still prefers one standalone ignored idea
    file under `factory/inputs/idea/default/` unless dependency ordering
    requires a batch request

## current blockers

1. the previous checked-in world model is stale again:
   - it still described `HEAD` as `f3eade6`
   - it did not include merged PR `#64`
   - it still treated dashboard bento/layout ownership as the next live lane
     even though that lane is already merged on `main`
2. workspace-local ignored residue can drift independently of `main` and must
   not be re-queued blindly
3. open PR `#61` still occupies the shared dashboard action-primitive lane, so
   new website cleanup must stay outside
   `button.tsx`, `mutation-dialog.tsx`, `tick-slider-control.tsx`, and the
   current-activity-card file set it already touches
4. the broader quality, token, hook/state, and linting asks remain broad, so
   they only become actionable when decomposed into small idea files with
   explicit file scope and observable acceptance criteria
5. the tracked maintainer backlog file itself is currently dirty in the local
   workspace, so meta updates must avoid overwriting `factory/logs/meta/asks.md`
   while still keeping the world model aligned to live code and merged history

## theory of mind

- merged PR history, open PR file sets, and live `HEAD` file reads must keep
  winning over both the checked-in meta view and ignored `factory/inputs/**`
  residue; this repository still changes quickly enough that the checked-in
  world model drifts within hours
- the highest-priority throttle/system-deficit ask is now effectively
  implemented on `main`, so the correct maintainer posture is to stop treating
  throttle redesign as the next default queue item unless a new gap appears
- the website-quality ask still has multiple open slices, but they need to be
  handled one narrow seam at a time:
  - PR `#61` owns shared action primitives
  - PR `#64` completed dashboard bento/layout primitive ownership
  - the next distinct live slice is the remaining real formatter and
    place-label helper ownership still living under
    `ui/src/components/dashboard/`
- ignored local workflow-input residue is now stale in several ways:
  - many idea files map to already-merged PRs
  - some older reserve ideas no longer match live code after later cleanups
  - only current live-code inspection can tell which residues still describe a
    real seam
- the smaller backend reserve seam in `pkg/interfaces/runtime_lookup.go` is
  real, but when a customer-ask-aligned website lane remains live and
  non-overlapping, that customer-visible lane should win first

## next best move

- update the checked-in meta world model and progress log now
- leave `factory/logs/meta/asks.md` unchanged; its local wording edit should
  remain intact and the checked-in backlog priority order is still correct
- queue one new ignored customer-ask-aligned idea for retiring the remaining
  real formatter and place-label helper ownership under
  `ui/src/components/dashboard/`
- keep the new lane focused on observable label/formatting behavior and direct
  import ownership, not on a whole-dashboard redesign, token migration, or
  cross-feature state reorganization

## customer asks

- `factory/logs/meta/asks.md` remains the only checked-in backlog surface
- no ask is marked urgent as of May 2, 2026 in the maintainer workspace
- the throttling/system-deficit ask is satisfied on `main` through merged PRs
  `#42`, `#46`, and `#48`
- the dashboard total-work summary bug described in
  `factory/logs/weird-number-summary.jsonl` is satisfied on `main` through
  merged PR `#62`
- the selected-work duplicate current-inference bug is satisfied on `main`
  through merged PR `#63`
- the dashboard bento/layout ownership ask slice is satisfied on `main`
  through merged PR `#64`
- the website-quality ask remains live, with open PR `#61` covering the shared
  action-primitive slice and the next unchecked narrow customer-facing slice
  now being dashboard-owned formatter and place-label helpers that still live
  under `ui/src/components/dashboard/`
