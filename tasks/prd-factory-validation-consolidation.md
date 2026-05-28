# PRD: Factory Validation Consolidation

## Introduction

Factory validation is currently spread across API request decoding, service-level editable factory checks, config validation, mapper validation, topology validation, and runtime defensive checks. This makes API error responses inconsistent and allows invalid factory definitions to pass one save path while failing later during runtime execution.

Consolidate factory validation into a canonical backend validator and expose it through a first-class validation API that accepts a factory definition and returns structured targets. The website must use those targets while customers edit the current activity graph so invalid workstations, work types, work states, resources, workers, edges, and exact factory component locations can be marked directly in the graph without blocking edit operations.

The desired outcome is that customers can see validation failures where they occur. For example, if a workstation lacks a reject or failure output, the reject/failure handle on that workstation should show a red error ring and attention treatment. If a work type has no completion or failure state, the work-type node should be marked. If a workstation routes the same work type to conflicting output states, the workstation output handle should be marked. If a work state has no terminal completion path, the work-state handle should show the issue.

This PRD does not require source file, line, column, byte-offset diagnostics, or React Flow handle identifiers in API responses. The target behavior is logical factory component validation that API clients and graph editors can map back to visible components.

## Goals

- Ensure factory write APIs reject invalid factory definitions before they can become the active runtime configuration.
- Add a `factory-validations` API that validates a submitted factory without saving it.
- Return all relevant validation failures as a list, not only the first error.
- Standardize validation targets across config load, editable factory save, named factory creation, current factory save, and explicit validation flows.
- Return stable logical targets that identify exact factory components, including workstation transition fields, work-type state collections, work states, resources, workers, edges/routes, and whole-factory failures.
- Let the website validate draft factory edits as customers add, remove, connect, and disconnect graph components.
- Show validation failures directly on graph nodes and handles without preventing continued editing.
- Preserve clear machine-readable and human-readable error data for API clients.

## User Stories

### US-001: Validate a Draft Factory Without Saving

**Description:** As a factory editor user, I want the website to validate my draft factory without saving it so that I can see problems while I am still editing.

**Acceptance Criteria:**

- [ ] A `factory-validations` API accepts a complete factory definition payload and returns validation targets without persisting or activating the submitted factory.
- [ ] A valid factory returns an empty targets list and a successful response.
- [ ] An invalid factory returns a successful validation response with targets, not a save failure response.
- [ ] The API response includes reusable validation `targets` with explicit factory-domain subjects, such as `{ type: "WORKSTATION", id: "process", location: "ON_REJECTION" }`.
- [ ] Contract tests verify the request and response schema, including an invalid factory example with multiple targets.
- [ ] Typecheck/lint passes.

### US-002: Return Structured Validation Failures From Factory Writes

**Description:** As a UI or CLI client, I want factory write APIs to return every blocking validation failure with an explicit factory subject and reason so that I can show users exactly what must be fixed before save.

**Acceptance Criteria:**

- [ ] Invalid factory write responses include validation targets for each blocking failure.
- [ ] Each validation target includes severity, stable rule/code, human-readable message, and structured subject metadata.
- [ ] A workstation named `bob` with invalid failure handling produces a target with subject `{ type: "WORKSTATION", id: "bob", location: "ON_FAILURE" }`.
- [ ] Existing API error families and top-level error fields remain compatible with current clients.
- [ ] Contract tests verify the OpenAPI response schema and examples for validation targets.
- [ ] Typecheck/lint passes.

### US-003: Consolidate Factory Validation in One Backend Package

**Description:** As a developer, I want factory validation to run through one canonical package so that new factory invariants are enforced consistently across save, load, activation, and draft validation paths.

**Acceptance Criteria:**

- [ ] A new `pkg/factory/validation` package exposes one primary validation entrypoint for complete factory definitions.
- [ ] The explicit `factory-validations` API, current factory save, named factory creation, editable factory validation, and config validation all delegate shared constraints to the canonical package.
- [ ] Runtime-only defensive errors remain as safety checks, but invalid graph structure is rejected before runtime activation.
- [ ] Tests prove the same invalid factory produces equivalent targets through explicit validation, current factory save, and config validation paths.
- [ ] Typecheck/lint passes.

### US-004: Mark Workstation Handle Failures in the Graph

