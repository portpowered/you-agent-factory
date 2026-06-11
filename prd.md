# PRD: Simple JavaScript Workflow Runtime Boundary

---
author: Codex
last modified: 2026, june, 11
status: draft
---

## Introduction

Implement the first real JavaScript workflow runtime boundary for simple final-only dynamic workflows. A JavaScript workflow fixture should execute through a controlled Go-owned runtime boundary, receive structured `args` and metadata, produce a final value through either a returned value or `workflow.final`, and project that terminal output through the existing JavaScript result validation and primary-result helpers.

This is the `RUN_SIMPLE-JS_RUNTIME` deliverable cell. It is independent from provider dispatch, CLI, MCP, API transport, session backend, and dashboard work. The runtime must be intentionally narrow: enough to prove simple workflow execution, deterministic failures, cancellation/timeout behavior, and default denial of direct host access.

## Context

### Customer Ask

Implement the simple JavaScript workflow runtime boundary so a real JavaScript workflow fixture can execute through the runtime path with `args`, `meta`, returned final value or `workflow.final`, structured result projection, deterministic errors, and cancellation/not-ready behavior.

### Problem

Dynamic workflow planning depends on a real JavaScript runtime cell that can prove execution semantics before session backend, CLI, MCP, or provider-dispatch integrations swap away from mocks. Without this boundary, later surfaces can only test fixture or fake-provider behavior and cannot verify whether JavaScript source execution, result projection, validation errors, timeout handling, and host-access restrictions behave correctly.

### High-Level Solution

Add the smallest testable JavaScript runtime mechanism that fits backend standards. Define a narrow runtime interface that executes already-loaded JavaScript source with structured inputs, metadata, policy, cancellation context, and result/artifact hooks. Support simple final-only workflows, reject or clearly diagnose unsupported final paths, and route terminal results through the existing JavaScript result and primary-result projection helpers. Add fixtures and runtime tests for successful finals, thrown errors, syntax or validation failures before execution, cancellation/timeout, unresolved final values, and denied host access.

## Project-Level Acceptance Criteria

- [ ] A `simple-final.workflow.js` fixture executes through the real runtime boundary and returns the expected structured result through existing result projection.
- [ ] Both returned final values and `workflow.final` final values are supported, or any unsupported final path fails with a clear diagnostic and a documented follow-up.
- [ ] Runtime failures are deterministic for thrown errors, syntax or validation failures before execution, unresolved final values, cancellation/timeout, and denied host access.
- [ ] The default runtime environment does not grant direct filesystem, process, network, shell, or other host access.
- [ ] The runtime API accepts explicit source, args, metadata, policy, cancellation context, and result/artifact hooks without relying on package-global mutable execution state.
- [ ] The implementation remains scoped to JavaScript runtime behavior and does not add provider dispatch bridge, HTTP/CLI/MCP wiring, dashboard UI, or full host API primitives.
- [ ] **Quality gate:** typecheck, lint, and focused tests pass, including `go test ./pkg/orchestrators/javascript/...` and a targeted host-access check such as `rg -n "os\\.Exec|exec\\.Command|net/http|fs\\." pkg/orchestrators/javascript`.

## Goals

- Execute a real simple JavaScript workflow fixture through the runtime boundary.
- Preserve canonical factory/session architecture by keeping this work inside JavaScript orchestrator runtime ownership, not transport or provider surfaces.
- Pass structured `args` and metadata into workflow source without source rewriting.
- Support final result projection through existing result validation and primary-result helpers.
- Produce clear, test-covered errors for execution, validation, cancellation, timeout, unsupported capability, and unresolved-result cases.
- Keep host access denied by default for filesystem, process, network, and shell capabilities.

## User Stories

### dynamic-workflows-cell-js-runtime-simple-001: Execute returned final workflow

**Description:** As a dynamic workflow implementer, I want a simple JavaScript workflow to return a structured final value through the runtime boundary so later session and transport work can rely on real execution semantics.

