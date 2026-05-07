# General Backend Standards

---
author: andreas abdi
last modified: 2026, may, 1
doc-id: STD-017
---

This document defines the baseline standards for backend systems built in this repository. It is intended to be broad enough for most backend services while still being concrete enough to review against.

For this repository, these standards apply directly to the Go backend under `cmd/`, `pkg/`, `internal/`, `api/`, and `tests/`.

## Usage

Every contributor who changes backend logic, APIs, runtime behavior, persistence behavior, background processing, or backend tests **MUST** review this standard before implementation or review.

## Quick Rules

- Prefer small, deeply understandable modules over large, clever ones.
- Keep state explicit, local, and minimal; prefer stateless functions and referentially transparent logic whenever practical.
- Separate pure domain logic from transport, IO, time, filesystem, environment, and process boundaries.
- Reject unexplained magic values, oversized functions, oversized files, and hidden side effects.
- Use linting, static checks, and CI gates to enforce backend quality rather than relying on reviewer memory alone.
- Favor a healthy test pyramid: few fast unit tests, majority integration tests, targeted functional tests, and explicit stress coverage where concurrency or scale matters.
- Instrument systems so logs, metrics, traces, and operational diagnostics make failures understandable.
- Design dependency calls with explicit latency, timeout, retry, and backoff behavior.
- Test performance, load, and failure modes intentionally rather than assuming correctness implies resilience.
- Keep public contracts, generated code, and handwritten domain logic clearly separated.
- Optimize for reduced complexity, clear interfaces, and easy changeability in the spirit of John Ousterhout's design guidance.

## Review Checklist

Before approval, reviewers **SHOULD** confirm:

- The change fits the repository's package boundaries and dependency direction.
- State and side effects are intentional, isolated, and no broader than necessary.
- Pure logic is testable without live IO, clocks, processes, or network dependencies.
- Magic literals, hidden policies, and unexplained special cases are avoided or named clearly.
- Functions and files remain small enough to understand quickly.
- The change includes the right mix of unit, integration, functional, and stress evidence.
- Network dependency behavior is explicit about timeout, retry, failure, and observability strategy.
- Operational signals are sufficient to diagnose latency, failures, and degraded dependency behavior.
- CI and lint surfaces would catch the class of failure introduced by this area in the future.
- Public contracts, generated artifacts, and runtime behavior remain aligned.

## Regulations

### 1. Architecture and Package Boundaries

Backend code **MUST** be organized around clear responsibilities and dependency direction.

Preferred structure:

- `cmd/` for entrypoints and process wiring
- `pkg/` for reusable domain, runtime, and service packages
- `internal/` for repository-local helpers, enforcement tools, and implementation details
- `api/` for public contracts and generated boundary inputs
- `tests/` for functional, integration-like, and stress-oriented verification

Rules:

- Entrypoints **MUST** be thin and delegate behavior into packages.
- Domain logic **MUST NOT** depend directly on CLI parsing, HTTP transport, filesystem layout, or process-global state unless that is the package's explicit purpose.
- Public contract translation **SHOULD** happen at boundaries rather than being spread through business logic.
- Generated code **MUST** remain generated and **MUST NOT** become the home for handwritten behavior.
- Dependency direction **SHOULD** flow from transport and runtime edges inward to domain logic, not the reverse.

### 2. Statefulness and Functional Style

Backend systems **MUST** prefer explicit data flow over hidden mutable state.

Rules:

- Stateless designs are the default.
- Pure functions **SHOULD** be preferred for parsing, validation, mapping, planning, selection, reduction, and rule evaluation.
- Referential transparency **SHOULD** be preserved whenever a function can reasonably be made deterministic from its inputs.
- Mutable shared state **MUST** be justified and narrowly scoped.
- Package-level mutable state **SHOULD NOT** be introduced except for well-understood infrastructure needs with clear lifecycle control.
- Side effects **MUST** be isolated behind interfaces or boundary functions.
- Time, randomness, environment reads, filesystem access, process execution, and network calls **SHOULD** be injected or wrapped so logic remains testable.
- State transitions **MUST** be explicit in code and easy to trace in tests.

Preferred pattern:

- compute or validate in pure helpers
- translate at the boundary
- perform side effects in a thin orchestration layer
- return explicit results and errors

### 3. Complexity Management and Ousterhout Preferences

Backend code **MUST** optimize for lower cognitive load, lower change amplification, and simpler reasoning.

In the spirit of John Ousterhout's design guidance:

- Complexity **MUST** be treated as a design bug, not only a readability issue.
- Modules **SHOULD** have simple interfaces and hide meaningful implementation detail behind them.
- Deep modules **SHOULD** be preferred over shallow pass-through abstractions.
- Comments **SHOULD** explain non-obvious intent, invariants, and design constraints, not repeat the code mechanically.
- Special cases **SHOULD** be eliminated when possible instead of accumulated.
- Changes that increase coupling, exception paths, or hidden dependencies **SHOULD** be treated skeptically even when they are locally convenient.

Review questions:

