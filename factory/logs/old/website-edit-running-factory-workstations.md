# PRD: Website Editing for Running Factory Workstations

---
author: Codex
last modified: 2026, may, 9
status: draft
---

## Context

### Customer Ask

Enable the website to modify the currently running factory. A customer should be able to open the website, see the running factory, click a workstation in the grid, view that workstation's current model, template, prompt, and related editable values, make changes in the website, and save those changes back to the factory by downloading the current factory definition from the API, modifying it, and reuploading it through the existing API surface. The runtime event stream must also reflect the modification so the website and functional tests can observe the updated factory state through canonical events.

### Problem

The website currently loads and displays the running factory, but it does not provide an end-to-end editing workflow for the live factory definition. Customers can inspect the current state, but they cannot safely use the website as the place to update workstation configuration such as prompt or model settings. That leaves a gap between observing the running factory and managing it.

The immediate need is not broad factory administration. The first meaningful behavior is narrower: customers should be able to select an existing workstation from the current factory view, inspect its current configuration, edit prompt-oriented and model-oriented values, and save those edits back through the factory API. Without this flow, customers cannot quickly tune workstation behavior from the same surface where they already monitor the factory.

The current dashboard also depends on the canonical factory event stream for topology and state refresh. If the running factory changes but the runtime does not emit an appropriate structure-change event, the website may save successfully without reflecting the update in the live view. That would create drift between the API write path, the event-driven website state, and replay or functional-test expectations.

### Solution

Add a website editing flow centered on an existing workstation selected from the running factory grid. When a customer selects a workstation, the website should fetch or derive the latest editable factory definition for that running factory, populate a workstation editing panel with the current workstation fields, allow the customer to modify supported values, validate the edited values in the UI, and save by submitting an updated factory definition back through the existing factory API.

The first release should focus on editing one existing workstation at a time within the current website experience. It should support prompt, model, and template editing for existing workstations. The save action should live in the page header, require explicit confirmation before overwriting the running factory definition, and tell the customer when they are forcefully applying their workstation or factory-model changes over newer server state. The runtime must emit a new canonical factory-change event after a save so the website can refresh from the event stream, and the first version of that event should keep its payload shape mostly aligned with the existing initial-structure payload to limit migration risk.

## Project Acceptance Criteria

- [ ] A customer can open the website, view the running factory, click a workstation tile in the grid, and see that workstation's current editable configuration in the current selection area
- [ ] The workstation editing surface shows the current prompt, model, template, and other supported editable workstation values sourced from the current running factory definition
- [ ] The website edits are based on the latest factory definition fetched from the factory API rather than mutating only local display state
- [ ] A customer can change supported workstation values, save the edit, and cause the website to upload an updated factory definition back to the factory API
- [ ] The save action is exposed in the header and requires an explicit overwrite confirmation before the updated factory definition is submitted
- [ ] The UI provides explicit loading, error, success, and validation states for factory fetch and save behavior
- [ ] If the running factory changes while the customer has unsaved edits open, the UI informs the customer that saving will forcefully overwrite the current workstation or factory-model values with their pending changes
- [ ] Validation prevents clearly invalid edits from being submitted, and the customer receives actionable field-level or form-level error feedback
- [ ] Saving a workstation modification emits a new canonical runtime factory-change event that lets the website refresh the updated factory structure through the event stream
- [ ] The first release remains scoped to editing existing workstation configuration and does not require audit history, rollback, approvals, or multi-workstation batch publishing
- [ ] Backend, contract, frontend, and functional verification cover the fetch-edit-save behavior, selected-workstation editing experience, and emitted event-stream updates after modification

## Goals

- [ ] Let customers edit an existing workstation from the same website that displays the running factory
- [ ] Support prompt editing plus model and template-oriented workstation values in the first release
- [ ] Make the current factory definition the source of truth for edits by fetching it from the factory API before save
- [ ] Save edits by reuploading an updated factory definition through an explicit header-driven website workflow
- [ ] Keep the live website view synchronized through canonical runtime events after a save
- [ ] Keep the first release narrow, understandable, and safe enough to validate without broad factory-management scope

