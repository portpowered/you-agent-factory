# Explicit session authority for concurrent customer invocations

## Status

In progress. The 2026-09-01 audit confirmed that explicit durable source
selection is already public and that ordinary hosted Factory Sessions can
overlap. Immutable Factory Session identity now reaches the provider command
effect boundary. Local CLI selection of an existing session, ordinary inline
JSON Factory execution through the durable endpoint, compatibility positional
input for remote runs, and explicit-session replay rehydration remain open.

## Problem

The Factory runtime supports concurrent explicit Factory Sessions, but several customer entry points still select the process-owned Current Factory or the implicit `~default` session. Reusing one assembled process for overlapping invocations therefore either produces a stable "runtime already bound" rejection or makes external-effect routing depend on mutable current-factory state.

The affected paths currently include:

- one-shot `Process.Execute` calls that exercise `you run` without explicit session authority;
- local CLI execution that cannot select an already-open Factory Session;
- ordinary inline JSON Factory execution through `POST /factory-sessions/async`,
  which currently reaches the JavaScript workflow validator despite the
  public `FACTORY_INLINE` definition contract;
- remote compatibility positional input, which is rejected unless an
  invocation signature normalized it to arguments;
- explicit non-default session replay, whose fresh process does not currently
  rehydrate the original public session route.

The external command-effect identity gap is complete: provider correlation and
script workflow context now copy the Factory Session ID through their owned
command requests into the platform request's policy-free execution scope.

This is a customer concurrency and testability gap, not a limitation of session-owned Factory execution. Hosted tests already demonstrate that multiple explicit Factory Sessions can overlap on one process.

## Customer outcome

Customers and API clients can start concurrent invocations on one hosted process while explicitly selecting the intended Factory or Factory Session. Each invocation retains isolated Work, events, responses, provider effects, and lifecycle controls. The implicit `~default` behavior remains available for a single Current Factory invocation but is no longer the only authority exposed by CLI-equivalent or inline-workflow entry points.

## Scope

- Add explicit Factory/session selection to customer invocation contracts that currently rely on the Current Factory.
- Resolve that selection once at admission and carry the immutable Factory Session ID through execution.
- Include an immutable execution/session identity at replaceable external-effect boundaries so deterministic adapters can route concurrent calls without reading mutable global state.
- Migrate affected functional packages to one hosted package process, one explicit session per scenario, and `t.Parallel()` for isolated scenarios.
- Remove characterization tests that merely assert the internal `~default` binding error after the supported concurrent customer path exists.

## Non-goals

- Removing the Current Factory or the `~default` session.
- Allowing concurrent mutation of one Factory Session by tests that require ordered customer actions.
- Introducing test-only production APIs, global mutable fixture selectors, sleeps, or process-per-scenario fallbacks.
- Combining independent Go packages into one process.

## Contract decisions to make

1. Define whether `you run` selects an existing session, opens a session for a Factory target, or supports both operations.
2. Extend the async Factory Session execution request with a customer-facing target/session selector consistent with existing Factory Session vocabulary.
3. Define precedence and validation when a Current Factory, Factory target, and session selector are all available.
4. Decide whether external command requests carry `FactorySessionID` directly or receive a more general immutable execution context.
5. Preserve CLI/API equivalence for shared invocation behaviors and generated OpenAPI clients.

## Implementation sequence

1. Specify CLI and HTTP acceptance criteria for explicit target/session selection, invalid combinations, missing sessions, and terminal sessions.
2. Update the authored OpenAPI fragments and CLI parsing contracts; regenerate Go and TypeScript clients.
3. Resolve session authority at the Factory Sessions boundary and remove downstream reliance on mutable Current Factory selection.
4. Propagate immutable session identity through Worker and Provider execution contexts and the replaceable command-effect boundary.
5. Add focused unit tests for parsing, precedence, mapping, and propagation. These tests must not start an assembled process.
6. Add a small integration test using a compiled `you` artifact to prove CLI transport wiring.
7. Convert the modes and inline-JavaScript functional scenarios to a shared hosted process with unique explicit sessions and parallel subtests.
8. Run normal and race-enabled package tests, then three full functional timing runs.

## Acceptance criteria

- Two concurrent customer invocations selecting different Factory Sessions complete successfully on one process.
- Work, Factory Events, response events, provider calls, and cleanup remain correlated to the selected session with no cross-session leakage.
- Inline JavaScript execution can select its owning Factory/session without mutating or depending on the Current Factory.
- Replaceable external effects can route concurrent calls using immutable execution identity.
- The race detector passes for every migrated functional package.
- Functional tests do not build a binary, do not assert internal binding details, and use `t.Parallel()` wherever their sessions are independent.
- Compiled CLI coverage remains in the integration lane and uses a prebuilt artifact.

## Dependencies and related work

- `docs/internal/development/plans/backlog/shared-functional-process-sessions.md` defines the broader package-migration pattern.
- `factory/docs/standards/testing-standards.md` defines the normative test-layer rules.
- Public naming and selector behavior must remain aligned with `docs/architecture/data-model.md`.

## Replanning triggers

- Session selection requires a breaking public contract rather than an additive selector.
- Execution identity cannot be propagated without changing a stable external-effect interface.
- A scenario proves that ordered actions intentionally share one customer session; that scenario remains serialized and documents the customer reason.
