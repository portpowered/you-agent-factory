# PRD: Wire Real-Backend Factory Session Lifecycle-Control APIs

## Introduction

Customers can already use the real durable JavaScript Factory Session backend for start, list, get, result, events, dispatch reads, and artifact reads. The remaining backend API parity gap is lifecycle control: durable session clients still receive `501` stubs or incomplete behavior when they try to pause, resume, cancel, terminate, approve, or retry dispatches through the public Factory Session API.

This work routes the existing lifecycle-control routes under `/factory-sessions/{session_id}` through the durable backend service and shared API surface mappers. HTTP clients should receive typed lifecycle-control success, no-op, invalid-state, terminal-session, conflict, and not-found responses while existing live Petri session compatibility remains intact for non-durable session IDs.

## Context

### Customer Ask

Wire real-backend Factory Session lifecycle-control APIs so HTTP clients can pause, resume, cancel, terminate, approve, and retry durable JavaScript Factory Sessions through the shared Factory Session API instead of receiving `501` stubs.

### Problem

The real-backend session parity and dispatch/artifact parity slices are complete, but lifecycle-control endpoints are still the route family explicitly left as deferred stubs. This blocks generated clients and API consumers from controlling durable JavaScript sessions through the same typed public API they use for reads and inspection. It also leaves typed control errors, idempotent request handling, and inspection-link behavior unproven at the HTTP boundary.

### Solution

Replace durable lifecycle-control stubs with handlers that route durable session IDs to the `factorysessionexecution.Service` lifecycle methods through `FactoryService` and shared `pkg/apisurface/factorysession` mappers. Use the real durable JavaScript runtime service for at least cancel or terminate loopback coverage, use fixture-backed fake service scenarios where real runtime support is not yet available, preserve existing live Petri behavior for non-durable IDs, and keep OpenAPI/generated clients synchronized if contract corrections are required.

## Goals

- Return typed lifecycle-control responses for durable cancel and terminate instead of `501` stubs.
- Return typed pause and resume success or invalid-state/conflict responses through shared service and mapper code.
- Wire approve and retry-dispatch through the same durable lifecycle-control API boundary, with fixture-backed coverage acceptable where runtime scenarios are not yet available.
- Preserve generated response vocabulary for typed errors, including operation, outcome, status, and inspection links when available.
- Map missing durable sessions to the existing typed not-found API shape.
- Preserve prior start/list/get/result/events and dispatch/artifact read parity after lifecycle-control wiring.
- Keep this lane scoped to lifecycle-control APIs and related regression checks.

## Project-Level Acceptance Criteria

- [ ] `POST /factory-sessions/{session_id}/cancel` or `POST /factory-sessions/{session_id}/terminate` for a runtime-backed durable JavaScript session returns a typed lifecycle-control response through the durable backend path and no longer returns a `501` stub.
- [ ] `POST /factory-sessions/{session_id}/pause` and `POST /factory-sessions/{session_id}/resume` return typed lifecycle-control responses or typed invalid-state/conflict responses through shared service and mapper code.
- [ ] `POST /factory-sessions/{session_id}/approve` and `POST /factory-sessions/{session_id}/retry-dispatch` are wired through the same durable lifecycle-control API boundary, with fixture-backed coverage acceptable where real runtime scenarios are not yet available.
- [ ] Typed control errors preserve generated response vocabulary, include operation/outcome/status when available, and map missing sessions to the existing not-found API shape.
- [ ] Focused tests prove lifecycle controls do not break prior start/list/get/result/events or dispatch/artifact read parity.
- [ ] The lane does not implement website inspection, MCP install work, live-provider bridge behavior, standalone workflow-run resources, `/workflow-previews` expansion, or another broad API/event batch.
- [ ] Quality gate passes: generated artifacts are synchronized when OpenAPI changes are made, typecheck passes, lint passes where applicable, and focused backend/API tests pass.

## User Stories

### dynamic-workflows-cell-real-backend-api-lifecycle-control-001: Cancel or Terminate Runtime-Backed Durable Sessions

**Description:** As an HTTP client controlling a durable JavaScript Factory Session, I want cancel or terminate to return a typed lifecycle-control response from the real backend so that I can stop durable work without receiving a stub response.

**Acceptance Criteria:**

- [ ] `POST /factory-sessions/{session_id}/cancel` or `POST /factory-sessions/{session_id}/terminate` routes `dur-sess-*` session IDs to the durable execution service instead of returning `501 NotImplemented`.
- [ ] At least one focused API server test starts or uses a runtime-backed durable JavaScript session and proves cancel or terminate returns a typed lifecycle-control response.
- [ ] The response includes generated lifecycle-control fields such as operation, outcome, lifecycle status, request identifier, and inspection links when available.
- [ ] Missing durable sessions return the existing typed not-found API response.
- [ ] Non-durable live session IDs preserve existing live Petri-compatible lifecycle behavior.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-real-backend-api-lifecycle-control-002: Pause and Resume Durable Sessions with Typed Outcomes

