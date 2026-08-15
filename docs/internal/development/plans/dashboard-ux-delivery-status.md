---
author: Agent Factory Team
last-modified: 2026-08-15
doc-id: agent-factory/plans/dashboard-ux-delivery-status
---

# Dashboard UX Delivery Status

## Audit baseline

This is a read-only delivery audit of the thirty historical dashboard UX
probes. The audited merged tree is `origin/main` at
`a6f52a5650ae1480a81a9dc4cbaf574ead2de581`, audited on 2026-08-15. At the
start of this audit `HEAD` matched that commit exactly and the product tree
had no diff against it. The audit does not treat the historical probe labels
as current status; the probe rows and rollups will be added against this
named baseline.

The audit scope is deliberately limited to evidence and documentation. It
does not change dashboard behavior, tests, generated artifacts, or runtime
code. The final implementation diff is allowed to contain this document and
the dated pointers in the two historical UX plans.

## Evidence order and status rules

Evidence is considered in this order:

1. An existing automated assertion that exercises the cited behavior on the
   named baseline and actually executes.
2. Demonstrable current source whose exact file and line range establishes the
   customer-visible contract when no behavior-specific assertion exists.
3. A reasoned source read naming every inspected file and the limitation that
   prevents stronger evidence.

Compilation, typechecking, generated types, PR titles, reliability-only
`thr-*` changes, file/symbol presence, and unrelated green suites are not
product-delivery evidence. A test that cannot start, or whose cited assertion
does not execute because setup/rendering fails, is not a pass. Any probe that
depends on such a blocked suite remains `OPEN` until another permitted form of
evidence proves the behavior.

The final matrix uses exactly these statuses:

- `FIXED`: every checkable sub-finding in the probe has passing evidence.
- `PARTIAL`: at least one sub-finding has passing evidence and at least one
  concrete sub-finding remains unresolved.
- `OPEN`: no sufficient delivery evidence exists, or a required behavior test
  is blocked before the cited assertion executes.
- `SUPERSEDED`: the historical finding no longer describes the current
  product contract, with the replacement contract named in the row.

## Executability evidence

The following commands were run from this checkout after installing the locked
UI dependencies with `bun install --frozen-lockfile`. Installation only
prepared the environment and is not product evidence.

| Command | Outcome | What executed and what it proves |
| --- | --- | --- |
| `bun run --cwd ui/packages/components check:declaration-runtime` | Pass: 68 declarations and 68 runtime modules; every declaration reference resolved. | The current bundled components artifact has a runtime counterpart for the graph entrypoint. |
| `bunx --no-install vitest run --config vitest.config.ts src/package-declaration-runtime.test.ts` from `ui/packages/components` | Pass: 1 file, 6 tests. | The behavior-specific declaration/runtime guard executed both the missing `./graph-edge` diagnostic and the aligned runtime-counterpart assertions in `ui/packages/components/src/package-declaration-runtime.test.ts:68-153`. |
| `bun run --cwd ui/packages/factory-graph check:components-runtime-resolution` | Pass: 27 focused Bun tests, 114 expectations. | The package-local source aliases were checked and the graph-editor control assertions actually ran under the factory-graph package configuration. React emitted non-fatal `act(...)` warnings; no assertion was skipped. |
| `bun run --cwd ui/packages/factory-graph test` | Pass: 7 files, 130 tests. | The package-owned semantic graph test assertions executed. |
| `bun test src/features/current-selection/work-selection/lib/selected-work-relationship-graph.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.bun.unit.test.ts src/features/trace-drilldown/lib/trace-relation-factory-graph.bun.unit.test.ts` from `ui` | Pass: 5 files, 16 tests, 42 expectations. | The selected-Work and trace relation graph projection assertions executed, including empty/error states, repeated relations, endpoint identity, and localized edge labels. |
| `bun test src/api/worker-sessions/api.bun.unit.test.ts` from `ui` | Pass: 1 file, 5 tests, 8 expectations. | The provider-neutral Worker Session URL, reconnect cursor, typed source failure, list shape, and malformed-frame assertions executed. |
| `go test ./pkg/services/worker_sessions/...` | Pass on the retry: all five Worker Session packages passed. The first attempt hit a transient missing Go build-cache artifact before compiling any package. | Worker Session contract, lifecycle, observation, transport, and wire tests executed on the named tree. |
| `bun run --cwd ui typecheck` | Pass. | TypeScript typechecking completed; this is a quality gate only and is not counted as product-delivery evidence. |
| `bun run --cwd ui/packages/factory-visualizers test` | Blocked: 7 files started, 28 tests passed, 3 failed. | The three graph-rendering assertions did not reach their intended renderer assertions because the mocked `@xyflow/react` module has no `useStore` export. The exact blocked tests are `src/factory-topology-replay.semantics.test.tsx` / “renders a guarded logical move with its public control details” and `src/factory-recording-topology-replay.states.test.tsx` / “invalidates cached ticks when same-identity recording evidence changes” plus “contains a controlled projection failure and removes stale ready content”. These failures are present on the unchanged audited tree; dependent probe claims will not be credited from this suite. |

