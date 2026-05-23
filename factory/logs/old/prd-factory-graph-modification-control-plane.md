# PRD: Factory Graph Modification Control Plane

---
author: Codex
last modified: 2026, may, 17
status: draft
related:
  - factory/logs/old/website-edit-running-factory-workstations.md
---

## Introduction

Extend the website factory graph from a primarily observational and workstation-editing surface into a modification control plane for the running factory. Customers should be able to use the React graph view to create, remove, and connect factory entities such as workstations, work states, work types, resources, and workers, then save those topology changes back through the factory API with event-stream updates that keep the live dashboard and replay consumers synchronized.

This PRD builds on the prior workstation-editing draft. That draft focused on editing fields on an existing workstation. This extension broadens the scope to graph-level topology modification while preserving the same product direction: the website should mutate the editable factory definition through typed API contracts, not by changing only local display state.

The implementation should use the existing full-definition editing pattern: download the current running factory definition, apply graph modifications to that full definition, and save the new complete definition. The graph should continue to use React Flow and extend its node handles, context menu, and interaction patterns where needed.

The editing entry point should be a visible icon control on the React Flow factory graph panel. Activating that control enters editor mode. In editor mode, the graph shows a bottom floating tool panel inspired by Figma-style editing toolbars. The first version of that panel should expose three primary tools: Add, Delete, and Connect.

## Goals

- Allow customers to switch the factory graph into an explicit modification mode.
- Support adding and removing workstations from the running factory definition.
- Support adding work states and work types used by factory work routing.
- Support adding and removing edges between workstations, work states, work types, resources, and workers where the factory model allows those relationships.
- Expose anchor points on graph nodes so customers can draw new valid edges directly in the graph.
- Expose workers in the graph so worker assignment and routing relationships are visible and editable.
- Save graph topology changes through typed factory API mutations and refresh the live UI through canonical factory-change events.
- Prevent factory topology edits while active work is running.
- Use hybrid logical timestamps, Lamport-style logical counters plus physical clock values, to detect and explain stale edits.
- Provide validation, confirmation, loading, error, and success states for all destructive or structure-changing actions.
- Provide reusable editor menu, popup, toolbar, and confirmation primitives that reuse existing shadcn-style components wherever available.

## User Stories

### US-001: Enter Graph Modification Mode

**Description:** As a customer, I want an explicit graph modification mode so that I can safely distinguish normal monitoring from factory editing.

**Acceptance Criteria:**
- [ ] The React Flow factory graph panel exposes an icon button for entering editor mode.
- [ ] The editor-mode icon button has an accessible name, visible focus state, and tooltip.
- [ ] Entering editor mode displays a bottom floating editor toolbar similar in density and placement to a Figma tool strip.
- [ ] The first bottom toolbar includes exactly three primary tool buttons: Add, Delete, and Connect.
- [ ] Normal monitoring interactions remain available when modification mode is off.
- [ ] Creation, deletion, edge dragging, and topology save controls are unavailable or disabled when modification mode is off.
- [ ] The UI indicates when there are unsaved graph changes.
- [ ] Leaving modification mode with unsaved changes requires the user to save, discard, or cancel.
- [ ] Keyboard-accessible controls and visible focus states are present.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-002: Add a Workstation

**Description:** As a customer, I want to add a workstation from the graph so that I can expand the running factory without editing configuration files by hand.

**Acceptance Criteria:**
- [ ] The Add tool opens a contextual menu or popover for choosing which entity to add.
- [ ] The Add menu includes workstation, worker, work type, work state, and resource options where those entity types are supported by the current factory definition.
- [ ] Modification mode exposes an add-workstation action through the Add tool menu.
- [ ] The user can provide required workstation fields such as name or identifier, work type support, template, prompt, and model where applicable.
- [ ] The UI validates duplicate names, missing required fields, and structurally invalid values before save.
- [ ] A newly added workstation appears in the graph as a pending unsaved change.
- [ ] Saving submits an updated factory definition through the typed factory API.
- [ ] A successful save emits a canonical factory-change event and refreshes the graph from the event stream.
- [ ] Failed saves preserve the pending workstation and show actionable errors.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-003: Remove a Workstation

**Description:** As a customer, I want to remove an existing workstation from the graph so that I can simplify or repair a running factory topology.

**Acceptance Criteria:**
- [ ] Modification mode exposes a remove action for workstation nodes.
- [ ] Removing a workstation requires explicit destructive confirmation.
- [ ] The confirmation summarizes affected edges and related references that will be removed or invalidated.
- [ ] The UI prevents removal when the backend says the workstation is required or currently unsafe to delete.
- [ ] Pending removal is visually distinct before save.
- [ ] Saving removes the workstation and any approved dependent topology changes from the factory definition.
- [ ] Failed saves restore the previous rendered topology and preserve useful error details.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-004: Add Work States and Work Types