**Description:** As an HTTP client managing a durable JavaScript Factory Session, I want pause and resume to return typed success or invalid-state responses so that generated clients can react without parsing strings.

**Acceptance Criteria:**

- [ ] `POST /factory-sessions/{session_id}/pause` routes durable session IDs through the durable lifecycle service boundary and returns a typed lifecycle-control response or typed invalid-state/conflict response.
- [ ] `POST /factory-sessions/{session_id}/resume` routes durable session IDs through the durable lifecycle service boundary and returns a typed lifecycle-control response or typed invalid-state/conflict response.
- [ ] Terminal sessions produce the generated terminal-session or invalid-state lifecycle-control vocabulary rather than an untyped server error.
- [ ] Fixture-backed fake service coverage is acceptable for pause/resume states that cannot yet be produced by the real runtime, but HTTP behavior must still be proven at the API boundary.
- [ ] Non-durable live session IDs continue to use the existing live Petri-compatible pause/resume behavior.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-real-backend-api-lifecycle-control-003: Approve and Retry Dispatch Through the Durable Boundary

**Description:** As an HTTP client resolving gated or failed durable work, I want approve and retry-dispatch to use the same typed lifecycle-control API boundary so that all lifecycle operations behave consistently.

**Acceptance Criteria:**

- [ ] `POST /factory-sessions/{session_id}/approve` normalizes the request through shared API surface code and delegates durable session IDs to the durable lifecycle service.
- [ ] `POST /factory-sessions/{session_id}/retry-dispatch` normalizes the request through shared API surface code and delegates durable session IDs to the durable lifecycle service.
- [ ] Approve and retry-dispatch responses use generated lifecycle-control success, no-op, invalid-state, terminal-session, or conflict shapes as appropriate for the scenario.
- [ ] Fixture-backed fake service scenarios cover approve and retry-dispatch when real runtime scenarios for guarded approvals or retryable provider dispatches are not yet available.
- [ ] The API does not introduce standalone workflow-run route families or `/workflow-previews` expansion for these controls.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-real-backend-api-lifecycle-control-004: Preserve Typed Control Errors and Request-Id Semantics

**Description:** As a generated-client consumer, I want lifecycle-control failures and request-id replays to use stable typed API shapes so that client error handling is deterministic.

**Acceptance Criteria:**

- [ ] Validation failures, missing durable sessions, terminal-session controls, invalid-state controls, and request-id conflicts map to generated typed API response shapes with existing HTTP status semantics.
- [ ] Missing durable sessions use the existing not-found API shape rather than a lifecycle-specific ad hoc error.
- [ ] Idempotent request-id replay returns the same typed lifecycle-control result for the same control tuple where the service supports replay.
- [ ] Conflicting request-id reuse returns the generated conflict response vocabulary without mutating session state.
- [ ] Error responses preserve operation, outcome, status, and inspection-link fields when the service result provides them.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-real-backend-api-lifecycle-control-005: Keep Lifecycle Contract and Mappers Synchronized

**Description:** As a maintainer of the public Factory Session API, I want lifecycle-control route contracts and mappers to stay aligned so that generated Go and TypeScript clients receive stable typed responses.

**Acceptance Criteria:**

- [x] If lifecycle-control route, schema, response, or error refs need correction, the authored OpenAPI fragments are updated and `make generate-api` synchronizes generated Go and TypeScript clients.
- [x] API surface mapper tests cover lifecycle-control request normalization and response/error shaping for success, no-op or terminal-session, conflict, and not-found cases.
- [x] Contract tests cover the existing lifecycle-control route family under `/factory-sessions/{session_id}` and the generated lifecycle-control response vocabulary.
- [x] The deferred-route regression no longer treats lifecycle-control routes as unsupported future routes; only genuinely unsupported future routes remain deferred.
- [x] Public vocabulary remains Factory Session lifecycle-control vocabulary and does not expose internal Petri-net terms or standalone workflow-run nouns.
- [x] Typecheck passes.
- [x] Tests pass.

### dynamic-workflows-cell-real-backend-api-lifecycle-control-006: Protect Prior Durable Read and Inspection Parity

**Description:** As a maintainer extending durable Factory Session parity, I want lifecycle-control wiring to preserve start, read, result, event, dispatch, and artifact behavior so that the new controls do not regress completed slices.

**Acceptance Criteria:**