The first runtime-resolution attempt, before dependency installation, stopped at
the preload with `happy-dom` missing (`0 pass, 1 error`). It was environment
setup failure, not a product result. After installation and package builds, the
same focused command reached and reported its 27 assertions above.

## Confirmed starting points

### Worker Sessions

The current Worker Session contract is provider-neutral at the canonical
identity boundary. `pkg/services/worker_sessions/contracts.go:85-114` exposes
observation, transcript, and retained/live stream operations by Worker Session
ID, including sessions with no Provider Session reference.
`pkg/services/worker_sessions/observation.go:118-227` defines the identity-only
requests and cursors; the cursor carries Worker Session ID, stream generation,
and position rather than falling back to provider identity.

Behavior-specific tests confirm the starting point rather than merely naming
symbols: `pkg/services/worker_sessions/internal/service/control_production_boundary_test.go:546-551`
reads a transcript by Worker Session ID, `:590-611` checks chronological
attempt ordering and exact dispatch/turn/provider correlation, and `:842-868`
checks replay-only records, terminal replay, summary, and closure. The UI
entry point is similarly observable in
`ui/src/api/worker-sessions/api.bun.unit.test.ts:10-63`, where provider-neutral
scoping, reconnect cursors, typed source failure, and observation shape are
asserted and executed by the command above.

### Factory graph visual plan

The separate graph visual plan starts from the canonical semantic graph used by
the activity dashboard, observe/edit modes, and package replay surfaces
(`docs/internal/development/plans/factory-graph-visual-ux.md:73-78`). It assigns
the shared visual contract, semantic state, Work-volume rules, node primitives,
group-region presentation, and package fixtures to `ui/packages/factory-graph`
(`:265-271`), while dashboard-owned state remains pending layout/history,
React Flow interaction wiring, save/reload, and timeline destinations
(`:273-280`). Its separate visual rollup will therefore attribute graph
presentation work to Stories 1-7 without double-counting dashboard Stories
9-10 or other probe owners.

## Story 2: graph and editor probes 5-12

The rows below split the compound historical finding into separately
checkable claims. The source and test paths are from the named `origin/main`
baseline above. A passing dashboard assertion can credit the dashboard host;
it does not credit a package renderer whose suite is blocked in the
executability table. Dashboard Stories 9-10 own editor transactions and
React Flow wiring. The separate factory-graph visual Stories 1-7 own shared
visual grammar, Work-volume presentation, node sizing, group regions, and
host parity; the attribution column names that boundary so one delivery is
not counted twice.

