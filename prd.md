# PRD: Recover CLI Start, Status, and Result Loopback Against Deterministic Factory Sessions

## Introduction

Recover the dynamic workflow CLI run/status/result path by routing `RUN_SIMPLE-CLI`, `RUN_ASYNC-CLI`, and `RESULT-CLI` behavior through deterministic Factory Session fixtures. A user should be able to start a JavaScript Factory Session from the CLI in sync or async mode, poll status, inspect dispatches/artifacts/events where the CLI supports those views, and retrieve final or partial result data without depending on the real JavaScript runtime, HTTP API wiring, dashboard UI, or the previously blocked work-plan token.

The concrete problem is that the earlier `dynamic-workflows-cell-cli-run-status-result` work item is blocked by failed paired plan token recovery. This narrower recovery plan keeps customer-visible CLI behavior moving by using the completed mock provider fixture path and the shared Factory Session execution request/status/result/dispatch/artifact types.

The high-level solution is to normalize CLI input into the shared execution domain, execute fixture-backed sync and async session flows, render concise human output and deterministic JSON output for all expected states, and prove recovery paths with focused CLI tests.

## Project-Level Acceptance Criteria

- [ ] A sync CLI run of the deterministic fixture reports a completed outcome and includes either the expected structured result or the expected stable result hash.
- [ ] An async CLI run returns a durable Factory Session id, and follow-up status/result commands report the same fixture state using that id.
- [ ] A timed-out sync run reports timeout without claiming completion, and cancel-on-timeout behavior is visible when requested.
- [ ] CLI output is concise for humans and deterministic for JSON across started, running, completed, timed-out, failed, not-ready, missing-session, and conflict outcomes.
- [ ] Focused CLI tests cover not-ready result, missing session, unsupported mode, bad source, and requestId conflict paths.
- [ ] Typecheck, lint, and focused tests pass, including `go test ./pkg/cli/session ./pkg/cli/workflow ./pkg/factorysessionexecution`.

## Goals

- Restore customer-testable CLI loopback for starting and inspecting JavaScript Factory Sessions with deterministic fixtures.
- Keep CLI/API behavior aligned by using shared domain or `apisurface` normalization for source, args, requestId, wait/timeout, and cancel-on-timeout semantics.
- Provide deterministic JSON output suitable for automation and concise human output suitable for terminal use.
- Prove timeout, cancellation, not-ready, missing-session, unsupported-mode, bad-source, and requestId conflict behavior with focused tests.
- Keep this recovery independent from API handlers, dashboard UI, and the real JavaScript runtime swap-in.

## User Stories

### dynamic-workflows-recovery-cli-run-status-result-001: Run a Fixture Synchronously to Completion
**Description:** As a CLI user, I want a sync run command to execute a deterministic JavaScript Factory Session fixture and return the completed result so that I can verify the run loopback from the terminal.

**Acceptance Criteria:**
- [ ] A sync run against the fixture exits successfully when the fixture reaches completed state.
- [ ] Human output clearly reports the completed outcome without dumping verbose internal state.
- [ ] JSON output includes deterministic session identity/status fields and either the expected structured result or the expected stable result hash.
- [ ] CLI input handling for source, args, requestId, wait, and timeout uses shared execution/domain normalization rather than transport-specific duplicate rules.
- [ ] Tests pass.
- [ ] Typecheck passes.

### dynamic-workflows-recovery-cli-run-status-result-002: Start Asynchronously and Inspect Status and Result
**Description:** As a CLI user, I want an async start command to return a durable Factory Session id so that I can poll status and retrieve the fixture result later.

**Acceptance Criteria:**
- [ ] Async start returns a durable session id in human output and JSON output.
- [ ] A follow-up status command using that session id reports the fixture state consistently.
- [ ] A follow-up result command using that session id returns completed result data when the fixture is complete.
- [ ] A result command against a still-running fixture reports not-ready without fabricating a final result.
- [ ] Tests pass.
- [ ] Typecheck passes.

### dynamic-workflows-recovery-cli-run-status-result-003: Report Timeout and Optional Cancel-on-Timeout
**Description:** As a CLI user, I want a sync run with a short timeout to report timeout accurately and optionally cancel the session so that automation can distinguish incomplete work from completed work.

**Acceptance Criteria:**
- [ ] A sync run that exceeds its wait/timeout reports a timed-out outcome and does not claim completion.
- [ ] JSON output for timeout is deterministic and includes enough state for automation to know the session did not complete.
- [ ] When cancel-on-timeout is requested, output visibly reports that cancellation was requested or applied according to fixture behavior.
- [ ] Tests prove both timeout-without-cancel and timeout-with-cancel behavior.
- [ ] Tests pass.
- [ ] Typecheck passes.

