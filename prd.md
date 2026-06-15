# PRD: Real-Backend Factory Session API Parity Slice

## Introduction

Wire the smallest real-backend Factory Session API parity slice for simple JavaScript-orchestrated sessions. HTTP clients should be able to start a final-only JavaScript Factory Session through the recovered real session backend, then list it, fetch it, read its result, and reconnect its canonical event stream with the same stable session identity, terminal status, and final-result semantics exposed by the recovered backend/CLI/MCP service path.

This project is intentionally narrow. It proves the real runtime-backed API lane for a simple session without expanding into website inspection, MCP host installation, dispatch/artifact reads, lifecycle controls, resume behavior, live-provider bridge parity, or standalone workflow-run resources.

## Context

### Customer Ask

The backend recovery lane is complete, and the next useful cell is a narrow API parity slice for the real JavaScript session backend. A test or smoke path must start a simple final-only JavaScript Factory Session through the API-backed real session backend and return a stable `FactorySession` identifier without requiring clients to parse logs. The same session must be visible through list/get/result APIs and canonical `FactoryEvent` event reads, including at least one reconnect cursor case.

### Problem

The real JavaScript session backend can now execute simple durable sessions, but the API-backed path needs focused parity proof. API clients need stable session identifiers, terminal status, result availability, final result payloads, and event replay through the shared Factory Session vocabulary. Without this slice, API consumers cannot rely on the recovered runtime through generated contracts, and maintainers lack focused contract, mapper, and server tests proving the real backend path rather than only fixtures or CLI/MCP paths.

### Solution

Use the existing `/factory-sessions` vocabulary and generated `FactorySession`/`FactoryEvent` contracts to wire or correct the smallest route family needed for a simple JavaScript final-only session: start, list, get, result, and event replay/reconnect. Normalize request, response, and error shaping through `pkg/apisurface`, keep JavaScript orchestrator state represented as shared Factory Session and Factory Event data, update OpenAPI fragments and generated clients only if the authored contract requires correction, and prove behavior with focused contract, mapper, API server, and real-backend loopback tests.

## Goals

- Start a simple final-only JavaScript Factory Session through the API-backed real session backend and return a stable `FactorySession` identifier.
- Make list, get, and result API responses agree with the recovered backend/CLI/MCP service semantics for terminal status and final result data.
- Expose canonical `FactoryEvent` envelopes for the session through the session events API and honor at least one reconnect cursor case.
- Keep request/response/error mapping transport-independent through `pkg/apisurface` rather than handler-local ad hoc shapes.
- Preserve existing live Petri session compatibility and current factory-session behavior.
- Keep generated OpenAPI, Go, and TypeScript clients synchronized if authored API fragments change.
- Prove the changed behavior with focused OpenAPI contract, mapper, and API server tests that exercise at least one real backend simple session path.

## Project-Level Acceptance Criteria

- [ ] A test or smoke path starts a simple final-only JavaScript Factory Session through the API-backed real session backend and receives a stable `FactorySession` identifier without parsing logs.
- [ ] List/get/result API responses for that session expose the same terminal lifecycle status, result status, and final-result semantics as the recovered backend/CLI/MCP service path.
- [ ] `GET /factory-sessions/{session_id}/events` or the equivalent generated handler path returns canonical `FactoryEvent` envelopes for that session and supports at least one reconnect cursor case.
- [ ] API responses keep JavaScript orchestrator state inside shared Factory Session and Factory Event vocabulary; no standalone workflow-run API noun or `/workflow-previews` expansion is introduced.
- [ ] Existing live Petri session compatibility and existing factory-session behavior continue to pass focused regression tests.
- [ ] OpenAPI contract tests, mapper tests, and focused API server tests cover the changed behavior; generated files are updated if authored OpenAPI changes.
- [ ] Repository quality gate passes: `make generate-api` when needed, focused Go tests, typecheck, lint, and `make api-smoke` when feasible are green.

## User Stories

### dynamic-workflows-cell-real-backend-api-session-parity-001: Start a simple real-backend JavaScript session through the API

**Description:** As an API client, I want to start a simple final-only JavaScript Factory Session through the real session backend so that I receive a stable session identifier from the API itself.

**Acceptance Criteria:**

