# General Backend Standards

---
author: andreas abdi
last modified: 2026, september, 1
doc-id: STD-017
---

This document defines the baseline standards for backend systems built in this repository. It is intended to be broad enough for most backend services while still being concrete enough to review against.

For this repository, these standards apply directly to the Go backend under `cmd/`, `pkg/`, `internal/`, `api/`, and `tests/`.

## Usage

Every contributor who changes backend logic, APIs, runtime behavior, persistence behavior, background processing, or backend tests **MUST** review this standard before implementation or review.

## Quick Rules

- Prefer small, deeply understandable modules over large, clever ones.
- Expose backend operations as methods on injected service implementations; reserve package-level functions for constructors, pure value operations, and private implementation helpers.
- Inject each service and external-effect dependency directly, once, through the canonical `pkg/wire` graph.
- Do not hide dependencies in parameter bags, service locators, secondary injectors, runtime factories, or lazy constructors.
- Keep state explicit, local, and minimal; keep pure computation referentially transparent whenever practical.
- Separate pure domain logic from transport, IO, time, filesystem, environment, and process boundaries.
- Reject unexplained magic values, oversized functions, oversized files, and hidden side effects.
- Use linting, static checks, and CI gates to enforce backend quality rather than relying on reviewer memory alone.
- Favor fast component-isolated unit tests, targeted customer-behavior
  functional tests, a deliberately small compiled-artifact integration suite,
  and dedicated load/stress coverage where concurrency or scale matters.
- Log service operations and important outcomes with structured, safe context; add metrics, traces, and operational diagnostics where the path warrants them.
- Design dependency calls with explicit latency, timeout, retry, and backoff behavior.
- Test performance, load, and failure modes intentionally rather than assuming correctness implies resilience.
- Keep public contracts, generated code, and handwritten domain logic clearly separated.
- Optimize for reduced complexity, clear interfaces, and easy changeability in the spirit of John Ousterhout's design guidance.

## Review Checklist

Before approval, reviewers **SHOULD** confirm:

- The change fits the repository's package boundaries and dependency direction.
- Operational behavior is exposed through a service interface and implemented by an injectable struct rather than a floating function.
- Services and external effects are injected directly once; no dependency bag, service locator, secondary injector, runtime factory, or hidden constructor was added.
- Production `New...` construction remains in the owning service's `wire/` provider or the canonical `pkg/wire` graph.
- State and side effects are intentional, isolated, and no broader than necessary.
- Pure logic is testable without live IO, clocks, processes, or network dependencies.
- Magic literals, hidden policies, and unexplained special cases are avoided or named clearly.
- Functions and files remain small enough to understand quickly.
- The change includes the right mix of unit, integration, functional, and stress evidence.
- Network dependency behavior is explicit about timeout, retry, failure, and observability strategy.
- Operational signals are sufficient to diagnose latency, failures, and degraded dependency behavior.
- Service operations log their start or accepted intent, terminal outcome, and relevant identifiers at appropriate levels without leaking sensitive payloads.
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
- Repository-specific ownership and placement **MUST** follow the current
  package map in `AGENTS.md` and `docs/architecture/packaged-structure.md`.
  Standards **MUST NOT** duplicate migration inventories, temporary package
  exceptions, or planned target paths as permanent architecture rules.

### Minimal Internal Transformations

- Each ownership boundary **MUST** keep one canonical representation for its
  domain state and policy. Internal code **MUST** reuse the owner-level
  representation directly rather than introduce a second model that only
  passes fields through.
- Necessary translation **MUST** happen once, at the narrow edge that owns the
  concrete external contract. An adapter is justified only for a concrete
  protocol, provider, persistence, compatibility, or other replaceable
  external boundary.
- Every adapter **MUST** name the external boundary, source owner, destination
  owner, preserved invariant, behavioral reason, and removal or consolidation
  rationale. The owning service root retains domain policy and state; private
  implementation types do not cross that boundary.
- In this repository, `pkg/transports/mapping` owns protocol-to-service
  representation conversion; `Providers` owns provider adaptation and one
  normalized execution attempt; `Models` owns local-model readiness and
  lifecycle; and `Workers` consumes Providers and Models through service-root
  contracts while owning request-scoped execution and retry policy.