## User Stories

### US-001: Select a workstation and inspect its editable configuration (P1)

**Description:** As a customer, I want to click a workstation in the running factory grid and see its current editable configuration so that I can understand what I am about to change.

**Acceptance Criteria:**
- [ ] Clicking a workstation in the grid updates the current selection UI to show that workstation as the active target
- [ ] The selection UI shows the workstation's current prompt, model, template, and any other first-release editable values that exist in the fetched factory definition
- [ ] The first release editable values are limited to prompt, model, and template
- [ ] The selection UI has explicit loading, empty, and error handling when workstation details cannot yet be shown
- [ ] Keyboard-accessible selection behavior and visible focus states are present for workstation tiles and edit controls
- [ ] Responsive behavior preserves core usability on mobile and desktop layouts
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-002: Edit supported workstation fields in the website (P1)

**Description:** As a customer, I want to edit a workstation's prompt, model, and template in the website so that I can tune the running factory without leaving the UI.

**Acceptance Criteria:**
- [ ] The website provides editable controls for the first-release workstation fields: prompt, model, and template
- [ ] The edit form is initialized from the currently fetched factory definition rather than from hardcoded defaults
- [ ] The UI tracks unsaved changes for the selected workstation during the current editing session
- [ ] Validation errors are shown before save when required fields are missing or field values are structurally invalid
- [ ] Changing values in the form does not immediately mutate the running factory until the customer explicitly saves
- [ ] The primary save action for the current edit session is surfaced from the page header rather than buried inside the workstation form body
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-003: Save workstation edits by updating the factory definition through the API (P1)

**Description:** As a customer, I want my workstation edits to be saved back to the running factory through the API so that the current factory reflects the changes I made in the website.

**Acceptance Criteria:**
- [ ] On save, the website uses the latest available factory definition as the mutation source, applies the selected workstation edits to that definition, and submits the updated definition through the factory API
- [ ] Save requires an explicit confirmation step that warns the customer they are overwriting the running factory definition
- [ ] Successful saves show a clear success state and refresh the UI so the workstation view reflects the saved configuration
- [ ] Failed saves show a clear error state and preserve the customer's unsaved edits in the form so they can recover without retyping
- [ ] If the website detects newer running-factory state while edits are pending, the save confirmation clearly states that proceeding forcefully applies the customer's workstation changes over the newer runtime definition
- [ ] The save path remains scoped to one workstation edit session at a time and does not require multi-workstation batch publishing in the first release
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### US-004: Backend, contracts, and event stream support website-driven factory mutation safely enough for the first release (P1)

**Description:** As a maintainer, I want the backend, contract, and event surfaces to support the website's fetch-edit-save workflow so that the UI is built on typed, testable, contract-aligned behavior and live event projections stay correct after edits.

**Acceptance Criteria:**
- [ ] The factory API exposes or already supports a typed way to retrieve the current editable factory definition for the running factory
- [ ] The factory API exposes or already supports a typed way to submit an updated factory definition back to the running factory
- [ ] The implementation updates existing API surfaces as needed rather than introducing parallel edit-only endpoints unless a gap is proven
- [ ] Contract alignment between OpenAPI, generated code, backend behavior, and frontend typed API usage is explicitly covered in the implementation plan
- [ ] Validation and error responses are concrete enough for the UI to present actionable save failures
- [ ] The runtime emits a new canonical factory-change event after a successful factory-definition modification so event-stream consumers can observe the new structure
- [ ] The new factory-change event keeps its payload mostly aligned with the existing initial-structure payload in the first release so backend, replay, and UI migrations stay narrow
- [ ] Tests cover the happy path and at least one invalid-update or failed-save path for the fetch-edit-save workflow
- [ ] Typecheck passes
- [ ] Tests pass

