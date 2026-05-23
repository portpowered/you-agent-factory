# PRD: Factory Graph Editor Observer Parity

## Introduction

Improve the factory graph editor so it feels substantially closer to the existing observer view instead of behaving like a separate, denser, harder-to-read graph surface. The current editor exposes a different node presentation, different edge and anchor behavior, a separate lane-based layout, and a confusing floating action button set. Those differences make editing feel more like manipulating an internal topology debugger than editing the same workflow customers already understand from observe mode.

This work should bring the editor and observer views closer together visually and structurally while keeping the current in-graph editing model. The scope is intentionally narrower than a full editor-shell redesign. We will not introduce a separate editor section or a wholly new page composition in this phase. Instead, we will improve the existing graph surface by reusing observer-oriented graph components and layout behavior wherever practical, simplifying the floating actions, suppressing edge labels until they are contextually useful, and fixing incorrect anchor routing so workstation outcomes connect through the right semantic handles.

## Goals

- Make the graph editor feel visually and behaviorally similar to the observer view.
- Reuse shared node, edge, and anchor rendering pieces between observe and edit flows wherever practical in this phase.
- Replace editor-only graph layout behavior with observer-layout reuse through a thin editor adapter.
- Simplify the editor floating action controls so each action is obvious and directly usable.
- Reduce graph noise by hiding edge labels except when they are contextually relevant.
- Fix incorrect edge anchoring so workstation route edges originate from the correct semantic anchors such as success, failure, reject, and continue.
- Keep the existing graph-in-place editing workflow rather than introducing a new split editor shell.

## User Stories

### US-001: Simplify the Editor Floating Action Buttons
**Description:** As a customer editing a factory graph, I want the floating action buttons to be concise and unambiguous so that I can tell what each control does without guessing.

**Acceptance Criteria:**
- [ ] The editor floating action controls remove the current non-functional `Add` button.
- [ ] The `+` action remains as the single add-entry control.
- [ ] The `Delete` action is represented with an icon button consistent with the other FAB controls.
- [ ] The `Connect` action is represented with an icon button consistent with the other FAB controls.
- [ ] Each FAB control has an accessible name, tooltip, visible focus state, and pressed state where applicable.
- [ ] FAB ordering and spacing remain usable on mobile and desktop widths.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-002: Add Visibility Presets as Button-Based Graph Focus Controls
**Description:** As a customer editing a dense graph, I want quick graph visibility presets so that I can focus on workflow, execution, or infrastructure without manually toggling lanes one at a time.

**Acceptance Criteria:**
- [ ] The editor replaces the current worker/resource visibility treatment with button-style visibility presets.
- [ ] The preset set includes `All`, `Workflow`, `Execution`, and `Infrastructure`.
- [ ] Preset controls use the same visual button family as the rest of the editor controls rather than a separate panel treatment.
- [ ] `Workflow` emphasizes workstations and core workflow progression.
- [ ] `Execution` emphasizes route and state relationships used to understand outcomes and transitions.
- [ ] `Infrastructure` emphasizes workers and resources that support the workflow.
- [ ] Changing presets updates the displayed graph without losing pending draft edits.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-003: Suppress Edge Labels Until They Are Contextually Relevant
**Description:** As a customer viewing the graph editor, I want edge labels hidden by default so that the graph is readable, but I still want route labels available when I am actively inspecting or creating a connection.

**Acceptance Criteria:**
- [ ] Edge labels are not always visible in the editor default state.
- [ ] Edge labels appear on hover or selection.
- [ ] Edge labels appear while actively creating or inspecting a connection.
- [ ] Suppressing labels does not remove the semantic distinction between connection types.
- [ ] The graph continues to expose accessible labels for screen-reader or interaction semantics even when inline labels are hidden.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-004: Reuse Observer-Style Node Presentation in the Editor
**Description:** As a customer editing the graph, I want editor nodes to use the same larger, clearer presentation as observer nodes so that I do not need to learn a second visual language to edit the same factory.

**Acceptance Criteria:**
- [ ] Editor nodes use the same or directly shared node shell, sizing approach, and visual hierarchy as observer nodes wherever practical.
- [ ] Editor workstation nodes are larger and closer in density to observer workstation nodes.
- [ ] Editor node typography, badges, and spacing no longer use a separate compressed formatting style when a shared observer pattern exists.
- [ ] Any editor-only state such as pending addition or pending removal layers on top of the shared base presentation rather than replacing it wholesale.
- [ ] Shared observer/editor node rendering does not regress pending-change visibility.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-005: Fix Semantic Edge Anchor Mapping for Workstation Outcomes
**Description:** As a customer editing workflow routes, I want success, failure, reject, and continue edges to originate from the correct anchors so that the graph matches the actual factory semantics.

**Acceptance Criteria:**
- [ ] Workstation route edges no longer visually originate from a single generic source anchor when they represent different outcomes.
- [ ] `Success`, `Failure`, `Reject`, and `Continue` edges render from their respective semantic anchors.
- [ ] Edge creation in connect mode uses the same semantic anchor mapping as edge display.
- [ ] Pending draft edges and existing saved edges follow the same anchor rules.
- [ ] Anchor corrections preserve the saved factory-definition semantics for each route type.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-006: Reuse Observer Layout Behavior Through an Editor Adapter
**Description:** As a customer editing the graph, I want the graph layout to feel like the observer view so that movement between monitoring and editing does not require re-learning the topology structure.