**Description:** As a customer, I want to create work states and work types in the graph editor so that I can define new categories of work and routing states used by the factory.

**Acceptance Criteria:**
- [ ] Modification mode exposes add actions for work state and work type nodes.
- [ ] The creation form clearly distinguishes work states from work types.
- [ ] The UI explains that work belongs to a work type, and each work type defines the supported ordered work states for that type.
- [ ] The UI validates required identifiers, display labels, and duplicate values before save.
- [ ] The UI validates that a work state is associated with a work type before save.
- [ ] Newly created work states and work types appear in the graph and can be connected to compatible nodes before save.
- [ ] Saving persists new work states and work types through the factory definition API.
- [ ] Backend validation errors map to field-level or form-level UI messages.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-005: Expose Edge Anchor Points

**Description:** As a customer, I want compatible anchor points on graph nodes so that I can drag edges between factory entities without guessing what relationships are allowed.

**Acceptance Criteria:**
- [ ] Nodes expose visible anchor points while modification mode is active.
- [ ] Anchor points are hidden, subdued, or read-only while modification mode is off.
- [ ] Anchor points communicate allowed incoming and outgoing connection types.
- [ ] Workstation nodes expose separate anchors for `onSuccess`, `onFailure`, `onReject`, and `onContinue` relationships where those transitions are supported.
- [ ] Workstation anchors model input and output gates explicitly rather than treating every workstation edge as the same relationship type.
- [ ] Dragging from an anchor previews a potential edge.
- [ ] Invalid targets are rejected before save with clear visual feedback.
- [ ] Keyboard-accessible edge creation is available through an alternate action path.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-006: Add and Remove Edges

**Description:** As a customer, I want to add and remove edges between workstations, work states, work types, resources, and workers so that I can change how work flows through the factory.

**Acceptance Criteria:**
- [ ] The Connect tool activates edge creation mode in the bottom editor toolbar.
- [ ] The graph supports creating valid edges by dragging between compatible anchor points.
- [ ] Connecting from a workstation transition anchor updates the corresponding input/output gate relationship in the draft factory definition.
- [ ] The Delete tool activates node or edge deletion behavior in the bottom editor toolbar.
- [ ] The graph supports removing existing edges in modification mode.
- [ ] Removing an edge requires confirmation when it may interrupt active work routing or worker availability.
- [ ] The UI validates edge compatibility before save and shows actionable errors for invalid connections.
- [ ] Pending edge additions and removals are visually distinct.
- [ ] Saving updates the factory definition and emits a canonical factory-change event.
- [ ] Failed saves preserve pending graph edits and expose recoverable error feedback.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-007: Show Workers in the Factory Graph

**Description:** As a customer, I want workers to appear in the factory graph so that I can understand which workers are attached to workstations, work states, resources, or work types.

**Acceptance Criteria:**
- [ ] Worker nodes are represented in the graph with clear visual differentiation from workstations and work states.
- [ ] Worker nodes show enough status to support topology decisions, such as active, idle, errored, or unavailable if those states exist in current data.
- [ ] Worker relationships are represented as edges where the factory model supports them.
- [ ] Modification mode allows users to edit worker-to-workstation relationships.
- [ ] Modification mode allows users to denote which work types or supported work categories a worker handles, where the factory definition supports that relationship.
- [ ] Workers can be filtered or collapsed if the graph becomes too dense.
- [ ] Existing monitoring behavior for workers is preserved.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-008: Save, Discard, and Refresh Graph Changes

**Description:** As a customer, I want reliable save and discard controls for graph modifications so that I can experiment without accidentally changing the running factory.

**Acceptance Criteria:**
- [ ] The header or graph editor shell exposes save and discard actions while unsaved graph changes exist.
- [ ] Save requires explicit confirmation summarizing created, deleted, and changed graph entities.
- [ ] Discard reverts the graph to the latest known server-backed factory topology.
- [ ] If the server topology changes while local edits are pending, the UI warns that saving will overwrite newer state unless the save is blocked by active work or stale timestamp validation.
- [ ] If active work exists in the factory, the UI disables topology save and explains that factory editing is unavailable while work is active.
- [ ] Successful saves clear the dirty state and refresh from the emitted factory-change event.
- [ ] The UI provides explicit loading, success, error, and stale-state handling.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-009: Backend and Contract Support for Topology Mutation

