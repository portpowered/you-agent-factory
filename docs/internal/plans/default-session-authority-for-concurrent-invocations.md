# Explicit session authority for concurrent customer invocations

## Status

Implementation and validation are complete.
Remote and local CLI invocations can select an already-open Factory Session,
inline JSON Factory Definitions execute through an invocation-owned session,
and fresh-process replay restores the explicit public session route. ACP
allocates its Factory Session identity before runtime construction and captures
the invocation's home/profile without a process-global environment lease. The
affected functional cohorts use shared processes, unique sessions, and parallel
customer scenarios.

## Problem

The Factory runtime supported concurrent explicit Factory Sessions, but several customer entry points selected the process-owned Current Factory or the implicit `~default` session. Reusing one assembled process for overlapping invocations therefore either produced a stable "runtime already bound" rejection or made external-effect routing depend on mutable current-factory state.

The corrected paths included:

- one-shot `Process.Execute` calls that exercise `you run` without explicit session authority;
- local CLI execution that previously could not select an already-open Factory Session;
- ordinary inline JSON Factory execution through `POST /factory-sessions/async`,
  which reached the JavaScript workflow validator without first resolving the
  public `FACTORY_INLINE` definition contract;
- local compatibility invocation without a hosted server;
- explicit non-default session replay in a fresh process; and
- ACP activation, whose production home/settings resolver previously read
  process-global environment state during startup and the first prompt.

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
- Resolve ACP home and operator settings from invocation/session-owned input,
  capture them on the Chat/Factory Session, and stop consulting or mutating
  process-global environment state after admission.
- Migrate affected functional packages to one hosted package process, one explicit session per scenario, and `t.Parallel()` for isolated scenarios.
- Remove characterization tests that merely assert the internal `~default` binding error after the supported concurrent customer path exists.

## Non-goals

- Removing the Current Factory or the `~default` session.
- Allowing concurrent mutation of one Factory Session by tests that require ordered customer actions.
- Introducing test-only production APIs, global mutable fixture selectors, sleeps, or process-per-scenario fallbacks.
- Combining independent Go packages into one process.

## Contract decisions to make

1. [x] Define remote `you run --session` as invocation of an existing hosted session; omission continues to open a durable remote session.
2. [x] Reuse the existing Factory Session selector at invocation admission; no duplicate async HTTP selector is required.
3. [x] An explicit session wins over Current Factory compatibility authority; omission retains `~default` behavior.
4. [x] Keep domain-owned `FactorySessionID` and project it to the platform-owned `ExecutionScopeID` at the effect boundary.
5. [x] Preserve CLI/API equivalence for shared invocation behaviors and generated OpenAPI clients.
6. [x] Capture ACP home/profile and operator-setting resolution at admission so
   two different profiles initialize concurrently without a process-global
   environment lease.

## Implementation sequence

1. [x] Specify CLI acceptance criteria for explicit target/session selection, invalid combinations, missing sessions, and terminal sessions.
2. [x] Update the authored CLI parsing contract and regenerate its consumers; the existing HTTP Factory Session contract required no change.
3. [x] Resolve session authority at the Factory Sessions boundary and remove downstream reliance on mutable Current Factory selection.
4. [x] Propagate immutable session identity through Worker and Provider execution contexts and the replaceable command-effect boundary.
5. [x] Add focused unit tests for manifest parsing, mapping, retained-event replay, and propagation. These tests do not start an assembled process.
6. [x] Preserve the existing compiled-artifact integration coverage for CLI transport wiring; functional tests use the in-process public command boundary.
7. [x] Convert run modes, AGY review, inline JavaScript, replay, packaged Factory, MCP, Workers, and CLI scenarios to shared processes with unique explicit sessions and parallel subtests.
8. [x] Replace ACP's process-global home resolver with an invocation/session-owned
   resolver and convert the Chat root-composition cohorts to independent
   parallel leaves with scenario-scoped observations.
9. [x] Run normal and race-enabled package tests, then the canonical full functional lane.

## Validation evidence

- The fresh bounded functional lane passed after the explicit-session
  migrations. The affected cohorts completed in 6.942 seconds or less:
  Models root composition (6.942s), Workers inference (3.955s), Workers CLI
  lifecycle (3.662s), Factory Session lifecycle (2.580s), Work routing
  (1.394s), Chat Session root composition (0.292s), and ACP stdio (0.032s).
- The complete bounded race lane passed with
  `go test -race ./tests/functional/... -p=2 -count=1 -timeout 300s`.
  Formerly blocked packages passed without detector findings, including ACP
  stdio (4.433s), Chat Session root composition (12.552s), Workers inference
  (12.024s), Workers CLI lifecycle (10.618s), Models root composition
  (12.538s), and Work routing (2.767s) under race instrumentation.
- Models catalog commands now own one runtime scope per overlapping command;
  the formerly flaky local diagnostic cohort passed 30 race-enabled
  repetitions and the whole package passed three race-enabled repetitions.
- Work routing uses the Go test runner's parallel scheduler rather than
  invoking `t.Run` from a goroutine pool. Its 11 customer scenarios passed 100
  race-enabled repetitions at eight-way concurrency after removing a
  fixture-router self-test that installed package-wide ambiguous selectors.
- The inference Worker Session history, cursor/follow, and replay journeys were
  re-audited and migrated from accidental `~default` authority to one explicit
  Factory Session carried into both the original and replay processes.
- Source audits found no package-wide mutex spanning a changed functional
  `Process.Execute`, no process-global environment mutation in the changed
  functional tests, and no remaining accidental default-session use in the
  migrated scenarios.
- The functional coverage floors for the internal `processlifecycle` and
  `runtimebinding` packages were reconciled to 131/174 (75.28%) and 293/508
  (57.67%). The former floors depended on timing-sensitive cancellation during
  startup and an accidental default-session collision. Those internal branches
  remain unit-tested; the functional lane now measures only the retained
  customer journeys, as required by the testing standard.

## Acceptance criteria

- Two concurrent customer invocations selecting different Factory Sessions complete successfully on one process.
- Work, Factory Events, response events, provider calls, and cleanup remain correlated to the selected session with no cross-session leakage.
- Inline JavaScript execution can select its owning Factory/session without mutating or depending on the Current Factory.
- Replaceable external effects can route concurrent calls using immutable execution identity.
- Two ACP invocations with different isolated customer homes initialize and
  execute concurrently without changing `os.Environ` or waiting on a fixture
  home mutex.
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