### US-005: Event-stream-driven website refresh is proven by functional tests (P1)

**Description:** As a reviewer, I want functional evidence that a workstation edit updates the website through emitted canonical events so that the live dashboard behavior matches the runtime contract.

**Acceptance Criteria:**
- [ ] Functional tests perform a workstation modification through the supported API path and assert that the resulting canonical event stream contains the new factory-change event
- [ ] Functional tests prove the website updates its displayed workstation prompt, model, or template after replaying the emitted events rather than only after a direct REST refetch
- [ ] Functional tests cover at least one repeated-edit or overwrite scenario where a later modification produces another structure-refresh event
- [ ] Frontend event-stream tests or replay-harness coverage prove the dashboard reducer and UI accept the emitted post-edit event shape
- [ ] Typecheck passes
- [ ] Tests pass

### US-006: The editing workflow stays intentionally narrow in the first release (P2)

**Description:** As a product owner, I want the first release to stay focused on editing existing workstation configuration so that the team can ship the core behavior without taking on broad factory-management complexity.

**Acceptance Criteria:**
- [ ] The first release does not include create-workstation, delete-workstation, reorder-workstations, or whole-factory topology editing
- [ ] The first release does not include audit history, rollback, approvals, collaborative editing, or version-diff UX
- [ ] Any unsupported fields in the factory definition are either shown read-only or excluded from the editing surface rather than being silently overwritten
- [ ] Documentation or implementation notes identify the supported editable fields for the initial release
- [ ] Typecheck passes

## High-Level Technical Design

The existing website already understands how to load and display the running factory. This feature should extend that experience rather than introduce a separate editor product. The grid remains the navigation surface. Selecting a workstation opens or updates a detail panel in the current selection area, and that panel becomes the editing surface for the chosen workstation.

The source of truth for edits is the current factory definition retrieved from the factory API. The UI should not treat the currently rendered workstation card as the canonical state to mutate. Instead, the typed API layer should retrieve the running factory definition, the feature state should derive the selected workstation's editable fields from that definition, and save should construct an updated definition that preserves untouched factory content while replacing only the supported fields for the selected workstation.

The website should route network behavior through typed API modules and stateful hooks rather than inline requests in components. The editing surface should make loading, fetch failure, validation failure, save-in-progress, save-failure, and overwrite-confirmation states explicit. Form state should remain local to the editing feature or an explicit client-state owner, while server state should continue to use the repository's approved query and mutation patterns. The primary save affordance should live in the page header so it stays visible while the customer works through workstation fields.

On the backend side, the preferred shape is a clear boundary that lets the UI fetch the editable definition and submit an updated definition without embedding transport-specific logic in the frontend. The plan should prefer updating the existing API surfaces rather than adding a second family of edit-specific endpoints. After a successful modification, the runtime must emit a new canonical factory-change event that tells the dashboard and replay consumers that factory structure changed. For the first release, that event should preserve a payload shape that is mostly the same as the current initial-structure payload so reducers and replay logic can adopt the new event type without also needing a broad payload rewrite.

## Functional Requirements