**Acceptance Criteria:**

- [ ] A `simple-final.workflow.js` fixture receives structured `args` and metadata from the runtime input.
- [ ] Executing the fixture through the runtime boundary returns the expected structured final value without using provider dispatch or transport wiring.
- [ ] The terminal value is projected through existing JavaScript result validation and primary-result helpers.
- [ ] Re-running the fixture with the same inputs produces the same result and no hidden host side effects.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-js-runtime-simple-002: Support or clearly reject `workflow.final`

**Description:** As a workflow author, I want the runtime to handle `workflow.final` consistently so simple final-only workflows have a clear supported or explicitly diagnosed completion path.

**Acceptance Criteria:**

- [ ] A workflow fixture that calls `workflow.final` either produces the expected structured final result or fails with a stable unsupported-path diagnostic.
- [ ] If `workflow.final` is unsupported in this slice, the diagnostic names the unsupported final mechanism and points to the follow-up runtime capability without panicking.
- [ ] The result selection rule is deterministic when a workflow both returns a value and attempts `workflow.final`.
- [ ] The behavior is covered by focused runtime tests.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-js-runtime-simple-003: Report pre-execution validation failures

**Description:** As a maintainer, I want invalid JavaScript source or invalid workflow shape to fail before execution so malformed workflows do not produce ambiguous runtime behavior.

**Acceptance Criteria:**

- [ ] A syntax-error fixture fails before workflow execution begins and returns a typed or otherwise stable diagnostic.
- [ ] A validation-failure fixture fails before execution when required runtime expectations for a simple workflow are missing or invalid.
- [ ] Pre-execution failures do not emit a final result or call result/artifact hooks as though execution succeeded.
- [ ] Error messages include enough source or validation context for a maintainer to identify the failing fixture behavior without reading runtime logs.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-js-runtime-simple-004: Surface deterministic execution failures

**Description:** As a workflow operator, I want thrown JavaScript errors and unresolved final values to produce clear runtime failures so failures can be shown consistently by later session and CLI surfaces.

**Acceptance Criteria:**

- [ ] A fixture that throws during execution returns a stable runtime failure with the JavaScript error message or code preserved in a safe diagnostic.
- [ ] A fixture that completes without a return value or accepted final value returns a stable unresolved-final failure rather than a successful empty result.
- [ ] Failure results do not pass through primary-result projection as successful final output.
- [ ] Runtime tests cover thrown error and unresolved final value cases.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-js-runtime-simple-005: Enforce cancellation and timeout behavior

**Description:** As a session backend implementer, I want the runtime to respect cancellation and timeout context so later factory sessions can stop JavaScript workflows predictably.

**Acceptance Criteria:**

- [ ] A runtime execution with a canceled context returns a canceled/not-ready style failure and does not report a successful final result.
- [ ] A fixture that exceeds the configured timeout returns a timeout diagnostic within the test's bounded wait period.
- [ ] Cancellation or timeout cleanup prevents result/artifact hooks from being called after the terminal canceled or timed-out outcome.
- [ ] Runtime tests cover explicit cancellation and timeout behavior without relying on long sleeps.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-cell-js-runtime-simple-006: Deny direct host access by default

**Description:** As a security-conscious maintainer, I want simple JavaScript workflows to run without filesystem, process, network, or shell access by default so the runtime boundary is safe before host APIs are deliberately added.

**Acceptance Criteria:**

- [ ] A fixture that attempts unsupported import, filesystem, process, network, or shell access fails with a clear denied-capability diagnostic.
- [ ] The default runtime policy exposes only the narrow globals needed for this cell, including structured `args`, metadata, and the supported final mechanism.
- [ ] Denied host access is covered by runtime tests and does not perform the attempted host side effect.
- [ ] A targeted code search such as `rg -n "os\\.Exec|exec\\.Command|net/http|fs\\." pkg/orchestrators/javascript` is reviewed so any matches are intentional and not default workflow access paths.
- [ ] Typecheck passes.
- [ ] Tests pass.

