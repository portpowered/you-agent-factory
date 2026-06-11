# PRD: Fake Session Service

## Introduction

Dynamic Workflows Batch 002 needs a deterministic in-memory durable Factory Session service so API, CLI, UI, and contract-test consumers can exercise session inspection behavior before real durable runtime persistence is complete. The service should return coherent `FactorySession`, `Dispatch`, `FactorySessionResult`, `FactoryArtifact`, and `FactoryEvent` projections for success, running, failed-with-partial, and interrupted scenarios.

The concrete change is to provide a fake session service that can be injected into durable Factory Session read and execution surfaces, backed by stable scenario data, with behavior that is deterministic across test runs and consistent across all projection endpoints.

## Context

### Customer Ask

Dynamic workflows Batch 002: implement a fake in-memory session service returning deterministic `FactorySession`, `Dispatch`, `FactorySessionResult`, `FactoryArtifact`, and `FactoryEvent` projections for success, running, failed-with-partial, and interrupted scenarios.

### Problem

The durable session API and downstream CLI/UI skeletons need realistic session data before the full durable runtime exists. Without a shared fake service, each consumer can invent its own partial view of sessions, causing contract drift across list, detail, result, dispatch, artifact, lifecycle, and event surfaces. The gap is especially risky for terminal-with-partial and interrupted scenarios because result availability, recoverability, artifacts, and event replay must agree.

### High-Level Solution

Add a deterministic in-memory implementation of the durable Factory Session service contract. It should load or define named scenarios, route start requests by stable request ids, project the same scenario state through session, result, dispatch, artifact, list, lifecycle, and event reads, and keep side effects isolated to the fake service instance. Tests should prove the projections remain internally consistent and can serve API consumer fixtures without changing public contracts.

## Goals

- Provide deterministic fake durable sessions for success, running, failed-with-partial, and interrupted scenarios.
- Ensure session detail, list summary, result, dispatch, artifact, and event reads agree for each scenario.
- Support idempotent start and lifecycle-control behavior where the public durable session contract requires it.
- Make the fake service safe for API, CLI, UI, and contract-test consumers without requiring real durable persistence.
- Keep all state explicit, isolated to one fake service instance, and safe for concurrent test access.
- Prove behavior with focused backend tests at the service and API-consumer boundary.

## Project-Level Acceptance Criteria

- [ ] A fake in-memory durable Factory Session service can be constructed with deterministic scenarios for success, running, failed-with-partial, and interrupted sessions.
- [ ] Starting or reading a fake session returns stable session ids, lifecycle state, phase/progress, result availability, dispatch counts, artifact counts, action availability, and inspection links for the selected scenario.
- [ ] Result, dispatch, artifact, and event reads are internally consistent with the session projection and return not-found or unavailable outcomes when the requested projection is not valid for that scenario.
- [ ] Repeated start requests and lifecycle controls with the same idempotency key return the same observable outcome without duplicating sessions, dispatches, artifacts, or events.
- [ ] Scoped listing distinguishes live, persisted, and all sessions while preserving deterministic ordering and recoverability for interrupted sessions.
- [ ] The fake service remains an injectable test/runtime double and does not change real runtime persistence, worker execution, or public OpenAPI schemas.
- [ ] Typecheck, lint, and focused Go tests pass, including service tests and API-surface consumer tests for the fake projections.

## User Stories

### fake-session-service-001: Provide deterministic fake scenario starts

**Description:** As an API or CLI consumer, I want a fake durable session start request to resolve to a known scenario so that tests and skeleton surfaces can inspect predictable session state.

**Acceptance Criteria:**

- [ ] Fake async start supports stable request ids for success, running, failed-with-partial, and interrupted scenarios.
- [ ] Each supported start returns the same session id, lifecycle status, result status, inspection links, and initial summary on repeated runs.
- [ ] Unknown request ids fail with an actionable service error instead of creating ad hoc scenarios.
- [ ] Repeating the same start request with the same idempotency key returns the original session outcome without creating a duplicate session.
- [ ] Concurrent repeated starts for the same scenario produce one observable session state and no duplicate dispatch, artifact, or event projections.
- [ ] Typecheck passes
- [ ] Tests pass

### fake-session-service-002: Read scoped fake session summaries and details

**Description:** As a dashboard or CLI user, I want fake session list and detail reads to show lifecycle, progress, result readiness, recoverability, and action availability so that inspection surfaces can be built against realistic durable sessions.

**Acceptance Criteria:**

- [ ] `live`, `persisted`, and `all` scoped list reads return deterministic rows with no duplicate session when a session qualifies for more than one scope.
- [ ] Running sessions appear as live and not terminal; success and failed-with-partial sessions appear as terminal persisted summaries; interrupted sessions appear as persisted and recoverable when the scenario says recovery is available.
- [ ] Session detail reads include stable phase/progress, result summary, artifact count, dispatch count, action availability, and inspection links that match the list summary for the same session.
- [ ] Missing session ids return a not-found outcome and do not mutate fake service state.
- [ ] Typecheck passes
- [ ] Tests pass

### fake-session-service-003: Return coherent fake results, dispatches, and artifacts

**Description:** As an inspection-surface implementer, I want result, dispatch, and artifact reads to line up with the fake session state so that consumers can validate terminal, partial, and running scenarios without custom fixtures per endpoint.

**Acceptance Criteria:**

- [ ] Successful sessions expose a final result with the expected primary result and optional artifacts.
- [ ] Failed-with-partial and interrupted sessions expose partial result data when requested in partial mode and report unavailable final results when no final result exists.
- [ ] Running sessions report result availability without pretending a terminal result exists.
- [ ] Dispatch list/detail reads return stable dispatch ids, statuses, timestamps, artifact refs, and interruption or failure information appropriate to the scenario.
- [ ] Artifact list/detail reads return stable artifact ids, retrieval refs, content metadata, and source dispatch linkage; missing artifact ids return not-found.
- [ ] Typecheck passes
- [ ] Tests pass