- Accepted: `pkg/transports/mapping` converts a protocol payload once into a
  service-root request, preserving service validation and policy ownership;
  `Providers` adapts one provider protocol into one normalized attempt, and
  `Workers` consumes that contract. Rejected: a `Workers` pass-through wrapper
  chain or a duplicate Providers/Models domain model that merely copies fields
  without a distinct external boundary, behavior, or preserved invariant.

### 2. Services, Dependency Injection, and Construction

Backend behavior **MUST** be exposed through explicit service contracts and assembled through one dependency-injection graph.

#### Service operations

- Product and orchestration operations **MUST** be methods on concrete service structs that implement interfaces exposed by the owning service root.
- Cross-service callers **MUST** depend on the narrow public interface of the owning service and **MUST NOT** call its internal implementation, constructor, or package-level operational helpers.
- Operational dependencies **MUST** be stored explicitly on the receiving service struct and supplied at construction time. A method **MUST NOT** discover collaborators from global state, context values, registries, or service locators.
- Exported package-level functions **MUST** be limited to focused construction providers in `wire/`, pure value operations where a receiver is not meaningful, and language- or framework-required adapters. They **MUST NOT** provide an alternate operational API beside the service interface.
- Unexported pure helpers **MAY** remain package-level when they have no injected dependencies or side effects. Helpers that require collaborators, IO, mutable lifecycle state, or operational policy **MUST** become methods on the owning implementation struct.
- Request, result, option, and domain-value structs **MAY** group operation data. They **MUST NOT** be used as disguised dependency containers.

#### Direct, single injection

- `pkg/wire` **MUST** construct the complete inert application graph once, construct each service once, and pass that same service instance directly to every consumer that needs it.
- A service **MUST** receive the specific peer-service interfaces and external-effect ports it consumes. It **MUST NOT** receive a broad bundle, `Dependencies`, `Deps`, `Params`, `Options`, `Services`, or similar grab-bag constructor struct merely to shorten a constructor signature.
- A constructor parameter struct **MAY** represent one cohesive configuration value, but it **MUST NOT** mix configuration with services, loggers, clocks, stores, clients, runners, or other behavioral dependencies.
- Services, transports, initializers, and runtime methods **MUST NOT** perform secondary injection, invoke child injectors, assemble another application graph, or create factories that defer service selection or dependency construction until an operation runs.
- `pkg/initializer` **MUST** activate and unwind roles already built by `pkg/wire`; it **MUST NOT** construct product services or behave as a second composition root.
- Session- or request-scoped domain state **MAY** be created during an operation. That creation **MUST** use the already-injected services and **MUST NOT** become another dependency-injection pass.

#### Constructor ownership

- Production calls to service and infrastructure constructors named `New...` **MUST** occur only in the owning service's focused `wire/` provider or in the canonical `pkg/wire` composition graph.
- Non-`wire` production code **MUST NOT** call `New...` to acquire a service, infrastructure adapter, client, store, runner, logger, clock, or other injectable collaborator. Those dependencies **MUST** be constructor-injected into the receiving service.
- Pure domain-value constructors, request/result builders, standard-library value creation, allocation through Go's built-in `new`, and test fixture construction are not dependency construction and **MAY** remain outside `wire/`.
- A `wire/` provider **MUST** construct only its focused implementation from explicit parameters. It **MUST NOT** hide a secondary graph, return a service bag for later lookup, or make runtime policy decisions.
- All `wire/` packages **MUST** remain inert composition: constructors, provider sets, and bindings. Downloading, checksum verification, cache management, runtime backend selection, retries, and lifecycle execution **MUST** belong to the owning service implementation or lifecycle owner. Returning a callback or closure does not permit hiding that behavior in Wire. Providers **MAY** choose a constructor from explicit configuration without performing operational work or interpreting domain policy.
- Tests **SHOULD** construct the implementation under test directly with explicit fakes. Application and functional tests **MUST** use the canonical `root.BuildProcess` and `pkg/wire` path described in Section 7.

### 3. Statefulness and Functional Style

