# PRD: Dashboard UI Nit Cleanup Bundle

## Context

### Customer ask

Bundle several remaining dashboard and graph-editor UI rough edges into one focused polish pass: tighten graph toolbar copy and tooltips, emphasize graph hover states, apply idle work-state phase colors, make topology save failures dismissible, stabilize workstation prompt diagnostics layout, restore work-type node selection, standardize current-selection expandable section shells, and remove low-value helper prose.

### Problem

Operators and factory editors encounter inconsistent interaction language and visual noise that erodes confidence without changing underlying factory behavior:

- Graph toolbar actions use redundant visible copy (for example `Add tool`, `Connect tool`) and the add control lacks a tooltip.
- Graph nodes and edges do not clearly emphasize hover, making inspection harder.
- Idle work-state nodes do not communicate phase through color unless selected, active, or in error.
- Topology save-failure banners persist with no dismiss action.
- Workstation prompt diagnostics can shift layout when validation state changes.
- Work-type nodes look selectable but do not reliably update current selection.
- Current-selection expandable sections use inconsistent shells (nested per-field boxes versus one section outline).
- Verbose helper prose in current-selection panels buries actionable warnings and fields.

### Solution

Deliver a frontend-only cleanup bundle that tightens existing dashboard primitives—toolbar messages, graph node/edge projections, phase styling helpers, topology notices, prompt diagnostics disclosure, selection wiring, `ExpandablePanelTrigger` shells, and message catalogs—without redesigning the dashboard or changing backend contracts.

## Introduction

This bundle groups related dashboard and graph-editor polish into one reviewable unit. The intent is not to redesign the dashboard, but to align interaction language and visual consistency while preserving current graph editing, save, selection, and validation behavior.

## Project-level acceptance criteria

- [ ] Graph toolbar controls use concise visible labels and keyboard-accessible tooltips, including add.
- [ ] Graph nodes and edges show accent hover emphasis without overriding selected, active-flow, validation-error, or muted states.
- [ ] Idle work-state nodes reflect canonical phase colors with correct precedence for active, selected, error, and muted states.
- [ ] Topology save-failure notices are dismissible without clearing drafts and reappear on later failures.
- [ ] Workstation prompt diagnostics stay layout-stable and default-hidden behind the existing autocomplete/help disclosure.
- [ ] Work-type nodes are either fully selectable or clearly non-interactive in unsupported modes.
- [ ] In-scope current-selection expandable sections share one coherent shell pattern and reduced low-value prose.
- [ ] UI typecheck, lint, and targeted tests pass for all changed behavior.

## Goals

- Make graph toolbar action labels and tooltips concise and consistent.
- Add clear hover emphasis for graph nodes and edges using semantic accent tokens.
- Ensure idle work-state nodes use semantic colors by work-state phase.
- Make topology save-failure notices dismissible.
- Keep prompt diagnostics layout stable and tucked under existing autocomplete/help disclosure.
- Restore selectability for work-type nodes where the UI implies they can be clicked.
- Standardize expandable section shells across current-selection configuration surfaces.
- Remove or shorten low-value explanatory prose that clutters current-selection panels.

## High-level technical design

All work stays in the dashboard UI (`ui/`). Canonical factory graph and selection state remain unchanged; projections and presentation layers own hover, color precedence, notice dismissal, disclosure layout, and copy.

| Area | Canonical state | Presentation / mutation boundary | Verification |
| --- | --- | --- | --- |
| Graph toolbar | Editor tool mode in graph editor store | Message catalog + toolbar button/tooltip components | Component tests + browser |
| Node/edge hover | React Flow pointer/hover UI state | Node/edge shell class mapping | Projection/component tests + browser |
| Idle work-state color | `place.state_category` / work-state type | `factory-graph-work-state-phase-styling` helpers on place nodes | Unit tests for precedence + browser |
| Topology save notice | Save result / error payload in editor flow | Notice UI with dismiss flag local to presentation | Flow/component tests + browser |
| Prompt diagnostics | Prompt validation + diagnostics arrays | Reserved fixed-height region inside autocomplete disclosure | Workstation prompt tests + browser |
| Work-type selection | Dashboard selection store | `onSelectWorkType` wiring on work-type nodes | Selection tests + browser |
| Expandable sections | Per-section expanded UI state | Shared `ExpandablePanelTrigger` + outer panel shell | Disclosure guard tests + browser |
| Prose cleanup | Message catalogs | Remove redundant helper strings | Message/catalog tests |