1. FR-1: The website must allow a customer to select an existing workstation from the running factory grid.
2. FR-2: Selecting a workstation must populate the current selection area with that workstation's current editable values from the running factory definition.
3. FR-3: The first release editing surface must support prompt, model, and template for existing workstations.
4. FR-4: The website must obtain the current factory definition from the factory API as the source of truth for edits.
5. FR-5: The website must allow a customer to modify supported workstation fields without applying changes to the running factory until save is explicitly requested.
6. FR-6: The website must validate supported workstation fields before submitting a save request and must show actionable validation feedback when input is invalid.
7. FR-7: On save, the website must apply the selected workstation edits to the current factory definition and submit the updated definition back through the factory API.
8. FR-8: Successful saves must update the UI to reflect the saved workstation configuration.
9. FR-9: Failed saves must preserve in-progress edits and show a recoverable error state.
10. FR-10: API usage for this workflow must be typed, centralized, and aligned with generated contracts rather than ad hoc component-level requests.
11. FR-11: The primary save action for workstation edits must be exposed in the page header.
12. FR-12: The first release must explicitly handle loading, empty, error, success, and destructive-or-overwriting confirmation states where relevant to the editing flow.
13. FR-13: If the running factory changes while the customer has unsaved edits open, the UI must inform the customer that saving will forcefully apply their changes over the newer runtime definition.
14. FR-14: A successful factory-definition modification must emit a new canonical factory-change event that lets the website and replay consumers observe the resulting structure change.
15. FR-15: The new factory-change event must keep a payload shape that is mostly the same as the existing initial-structure payload in the first release unless a documented contract gap requires a small additive difference.
16. FR-16: Functional tests must verify both the API-side mutation and the emitted event-stream behavior that drives the website refresh after modification.
17. FR-17: The first release must remain limited to editing existing workstation configuration and must not expand into general-purpose whole-factory editing.

## Non-Goals

- No creation or deletion of workstations in the first release
- No editing of overall factory topology, orchestration wiring, or unrelated factory-wide resources unless they are required to preserve the selected workstation update
- No audit log, history browser, rollback, approval workflow, or collaborative editing
- No optimistic live mutation of the running factory while the user is still typing
- No multi-workstation batch editing or batch publish workflow
- No concurrency-control or version-history UX beyond basic first-release validation and error handling
- No unsupported-field mutation that silently rewrites unknown parts of the factory definition
- No requirement to add audit history or rollback as part of the event-stream change work

## Design Considerations

- The existing factory grid should remain the entry point for editing to keep the workflow discoverable in the surface customers already use.
- The workstation tile interaction should use semantic interactive elements, keyboard support, and visible focus styling.
- The current selection area should clearly separate read-only context from editable fields so customers know what will change on save.
- The save action should be available from the header and its confirmation step should clearly explain that the user is overwriting the running factory definition.
- Save, cancel, loading, overwrite-warning, and error states should be visually obvious and should not require the customer to infer network state.
- On smaller screens, the selection and editing flow must remain usable without horizontal scrolling for standard form interactions.

## Technical Considerations

- Prefer a vertically sliced implementation across `ui/src/api`, `ui/src/features`, and shared UI components rather than page-local fetch and form logic.
- Frontend server state should use approved query and mutation abstractions, and client edit state should live in explicit form state rather than being mixed into display-only store state.
- OpenAPI, generated contracts, backend handlers, and frontend typed wrappers must stay aligned while evolving the existing API surface for this workflow.
- Save behavior should update only the targeted workstation fields while preserving untouched portions of the fetched factory definition.
- Validation responsibilities should be explicit about what is enforced in the browser, what is enforced by the API, and how backend validation errors map to UI feedback.
- The new factory-change event contract must stay coherent for live SSE consumers, historical replay, and selected-tick projection behavior.
- The first-release factory-change payload should reuse the existing initial-structure fields and meanings wherever practical, preferring additive metadata over structural divergence.
- Tests should include frontend component or integration coverage for the workstation selection and edit UX, backend or contract coverage for definition fetch and update behavior, and functional coverage for post-save emitted event streams.
- If the running factory view refreshes in the background, the editing workflow should define how it avoids clobbering the user's unsaved form state during the current session.

## Success Metrics

- A customer can complete the path from workstation selection to saved prompt or model update entirely within the website
- The website becomes a viable first-stop editing surface for common workstation tuning tasks on a running factory
- Save failures return actionable feedback instead of forcing the customer to guess what went wrong
- The implementation ships with contract-aligned backend, frontend, and functional coverage for the fetch-edit-save flow and its resulting event-stream updates

## Open Questions

- Whether the overwrite warning should appear only at save time or also as a persistent banner immediately when newer runtime structure is detected during an active edit session