**Description:** As a factory editor user, I want validation failures to appear on the exact workstation handles that are invalid so that I can quickly repair missing or conflicting routes.

**Acceptance Criteria:**

- [ ] When a workstation is missing a required reject route, the validation target identifies `type: "WORKSTATION"`, the workstation id, and `location: "ON_REJECTION"`.
- [ ] When a workstation is missing a required failure route, the validation target identifies `type: "WORKSTATION"`, the workstation id, and `location: "ON_FAILURE"`.
- [ ] When a workstation exports the same work type to conflicting work states, the validation target identifies `type: "WORKSTATION"`, the workstation id, and `location: "OUTPUTS"`.
- [ ] The current activity graph renders targeted handles with a red error ring and attention treatment.
- [ ] The visual treatment is accessible, does not rely on color alone, and respects reduced-motion preferences.
- [ ] Clicking or selecting the marked workstation exposes the relevant validation message in the current activity graph failure dialog.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-005: Mark Work Type and Work State Failures in the Graph

**Description:** As a factory editor user, I want validation failures on work types and work states to appear on the affected graph component so that state-model problems are visible without reading raw JSON.

**Acceptance Criteria:**

- [ ] When a work type does not declare a completion state, the validation target identifies `type: "WORK_TYPE"`, the work-type id, and `location: "STATES"`.
- [ ] When a work type does not declare a failure state, the validation target identifies `type: "WORK_TYPE"`, the work-type id, and `location: "STATES"`.
- [ ] When a work state cannot target a terminal completion path, the validation target identifies `type: "WORK_STATE"`, the work-state id, and `location: "TERMINAL"`.
- [ ] The current activity graph renders targeted work-type and work-state components with an error treatment that is consistent with workstation handle errors.
- [ ] Clicking or selecting the marked work type or work state exposes the relevant validation message in the current activity graph failure dialog.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-006: Refresh Validation Feedback as Customers Edit

**Description:** As a factory editor user, I want validation feedback to update after I add or remove graph components so that stale errors disappear and new errors are visible before save.

**Acceptance Criteria:**

- [ ] Adding a workstation, worker, resource, work type, or work state triggers validation against the current draft factory.
- [ ] Removing a workstation, worker, resource, work type, or work state triggers validation against the current draft factory.
- [ ] Connecting or disconnecting graph relationships triggers validation against the current draft factory.
- [ ] Validation feedback updates without discarding unsaved edits.
- [ ] Validation failures do not prevent editing actions from completing unless the user attempts to save through a write API that rejects blocking errors.
- [ ] Tests cover at least one add, one remove, and one connect or disconnect flow where validation targets change after the operation.
- [ ] Verify in browser using dev-browser skill.
- [ ] Typecheck/lint passes.

### US-007: Preserve Existing Validation Coverage While Changing Ownership

**Description:** As a maintainer, I want the consolidation to preserve existing validation behavior so that moving validation does not silently weaken config safety.

**Acceptance Criteria:**

- [ ] Existing validation rules for input types, place references, workstation kinds, classifier constraints, cron/poller/repeater constraints, worker references, per-input guards, resources, required tools, and bundled files still run.
- [ ] Existing config validator tests are migrated or adapted to assert canonical validation targets.
- [ ] Existing API tests for bad factory payloads continue to pass after updating expected target fields.
- [ ] The implementation does not remove validation rules unless a replacement rule is added in the canonical package.
- [ ] Typecheck/lint passes.

## Functional Requirements