## User Stories

### US-001: Polish graph toolbar hints

**Description:** As a factory editor, I want graph toolbar controls to use concise labels and helpful tooltips so that the tool row is easy to scan.

**Acceptance Criteria:**

- [ ] The hide/show node-classes control uses concise visible copy such as `Show` / `Hide`, not redundant wording like `Show tool` or `Hide or show`.
- [ ] Delete and connect tools use the same concise visible-label style as show/hide.
- [ ] The add control exposes tooltip text on hover and keyboard focus (for example `Add`).
- [ ] Toolbar buttons retain accessible names, `aria-pressed` where applicable, and existing disabled behavior.
- [ ] Toolbar tests cover updated copy and the add tooltip.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-002: Emphasize graph hover states

**Description:** As a factory editor, I want nodes and edges to pop in the accent color on hover so that I can see exactly which graph item I am inspecting.

**Acceptance Criteria:**

- [ ] Hovering a graph node visibly accents the node border without overriding selected, validation-error, active-flow, or muted states.
- [ ] Hovering a graph edge visibly accents the edge stroke without overriding validation-error or selected states.
- [ ] Hover styles use existing semantic accent tokens, not ad hoc raw colors.
- [ ] Hover emphasis works in both dashboard graph viewing and factory graph editor modes.
- [ ] Tests cover hover-safe class/state precedence for nodes and edges.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-003: Apply idle work-state phase colors

**Description:** As an operator, I want inactive work-state nodes to show phase-specific colors so that the graph communicates state meaning even when nothing is selected or active.

**Acceptance Criteria:**

- [ ] Idle `FAILED` work states use danger/red semantic surface styling.
- [ ] Idle `TERMINAL` work states use the existing success semantic styling from the phase token map.
- [ ] Idle `PROCESSING` work states use warning/yellow semantic surface styling.
- [ ] Idle `INITIAL` work states use default/info semantic styling.
- [ ] Active, selected, validation-error, and muted states keep precedence over idle phase styling.
- [ ] Tests cover idle phase colors and precedence order.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-004: Dismiss topology save failure notices

**Description:** As a factory editor, I want to dismiss a topology save-failure notice after I have read it so that stale error text does not permanently crowd the graph.

**Acceptance Criteria:**

- [ ] Topology save-failure notices include a close action using the same icon-button style as nearby dashboard controls.
- [ ] Activating close hides that specific failure notice without clearing the unsaved graph draft or save eligibility state.
- [ ] A later save failure shows the notice again, including after a prior dismissal.
- [ ] Keyboard users can focus and activate the close action.
- [ ] The notice is announced as an alert when first shown.
- [ ] Tests cover dismiss and re-show on a later failure.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-005: Stabilize workstation prompt diagnostics layout

**Description:** As a factory editor, I want prompt diagnostics tucked under the autocomplete/help disclosure with a stable reserved region so that prompt editing does not jump the surrounding layout.

**Acceptance Criteria:**

- [ ] Prompt diagnostics render inside the existing autocomplete/help expandable disclosure by default (collapsed unless the operator expands it or syntax errors require inline attention per current rules).
- [ ] The diagnostics region keeps a fixed reserved height so idle, loading, empty, error, and populated diagnostic states do not shift adjacent fields.
- [ ] Diagnostics content updates in place without mounting/unmounting the surrounding prompt editor layout.
- [ ] Actionable syntax/validation messages remain available when diagnostics block save.
- [ ] Tests cover collapsed-default behavior, reserved layout stability, and unchanged save-blocking semantics.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-006: Restore work-type node selection

**Description:** As an operator, I want work-type nodes that look selectable to respond to click and keyboard selection so that graph navigation feels consistent.

**Acceptance Criteria:**

- [ ] Work-type nodes intended to be selectable update current selection on click.
- [ ] Work-type selection is keyboard reachable and exposes an accessible selected state where applicable.
- [ ] In modes where work-type selection is unsupported, work-type nodes do not present as clickable controls.
- [ ] Workstation and work-state node selection behavior is unchanged.
- [ ] Tests cover selectable and intentionally non-selectable work-type rendering paths.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-007: Standardize current-selection expandable section shells

**Description:** As an operator, I want current-selection sections to share the same expandable shell so that workstation, active work, history, and related configuration panels feel coherent.

**Acceptance Criteria:**