Backend systems **MUST** prefer explicit data flow over hidden mutable state.

Rules:

- Stateless service implementations are preferred when lifecycle state is unnecessary.
- Pure computation **SHOULD** be preferred for parsing, validation, mapping, planning, selection, reduction, and rule evaluation, whether expressed as value methods or private helpers.
- Referential transparency **SHOULD** be preserved whenever a function can reasonably be made deterministic from its inputs.
- Mutable shared state **MUST** be justified and narrowly scoped.
- Package-level mutable state **SHOULD NOT** be introduced except for well-understood infrastructure needs with clear lifecycle control.
- Side effects **MUST** be isolated behind injected service or external-effect interfaces and invoked from service methods or transport adapters.
- Time, randomness, environment reads, filesystem access, process execution, and network calls **SHOULD** be injected or wrapped so logic remains testable.
- State transitions **MUST** be explicit in code and easy to trace in tests.

Preferred pattern:

- compute or validate in pure helpers
- translate at the boundary
- perform side effects through injected service or external-effect interfaces in a thin service method
- return explicit results and errors

### 4. Complexity Management and Ousterhout Preferences

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

### 5. Linting, Static Analysis, and Code Shape

Backend quality **MUST** be enforced mechanically wherever possible.

Rules:

- Formatting, linting, vetting, and dead-code checks **MUST** pass before merge.
- Production dead-code analysis includes both repository entrypoints and the
  actual generated golangci module host retained by `make golangci-build`.
  Each program uses its own production module graph; compiler-declared host
  package ownership reconciles the two reachability reports.
  Tests are not reachability roots; unreachable plugin/analyzer functions
  remain findings. Missing host metadata fails closed, and baseline updates
  remain deletion-only for checker retirement.
- The repository **SHOULD** maintain static rules for prohibited patterns rather than relying on tribal knowledge.
- Repository checks **SHOULD** enforce constructor placement, prohibit dependency-container grab bags, and detect operational package-level functions as these rules become mechanically identifiable.
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

Repository Go size and complexity enforcement:

- `make golangci` runs pinned golangci-lint v2.11.4 and shared analyzers through
  its supported module plugin. The strict repository configurations preserve
  exact-debt diagnostics independently of built-in changed-line filtering.
  Shared baseline-growth and compiler-owner analyzers reject established debt
  growth and vanished owners/sources using Git objects and compiler metadata.
  Ordinary and complete-tag runs retain default-only and tagged source coverage;
  platform-inactive debt owners resolve through supported GOOS metadata.
  Its built-in size rules apply to handwritten Go under `cmd/`,
  `internal/`, `pkg/`, and `tests/`, including `_test.go` files.
- Revive `file-length-limit` allows 1000 lines after excluding comment and
  blank lines. Revive `function-length` allows 100 physical lines inside a
  function's braces (including comments and blanks); statement counting is
  disabled. Gocyclo reports cyclomatic complexity greater than 15.
- Generated files are excluded through the committed configuration. Only the
  two named revive rules are enabled; revive's default rules are disabled.
- The changed-line ratchet uses the merge base with `origin/main` and requires
  fetched history. The canonical target checks that prerequisite explicitly,
  including for clean input. It accepts unchanged debt and keeps
  `whole-files: false`.
  Function length and complexity report at the declaration, so body-only
  growth can be filtered when that line is unchanged. In the pinned revive
  version, file length reports at the file's final line. Moving or changing a
  reported location can expose old debt again.
- A necessary exemption **MUST** use a narrowly scoped `//nolint:revive` or
  `//nolint:gocyclo` with an actionable reason. The former size checkers and
  their exemption-budget ledger are retired; their old directives are inert.
- `make lint-migration-smoke LINT_MIGRATION_COHORT=size` exercises the actual
  pinned configuration in disposable modules and git histories, observing
  accepted limits, deliberate violations, generated exclusions, narrow
  suppression, ratchet behavior, and configuration/history errors. This is a
  lint/static gate rather than an application functional test.

Repository package-shape policy:

