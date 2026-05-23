# PRD: Factory Session API Cleanup

## Introduction

Clean up the public Agent Factory API so its resource vocabulary matches the architecture and customer-facing data model. The current API mixes factory definitions, live sessions, current-factory views, and editor-specific representations in ways that make paths and payloads hard to understand. This work will introduce a canonical resource model centered on `factories`, `factory-sessions`, and `work`, collapse the separate editable-definition document into the main `Factory` resource by storing version metadata on the factory itself, and replace the current path family with clearer session-scoped routes.

This PRD assumes a breaking API change for the next version. Old route compatibility is out of scope for this effort.

## Goals

- Establish one canonical public vocabulary for factory definitions, live factory sessions, current session-scoped factory reads, and work-related resources.
- Replace ambiguous or misleading route families such as `/factory/~current` and `/factories/{factory_id}/factory/~current`.
- Make `Factory` the canonical API representation for a factory definition, including server-managed version metadata required for safe replacement writes.
- Remove the separate editable-definition resource shape from the public API and UI contract.
- Align OpenAPI, backend handlers, generated clients, CLI calls, and UI API wrappers on the new surface.
- Leave the public API easier to explain to users and easier to extend without special-case sentinel segments such as `~current`.

## User Stories

### US-001: Define canonical API resource vocabulary
**Description:** As a maintainer, I want a documented public resource vocabulary so that future API changes use consistent names and path semantics.

**Acceptance Criteria:**
- [ ] Add or update architecture-facing documentation that defines the canonical meaning of `Factory`, `Factory Session`, `Work`, `Work Request`, `Provider Session`, and `Current Factory`.
- [ ] The documentation states that `Factory` means a persisted factory definition and API resource representation, not a live runtime session.
- [ ] The documentation states that `Factory Session` means one live running factory instance in the shared host.
- [ ] The documentation states that internal Petri-net concepts such as tokens and transitions are not primary public API resources.
- [ ] Lint and repository documentation checks pass if applicable.

### US-002: Redesign factory-definition routes
**Description:** As an API consumer, I want factory-definition routes that clearly refer to factory definitions so that I can understand what resource I am reading or mutating.

**Acceptance Criteria:**
- [ ] The OpenAPI contract publishes a canonical `factories` route family for persisted factory definitions.
- [ ] The contract does not use `/factory/~current` or `/factories/{factory_id}/factory/~current` in the new version.
- [ ] The contract no longer uses `factory_id` to identify a live session.
- [ ] Path parameter names distinguish persisted factory identifiers from session identifiers.
- [ ] OpenAPI bundle and contract smoke checks pass.

### US-003: Redesign session-scoped factory routes
**Description:** As an API consumer, I want live session routes under `/factory-sessions/{session_id}/factory` so that session-scoped reads and writes are obvious.

**Acceptance Criteria:**
- [ ] The canonical session-scoped route family is rooted at `/factory-sessions/{session_id}`.
- [ ] The current active factory for a session is available through `/factory-sessions/{session_id}/factory`.
- [ ] Session-scoped factory replacement writes target `/factory-sessions/{session_id}/factory`.
- [ ] Session-scoped work and runtime routes use `session_id` consistently where they refer to a live session.
- [ ] OpenAPI bundle and contract smoke checks pass.

### US-004: Fold editable-definition metadata into `Factory`
**Description:** As a graph-editor client, I want the main `Factory` resource to include version metadata so that I can load and save one canonical document instead of switching between parallel types.

**Acceptance Criteria:**
- [ ] The public API `Factory` schema includes server-managed version metadata required for stale-write detection.
- [ ] The version metadata is present on factory reads used for editing and save flows.
- [ ] The standalone `EditableFactoryDefinition` response shape is removed from the public API contract.
- [ ] The standalone `SaveEditableFactoryDefinitionRequest` request shape is removed or replaced by the canonical factory write shape.
- [ ] Stale-version writes still return a machine-readable conflict error.
- [ ] OpenAPI bundle and generated artifact checks pass.

### US-005: Update backend service and handler boundaries
**Description:** As a backend maintainer, I want service and handler contracts to match the new public resource model so that route behavior and domain concepts stay aligned.

**Acceptance Criteria:**
- [ ] Backend API handlers no longer expose route or method names that reference `editable current factory definition`.
- [ ] API-surface interfaces use names that distinguish factory definitions from factory sessions.
- [ ] Server request validation and error handling still support invalid factory payload, stale version, not found, and factory-not-idle cases.
- [ ] Session-scoped handler methods use `session_id` terminology consistently in parameters, logs, and errors.
- [ ] Backend tests covering OpenAPI surface, handler behavior, and session scoping are updated to the new route family.
- [ ] Backend lint and test suites for touched areas pass.

### US-006: Regenerate and migrate downstream consumers
**Description:** As a UI and CLI consumer, I want generated clients and handwritten API wrappers to match the new contract so that the product continues working after the API redesign.

**Acceptance Criteria:**
- [ ] Regenerate the bundled OpenAPI artifact and checked-in generated Go and TypeScript clients from the new contract.
- [ ] CLI code that currently calls `/factory/~current` is updated to the new canonical route.
- [ ] UI API wrappers and tests are updated to the new factory and factory-session route families.
- [ ] UI code no longer depends on the separate editable-definition document type.
- [ ] Storybook, integration, or targeted UI tests that assert route paths are updated.
- [ ] Typecheck and relevant frontend test suites pass.

### US-007: Persist version metadata with factory config
**Description:** As a system operator, I want factory version metadata stored with the factory config so that optimistic concurrency is durable across process restarts and session reloads.