- [ ] Starting a simple final-only JavaScript session through the generated API-backed start path returns a stable `FactorySession` identifier and request identifier in the response body.
- [ ] The test path proves the real session backend is used for at least one successful simple JavaScript session start; mock or fixture services are used only to isolate contract expectations in separate tests.
- [ ] Reusing an idempotent start request returns the same stable session identity according to the recovered service semantics.
- [ ] Invalid or unsupported start input returns a typed API error without creating a visible session.
- [ ] Existing live Petri session start behavior remains compatible.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-real-backend-api-session-parity-002: Expose the real session in list and get APIs

**Description:** As an API client, I want the real JavaScript Factory Session I started to appear in list and get responses so that API polling sees the same session identity and lifecycle status as the backend service.

**Acceptance Criteria:**

- [ ] `GET /factory-sessions` with the relevant supported scope includes the real JavaScript session with the same session identifier returned by start.
- [ ] `GET /factory-sessions/{session_id}` returns the same terminal lifecycle status, source identity summary, phase/progress summary, and result availability semantics as the recovered backend service read model.
- [ ] Missing session reads return the existing typed not-found API error shape.
- [ ] Live Petri session rows remain visible through their existing list/get compatibility behavior.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-real-backend-api-session-parity-003: Read the final result with backend/API semantic parity

**Description:** As an API client, I want to read the final result for the completed JavaScript Factory Session so that API consumers receive the same terminal result semantics as CLI and MCP clients.

**Acceptance Criteria:**

- [ ] `GET /factory-sessions/{session_id}/results` or the current generated result handler returns the final result for the completed simple session without requiring log parsing.
- [ ] The API result response exposes the expected terminal result status and either the structured final value or stable result hash used by the recovered backend/CLI/MCP path.
- [ ] Result reads before terminal availability return the existing typed not-ready or unavailable response while retaining the requested session identity.
- [ ] Mapper tests prove API result response shaping uses the shared apisurface projection rather than a handler-local result struct.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-real-backend-api-session-parity-004: Replay canonical Factory Session events with reconnect cursor support

**Description:** As an API client, I want to read canonical events for the JavaScript Factory Session and reconnect from a cursor so that event consumers can recover session state without special workflow-run logic.

**Acceptance Criteria:**

- [ ] `GET /factory-sessions/{session_id}/events` or the equivalent generated handler returns canonical `FactoryEvent` envelopes for the session, including lifecycle and result-update events expected for a final-only JavaScript run.
- [ ] Event envelopes include stable session context such as session identifier, sequence or event identifier, orchestrator identity where applicable, and event type names from the shared factory event contract.
- [ ] At least one reconnect cursor case returns only events after the cursor and preserves deterministic event ordering.
- [ ] An unknown or expired reconnect cursor returns the existing typed reconnect-cursor error without changing session state.
- [ ] Event replay tests prove the API event sequence can reconstruct the same terminal session status and result availability as the read/result APIs.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-real-backend-api-session-parity-005: Keep public contract and generated clients aligned for the narrow API slice

**Description:** As an API maintainer, I want any required OpenAPI corrections for this slice to be authored and generated consistently so that backend handlers, generated clients, and tests share one contract.

**Acceptance Criteria:**

- [ ] If route, schema, parameter, or response corrections are required, they are authored in `api/openapi-main.yaml` or component fragments under `api/components/` and then regenerated into bundled Go and TypeScript artifacts.
- [ ] OpenAPI contract tests cover the start/list/get/result/events response shapes and reconnect cursor parameter or error shape used by this slice.
- [ ] API mapper tests cover request normalization, response projection, and typed error mapping for the changed start/read/result/event behavior.
- [ ] Generated files remain synchronized with authored OpenAPI; if no authored contract change is required, the implementation documents that no regeneration diff is expected.
- [ ] No standalone workflow-run API resource, route family, or generated type is introduced.
- [ ] Typecheck passes
- [ ] Tests pass

### dynamic-workflows-cell-real-backend-api-session-parity-006: Record deferred route families as follow-up cells

**Description:** As a maintainer, I want this API parity lane to explicitly defer unrelated route families so that the implementation stays focused on the simple real-backend session path.

**Acceptance Criteria:**

- [ ] The implementation notes or task follow-up record dispatch reads, artifact reads, lifecycle controls, website inspection, MCP host installation, resume behavior, and live-provider bridge parity as deferred follow-up cells when they are encountered.
- [ ] No dispatch/artifact/lifecycle-control route behavior is added or changed except where an existing contract must remain compiling and compatible.
- [ ] Public documentation or API descriptions changed by this lane use Factory Session and Factory Event vocabulary, not new workflow-run nouns.
- [ ] Typecheck passes
- [ ] Tests pass

## High-Level Technical Design