- Contributors **SHOULD** split packages by durable responsibility and remove dead files. A fixed per-directory Go file budget is no longer enforced.
- The golangci migration retires `pkg-file-count`, its filesystem walker, and `backend-package-file-count.json`; file count is not a replacement analyzer rule.
- File length, function length, and complexity remain governed by the pinned built-in rules above. Dependency direction and service shape retain their separate lint gates.


### 6. Error Handling and Contracts

Backend systems **MUST** communicate failure clearly and preserve contract correctness.

Rules:

- Errors **MUST** be explicit and actionable.
- Returned errors **SHOULD** preserve enough context to diagnose the failed operation.
- Panic paths **MUST NOT** be used for expected operational failures.
- Validation, normalization, and translation at public boundaries **MUST** be deliberate and test-covered.
- Contract drift between schemas, generated code, and runtime behavior **MUST** be guarded by automated checks.
- Backward compatibility expectations **MUST** be documented when public behavior changes.

### 7. Testing Strategy and Test Pyramid

Backend changes **MUST** include evidence at the correct testing layer.
Factory planning, implementation, review, and validation **MUST** use
[`factory/docs/standards/testing-standards.md`](../../../../factory/docs/standards/testing-standards.md)
as the authoritative layer-classification and execution standard. The rules
below summarize backend-specific application of that standard.

The expected testing layers are:

- unit tests for package-owned pure logic, mappings, selectors, reducers,
  parsers, validation, and isolated components
- functional tests for customer-visible flows through public application
  boundaries with controlled external effects
- integration tests for a small number of real-boundary properties exercised
  through an already compiled deliverable
- dedicated load or stress tests for concurrency, throughput,
  resource exhaustion, and long-running behavior
- contract tests for schema alignment, generated artifacts, and public surface guarantees
- asset conformance tests for declared external artifacts, pinned dependencies, and published locations

Rules:

- Most confidence **SHOULD** come from fast unit tests and targeted functional
  customer-behavior tests.
- A unit test **MUST** exercise one individual component, struct, class, or
  function with controlled collaborators. A package, composed service graph,
  or monolithic test executable does not expand the unit's scope. Loading,
  installing, materializing, or running real packaged factories belongs in
  functional coverage through the public application boundary; focused
  installer/loader/serializer unit tests use tiny synthetic inputs and explicit
  dependency doubles.
- When removing composition or packaged-factory tests from the unit lane,
  coverage floors **MUST NOT** require recreating those fixtures. Reconcile
  incidental unit coverage only after auditing the retained guarantees and
  measuring production-block coverage in the appropriate lanes. Keep focused
  component assertions for behavior that lacks equivalent coverage, record
  the affected package floors and evidence, and leave unrelated floors and
  remediation holds unchanged. A floor **MUST NOT** be lowered solely to pass
  CI.
- The full Go unit lane **SHOULD** use the consolidated monolith pipeline
  defined in the factory testing standard: preserve package/component
  isolation, validate reusable binaries through the Go toolchain, and execute
  fresh tests. Minimize separate binaries to justified isolation or dependency
  exceptions. Optimize total user plus kernel CPU toward a subminute warm-build
  suite budget; include discovery, validation/build/link, execution, and
  reporting, and report cold and changed-source rebuild costs separately.
- Functional tests **SHOULD** focus on high-value end-to-end behavior, not every branch.
- Functional application tests **MUST** construct the reusable customer process
  through `root.BuildProcess`, provide product configuration through the same
  CLI arguments, environment, working directory, and streams available to a
  customer, and replace only exact external effects through `edges.Edges`.
  They **MUST NOT** import or mutate `runtimeinput.Config`, expose a runtime
  configuration callback, construct a second application graph, or expose an
  internal engine-state snapshot through functional support. Assertions
  **MUST** use public CLI, HTTP, MCP, Factory Session, Work, and Factory Event
  contracts or observations made by the injected external-effect edge.
- Functional application tests **MUST** execute scenarios through
  `Process.Execute` after constructing through `root.BuildProcess`. They
  **MUST NOT** build or invoke the `you` CLI executable. Tests that prove OS
  process, pipe, signal, executable, or exit-status behavior belong in the
  integration lane and **MUST** run through the real built binary there. The Go
  test executable is also a real executable for this classification; helper
  modes, long-test names, and build tags do not make process behavior
  functional coverage.