### fake-session-service-004: Replay canonical fake session events

**Description:** As a timeline or reconnect consumer, I want fake sessions to expose ordered Factory Events so that event-driven projections can be tested against the same scenarios as direct reads.

**Acceptance Criteria:**

- [ ] Each fake scenario returns an ordered event stream with stable event ids, sequence values, session context, and event kinds that match the scenario lifecycle.
- [ ] Event replay reconstructs the same session result status, dispatch status, artifact refs, and interrupted or partial terminal state exposed by direct projection reads.
- [ ] Reconnect reads after a valid event id or sequence return only later events.
- [ ] Reconnect reads after an unknown cursor return the documented cursor-not-found error without falling back to a full replay.
- [ ] Event reads for a missing session return not-found and do not mutate fake service state.
- [ ] Typecheck passes
- [ ] Tests pass

### fake-session-service-005: Apply fake lifecycle controls consistently

**Description:** As an operator testing durable session controls, I want fake pause, resume, cancel, terminate, approve, and retry-dispatch calls to return contract-shaped outcomes so that consumers can build control UX and CLI flows before real execution is available.

**Acceptance Criteria:**

- [ ] Supported controls return `ACCEPTED`, `NO_OP`, `INVALID_STATE`, `TERMINAL_SESSION`, or `CONFLICT` according to the current fake session lifecycle.
- [ ] Lifecycle control responses include inspection links for follow-up session, result, dispatch, and artifact reads when those links are valid for the scenario.
- [ ] Repeating a lifecycle control with the same idempotency key returns the same outcome and does not append duplicate events or duplicate dispatch retries.
- [ ] Controls against terminal success or failed-with-partial sessions do not move the session back into a running state.
- [ ] Retry-dispatch updates only the targeted retryable dispatch scenario and leaves unrelated dispatches and artifacts unchanged.
- [ ] Typecheck passes
- [ ] Tests pass

## High-Level Technical Design

The fake service should live at the durable session execution boundary and implement the same service contract used by API handlers and downstream test doubles. Scenario data should be explicit and deterministic. It may be authored as contract fixtures plus built-in scenarios when that keeps API contract validation and service behavior aligned.

State ownership belongs to the fake service instance. Starts and lifecycle controls may mutate the in-memory copy for that instance, but fixture definitions should remain immutable. All reads should project from the same session state so list rows, details, results, dispatches, artifacts, and events cannot drift.

Projection rules should favor existing durable Factory Session vocabulary: `FactorySession`, `FactorySessionResult`, `Dispatch`, `FactoryArtifact`, and `FactoryEvent`. Internal implementation may use helper structs, but public behavior should remain aligned with the customer-facing durable session model and generated OpenAPI types.

Concurrency should be explicit. The fake service is primarily for tests and skeleton surfaces, but idempotent starts and control operations should be safe under concurrent access so automated tests can run with race detection where practical.

## Functional Requirements

- FR-1: The fake service must implement the durable session execution, listing, projection, lifecycle, and event-read service methods needed by API and test consumers.
- FR-2: The service must support deterministic success, running, failed-with-partial, and interrupted scenarios.
- FR-3: Start requests must route to scenarios by stable request id or equivalent explicit scenario selector and must reject unknown selectors.
- FR-4: Idempotent start replay must return the original observable result for the same request identity.
- FR-5: Session list and detail projections must include lifecycle status, phase/progress, result summary, artifact count, dispatch count, recoverability, action availability, and inspection links where supported by the contract.
- FR-6: Result reads must support final and partial modes and must distinguish available, partial, unavailable, and not-found outcomes.
- FR-7: Dispatch and artifact reads must support list and detail retrieval with stable ids and API-relative artifact retrieval refs.
- FR-8: Event reads must return canonical ordered Factory Events and support reconnect filtering by cursor.
- FR-9: Lifecycle controls must return contract-shaped outcomes and preserve idempotency.
- FR-10: Focused tests must prove projection consistency across direct reads and event replay.

## Non-Goals

- No real durable persistence, database migrations, or filesystem-backed checkpoint store implementation.
- No real worker execution, model invocation, JavaScript runtime execution, or scheduler behavior.
- No new public REST routes or OpenAPI schema changes unless existing contract gaps are discovered during implementation.
- No frontend visual changes in this work item; UI consumers may use the fake service through existing or separately planned API surfaces.
- No broad cleanup of durable session packages, CLI commands, dashboard views, or generated clients outside what is required to inject and verify the fake service.

## Supporting Technical And UX Considerations

- Keep scenario names and request ids readable because they will appear in tests and fixture-driven debugging.
- Use API-relative links and artifact refs so consumers do not depend on host-local filesystem paths.
- Preserve live-session compatibility surfaces while adding fake durable behavior behind explicit durable session seams.
- Prefer behavioral tests over inventory or topology tests; prove what the service returns and how consumers observe it.
- Log or classify fake-service errors enough for failed tests and skeleton API responses to be diagnosable.
- Avoid exposing internal Petri-net vocabulary in API-facing fake projections unless the existing contract already requires it.

## Success Metrics

- API, CLI, and UI skeleton implementers can use the same fake session ids and scenario names without inventing local fixture variants.
- Contract and service tests prove success, running, failed-with-partial, and interrupted scenarios across list, detail, result, dispatch, artifact, and event reads.
- Idempotency and reconnect behavior are deterministic under repeated and concurrent test runs.
- No generated OpenAPI drift or real runtime behavior regression is introduced by the fake service.

## Open Questions

- None. The required scenario set and projection families are defined by the customer ask.