| # | Checkable sub-findings and current evidence | Status | Dashboard ownership | Factory-graph visual attribution | Exact remaining customer work |
| --- | --- | --- | --- | --- | --- |
| 5 | **Concurrent Work is retained.** `ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-active-executions.ts:8-37` keeps active executions distinct while grouping them by workstation; `ui/packages/factory-graph/src/semantic-workstation-node.tsx:97-99,271-289` renders each execution/work item with a `dispatch_id:work_id` key. The executed test `ui/src/features/flowchart/components/current-activity-node-hover-surfaces.test.tsx:229-299` (`uses compact spacing on active workstation work item rows`) renders three concurrent dispatch/work pairs and preserves selected Work. **Observe immutability is not complete.** `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:750` sets `nodesDraggable={true}`, and `ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-view-model.ts:348-422` applies position changes to transient display state; authored persistence is only gated by optional callbacks at `:523-532`. **Graph failure is not contained.** `ui/src/features/workflow-activity/lib/react-flow-current-activity-card-errors.ts:13-21` treats every React Flow error as fatal and throws; the executed test `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport-errors.test.tsx:136-147` (`throws when React Flow reports an edge endpoint handle mismatch`) confirms the escape behavior. | PARTIAL | Story 9: observe-mode transaction/immutability. Story 10: graph error containment and current graph interaction. | Stories 1-2 and 5: semantic node state, Work-volume rendering, and host parity only. The blocked factory-visualizers renderer assertions receive no credit. | Make observe mode non-draggable and ignore transient layout/viewport mutation outside edit mode; add a card-local error boundary with localized recovery/retry. Do not claim package renderer parity until the documented `useStore` mock blocker is cleared or an equivalent assertion executes. |
| 6 | **Selected group creation and geometry are delivered.** `ui/src/features/factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook.component.test.ts:677-726` (`creates a fitted group from the selected node geometry in one history step`) passes; no-selection creation still intentionally produces an empty group through `:539-562`, so the historical “empty-first” behavior is only fixed for the selected-node path. **Groups remain visible in observe mode.** The executed `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport-visual-groups.test.tsx:150-197` (`uses the shared read-only region in observer mode and edit composition while editing`) passes. **Click-through, keyboard selection/movement, membership history, and finite bounds have evidence:** `ui/src/features/factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-layer.component.test.tsx:113-169` covers transparent interiors and keyboard affordances; `ui/src/features/factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook.component.test.ts:774-826` covers membership undo/redo; `ui/src/features/factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups.test.ts:350-364` covers undersized bounds. **Cancel is incomplete.** `ui/src/features/factory-graph-editor/components/flow/visual-groups/interaction/factory-graph-visual-group-layer-interactions.ts:178-394` implements pointer down/move/up and keyboard movement but has no pointer-cancel or Escape restoration path. | PARTIAL | Story 9: grouping transaction, validation, visibility, click-through, and cancel. | Stories 3-5: geometry, group-region presentation, and cross-host parity. Group interaction history remains dashboard-owned. | Decide and implement the empty-group contract for a no-selection Create action if empty-first is still a defect; add pointer-cancel and Escape cancellation that restores group/member previews and clears the drag session. |
| 7 | **Known Add actions complete successfully and field errors are associated.** `ui/src/features/workflow-activity/hooks/use-current-activity-graph-add-controller.ts:52-103` clears the Add tool after a successful submit, and the executed `ui/src/features/workflow-activity/hooks/use-current-activity-graph-add-controller.test.tsx:47-88` (`applies resolved layout placement after a successful add submit`) and `:185-216` (`surfaces add failures as field errors`) cover success and field association. **Unknown actions still fall through.** `ui/src/features/factory-graph-editor/lib/editor/factory-graph-editor-additions.ts:138-194` handles known kinds and returns a workstation draft for any other cast value. **Cancel still leaves Add active.** `ui/src/features/workflow-activity/hooks/current-activity-graph-state-value.ts:318-321` closes the dialog by clearing only draft/errors, while `setActiveTool(null)` is present only on submit at the add controller `:75-90`. **Add is outside Undo/Redo.** `ui/src/features/factory-graph-editor/hooks/use-editable-factory-graph.ts:115-143` wires topology mutations separately from `undoLayout`/`redoLayout`, and `ui/src/features/factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook.ts:177-207,481-525` records only layout commands. | PARTIAL | Story 10: typed Add actions, fail-closed cancellation, and progressive graph interaction. Story 9: unified history. | Story 5 host parity only; the visual package does not own Add-dialog policy or editor transaction history. | Validate the action ID before casting and fail closed with an associated notice; clear the Add tool on dialog cancel; put node additions and their placement in the same domain history as other editor changes. |
| 8 | **Pending connection feedback is partly delivered.** `ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-connections.ts:70-103` toggles the same source anchor off and reports a missing-source notice; `:120-142` clears pending state when the tool becomes non-interactive; the executed `ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-controllers.test.tsx:159-208` (`commits %s keyboard anchor connections and clears the pending source`) and `:210-255` (`shows actionable connection notices for invalid connection paths`) pass. The executed `ui/src/features/factory-graph-editor/lib/editor/factory-graph-editor-connections.test.ts:278-312` (`rejects incompatible anchors with a user-facing notice`) also passes. **Explicit cancel is missing:** the controller has no cancel action and the viewport Escape path clears graph selection (`ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:585-625`), not connection intent. **Duplicates are silent:** `ui/src/features/factory-graph-editor/lib/editor/factory-graph-editor-connections.ts:411-428` returns the unchanged draft when an edge ID already exists without a notice. **Topology is outside history** through the separate mutation/layout wiring cited for probe 7. | PARTIAL | Story 9: topology history. Story 10: connection intent/cancel, endpoint eligibility, and duplicate feedback. | No graph-visual transaction credit; package Story 5 can consume the rendered topology but does not own connect/disconnect history. | Add an explicit Cancel/Escape connection-intent path, surface duplicate-edge eligibility as user feedback, and record connect/disconnect with the unified domain history. |
| 9 | **Selection and successful deletion have shared contracts.** `ui/src/features/factory-graph-editor/lib/selection/factory-graph-editor-selection.ts:3-25,149-191` defines node/edge sets plus a primary target; the executed `ui/src/features/factory-graph-editor/lib/selection/factory-graph-editor-selection.test.ts:55-187` covers `replaces node and edge selection and resolves the latest primary target`, `adds nodes and edges while updating the primary target to the latest addition`, `removes nodes and edges and re-resolves the primary target`, and React Flow node/edge changes. `ui/src/features/workflow-activity/state/factory-graph-editor-selection-bridge.ts:5-25` carries the same snapshot, and `ui/src/features/factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete.test.ts:74-191` covers confirmation plans, cascades, mixed selection, and pruning deleted IDs. Observer deletion is guarded by the executed viewport test `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.test.tsx:403-442` (`does not delete graph entities while outside editor mode`). **Failed-confirmation cleanup is still unsafe:** `ui/src/features/workflow-activity/hooks/use-current-activity-graph-card-view-model.ts:198-209` prunes the current selection whenever a removal was pending, without checking whether `confirmRemoval()` actually applied. **Delete is not undoable:** the removal actions are draft mutations while the exposed history remains layout-only (`use-editable-factory-graph.ts:115-143`). | PARTIAL | Story 9: selection bridge, failed confirmation, and unified delete history. Story 10: editor-mode eligibility and selection/delete controls. | Story 5 host parity may verify visual selection treatment later; no package renderer delivery is credited for this transaction row. | Prune selection only after a confirmed successful removal, add a cross-host observer/editor/group parity assertion, and include node/edge deletion and cascades in document Undo/Redo. |
| 10 | **Save summaries and stale blocking are present.** The executed `ui/src/features/workflow-activity/hooks/use-graph-editor-save-flow.component.test.ts:194-234` covers layout-only, topology-only, and mixed summaries; `:258-270` (`blocks saving with stale-draft messaging when the document version drifts`) covers stale blocking. Current source gates `canSave` on `!isStale` for all pending kinds (`ui/src/features/factory-graph-editor/hooks/use-editable-factory-graph-save-controller.ts:37-66` and `ui/src/features/workflow-activity/hooks/use-graph-editor-save-flow.ts:71-83`), so layout-only bypass is no longer supported by the source contract, though the executed stale test is topology-only. **Reset/scope isolation is observable:** `ui/src/features/factory-graph-editor/hooks/use-editable-factory-graph.document-plane.test.tsx:111-186` avoids rehydrating stale data and `:188-245` drops a dirty draft on scope change; editor chrome reset is covered by `ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-editor.test.tsx:504-553`. **History remains layout-only:** `ui/src/features/factory-graph-editor/hooks/use-editable-factory-graph.ts:115-143` and `ui/src/features/factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook.ts:177-207,481-525` expose only layout history. | PARTIAL | Story 9: one document/history, save/reset/discard, stale protection, and dirty scope changes. | None; editor transaction and persistence policy remain dashboard-owned. | Replace layout-only Undo/Redo with complete document history and add an explicit dirty-session-switch decision so scope changes cannot silently discard customer edits; add a layout-only stale-save regression assertion. |
| 11 | **Persistence and resize controls are partly corrected.** `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:523-532` supplies node/viewport persistence callbacks only when `canMoveLayout` is true, and `:761-768` persists viewport through that optional callback. `ui/src/features/workflow-activity/hooks/use-current-activity-graph-card-view-model.ts:46-90` enables graph-node resize/fit/reset only in edit mode; the executed `ui/src/features/factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook.component.test.ts:94-168` (`records resize, fit, and reset as atomic commands with exact undo and redo`) passes. **Observe presentation is still mutable:** `ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:750-768` keeps nodes draggable and accepts viewport movement even when persistence callbacks are absent. **Cancel is incomplete:** the group interaction handler has no pointer-cancel/Escape path (`ui/src/features/factory-graph-editor/components/flow/visual-groups/interaction/factory-graph-visual-group-layer-interactions.ts:178-394`). **Global Fit does not include group bounds:** the dashboard uses generic `DashboardGraphControls` options (`ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:852-881`), while initial focus is derived from graph nodes only (`ui/src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-view-model.ts:488-495`). | PARTIAL | Story 9: observe immutability, layout history, group fit/cancel. Story 10: progressive controls and Fit behavior. | Stories 3-5: node sizing, group geometry, and host parity; the dashboard owns the global Fit and persistence policy. | Disable or fully neutralize observe drag/viewport updates, add cancellation/restoration for node/group move and resize gestures, and include persisted group bounds in global Fit. |
| 12 | **Legend and control semantics are substantially covered.** The executed `ui/src/features/workflow-activity/components/dashboard-flow-axis-legend.test.tsx:12-226` covers minimized/expanded vocabulary, localized labels, fallback copy, and lifecycle swatches. The correctly preloaded native Bun suite `ui/src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.bun.component.test.tsx:119-184,521-579,596-797` covers keyboard reachability, accessible names/pressed state, mounted disabled actions, unavailable reasons, and visibility controls. **Targets remain below the historical 44 px requirement:** `ui/src/components/ui/dashboard-action-button.tsx:9-13,37-47` gives text actions `min-h-10` and icon-only actions their `IconButtonShell`; the executed Bun assertions at `:175-184` observe `h-10` toolbar buttons. **Narrow overlays are not reserved against one another:** the legend is absolute across `left-4 right-4 top-4` (`ui/src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx:640-647`), while the Add popover disables collision avoidance (`ui/src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.tsx:435-442`); no responsive collision/inset assertion was found. | PARTIAL | Story 10: legend semantics, control eligibility, keyboard/touch reachability, and overlay insets. | Stories 1, 3, and 5: shared visual grammar, sizing primitives, and host parity only; dashboard overlay placement remains Story 10. | Raise every graph toolbar/legend action to the required target size and reserve responsive overlay insets, with a narrow-canvas behavior test proving the legend, notices, popovers, and controls do not collide. |

### Story 2 verification

These focused commands were run against the unchanged product tree at the
audit baseline; they verify the cited assertions but do not alter the audit
classification rules.

- `bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=2 --retry=0 src/features/factory-graph-editor/lib/editor/factory-graph-editor-connections.test.ts src/features/factory-graph-editor/lib/operations/factory-graph-operations.test.ts src/features/factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete.test.ts src/features/factory-graph-editor/lib/selection/factory-graph-editor-selection.test.ts src/features/factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups.test.ts`: pass, 5 files and 52 tests.
- `bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-component --maxWorkers=2 --retry=0` with the eleven dashboard graph/editor component paths cited in this row set: pass, 11 files and 156 tests.
- `bun test --preload ./src/testing/bun/component.setup.ts --reporter=dots --timeout=10000 src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.bun.component.test.tsx`: pass, 27 tests and 114 expectations.
- `bun test --preload ./src/testing/bun/component.setup.ts --reporter=dots --timeout=10000` with the four group/layout component files cited above: pass, 47 tests and 135 expectations.
- `bun run --cwd ui typecheck`: pass. This remains a quality gate, not delivery evidence.