**Acceptance Criteria:**
- [ ] The editor stops relying on its current fixed lane layout when observer layout primitives can provide the same placement more clearly.
- [ ] The implementation reuses the observer layout behavior through a thin editor adapter rather than maintaining a fully separate editor layout system.
- [ ] Editor-specific needs such as hidden infrastructure presets or pending draft entities are handled without forking the shared layout logic unnecessarily.
- [ ] The editor layout preserves usable positioning for selection, dragging, and connection interactions.
- [ ] Layout reuse does not regress observer-view layout behavior.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-007: Share Graph Rendering Components Between Observe and Edit Modes
**Description:** As a maintainer, I want the observer and editor views to share graph rendering components so that visual parity fixes do not require parallel maintenance in disjoint implementations.

**Acceptance Criteria:**
- [ ] The implementation identifies and extracts shared graph rendering seams for nodes, edges, anchors, or related graph-surface presentation where reuse is practical in this phase.
- [ ] Observe and edit modes no longer maintain unnecessarily separate implementations for the same visual graph concepts.
- [ ] Editor-specific behavior remains layered through props or adapters rather than duplicating base presentation components.
- [ ] Shared abstractions remain readable and do not collapse observe and edit logic into one opaque component.
- [ ] Existing observer behavior remains intact after refactoring.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-008: Preserve Existing In-Place Editing Workflow Without a New Editor Shell
**Description:** As a stakeholder, I want parity improvements delivered without a separate editor section so that we improve clarity first without widening the surface area of this feature.

**Acceptance Criteria:**
- [ ] The implementation keeps the current editor embedded in the existing factory graph card rather than introducing a dedicated editor page section or split-pane inspector workflow.
- [ ] Parity improvements are applied within the existing graph card and toolbar/FAB interaction model.
- [ ] Any structural cleanup needed for shared components stays scoped to parity, layout, anchor correctness, and control simplification.
- [ ] The implementation does not add a separate editor-only shell architecture in this phase.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

## Functional Requirements

1. FR-1: The editor must remove the current non-functional `Add` FAB action and keep `+` as the add-entry control.
2. FR-2: The editor must represent `Delete` and `Connect` as icon buttons consistent with the rest of the FAB controls.
3. FR-3: The editor must provide button-based graph visibility presets for `All`, `Workflow`, `Execution`, and `Infrastructure`.
4. FR-4: Visibility presets must update only graph presentation and must not discard or mutate pending draft changes.
5. FR-5: The editor must suppress inline edge labels by default.
6. FR-6: The editor must reveal edge labels on hover, selection, and while actively creating or inspecting a connection.
7. FR-7: The editor must retain accessible edge naming even when inline labels are visually suppressed.
8. FR-8: The editor must reuse observer-style node presentation patterns wherever practical, including larger node sizing and shared visual hierarchy.
9. FR-9: Editor-only draft states such as pending addition or pending removal must layer on top of shared node presentation rather than replace it with a separate base format.
10. FR-10: Workstation route edges must render from the correct semantic anchors for success, failure, reject, and continue.
11. FR-11: Connect-mode edge creation must use the same semantic anchor mapping as displayed edges.
12. FR-12: The editor must reuse observer layout behavior through a thin adapter rather than maintaining a wholly separate editor lane layout when shared primitives can support the need.
13. FR-13: Shared observer layout reuse must continue to support editor interactions including selection, dragging, fit-view behavior, and connection gestures.
14. FR-14: The implementation must reuse shared graph rendering components for nodes, edges, anchors, or related presentation seams wherever practical in this phase.
15. FR-15: Shared graph rendering abstractions must preserve clear ownership of observer-only behavior versus editor-only behavior through explicit props, adapters, or wrappers.
16. FR-16: The implementation must keep the current editor embedded in the existing factory graph card and must not introduce a separate editor section in this phase.
17. FR-17: All UI changes must preserve accessible semantics, keyboard interaction, visible focus states, and responsive behavior.

## Non-Goals

- No new separate editor page section, split-pane editor shell, or inspector-first redesign in this phase.
- No full unification of the entire observe and edit rendering pipelines if that would require a broader architecture project.
- No backend or API contract redesign for graph editing beyond what is already required by the current editor.
- No expansion of editing scope into new entity types beyond the existing editor’s supported graph entities.
- No wholesale replacement of the current save/discard workflow.

## Design Considerations

- The editor should feel like the observer view with editing affordances layered on top, not like a different product surface.
- The FAB should prioritize clarity over density. If an action is not independently meaningful, it should not have its own button.
- Visibility presets should read as fast focus modes rather than low-level density toggles.
- Pending draft states should remain visible, but they should not force a radically different node visual language.
- Edge labels should become contextual detail, not persistent visual clutter.

## Technical Considerations

- Reuse existing observer graph node, edge, anchor, and layout components wherever practical before creating editor-only replacements.
- Prefer thin adapter layers that feed editor draft state into shared observer-oriented rendering primitives.
- Keep editor-specific semantics explicit so shared components do not become hard to reason about.
- Layout reuse should start from the observer graph layout path and add the minimum editor-specific adaptation needed for hidden presets, draft overlays, and connection handles.
- Tests should emphasize observable UI behavior: control visibility, label suppression behavior, anchor correctness, preset filtering, and layout parity expectations.

## Success Metrics

- Customers can switch between observe and edit modes without feeling that they are learning a second graph interface.
- The editor graph is easier to scan because inline edge labels no longer dominate the canvas.
- The FAB no longer includes controls that appear redundant or non-functional.
- Outcome routes visibly map to the correct workstation anchors, reducing confusion about factory semantics.
- Shared rendering components reduce future drift between observe and edit graph surfaces.

## Open Questions

- How closely can the current editor reuse observer edge rendering before editor-specific connection affordances require a dedicated wrapper?
- Do the `Workflow` and `Execution` presets need more explicit visual definitions once implemented in Storybook, or is the initial behavior clear enough from the graph itself?
- Should some edge labels remain persistently visible for very small graphs, or should contextual display remain consistent regardless of topology size?