- Functional tests **MUST** use Factory Sessions wherever the behavior admits
  a session, reuse one package process where safe, and run independent
  scenarios in parallel. Serialization requires a documented customer-visible
  invariant; shared fixture state or route collisions are harness defects. A
  package with a small local `~default` cohort and independent hosted-session
  behavior must partition those phases rather than serializing every scenario.
- Functional package concurrency **MUST** use the canonical functional-lane job
  budget. Independent tests should overlap inside that budget, but broad
  verification **MUST NOT** rely on the test tool's unbounded package wildcard
  default. Parallel children must retain parent-owned fixtures through
  `t.Cleanup`, and their cleanup may assert only scenario-owned resources while
  peers remain live.
- A shared functional application process **MUST NOT** imply a shared mutable
  customer home. Concurrent invocations must own separate profiles whenever
  first-run installation, migration, selection, or cleanup can write profile
  state. A shared profile is allowed only after its prerequisite bootstrap has
  completed and the tests prove concurrent commands do not contend on its
  installation or migration locks.
- A functional scenario that reads a deliberately global customer projection
  may use a documented isolated observation window. Reviewers **MUST** verify
  the emitted public request actually has global semantics and that any claimed
  session, Work, or route selector reaches the product boundary; tests must not
  rely on package quiescence to make an ignored selector appear effective.
- Functional tests **MUST** prefer public CLI invocation over HTTP/API for
  ordinary customer flows. HTTP or API entry **MAY** be used only for
  API-owned contracts or explicit CLI+API parity cells.
- External effects **MUST** be replaced only through `edges.Edges`. Functional
  tests **MUST** prefer `ProviderCommandRunner` and other command-runner edge
  mocks over custom in-process provider fakes.
- Asset conformance tests **MUST** read the real declared artifact. The
  `edges.Edges` substitution rule does not apply to an asset conformance test. An
  asset conformance test **MUST** fail when a declared artifact is absent,
  unresolvable, or smaller than its documented minimum size. A hermetic test
  **MUST NOT** be cited as evidence that a real pinned dependency is intact.
- Functional tests **MUST** prefer mocked Codex or another real
  inference-provider variant through the command-runner edge and sanitized
  goldens over `--with-mock-workers` / `MockWorkers`, except for cells under
  `tests/functional/workers/mock/...` that own the workers/mock feature.
- Functional tests **MUST NOT** add sleeps or timeout-padded wait helpers as the
  default synchronization strategy. Prefer fixing readiness or latency root
  causes first. Any sleep, polling loop, or timeout-padded wait helper **MUST**
  include an in-code justification for why deterministic observation or edge
  mocking cannot substitute.
- Hosted readiness deadlines **MUST** measure host readiness, not unrelated
  fixture bootstrap. When first-run initialization is outside the customer
  claim, prepare the test-owned home through the same reusable public process
  before starting the host. When first-run behavior is the claim, keep it in
  the measured path and assert its customer-visible result.
- Integration tests **MUST** consume an artifact compiled once by the invoking
  build or release lane. They **MUST NOT** compile inside test setup and
  **MUST** keep the real-boundary case set intentionally small.
- `make golangci` enforces the functional OS boundary with depguard (`os/exec`
  imports) and type-aware forbidigo (`Command`/`CommandContext`). These rules
  cover functional scenarios and helpers, excluding shared
  `tests/functional/internal/support` and testdata. Existing sites use the
  `origin/main` merge-base ratchet; new or moved sites fail. Real OS proof
  belongs in integration. Backend Lint owns this enforcement.
- `make golangci` enforces functional composition through depguard (secondary
  composition imports), forbidigo (retired harness helpers), and the shared
  Layering/Behavior analyzers (dedicated-provider imports, provider-local support,
  and configuration callbacks). Provider scenarios use canonical shared support
  and exact public effect ports. Required Backend Lint owns static enforcement;
  functional coverage preserves its runtime selection and concurrency budget.
- Inventory, package-shape, dependency-direction, source-topology, and similar
  structural enforcement **MUST** be implemented as lint or static checks, not
  runtime tests, unless that structure is itself a published customer contract.