- [ ] In-scope current-selection expandable sections use the shared `ExpandablePanelTrigger` disclosure pattern.
- [ ] Configuration sections use one outer panel/shell around grouped content instead of separate outlined boxes around every individual field where the shared pattern applies.
- [ ] Workstation configuration, active work, history, and comparable current-selection sections share consistent header, border, spacing, and expanded-content rhythm.
- [ ] Expanded/collapsed behavior and accessibility relationships (`aria-expanded`, `aria-controls`, headings) are preserved.
- [ ] Disclosure guard checks pass or are intentionally updated for standardized paths.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-008: Reduce low-value current-selection prose

**Description:** As an operator, I want current-selection panels to show concise operational copy so that important fields and warnings are not buried under explanatory text.

**Acceptance Criteria:**

- [ ] Remove or shorten verbose helper prose that repeats field labels, summaries, or warnings (for example long shared-worker impact paragraphs when the UI already names affected workstations).
- [ ] Preserve actionable warnings, validation messages, overwrite hints, and save-blocking notices.
- [ ] Remaining helper copy is concise and tied to a concrete decision, consistent with website standards for dense surfaces.
- [ ] Copy changes go through existing message catalogs, not new inline strings.
- [ ] Message/catalog tests are updated where assertions depend on removed prose.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

## Functional Requirements

1. **FR-1:** Graph toolbar visible labels and tooltip text use concise, consistent action wording.
2. **FR-2:** The add toolbar action exposes tooltip text on hover and keyboard focus.
3. **FR-3:** Node hover styling uses accent tokens and does not override selected, error, active-flow, or muted visual states.
4. **FR-4:** Edge hover styling uses accent tokens and does not override selected or error visual states.
5. **FR-5:** Idle work-state node colors derive from canonical work-state phase/category data.
6. **FR-6:** Work-state phase colors preserve higher-priority active, selected, validation-error, and muted states.
7. **FR-7:** Topology save-failure notices are dismissible without mutating draft or save state.
8. **FR-8:** A new topology save failure re-displays the notice after dismissal.
9. **FR-9:** Prompt diagnostics remain layout-stable across idle, loading, empty, error, and diagnostic states inside the autocomplete/help disclosure.
10. **FR-10:** Work-type node selection either works accessibly or renders as intentionally non-interactive in unsupported modes.
11. **FR-11:** Current-selection expandable sections use shared disclosure primitives and consistent panel shells.
12. **FR-12:** Prose cleanup preserves critical warnings and validation information.

## Non-Goals

- No backend validation, factory runtime behavior, or OpenAPI contract changes.
- No graph layout algorithm changes or new graph editing tools.
- No workstation type-conversion behavior (see `prd-workstation-type-conversion-configuration.md`).
- No full dashboard design-system rewrite or broad color-token migration.
- No rewrite of all historical current-selection copy—only remaining visible low-value prose called out in current issues.

## Design considerations

- Reuse dashboard action buttons, tooltip buttons, `ExpandablePanelTrigger`, semantic color tokens, and graph node/edge primitives.
- Prefer icon-led toolbar actions with concise tooltips over long visible helper copy.
- Hover effects use border/stroke emphasis rather than layout-changing transforms.
- Reserve red/danger for errors and failed states; use accent for hover and selection emphasis.
- Place notice close actions consistently with other dashboard icon buttons.

## Technical considerations

- Hover and idle color styling stay in projection/component layers; do not mutate canonical graph data.
- Reuse `factory-graph-work-state-phase-styling` (or equivalent) for idle phase colors rather than duplicating mappings.
- Prompt diagnostics should use the existing reserved-region pattern and preserve screen-reader behavior for save-blocking issues.
- Standardized expandable sections must keep disclosure lint guard compatibility.
- Tests assert observable behavior and state precedence, not long class-string snapshots.

## Success metrics

- Operators identify graph toolbar actions from concise labels and tooltips without reading redundant words.
- Hovered graph items are visually obvious without masking selected or error states.
- Idle work-state nodes communicate phase through semantic color at a glance.
- Save-failure notices can be dismissed and reappear on later failures.
- Prompt editing no longer causes diagnostics-driven layout jumping.
- Work-type node selection matches visual affordance.
- Current-selection sections read as one coherent UI family with minimal clutter.

## Resolved decisions

- **Terminal/success work-state color:** Use the existing phase token map (`TERMINAL` → success semantic styling). Do not introduce a separate blue/info terminal treatment.
- **Helper prose to preserve:** Keep only actionable warnings, validation, overwrite, and save-blocking notices. Remove low-value explanatory paragraphs per website standards for dense surfaces.