1. **Canonical API vocabulary:** Use existing `/factory-sessions` route family and generated `FactorySession`, `FactorySessionResult`, and `FactoryEvent` shapes. Do not add workflow-run resources or expand `/workflow-previews`.
2. **Real backend proof:** At least one focused API server or smoke test must run a simple final-only JavaScript session through the real `factorysessionexecution` runtime-backed service path. Fixture or fake services may still be used for mapper and contract expectation tests.
3. **Transport-independent mapping:** Normalize API start, list, get, result, event, and error shapes through `pkg/apisurface` factory-session mappers. Handlers should delegate shaping rather than embedding one-off response structs.
4. **State ownership:** Per-session runtime state remains in the session execution/runtime layer. `FactoryService` coordinates dependencies and service access but does not become the owner of durable per-session state.
5. **Event consistency:** The event API reads canonical session lifecycle/result events from the same source used by recovered service projections. Reconnect cursor filtering must be deterministic and must not mutate session state.
6. **Contract generation:** If authored OpenAPI needs correction, update fragments under `api/components/` or `api/openapi-main.yaml`, run `make generate-api`, and keep `api/openapi.yaml`, generated Go server/client code, and generated TypeScript client code synchronized.
7. **Compatibility:** Preserve existing live Petri session APIs and factory-session behavior. JavaScript orchestrator state should appear as shared Factory Session data, not a separate projection family.

## Functional Requirements

- FR-1: The API must support starting a simple final-only JavaScript Factory Session through the real recovered session backend.
- FR-2: Start responses must include a stable `FactorySession` identifier that clients can use for list, get, result, and event reads.
- FR-3: List and get APIs must expose the started session with lifecycle status and result availability semantics matching the backend service read model.
- FR-4: Result reads must expose the terminal final result status and final value or stable result hash without log parsing.
- FR-5: Event reads must return canonical `FactoryEvent` envelopes for the session.
- FR-6: Event reconnect must support at least one cursor path that filters already-seen events and one typed failure path for an invalid cursor.
- FR-7: API request, response, and error shaping must flow through shared apisurface mappers.
- FR-8: Authored OpenAPI and generated artifacts must stay synchronized when contract changes are required.
- FR-9: Existing live Petri session compatibility must not regress.

## Non-Goals

- No website or dashboard inspection parity.
- No MCP host installation or install-smoke changes.
- No full dispatch read, artifact read, lifecycle-control, resume, or live-provider bridge parity.
- No standalone workflow-run API resources or workflow-run nouns.
- No `/workflow-previews` expansion.
- No broad handler decomposition, unrelated refactors, or generated-file churn outside the narrow API slice.

## Supporting Technical and UX Considerations

- Relevant references include `docs/temp/customer-ask.md`, `docs/temp/dynamic-workflow-plan.md`, `docs/internal/development/plans/dynamic-workflows/dynamic-workflow-design.md`, `docs/internal/processes/api-relevant-files.md`, `docs/architecture/architecture.md`, and `docs/architecture/data-model.md`.
- The hard dependency is the completed `dynamic-workflows-recovery-session-backend-runtime` lane with recovered real JavaScript session backend behavior for simple final-only runs.
- Use existing Factory Session and Factory Event schemas where possible. Contract edits should correct missing or inconsistent existing surfaces, not create parallel concepts.
- Frontend-visible behavior is not part of this lane, but generated TypeScript types must remain synchronized if OpenAPI changes.
- Error responses should keep existing typed error vocabulary for missing sessions, not-ready results, request conflicts, invalid starts, and reconnect cursor failures.
- Focused verification commands expected for implementation include `make generate-api` when needed, `go test ./pkg/api/contracttests ./pkg/api/servertests ./pkg/apisurface/... ./pkg/factorysessionexecution/... ./pkg/orchestrators/javascript/...`, and `make api-smoke` when feasible.

## Success Metrics

- One real-backend API test starts a simple JavaScript session and observes the same session through start, list, get, result, and events.
- API result semantics match recovered backend/CLI/MCP expectations for a completed final-only session.
- Event reconnect returns deterministic canonical `FactoryEvent` envelopes after a cursor.
- Contract, mapper, and focused server tests cover the changed behavior without requiring website, MCP install, dispatch, artifact, or lifecycle-control work.
- No new public workflow-run API resource or noun appears in the changed contract.

## Open Questions

None. The lane is deliberately scoped to the existing Factory Session API vocabulary and the recovered real JavaScript session backend for simple final-only runs.