### dynamic-workflows-recovery-cli-run-status-result-004: Inspect Dispatches, Artifacts, and Events From the CLI
**Description:** As a CLI user, I want supported inspection commands to list dispatches, artifacts, and polled events for a fixture-backed Factory Session so that I can understand partial session state before or after completion.

**Acceptance Criteria:**
- [ ] Dispatch list output shows deterministic fixture dispatch records when the CLI command exists for this surface.
- [ ] Artifact list output shows deterministic fixture artifact records when the CLI command exists for this surface.
- [ ] Event polling output reports fixture lifecycle/result events in stable order when the CLI command exists for this surface.
- [ ] Unsupported inspection modes fail with a clear unsupported-mode outcome rather than falling through to ambiguous errors.
- [ ] Tests pass.
- [ ] Typecheck passes.

### dynamic-workflows-recovery-cli-run-status-result-005: Cover Recovery and Error Paths
**Description:** As a CLI user or maintainer, I want invalid or incomplete fixture interactions to return stable errors so that recovery behavior is predictable and testable.

**Acceptance Criteria:**
- [ ] A missing session id returns a clear missing-session outcome in human and JSON output.
- [ ] A bad source returns a clear source error without starting a session.
- [ ] A repeated conflicting requestId returns a conflict outcome without creating ambiguous duplicate results.
- [ ] Not-ready result, missing session, unsupported mode, bad source, and requestId conflict paths have focused CLI coverage.
- [ ] Tests pass.
- [ ] Typecheck passes.

## High-Level Technical Design

The CLI should adapt user input into the shared Factory Session execution request shape and avoid inventing CLI-only semantics for source resolution, args, requestId, wait/timeout, or cancel-on-timeout. Deterministic mock provider fixtures are the execution dependency for this recovery batch. The mock path should return shared status, result, dispatch, artifact, and event shapes so later real JavaScript runtime and backend swap-in can preserve the same CLI behavior.

Command output should have two stable modes: concise human output for terminal use and deterministic JSON output for tests and automation. Status vocabulary should remain Factory Session centered and avoid exposing internal Petri-net terms in public output.

This work should remain independent from HTTP API handlers and dashboard UI. Later API parity can reuse the same shared execution/domain behavior after the API cell is ready.

## Functional Requirements

- FR-1: The CLI must support fixture-backed sync start for JavaScript Factory Sessions and return completed result data or a stable result hash.
- FR-2: The CLI must support fixture-backed async start and return a durable Factory Session id.
- FR-3: The CLI must support follow-up status and result retrieval by Factory Session id.
- FR-4: The CLI must report not-ready result state without fabricating completion.
- FR-5: The CLI must report timed-out sync runs without claiming completion.
- FR-6: The CLI must make cancel-on-timeout behavior visible when requested.
- FR-7: The CLI must provide deterministic JSON output for started, running, completed, timed-out, failed, not-ready, missing-session, unsupported-mode, bad-source, and conflict outcomes.
- FR-8: Where existing command structure supports it cleanly, the CLI must expose dispatch list, artifact list, and event polling over the fixture-backed session state.

## Non-Goals

- Do not implement or wait for the real JavaScript runtime swap-in.
- Do not wire new HTTP API handlers or dashboard UI behavior in this recovery batch.
- Do not move or retry the failed paired plan token `work-plan-55`.
- Do not introduce a standalone DynamicWorkflowRun product model; use Factory Session, Dispatch, FactoryArtifact, and FactoryEvent vocabulary.
- Do not perform broad CLI command rewrites or unrelated cleanup outside the run/status/result recovery behavior.

## Supporting Technical and UX Considerations

- Use mock provider fixtures first, based on the completed `dynamic-workflows-cell-provider-fixtures` / `FOUND-SESSION_MOCK` behavior.
- Preserve CLI and future API equivalence by normalizing shared request and result semantics outside the transport boundary.
- Keep side effects isolated at the CLI execution boundary; fixture state should be deterministic and test-controllable.
- Use package-level CLI tests for command behavior and package execution tests for fixture/provider behavior.
- Human output should be short and actionable; JSON output should be stable enough for exact assertions.
- Logs and errors should include enough context to diagnose source errors, requestId conflicts, timeouts, and missing sessions without exposing internal implementation details.

## Verification

- `go test ./pkg/cli/session ./pkg/cli/workflow ./pkg/factorysessionexecution`
- Run broader repository verification only if implementation touches shared contracts beyond the CLI/session execution boundary.

## Success Metrics

- A maintainer can run one sync fixture command and observe completed result data or a stable result hash.
- A maintainer can run one async fixture command, copy the returned Factory Session id, and retrieve matching status/result data.
- Timeout, cancel-on-timeout, not-ready, missing-session, unsupported-mode, bad-source, and requestId conflict outcomes are covered by deterministic tests.
- No API handler, dashboard UI, or real runtime dependency is required to prove this recovery batch.

## Open Questions

None for this recovery batch.