- FR-1: The system must define a canonical factory validation target type under `pkg/factory/validation`.
- FR-2: The system must expose a RESTful validation-only API, `POST /factory-validations`, that creates a validation result for a submitted complete factory definition without saving or activating it.
- FR-3: Factory write APIs must serialize blocking validation targets into API error responses for invalid factory submissions.
- FR-4: The validation-only API must return validation targets in a successful response so the website can validate drafts without treating invalid drafts as failed network operations.
- FR-5: Each validation target must include `severity`, stable `code`, human-readable `message`, and one structured `subject`.
- FR-6: Validation severities must support `error`, `warning`, and `hint`; only `error` is blocking for writes in the first implementation phase.
- FR-7: Each subject must identify the affected factory component without requiring clients to parse `path`, using `type`, `id`, and `location`.
- FR-8: Subject `type` must support at least `FACTORY`, `WORKSTATION`, `WORK_TYPE`, `WORK_STATE`, `WORKER`, `RESOURCE`, and `ROUTE`.
- FR-9: Subject `location` must be a factory-domain location, such as `ON_REJECTION`, `ON_FAILURE`, `OUTPUTS`, `INPUTS`, `STATES`, `TERMINAL`, `REFERENCE`, or `DEFINITION`; it must not be a React Flow node id, handle id, or component implementation detail.
- FR-10: Current factory save must reject invalid topology before activation or persistence as the active definition.
- FR-11: Named factory creation must reject invalid factory payloads using the same validation targets.
- FR-12: Config load or config mapping paths must expose the same canonical validation result shape, even if callers later format it for CLI output.
- FR-13: Structural validation must include duplicate IDs/names, dangling references, invalid place references, missing required outcome routes, invalid workstation kinds, invalid worker references, resource errors, required tool errors, bundled file errors, missing work-type completion/failure states, conflicting workstation output routes for a work type, and work states with no terminal completion path.
- FR-14: Validation must aggregate all discovered targets where practical instead of stopping at the first failure.
- FR-15: API error responses must remain backward compatible at the top level: `message`, `family`, and `code` stay present.
- FR-16: OpenAPI schemas and generated clients must be updated when the validation target response shape changes.
- FR-17: The current activity graph must keep the canonical factory draft as the source of truth and treat React Flow nodes, edges, handles, and highlighted error states as projections from that draft plus validation targets.
- FR-18: The current activity graph must map factory-domain validation targets to rendered nodes and handles in its projection layer.
- FR-19: The current activity graph failure dialog must list validation targets for the currently selected graph component, including messages attached to the selected node, its handles, and its incident edges.
- FR-20: The graph editor must refresh validation after draft mutations, including add, remove, connect, disconnect, and field edits that affect topology.

## Proposed API Shape

The final OpenAPI names may follow existing generated naming conventions, but both the validation-only API and factory write errors should reuse the same `targets` shape. Targets must describe factory model components, not React Flow components.

```json
{
  "targets": [
    {
      "code": "factory.workstation.missingRejectionRoute",
      "severity": "error",
      "message": "Workstation process must define a reject route.",
      "subject": {
        "type": "WORKSTATION",
        "id": "process",
        "location": "ON_REJECTION"
      }
    },
    {
      "code": "factory.workType.missingCompletionState",
      "severity": "error",
      "message": "Work type task must declare a completion state.",
      "subject": {
        "type": "WORK_TYPE",
        "id": "task",
        "location": "STATES"
      }
    }
  ]
}
```

Required target fields:

- `code`: Stable machine-readable rule id, for example `factory.workstation.missingFailureRoute`.
- `severity`: `error` | `warning` | `hint`.
- `message`: Human-readable explanation suitable for dialogs and summaries.
- `subject`: Required primary factory object identity and model location.

Required subject fields:

- `type`: Factory-domain component type, such as `WORKSTATION`, `WORK_TYPE`, `WORK_STATE`, `WORKER`, `RESOURCE`, `ROUTE`, or `FACTORY`.
- `id`: Stable component id or name, such as `process`, `task`, `task:complete`, `executor-slot`, or a route id when available.
- `location`: Factory-domain component location, such as `ON_REJECTION`, `ON_FAILURE`, `OUTPUTS`, `INPUTS`, `STATES`, `TERMINAL`, `REFERENCE`, or `DEFINITION`.

Example targets:

- Missing workstation reject route: `{ "type": "WORKSTATION", "id": "process", "location": "ON_REJECTION" }`.
- Missing workstation failure route: `{ "type": "WORKSTATION", "id": "process", "location": "ON_FAILURE" }`.
- Conflicting workstation outputs for the same work type: `{ "type": "WORKSTATION", "id": "process", "location": "OUTPUTS" }`, with the conflicting work type/state names in `message`.
- Missing work-type completion state: `{ "type": "WORK_TYPE", "id": "task", "location": "STATES" }`.
- Missing work-type failure state: `{ "type": "WORK_TYPE", "id": "task", "location": "STATES" }`.
- Work state with no terminal completion path: `{ "type": "WORK_STATE", "id": "task:complete", "location": "TERMINAL" }`.