**Description:** As a maintainer, I want typed API and runtime support for graph topology mutations so that the website does not rely on ad hoc local graph edits.

**Acceptance Criteria:**
- [ ] The OpenAPI contract exposes or confirms typed endpoints for retrieving the editable running factory definition.
- [ ] The OpenAPI contract exposes or confirms typed endpoints for submitting a full updated factory definition.
- [ ] Backend validation rejects invalid nodes, invalid edges, duplicate identifiers, dangling references, and unsupported deletion attempts.
- [ ] Backend validation rejects topology modifications while the running factory has active work.
- [ ] Definition fetch and save responses carry hybrid logical timestamp metadata, including a Lamport-style logical component and physical timestamp component.
- [ ] Error responses include enough structure for the website to map errors to graph nodes, edges, or form fields.
- [ ] Successful topology mutations emit a canonical factory-change event.
- [ ] Event payloads include enough graph structure for live dashboard reducers and replay consumers to rebuild topology.
- [ ] Backend, OpenAPI generation, frontend API wrappers, and tests remain contract-aligned.
- [ ] Tests pass.

### US-010: Reusable Editor Menus, Popups, and Toolbar Components

**Description:** As a frontend maintainer, I want reusable menu, popup, toolbar, and confirmation components for graph editing so that factory editor controls stay consistent and can be reused across future graph surfaces.

**Acceptance Criteria:**
- [ ] The bottom editor toolbar is implemented as a reusable component with typed props for active tool, disabled state, pending-change state, and callbacks.
- [ ] The Add menu is implemented with reusable menu or popover primitives rather than page-local bespoke markup.
- [ ] Delete confirmations reuse the existing dialog/confirmation component pattern.
- [ ] Existing shadcn-style components under `ui/src/components/ui/` are reused where available before adding new primitives.
- [ ] Any newly needed menu, popover, tooltip, or toolbar primitive is added under the shared UI component layer with stories or tests appropriate to the repository pattern.
- [ ] The toolbar and popups are keyboard reachable, screen-reader named, and usable without right-click.
- [ ] Typecheck passes.
- [ ] Verify in browser using dev-browser skill.

### US-011: Frontend Tests for Graph Editing Operations

**Description:** As a reviewer, I want component and integration tests for graph editing operations so that add, delete, and connect behavior is verified before wiring to a live backend.

**Acceptance Criteria:**
- [ ] Tests prove that entering editor mode shows the bottom toolbar with Add, Delete, and Connect controls.
- [ ] Tests prove that Add can create at least one workstation, worker, work type, work state, and resource draft through the contextual menu where supported.
- [ ] Tests prove that saving an add operation performs the expected mocked network request with a full updated factory definition.
- [ ] Tests prove that Delete marks a selected node or edge for removal and that save performs the expected mocked network request.
- [ ] Tests prove that Connect creates an edge between compatible anchors and updates the corresponding input/output gate relationship in the draft definition.
- [ ] Tests cover `onSuccess`, `onFailure`, `onReject`, and `onContinue` workstation anchors.
- [ ] Tests cover success, failure, reject, and continue outcomes for mocked save behavior where those callback states exist in the UI flow.
- [ ] Tests verify `onSuccess`, `onFailure`, `onReject`, and `onContinue` callbacks or state transitions as separate behaviors rather than a single generic completion path.
- [ ] Tests use mocked network behavior and do not require a live backend.
- [ ] Typecheck passes.
- [ ] Tests pass.

### US-012: Storybook Coverage for Editor Mode Graph States

**Description:** As a designer and maintainer, I want Storybook stories for workstation editing states so that graph editor behavior can be inspected without running a full factory.

**Acceptance Criteria:**
- [ ] Storybook includes an observe-mode factory graph story with editor controls hidden or disabled.
- [ ] Storybook includes an editor-mode story showing the bottom Add/Delete/Connect toolbar.
- [ ] Storybook includes stories for workstation nodes with `onSuccess`, `onFailure`, `onReject`, and `onContinue` anchors visible.
- [ ] Storybook includes stories for Add menu open state and Delete confirmation state.
- [ ] Storybook includes stories for Connect mode with a pending edge preview or selected source anchor.
- [ ] Storybook includes examples for active-work-blocked editing and stale timestamp warning states.
- [ ] Storybook stories use representative factory definitions and mocked network handlers where network behavior is demonstrated.
- [ ] Storybook test runner or story interaction tests cover the primary toolbar controls.
- [ ] Typecheck passes.
- [ ] Storybook checks pass.