- Does this change make future changes easier or harder?
- Does this module hide complexity or merely move it around?
- Are invariants obvious from the interface and comments?
- Is there a simpler design with fewer branches, modes, or policy flags?

### 4. Linting, Static Analysis, and Code Shape

Backend quality **MUST** be enforced mechanically wherever possible.

Rules:

- Formatting, linting, vetting, and dead-code checks **MUST** pass before merge.
- The repository **SHOULD** maintain static rules for prohibited patterns rather than relying on tribal knowledge.
- Magic values **SHOULD NOT** appear inline when a named constant, type, or helper would communicate intent better.
- Sentinel strings, status literals, retry counts, timeout durations, buffer sizes, and policy values **SHOULD** be named and scoped appropriately.
- Functions **SHOULD** remain short enough to understand in one pass.
- Files **SHOULD** remain focused enough that one primary responsibility is obvious.
- Dense switch trees, boolean flag explosions, and hidden branching helpers **SHOULD** be refactored before they become structural debt.
- Unused code, dead branches, retired compatibility shims, and vestigial helpers **MUST** be removed.

Default review thresholds:

- Go functions longer than 80 lines **SHOULD** be treated as exceptions that need justification.
- Files that accumulate multiple unrelated responsibilities **SHOULD** be split.
- New package-level variables **SHOULD** be reviewed with extra scrutiny.

### 5. Error Handling and Contracts

Backend systems **MUST** communicate failure clearly and preserve contract correctness.

Rules:

- Errors **MUST** be explicit and actionable.
- Returned errors **SHOULD** preserve enough context to diagnose the failed operation.
- Panic paths **MUST NOT** be used for expected operational failures.
- Validation, normalization, and translation at public boundaries **MUST** be deliberate and test-covered.
- Contract drift between schemas, generated code, and runtime behavior **MUST** be guarded by automated checks.
- Backward compatibility expectations **MUST** be documented when public behavior changes.

### 6. Testing Strategy and Test Pyramid

Backend changes **MUST** include evidence at the correct testing layer.

The expected testing layers are:

- unit tests for pure logic, mappings, selectors, reducers, parsers, and validation
- package integration tests for component collaboration inside a bounded runtime slice
- functional tests for user-visible or system-visible flows across subsystem boundaries
- stress or race-oriented tests for concurrency, throughput, resource exhaustion, and long-running behavior
- contract tests for schema alignment, generated artifacts, and public surface guarantees

Rules:

- Most confidence **SHOULD** come from fast unit and package-level integration tests.
- Functional tests **SHOULD** focus on high-value end-to-end behavior, not every branch.
- Stress tests **SHOULD** exist where concurrency, queues, retries, watchers, or scheduler behavior create risk.
- Contract tests **SHOULD** protect generated surfaces, schema completeness, and compatibility boundaries.
- Slow tests **MUST** justify their cost by protecting a real regression class.
- Flaky tests **MUST** be fixed or removed quickly.

Minimum expectations for non-trivial backend changes:

- Pure logic has direct unit coverage where applicable.
- Cross-package behavior has integration or functional coverage.
- Concurrency-sensitive behavior has race, stress, or repeat-run coverage where relevant.
- Public contract changes have contract or smoke coverage.

### 7. CI/CD and Automated Enforcement

Best practices **MUST** be enforced by CI/CD, not only by documentation.

Rules:

- Pull requests **MUST** pass the repository's required build, lint, and test workflows before merge.
- CI **MUST** verify formatting or generated-artifact stability where drift would create review noise or release risk.
- CI **MUST** run static analysis and lint surfaces that catch dead code, contract drift, or prohibited patterns.
- CI **SHOULD** fail when generated artifacts are stale relative to source contracts.
- Functional enforcement **SHOULD** exist for repository-specific rules that generic linters cannot express.
- Fast-failing verification stages **SHOULD** run before slower functional suites when possible.
- Release-critical contract, migration, or schema checks **MUST** be automated.

Recommended CI shape:

- type or compile validation first
- lint and static enforcement second
- contract and generation checks next
- unit and integration tests next
- functional and stress coverage in the appropriate lanes

### 8. Concurrency, Runtime Safety, and Resource Use

Backend runtime behavior **MUST** remain safe under concurrent and adverse conditions.

Rules:

- Concurrency boundaries **MUST** be explicit.
- Ownership of mutable data **MUST** be obvious.
- Retries, timeouts, cancellation, and backpressure behavior **MUST** be deliberate.
- Resource acquisition and cleanup **MUST** be paired clearly.
- Long-lived goroutines, watchers, subprocesses, and background loops **MUST** have explicit shutdown behavior.
- High-volume paths **SHOULD** be measurable and testable under stress.
- Race-prone state mutation **MUST** be guarded by design, not luck.

Verification:

- Race or stress coverage **SHOULD** exist where concurrency is core to the feature.
- High-risk runtime behavior **SHOULD** include repeated-run or soak-style verification when appropriate.

### 9. Network Traffic and Dependency Behavior