## High-Level Technical Design

The runtime boundary should be a small Go-owned interface for executing already-loaded JavaScript source. Inputs should be explicit: source identity/content, structured `args`, validated metadata, effective policy, cancellation context, and narrow hooks for final result or artifacts. Outputs should distinguish successful final results from validation failures, runtime failures, cancellation, timeout, denied capabilities, and unresolved finals.

This cell should use the real runtime path for simple-final workflows and depend on the JavaScript validation/source packages under `pkg/orchestrators/javascript` plus the result projection helpers under `pkg/orchestrators/javascript/result`. It should avoid broad host APIs and provider dispatch. Any runtime library choice must be testable in Go and configured so the default execution environment does not expose direct filesystem, process, network, or shell access.

Result handling should reuse existing JavaScript result validation and primary-result helpers. Runtime tests should exercise fixtures rather than asserting package topology or implementation inventories. The only structural verification called out is the targeted host-access search because the customer explicitly requires proving that default runtime access is not accidentally implemented through host APIs.

## Functional Requirements

1. FR-1: The runtime must execute a simple JavaScript workflow source with structured `args` and metadata.
2. FR-2: The runtime must return or project a successful structured final result for the `simple-final.workflow.js` fixture.
3. FR-3: The runtime must support returned final values and either support `workflow.final` or fail unsupported `workflow.final` usage with a clear diagnostic and documented follow-up.
4. FR-4: The runtime must validate syntax and simple workflow shape before execution where practical and must avoid reporting pre-execution failures as successful final results.
5. FR-5: The runtime must return deterministic diagnostics for thrown JavaScript errors and unresolved final values.
6. FR-6: The runtime must respect cancellation context and configured timeout limits.
7. FR-7: The runtime must deny direct filesystem, process, network, shell, and unsupported import access by default.
8. FR-8: The runtime must route terminal successful results through existing JavaScript result validation and primary-result projection helpers.
9. FR-9: Runtime tests must cover success, alternate final path, validation failure, thrown error, unresolved final, cancellation/timeout, and denied host access.

## Non-Goals

- No provider dispatch bridge.
- No full host API primitives such as `agent`, `parallel`, `pipeline`, artifacts, checkpoints, budgets, or live child execution.
- No HTTP, CLI, MCP, or dashboard wiring.
- No durable factory-session backend implementation.
- No public OpenAPI contract changes or generated client updates.
- No direct workflow filesystem, process, network, or shell access by default.

## Supporting Technical and UX Considerations

- Backend ownership should remain under JavaScript orchestrator packages and should keep side effects isolated behind the runtime boundary.
- Runtime errors should be safe to expose through later CLI/API/MCP/session surfaces: concise, stable, and actionable without leaking host internals or secrets.
- Cancellation and timeout behavior should use Go context and bounded tests rather than unbounded sleeps or goroutine leaks.
- The implementation should be narrow enough that later cells can add session backend, CLI, MCP, and host primitives without changing the basic final-result contract.
- There is no visible frontend behavior in this cell, so browser verification is not required.

## Success Metrics

- `go test ./pkg/orchestrators/javascript/...` passes with fixture-backed runtime coverage for all required success and failure modes.
- A simple returned-final workflow produces the expected structured primary result through the runtime boundary.
- `workflow.final` behavior is either supported by tests or rejected with a stable diagnostic and named follow-up.
- Host access denial tests demonstrate that default workflows cannot directly read files, spawn processes, access the network, or execute shell commands.
- Later session backend, CLI, and MCP cells can depend on this runtime boundary without using mock execution for simple-final workflows.

## Open Questions

None. If `workflow.final` cannot be safely supported in this slice, the accepted behavior is a clear unsupported-path diagnostic plus a documented follow-up.