## Non-Goals

- No file path, line, column, byte offset, or source-text diagnostics in this phase.
- No logical `path` or `indexPath` requirement in the first implementation phase. Those fields can be added later if deeper state targeting requires them.
- No editor source-map support for `factory.json` or split `AGENTS.md` files.
- No broad redesign of the factory data model.
- No removal of runtime defensive checks; runtime should still guard against impossible states.
- No blocking of draft edit actions solely because validation errors exist.
- No requirement to render warning and hint UI in the first implementation phase, even though the API should expose those severities.
- No literal audio alert requirement for validation feedback; attention treatment should be visual and accessible by default.

## Design Considerations

- API clients should not need to parse plain English error messages or logical paths to identify the failing object, field, route, or rendered graph handle.
- Target examples should use explicit `subject` values rather than path parsing.
- The graph editor should be able to attach validation targets to workstation nodes, work-type nodes, work-state nodes, worker nodes, resource nodes, edges/routes, exact handles, fields in dialogs, and save-level summaries.
- Existing `ErrorResponse.targets` should be evolved and reused so write API errors and validation-only responses share the same target shape.
- Handle error styling should be visible at normal graph zoom levels. A red ring, small error dot, or pulsing outline is acceptable when it does not obscure the handle or make edge creation difficult.
- Motion should respect reduced-motion settings. If the design uses a pulsing "beep" visual, it must stop or simplify under reduced motion.
- The current activity graph failure dialog should group messages by selected component and show handle-specific copy when a selected node has multiple failing handles.

## Technical Considerations

- Existing validation surfaces include `pkg/config/config_validator.go`, `pkg/service/factory_editable_definition.go`, `pkg/factory/state/validation`, config mapping, API body decode checks, and runtime transitioner errors.
- `pkg/factory/validation` should avoid importing generated API types directly if possible. API handlers or service adapters can translate canonical validation targets into generated API response types.
- Structural route validation should operate on a canonical factory representation that has enough information to determine default rejection and failure routing behavior.
- The validation package should define stable rule identifiers, for example `factory.workstation.missingRejectionRoute`, `factory.workstation.missingFailureRoute`, `factory.workType.missingCompletionState`, `factory.workType.missingFailureState`, `factory.workstation.conflictingWorkStateOutputs`, and `factory.workState.missingTerminalCompletionPath`.
- Generated OpenAPI artifacts and frontend API types must be regenerated after schema changes.
- Frontend network access should go through typed API modules and React Query hooks, not inline fetch calls from graph components.
- Frontend draft state should remain canonical factory data. React Flow graph nodes, edges, handles, and validation markers should be disposable projections from factory data and validation targets.
- Validation requests should run after topology-affecting operations and direct validate actions so the editor remains responsive while customers perform rapid edits.
- Validation request races should be handled so older responses do not overwrite newer draft targets.
- UI tests should prove both pure target-to-graph projection behavior and at least one mounted current activity graph path where a failing handle is visibly marked.

## Success Metrics

- The known invalid factory shape that previously reached runtime now fails during save with targets for missing rejection or failure routes.
- The explicit validation API returns multiple validation targets for one invalid submitted factory without saving it.
- The website highlights the exact failing handle for missing workstation reject/failure routes.
- The website highlights the affected work-type node when completion or failure states are missing.
- The website updates validation markers after add, remove, connect, and disconnect operations without losing unsaved edits.
- Factory write API tests can assert multiple validation targets from one invalid request.
- No existing config validation rule loses test coverage during consolidation.
- Runtime logs no longer show transitioner failures caused by factory definitions accepted through save APIs.

## Decisions

- The validation-only API should be RESTful: `POST /factory-validations` creates and returns a validation result for the submitted factory definition.
- API validation targets must reflect only the factory data model. They must not expose React Flow node ids, handle ids, or UI component implementation details.
- Draft validation should run after topology-affecting operations and direct validate actions.
- Write API errors should reuse `ErrorResponse.targets` with the extended validation target shape.
- The API should expose `error`, `warning`, and `hint` severities. Only errors block writes in the first implementation phase.

## Open Questions

- Should deeper state-specific targets add a nested subject shape in a later phase, or should that later phase add logical `path` fields?