Backend systems **MUST** treat network and dependency interactions as failure-prone boundaries.

Rules:

- Every outbound dependency call **MUST** define timeout behavior explicitly.
- Dependency interactions **MUST** account for latency, partial failure, total failure, and degraded upstream behavior.
- Retries **MUST NOT** be added blindly; they **MUST** be deliberate, bounded, and safe for the operation being retried.
- Exponential backoff with jitter **SHOULD** be the default retry strategy when retries are appropriate.
- Non-idempotent operations **MUST NOT** be retried unless the operation contract explicitly supports safe retry behavior.
- Cancellation and deadline propagation **SHOULD** be preserved across dependency boundaries where the runtime model supports it.
- Circuit breaking, fail-fast behavior, or bounded concurrency **SHOULD** be considered when repeated upstream failure could amplify outages.
- Dependency clients **SHOULD** surface structured results that make latency, status, retry attempts, and failure modes observable in tests and production.
- Fallback behavior **MUST** be explicit; silent degradation that hides correctness risk is prohibited.

Minimum observability for dependency calls:

- latency
- success rate
- failure rate
- timeout rate
- retry count
- fault or upstream error classification

Verification:

- Integration or functional tests **SHOULD** cover dependency timeout and failure paths.
- Retry behavior **SHOULD** be verified so it does not create duplicate side effects or retry storms.

### 10. Observability, Logging, Metrics, and Tracing

Backend systems **SHOULD** be diagnosable when they fail in production-like environments.

Rules:

- Logs **SHOULD** provide enough structured context to trace failures and important state transitions.
- Metrics **SHOULD** exist for throughput, latency, failures, saturation, retries, and queue or backlog pressure where relevant.
- Tracing **SHOULD** exist for requests or workflows that cross important subsystem or dependency boundaries.
- Observability **MUST** cover both local failures and downstream dependency failures.
- Diagnostics **MUST NOT** leak secrets or sensitive payloads.
- Operationally significant failures **SHOULD** emit actionable context rather than generic messages.
- Debug helpers **MUST** remain intentional and **MUST NOT** become hidden runtime dependencies.
- When a stateful workflow fails, the relevant transition history **SHOULD** be recoverable through logs, events, or test artifacts.

Recommended observability outcomes:

- A failing request can be correlated across logs, metrics, and traces.
- Operators can distinguish local faults from dependency faults.
- Latency regressions are detectable before they become outages.
- Retry storms, queue buildup, and resource exhaustion become visible quickly.

### 11. Performance, Load, Stress, and Failure Modes

Backend systems **MUST** be evaluated for resilience under realistic traffic and failure conditions.

Rules:

- Performance expectations **SHOULD** be defined for critical paths, especially high-volume or latency-sensitive ones.
- Load testing **SHOULD** validate expected throughput and latency under representative traffic.
- Stress testing **SHOULD** explore behavior near or beyond capacity limits.
- Failure-mode testing **SHOULD** cover upstream slowness, dependency outages, malformed inputs, resource exhaustion, and retry amplification risks.
- Backpressure behavior **MUST** be intentional for queues, schedulers, worker pools, and other bounded resources.
- Capacity-sensitive code **SHOULD** expose enough instrumentation to understand saturation and collapse behavior.
- Performance regressions **SHOULD** be caught in CI or scheduled verification lanes when the affected surface is operationally critical.

Verification:

- Critical services or workflows **SHOULD** have repeatable load or stress harnesses.
- Performance tests **SHOULD** measure both successful operation and degraded or failing dependency scenarios.
- Failure injection or simulation **SHOULD** be used when the real-world failure class is important and hard to observe through happy-path tests alone.

## Delivery Checklist

Before merge, authors **SHOULD** confirm:

- Package boundaries and dependency direction remain clear.
- Stateful behavior is minimized and explicit.
- Pure logic is separated from side effects where practical.
- Magic values, oversized functions, and oversized files were addressed.
- Appropriate tests exist at unit, integration, functional, contract, and stress layers as needed.
- Dependency calls define timeout, retry, failure, and backoff behavior explicitly.
- Logs, metrics, traces, and diagnostics are sufficient for the affected operational path.
- Performance, load, and failure-mode behavior were considered for the affected runtime path.
- CI enforces the important invariants for this area.
- Public contracts and generated artifacts remain aligned.
- Runtime diagnostics and failure behavior remain understandable.

## Notes for This Repository

These standards are intentionally general, but the current repository stack suggests the following defaults:

- Use thin `cmd/` entrypoints and keep backend behavior in `pkg/` and `internal/`.
- Treat `make lint`, `go vet`, and repository-specific dead-code or guard checks as required quality gates.
- Keep OpenAPI and generated artifacts aligned through automated smoke or contract checks.
- Use `tests/functional_test/` for high-value system behavior, not for replacing package-level unit coverage.
- Use `tests/stress/` and similar suites for concurrency, throughput, and resource-boundary risks.
- Treat dependency-heavy flows as observability-critical and ensure timeout, retry, and failure behavior is testable.