- [ ] Focused regression coverage proves durable JavaScript start, list, get, result, and event read/reconnect behavior still passes after lifecycle-control wiring.
- [ ] Focused regression coverage proves durable dispatch and artifact list/detail reads still pass after lifecycle-control wiring.
- [ ] Tests demonstrate lifecycle-control calls do not remove stable inspection links for session, result, dispatch, artifact, or event reads when those links are available.
- [ ] Website inspection parity and live-provider bridge parity are recorded as named follow-up cells if encountered rather than implemented in this lane.
- [ ] Typecheck passes.
- [ ] Tests pass.

## High-Level Technical Design

Durable lifecycle-control requests should stay under the existing public Factory Session route family. API handlers identify durable session IDs and delegate approve, pause, resume, cancel, terminate, and retry-dispatch through `FactoryService` to `factorysessionexecution.Service`. Non-durable session IDs continue on the existing live Petri-compatible path.

Request normalization and response shaping belong in `pkg/apisurface/factorysession`, especially the existing control request and lifecycle-control response/error mappers. Handlers should not build durable lifecycle projections locally. Per-session runtime state remains owned by the durable session runtime/service layer; `FactoryService` coordinates access and dependencies but does not become the state owner.

Tests should prove behavior at the API boundary and mapper boundary. At least cancel or terminate must use the real durable JavaScript runtime service for loopback coverage. Pause/resume, approve, retry-dispatch, and typed conflict cases may use fixture-backed fake service scenarios when real runtime support is not yet available. If OpenAPI lifecycle-control schemas or refs need correction, update authored fragments first, then run `make generate-api`.

## Functional Requirements

- FR-1: `POST /factory-sessions/{session_id}/cancel` must return a typed lifecycle-control response for durable JavaScript Factory Sessions through the durable execution service.
- FR-2: `POST /factory-sessions/{session_id}/terminate` must return a typed lifecycle-control response for durable JavaScript Factory Sessions through the durable execution service.
- FR-3: `POST /factory-sessions/{session_id}/pause` and `/resume` must return typed lifecycle-control responses or typed invalid-state/conflict responses for durable sessions.
- FR-4: `POST /factory-sessions/{session_id}/approve` and `/retry-dispatch` must be wired through the same durable lifecycle-control service and mapper boundary.
- FR-5: Missing durable sessions must map to the existing not-found API shape.
- FR-6: Typed lifecycle-control errors must preserve generated response vocabulary, including operation, outcome, status, and inspection links when available.
- FR-7: Request-id replay and conflict behavior must be deterministic and must not mutate unrelated session state.
- FR-8: Existing live Petri session compatibility must remain intact for non-durable session IDs.
- FR-9: Prior durable start/list/get/result/events and dispatch/artifact read parity must continue to pass focused regression coverage.
- FR-10: Public route structure must remain under `/factory-sessions/{session_id}` lifecycle controls without introducing workflow-run resources.

## Non-Goals

- No website or dashboard inspection parity.
- No MCP host installation or install-smoke changes.
- No live-provider bridge parity for JavaScript child-agent dispatch execution.
- No standalone workflow-run API resources or `/workflow-previews` expansion.
- No broad API/event route family beyond lifecycle controls and the regression checks needed to protect completed parity slices.
- No broad handler refactor, package reshaping, or unrelated cleanup.

## Supporting Technical and UX Considerations

- Relevant references include `docs/temp/customer-ask.md`, `docs/temp/dynamic-workflow-plan.md`, `docs/internal/development/plans/dynamic-workflows/dynamic-workflow-design.md`, `docs/internal/processes/api-relevant-files.md`, `docs/architecture/architecture.md`, and `docs/architecture/data-model.md`.
- Hard dependencies are completed `dynamic-workflows-cell-real-backend-api-session-parity`, completed `dynamic-workflows-cell-real-backend-api-dispatch-artifact`, and existing lifecycle-control service methods plus `pkg/apisurface/factorysession` mappers.
- Recommended verification commands are `make generate-api` when contract files change, `go test ./pkg/api/servertests ./pkg/api/contracttests ./pkg/apisurface/... ./pkg/factorysessionexecution/...`, and `make api-smoke` when feasible.
- The change is backend/API-visible, not browser-visible. Direct browser verification is not required unless implementation expands into dashboard behavior, which is out of scope for this lane.

## Success Metrics

- Durable lifecycle-control routes return typed responses instead of `501` stubs.
- At least one real runtime-backed cancel or terminate API test proves the durable backend path.
- Generated clients can distinguish accepted, no-op, invalid-state, terminal-session, conflict, and not-found outcomes using typed response vocabulary.
- Prior durable read and inspection parity tests remain green.
- No new workflow-run API resource, website behavior, MCP install behavior, or live-provider bridge behavior is introduced.

## Open Questions

None. Where real runtime scenarios are not yet available for a lifecycle operation, implementation should use fixture-backed coverage for this lane and record the missing real-provider/runtime scenario as follow-up work rather than expanding scope.