## Functional Requirements

1. FR-1: The website must provide an explicit graph modification mode separate from normal monitoring mode.
2. FR-2: The graph must support right-click or equivalent context actions for node and canvas operations, with keyboard-accessible alternatives.
3. FR-3: The React Flow factory graph panel must expose an icon button for entering editor mode.
4. FR-4: Editor mode must show a bottom floating toolbar with Add, Delete, and Connect as the first-release primary tools.
5. FR-5: The Add tool must open a contextual menu or popover for choosing workstation, worker, work type, work state, resource, or other supported factory entity types.
6. FR-6: The graph must support adding workstation nodes with validated required fields.
7. FR-7: The graph must support removing workstation nodes with destructive confirmation and dependency summaries.
8. FR-8: The graph must support adding work state nodes.
9. FR-9: The graph must support adding work type nodes.
10. FR-10: Work type editing must support defining the ordered work states that work of that type may move through.
11. FR-11: The graph must expose workers as graph nodes or grouped graph entities.
12. FR-12: Worker graph editing must support assigning workers to workstations.
13. FR-13: Worker graph editing must support denoting which work types or supported work categories a worker handles, where supported by the factory definition.
14. FR-14: The graph must expose resources as connectable graph entities when resources participate in factory topology.
15. FR-15: Nodes must expose anchor points for valid incoming and outgoing relationships while modification mode is active.
16. FR-16: Workstation nodes must model `onSuccess`, `onFailure`, `onReject`, and `onContinue` as separate anchors where those transitions are supported.
17. FR-17: The graph must support dragging new edges between compatible anchor points.
18. FR-18: Connect operations from workstation anchors must modify the appropriate input/output gate relationship in the draft factory definition.
19. FR-19: The graph must support removing existing edges in modification mode.
20. FR-20: The UI must validate graph changes before submission and prevent known-invalid topology from being saved.
21. FR-21: The save flow must submit a full updated factory definition through typed API modules and stateful hooks, not component-local ad hoc requests.
22. FR-22: The save flow must preserve untouched factory definition content when applying graph topology changes.
23. FR-23: The UI must expose pending additions, removals, and edge changes visually before save.
24. FR-24: Save confirmation must summarize the pending graph changes.
25. FR-25: Failed saves must preserve local pending edits and provide actionable feedback.
26. FR-26: Successful saves must emit canonical runtime events that refresh the graph through the same live projection path used by monitoring.
27. FR-27: The system must prevent topology edits from being saved while active work exists in the running factory.
28. FR-28: Definition fetch, save, and stale-edit detection must use hybrid logical timestamps with Lamport-style logical counters plus physical timestamp values.
29. FR-29: The implementation must continue to use React Flow for the graph and extend it with custom handles, context menus, and node/edge controls as needed.
30. FR-30: The implementation must include frontend, backend, contract, and functional verification for graph mutation behavior.
31. FR-31: Editor toolbar, add menu, delete confirmation, and related popups must be reusable components that reuse existing shadcn-style UI primitives where available.
32. FR-32: Functional component tests must verify add, delete, and connect operations with mocked network behavior.
33. FR-33: Storybook must include editor-mode graph stories for toolbar behavior, Add menu behavior, Delete confirmation, Connect mode, and workstation transition anchors.
34. FR-34: The graph editor must remain usable on mobile, tablet, and desktop layouts, even if advanced drag editing is supplemented by form-based alternatives on small screens.

## Non-Goals

- No multi-user collaborative graph editing in this phase.
- No visual diff history, rollback browser, or approval workflow in this phase.
- No automatic topology optimization or AI-suggested rewiring in this phase.
- No guarantee that every factory definition field becomes editable in the graph.
- No live mutation while the user is dragging or typing; changes remain pending until explicit save.
- No requirement to preserve unsupported unknown topology changes if the backend contract cannot safely round-trip them; such limitations must be documented before implementation.
- No custom graph engine rewrite; continue using React Flow and document any limitations discovered during implementation.
- No topology editing while active work exists in the running factory.
- No permission gates in this phase because the product does not currently have a permissions model.

## Design Considerations