**Acceptance Criteria:**
- [ ] The persisted factory configuration format stores the version metadata used by API reads and writes.
- [ ] Factory load and save logic preserves version metadata correctly across round trips.
- [ ] Saving a factory updates version metadata deterministically.
- [ ] Runtime activation and named-factory replacement flows continue to work with the persisted version field present.
- [ ] Regression tests cover persisted version readback and stale-save detection across reloads.

## Functional Requirements

1. FR-1: The system must define `Factory` as the canonical public API resource for a persisted factory definition and include server-managed version metadata on that resource.
2. FR-2: The system must define `Factory Session` as the canonical public API resource for a live running factory instance and identify it with `session_id`.
3. FR-3: The system must expose the session-scoped active factory definition at `/factory-sessions/{session_id}/factory`.
4. FR-4: The system must remove `~current` sentinel segments from the canonical public route vocabulary.
5. FR-5: The system must not use `factory_id` to refer to a live session anywhere in the new public API contract.
6. FR-6: The system must provide distinct route families for persisted factory definitions and live factory sessions.
7. FR-7: The system must support full replacement writes of a session-scoped factory through the canonical factory route using the unified `Factory` representation.
8. FR-8: The system must reject stale factory writes using persisted version metadata and return a structured conflict error.
9. FR-9: The system must keep invalid factory payload validation behavior, including structured error targets where applicable.
10. FR-10: The system must persist factory version metadata with the stored factory config so version checks survive restarts and reloads.
11. FR-11: The system must update OpenAPI source, bundled spec, generated Go client/server artifacts, generated TypeScript artifacts, backend handlers, CLI callers, and UI wrappers in the same implementation wave.
12. FR-12: The system must update contract and behavior tests so the new route vocabulary is enforced mechanically.
13. FR-13: The system must keep provider-session diagnostics as a distinct diagnostic resource and not merge them into factory or work resources.
14. FR-14: The system must keep internal Petri-net runtime concepts out of the primary public resource vocabulary unless intentionally published as separate debug-only APIs.

## Non-Goals

- Maintaining backward compatibility for legacy routes in the next-version API.
- Introducing a separate versioned compatibility layer that serves both old and new route families at once.
- Redesigning the full work, event, or provider-session payload semantics beyond the naming and scoping changes needed for consistency.
- Adding entirely new user-facing factory editing capabilities beyond unifying the existing read and save contract.
- Exposing raw token, place, transition, or edge resources as first-class public API entities.

## Design Considerations

- Route names should read naturally from left to right and use plural collections for top-level resource families.
- Session-scoped APIs should consistently use `/factory-sessions/{session_id}/...`.
- The API should avoid magic path segments such as `~current` and avoid embedding UI-only terms into canonical resource names where a plain resource representation will do.
- The `Factory` payload should remain understandable as authored config plus server-managed metadata, with clear field documentation explaining which fields are customer-authored and which are system-managed.

## Technical Considerations

- The authored OpenAPI source of truth lives in [api/openapi-main.yaml](/Users/abdifamily/infinite-you/api/openapi-main.yaml:1), with the bundled artifact in [api/openapi.yaml](/Users/abdifamily/infinite-you/api/openapi.yaml:1).
- Generated outputs that must stay aligned include [pkg/api/generated/server.gen.go](/Users/abdifamily/infinite-you/pkg/api/generated/server.gen.go:1), [pkg/generatedclient/client.gen.go](/Users/abdifamily/infinite-you/pkg/generatedclient/client.gen.go:1), and [ui/src/api/generated/openapi.ts](/Users/abdifamily/infinite-you/ui/src/api/generated/openapi.ts:1).
- Existing handler and service seams that currently separate current-factory and editable-definition behavior include [pkg/api/handlers.go](/Users/abdifamily/infinite-you/pkg/api/handlers.go:349), [pkg/apisurface/contract.go](/Users/abdifamily/infinite-you/pkg/apisurface/contract.go:15), and [pkg/service/factory.go](/Users/abdifamily/infinite-you/pkg/service/factory.go:1479).
- Existing session-route tests and OpenAPI surface assertions will need coordinated updates, including [pkg/api/openapi_contract_surface_test.go](/Users/abdifamily/infinite-you/pkg/api/openapi_contract_surface_test.go:174) and session-scoping tests under `pkg/api/`.
- The implementation must preserve stale-write detection, runtime-idle enforcement, and topology validation behavior while changing shapes and names.
- Because this is a breaking change, release notes and migration guidance should be prepared for CLI and UI consumers even though legacy-route compatibility is out of scope.

## Success Metrics

- All canonical public routes for factory definitions and live sessions can be explained using the documented resource vocabulary without referring to legacy exceptions.
- No canonical route in the new API uses `~current` or a live session identified as `factory_id`.
- The public API uses one canonical factory read/write representation instead of a separate editable-definition envelope.
- Contract smoke tests, generated artifact checks, backend tests, CLI tests, and UI type/test surfaces all pass on the new route family.
- Maintainers can add a new session-scoped factory subresource without needing a parallel legacy-shaped route family.

## Open Questions

- What exact persisted identifier shape should canonical `factories/{factory_id}` use for stored factory definitions: name-based identifiers, opaque IDs, or both?
- Should the default session remain addressable as a normal `session_id` value such as `~default`, or should the API add a dedicated alias route for the default session?
- Should persisted factory-definition writes and session-scoped active-factory writes share the exact same request and response schema, or should session writes include additional runtime status fields?
- Should provider-session diagnostics remain query-based or be reshaped into a more resource-like path as part of the same API cleanup wave?