- Functional test sources **MUST** live under
  `tests/functional/<domain>/<subsection>/...`, where `<domain>` is a durable
  product-domain noun such as `transport`, `workers`, `orchestration`,
  `workstations`, `work`, `sessions`, `factory`, `providers`,
  `provider_sessions`, `events`, `models`, `guards`, `resources`,
  `observability`, `product`, or
  `resilience`. Provider-specific root-process scenarios live under
  `tests/functional/providers/<provider>/...`; broader worker execution
  behavior remains under `tests/functional/workers/<subsection>/...`.
  There is no durable `features/` wrapper and no transport-first ownership for
  domain behavior: `transport` owns transport mechanics only, and domain proofs
  live under their domain nouns even when the scenario enters through CLI,
  HTTP, or MCP. `tests/functional/internal/support` is the only shared harness
  exception and is not a scenario owner. The migration-only
  `tests/functional/runtime_api` package is deletion-only debt and **MUST NOT**
  receive new files or scenarios; its exact file and `Test*` inventory is
  enforced by `make pkg-structure`. Historical catch-alls such as `smoke` and
  `workflow` are not durable owners for new scenarios.
- Load and stress tests **MUST** live under a dedicated `tests/load/`,
  `tests/stress/`, or more specific performance package rather than functional
  or integration packages. They **SHOULD** exist where concurrency, queues,
  retries, watchers, schedulers, throughput, or saturation create risk.
- Contract tests **SHOULD** protect generated surfaces, schema completeness, and compatibility boundaries.
- Slow tests **MUST** justify their cost by protecting a real regression class.
- Flaky tests **MUST** be fixed or removed quickly.
- Test optimization **MUST NOT** weaken a distinct customer-visible behavior.
  Persistence and replay claims **MUST** be observed through a public customer
  surface; internal artifact decoding belongs with the owning serializer's
  unit or contract coverage.

Minimum expectations for non-trivial backend changes:

- Pure logic has direct unit coverage where applicable.
- Customer behavior crossing packages has functional coverage; real compiled
  artifact boundaries have limited integration coverage.
- Concurrency-sensitive behavior has stress or repeat-run coverage where relevant.
- Public contract changes have contract or smoke coverage.

### Timing in tests

Fixed sleeps and short fixed deadlines are the dominant source of CI-load flakes.

- Tests **MUST** wait for an event, channel, or observable condition, not for elapsed time. Poll a condition (for example the functional `WaitFor...` support helpers) instead of `time.Sleep`.
- Code under test that depends on time **MUST** take the injectable `pkg/platform/clock` source, and tests **SHOULD** use `clock.Deterministic` so time advances only when the test says so.
- A timeout in a test is a failure ceiling, not an expectation. It **MUST** be generous (tens of seconds or more) so a loaded host does not trip it, and a test **MUST NOT** assert that elapsed time falls inside a tight window.
- `make golangci` (part of `make lint`) runs the compiler-backed `testsleep` analyzer through the supported golangci module plugin. It ratchets `time.Sleep`, literal deadlines of five seconds or less (`time.After`, `time.NewTimer`, `time.AfterFunc`, `context.WithTimeout`/`WithDeadline`), and elapsed-time comparisons in tests and test helpers. Exact debt in `internal/lint/analyzers/baseline.txt` retains file, declaration, kind and occurrence; moving lines preserves debt, new sites fail, and removing sites requires deleting stale keys. Compiler metadata also rejects allowances for vanished package owners. Only a newly migrated rule may seed existing observed debt once; established rules cannot gain keys. A genuinely necessary site may be exempted inline with `//nolint:testsleep // reason`; the reason is mandatory.

### 8. CI/CD and Automated Enforcement

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

#### CI pipeline policy

- CI **MUST NOT** run tests with `-race`.
- Each test suite **MUST** run in one primary job: backend unit plus coverage,
  backend functional, backend integration, and frontend. Suites **MUST NOT**
  be sharded or split into component, witness, or race steps or jobs.
- New checks **MUST** join their suite's primary job or become a
  golangci-lint/go-analysis rule; they **MUST NOT** introduce a new job.
  Static checks **MUST** belong in lint.