- Modification mode should feel like a work surface, not a landing page. The graph remains the primary screen.
- React Flow should remain the graph foundation. Custom handles, node renderers, edge renderers, context menus, and keyboard alternatives should be layered into the existing graph rather than replacing it.
- Editor mode should be entered through an icon button on the factory graph panel.
- Editor mode should show a bottom floating toolbar, visually similar to a compact Figma tool strip, with a dark or high-contrast surface if it matches the existing design system.
- The first toolbar tools are Add, Delete, and Connect. Save and discard may remain in the header or editor shell rather than competing with graph manipulation tools.
- The Add button should open a contextual menu or popover listing supported entity types.
- Delete should operate on the current selected node or edge and use confirmation when destructive.
- Connect should put the graph into an explicit connection mode and emphasize available anchors.
- Context menus should be available by right-click, long-press, or an explicit more-actions button for accessibility and touch support, but should not be the only way to access core Add/Delete/Connect behavior.
- Toolbar actions should use familiar icons with accessible names and tooltips.
- Node types should be visually distinct without relying on color alone.
- Anchor points should appear only when useful so normal monitoring remains calm and readable.
- Workstation transition anchors should be visually distinct enough that `onSuccess`, `onFailure`, `onReject`, and `onContinue` can be inspected and connected without ambiguity.
- Pending graph changes should be visible but restrained: created nodes, deleted nodes, added edges, and removed edges should each have distinct states.
- Dense worker/resource graphs should support filtering, grouping, or collapsing to avoid overwhelming the view.
- Destructive actions should use confirmation dialogs that name the affected entity and summarize downstream impact.
- Mobile users should have a non-drag fallback for creating edges, such as selecting a source node, choosing "connect", then selecting a compatible target.

## Technical Considerations

- Follow `docs/internal/standards/code/general-website-standards.md`: typed API modules, stateful hooks, explicit loading/error/success states, accessible controls, and responsive verification.
- Reuse shared shadcn-style primitives from `ui/src/components/ui/` where available, including button and dialog patterns. Add reusable shared primitives for menu, popover, tooltip, or toolbar behavior only when the existing component layer does not already provide them.
- Treat server state and client edit state separately. Server topology should come from API/query/event projections; pending graph edits should live in explicit graph editor state.
- Use the existing full-definition fetch-edit-save model: fetch the current factory definition, apply graph changes to the full definition, and submit the complete updated definition back through the factory API.
- Define a stable graph view model that maps factory concepts to graph nodes, handles, and edges without leaking rendering-library details into API code.
- Define compatibility rules for allowed edge types in one shared or mirrored validation layer so UI prevalidation and backend enforcement do not drift.
- Model workstation `onSuccess`, `onFailure`, `onReject`, and `onContinue` as separate graph handles and as separate draft-definition mutation targets so tests can verify each relationship independently.
- Extend canonical factory-change events to include enough topology data for node and edge changes, worker/resource visibility, and replay correctness.
- Use hybrid logical timestamps for concurrency, with a Lamport-style logical component and an actual physical timestamp component, similar in spirit to CockroachDB's timestamp model.
- Reject topology saves while active work exists. The UI may still allow draft edits, but save must be disabled or rejected until active work drains.
- Ensure deletion semantics are explicit. Because active-work edits are disallowed, deletion can focus on configuration dependencies such as edges, worker assignments, resource references, and work type support.
- Include tests for graph reducer/view-model logic, UI interactions, backend validation, OpenAPI contract generation, and event-stream replay.
- Frontend tests should mock network requests and assert that add, delete, and connect operations produce the expected full-definition save payload.
- Storybook stories should document observe mode, editor mode, Add menu, Delete confirmation, Connect mode, active-work-blocked state, stale timestamp state, and workstation transition anchors.
- Verify graph interactions with functional browser tests at desktop and at least one constrained/mobile viewport.

## Success Metrics

- A customer can add a workstation, connect it to valid graph entities, save, and see the updated topology reflected through live events.
- A customer can enter editor mode from an icon on the factory graph panel and use a bottom toolbar for Add, Delete, and Connect.
- A customer can remove a workstation or edge only after seeing a clear confirmation of impact.
- A customer can create work states or work types without editing factory files manually.
- A customer can assign workers to workstations and denote which work categories they handle through graph relationships.
- A customer can connect workstation `onSuccess`, `onFailure`, `onReject`, and `onContinue` paths without those relationships collapsing into one generic edge type.
- Invalid topology edits are caught before or during save with actionable errors.
- Attempts to save topology changes while active work exists are blocked with a clear explanation.
- Functional tests prove that saved topology changes update the website through canonical event replay, not only through direct REST refetch.

## Open Questions

- Are resources required in the first release, or should the first visible graph scope be workstations, work states, work types, and workers?
- Should workstation deletion be hard delete, disabled delete, or soft disable when no active work exists but historical work references the workstation?
- Which exact active-work states should block topology editing, and which terminal or paused states are safe?
- Which worker-to-workstation and worker-to-work-type relationships are required in the first editable release versus later releases?