- Network-dependent product checks **MUST NOT** run in PR CI.
- Existing Workflow Lint **MUST** reject added `ci.yml` job IDs against the
  `origin/main` merge base and fail closed when comparison history is absent.
  Unchanged legacy jobs remain accepted during migration; deletions are allowed.
  Automatic executable `-race` detection is skipped: the current Node harness
  has no YAML/shell parser, and text matching cannot reliably distinguish
  executable flags from comments and strings. Review **MUST** enforce the
  no-race rule, including indirect Make/shell invocations.

### 9. Concurrency, Runtime Safety, and Resource Use

Backend runtime behavior **MUST** remain safe under concurrent and adverse conditions.

Rules:

- Concurrency boundaries **MUST** be explicit.
- Ownership of mutable data **MUST** be obvious.
- Retries, timeouts, cancellation, and backpressure behavior **MUST** be deliberate.
- Resource acquisition and cleanup **MUST** be paired clearly.
- Long-lived goroutines, watchers, subprocesses, and background loops **MUST** have explicit shutdown behavior.
- Packages that spawn goroutines or processes **SHOULD** enable `go.uber.org/goleak` via `TestMain` (`goleak.VerifyTestMain(m)`), adding only narrow `IgnoreTopFunction` entries with a comment for known long-lived globals. Do not ignore leaks from production code.
- High-volume paths **SHOULD** be measurable and testable under stress.
- Race-prone state mutation **MUST** be guarded by design, not luck.

Verification:

- Stress or repeat-run coverage **SHOULD** exist where concurrency is core to the feature.
- High-risk runtime behavior **SHOULD** include repeated-run or soak-style verification when appropriate.

### 10. Network Traffic and Dependency Behavior

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

### 11. Observability, Logging, Metrics, and Tracing

Backend systems **SHOULD** be diagnosable when they fail in production-like environments.

Rules:

- Public service operations and operationally significant background actions **MUST** emit structured logs at appropriate levels.
- Operation logs **MUST** make the accepted intent or start, terminal outcome, relevant stable identifiers, duration when useful, and actionable error context observable. High-frequency paths **MAY** sample or aggregate successful-operation logs when per-operation logging would create harmful volume.
- Logs **MUST** use the repository's injected logging abstraction. Services **MUST NOT** construct or discover their own logger.
- Logs **SHOULD** provide enough structured context to trace failures and important state transitions across service boundaries.
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

### 12. Performance, Load, Stress, and Failure Modes

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
- Operations are methods on injectable service implementations and cross-service calls use service-root interfaces.
- Dependencies are injected directly once, without grab-bag parameter structs, service lookup, secondary injection, runtime factories, or out-of-`wire` service construction.
- Stateful behavior is minimized and explicit.
- Pure logic is separated from side effects where practical.
- Magic values, oversized functions, and oversized files were addressed.
- Appropriate tests exist at unit, integration, functional, contract, and stress layers as needed.
- Dependency calls define timeout, retry, failure, and backoff behavior explicitly.
- Service operations emit structured, safe, actionable logs; metrics, traces, and diagnostics are sufficient for the affected operational path.
- Performance, load, and failure-mode behavior were considered for the affected runtime path.
- CI enforces the important invariants for this area.
- Public contracts and generated artifacts remain aligned.
- Runtime diagnostics and failure behavior remain understandable.

## Notes for This Repository

These standards are intentionally general, but the current repository stack suggests the following defaults:

- Use thin `cmd/` entrypoints and keep backend behavior in `pkg/` and `internal/`.
- Treat `make lint`, `go vet`, and repository-specific dead-code or guard checks as required quality gates enforced by hosted CI; run only the changed lint target locally.
- Keep OpenAPI and generated artifacts aligned through automated smoke or contract checks.
- Use `tests/functional/` for new high-value system behavior, not for replacing
  package-level unit coverage. `tests/functional_test/` is legacy fixture and
  compatibility coverage, not the destination for new scenarios.
- Use `tests/stress/` and similar suites for concurrency, throughput, and resource-boundary risks.
- Treat dependency-heavy flows as observability-critical and ensure timeout, retry, and failure behavior is testable.
