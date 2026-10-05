# Factory testing standards

---
author: andreas abdi
last modified: 2026, september, 1
doc-id: FSTD-005
---

This standard is the test-layer authority for factory planning, implementation,
review, and validation. It defines which kind of proof belongs in each suite,
what that proof may observe, and how tests remain fast, deterministic, and
customer-centered. Repository engineering standards still apply; when older
guidance classifies a test differently, this document governs factory work.

## Quick rules

- Choose the lowest test layer that can prove the behavior without crossing a
  boundary that layer does not own.
- Unit tests prove one individual component, struct, class, or function in
  isolation. A package, service family, or shared test binary is not a unit.
- The full Go unit lane **SHOULD** use the consolidated monolith pipeline:
  validate and build reusable test binaries once, then execute fresh tests.
  Optimize aggregate user and kernel CPU time toward a subminute warm-build
  suite budget, with cold and changed-source build costs reported separately.
- Functional tests prove customer-observable behavior through a public
  application boundary with controlled external effects.
- Functional tests **MUST** use Factory Sessions wherever the behavior can be
  expressed as a session and **MUST NOT** build or invoke a CLI binary.
- Functional tests **MUST** run in parallel unless a customer-visible invariant
  requires serialization and the test documents that invariant.
- Integration tests **MUST** exercise an already compiled deliverable and stay
  deliberately small.
- Contract checks prove a published contract. Repository shape, source
  topology, inventories, and dependency-direction rules belong in lint or
  static checks, not runtime tests.
- Load and stress tests belong in their dedicated suites and never hide inside
  unit, functional, or integration packages.
- A test that cannot name the behavior and observer it protects **MUST** be
  removed, rewritten as behavioral proof, or moved to the appropriate static
  quality gate.

## 1. Classify by behavior and boundary

Use this table before adding or changing a test:

| Layer | Proves | Allowed boundary | Must not become |
| --- | --- | --- | --- |
| Unit | One package-owned operation or subcomponent | Direct call with explicit fakes or in-memory collaborators | A full application, transport, process, or cross-package journey |
| Functional | A customer-observable use case | Public CLI command contract through `Process.Execute`, HTTP, MCP, ACP, Factory Session, Work, or Factory Event contracts | An assertion about internal topology, ledgers, constructor counts, package shape, or a compiled executable |
| Contract | A published schema, protocol, serialization, compatibility, or generated-surface guarantee | The authored contract and its public representations | A source inventory or substitute for runtime behavior |
| Integration | A small proof that the compiled deliverable crosses a real production boundary | An already built binary, package, image, or other release artifact with production wiring | An exhaustive behavior matrix or a build performed by the test |
| End-to-end / release smoke | One critical journey through the delivered system in a release-like environment | Actual customer entry point and delivered artifacts | Broad branch coverage or load testing |
| Load / stress | Capacity, throughput, latency, saturation, backpressure, or endurance | A dedicated controlled performance environment | A functional or integration test with a large loop |
| Lint / static check | Source ownership, dependency direction, inventories, generated drift, naming, or repository shape | Source and metadata inspection | A runtime test |

A test's name, directory, or number of participating packages does not decide
its layer. The behavior proved and the production boundary crossed do.

## 2. Unit tests

Unit tests **MUST** exercise only the package-owned component under test. They
**MUST NOT** assemble the root process, start transports, invoke an executable,
or validate an overall customer journey. A unit test that drives the whole
system with mocks is a functional test in disguise and **MUST** be rewritten or
moved.

The unit **MUST** be one named component, struct, class, or function with its
own behavior. A package containing several collaborating implementations is
not itself a unit. Private methods and structures that implement that component
may participate; independently owned validation, persistence, loading,
rendering, scheduling, or execution components **MUST** be controlled
collaborators rather than additional real implementations under test. Sharing
a monolithic executable changes test packaging, not this boundary.

Unit tests **MUST NOT** load the published packaged-factory catalog, install or
materialize real packaged factories, or run packaged workflows as a general
fixture. Those customer behaviors belong in functional tests through the public
application boundary, with Factory Sessions for execution. Installer policy,
serializer behavior, or a loader's individual operation may have focused unit
tests using tiny synthetic inputs and explicit controlled collaborators.
Published asset/schema conformance belongs in its contract or static gate;
capacity-sized payloads belong in load/stress tests. A composed packaged-factory
test **MUST** be removed when existing functional coverage proves the same
behavior, or replaced with focused unit proof and the smallest necessary
functional scenario. Moving a test to another directory while retaining
internal graph construction does not make it a valid functional test.

Dependencies outside the component **MUST** be represented by narrow fakes,
stubs, test doubles, or in-memory implementations supplied through the same
testable boundary used by production. Unit tests **MUST NOT** validate the
implementation of those dependencies. Real time, randomness, environment,
network, subprocess, and filesystem effects **SHOULD** be replaced by explicit
boundaries.

Temporary files are allowed only when file behavior is the package-owned
subject. Use an isolated temporary directory and assert the package's public
file result, not incidental directory layout or internal write order.
Diagnostic assertions **MUST** distinguish the diagnostic from incidental
paths and test names; a substring found only in a temporary-directory name
does not prove the expected error.

Unit tests **SHOULD** be table-driven where that improves clarity, run in
parallel when state ownership permits, and finish quickly enough to remain the
default local feedback loop.

Unit tests **MUST NOT** import or invoke Wire providers. Construct components
directly with explicit test dependencies when construction is required. `wire/` packages
**MUST NOT** host unit tests; behavioral code and its tests belong to the owning
implementation. Compilation and generation-drift checks validate construction;
binding inventories, constructor counts, and injected-instance identity checks
belong in static checks when a concrete engineering rule requires them. The
functional suite proves application behavior through the canonical graph.

Tests for components within one service owner **MAY** share a test package to
reduce repeated binary linking. Each test **MUST** retain component isolation
and test-owned mutable state. Consolidation **MUST NOT** require new production
exports, application construction, or weaker assertions. Compare fresh test
execution CPU time with build-cache conditions stated explicitly; fewer
binaries alone do not demonstrate a performance improvement. Coverage checks
**MUST** instrument the tested implementations, including imported packages
through scoped `-coverpkg` selection.

### Consolidated Go unit pipeline

The full unit lane **SHOULD** consolidate compatible tests into one monolithic
test executable. Additional binaries are appropriate for an actual process
isolation requirement or an incompatible test dependency graph. The runner
owns discovery, build validation, execution, diagnostics, and those exceptions;
consolidation does not remove the runner or turn unit tests into system tests.
Focused package execution **MUST** remain available for local development.

The current opt-in implementation is `make test-unit-monolith`, or
`go run ./cmd/unitlane -monolith -count=1`. It requires Python 3 for overlay
generation and uses native `go test -c` for build validation and registration.
It always executes tests fresh; ordinary `make test` retains its existing
package runner until full coverage and rebuild comparisons justify adoption.
`make test-unit-monolith-prepare` builds the coordinator and all selected test
binaries once. `make test-unit-monolith-prebuilt` executes that explicit source
snapshot without rediscovery, regeneration, or build validation. Compact
execution invokes no Go tools; detailed reporting retains `go tool test2json`.
Re-run preparation after source, asset, dependency, toolchain, platform, build
tag, or instrumentation changes. Prepared execution **MUST** include native
exceptions and required helper processes; removing them would omit behavior.
Compact reporting executes all registered tests but inventories only merged
top-level tests and native package completion. Use `-monolith-details` for full
subtest inventory comparisons; a smaller reporting inventory is not fewer tests.
`make test-stress-fixtures` runs the retained large checkpoint, ASR, and metrics fixtures
outside the unit lane. Owner-scoped `stresstests/` packages are classified as
stress, including when discovered beneath `pkg/`.

Source ownership and Go package boundaries **MUST** remain intact even when
tests from multiple services share an executable. Consolidation **MUST NOT**
merge production packages, introduce production exports for test registration,
bypass Go `internal` visibility, inject Wire, or weaken assertions. Preserve
package working directories, source locations, test names, helper-process
selection, cleanup ordering, and parallel-subtest behavior. Process-wide
environment and mutable globals require explicit ownership and restoration;
serialize incompatible groups or retain a separate binary until isolation is
established. Preserve `TestMain` setup, teardown, exit status, and leak checks;
do not silently omit a package because its setup cannot be consolidated.
Leak verification **MUST** run after the relevant parallel tests and cleanup
have completed; broadening ignore lists to make consolidation pass is not an
equivalent check. Disable incidental framework host probing through explicit
test setup when it is outside the behavior being proved. For example,
in-process Cobra command tests do not need Windows Explorer-launch process
scans; actual executable-launch behavior belongs in integration testing.

Preparation **MUST** let the Go toolchain validate source and build inputs.
Validation includes test and production
sources, dependencies, embedded assets, toolchain, build tags, target platform,
and instrumentation flags. A persistent executable path alone is not proof
that the artifact is current. Explicit prepared execution may skip validation
for repeated runs of the same source snapshot; label that measurement as
execution-only and record the preparation identity. Build-cache reuse is encouraged; test-result
cache hits **MUST NOT** count as fresh execution in latency measurements.

Harness migrations **MUST** account for every selected package and test,
including native exceptions, skipped cases, examples, and fuzz seed cases.
Compare executed cases and scoped implementation coverage, including imported
implementations, before claiming equivalent coverage. An unchanged test count
alone is not statement-coverage evidence. Failures in either the merged or
separate binaries **MUST** fail the lane.

Measure aggregate user plus kernel CPU for the runner and all descendants.
Include discovery/generation, source validation, compilation/linking, process
startup, fresh test execution, reporting, and cleanup. Report warm unchanged
builds, representative source-change rebuilds, and cold builds separately.
Attribute costs by stage; do not describe the entire build stage as linking or
the entire native `go test` stage as test execution. Report wall time separately
and retain the observed sample range. Required correctness and coverage runs
**SHOULD** emit these diagnostics instead of duplicating the suite solely for
timing.

Report execution-only prepared measurements separately from complete
prepare-and-execute measurements. Package process-CPU deltas attribute the
shared process's work but exclude helper descendants; aggregate user plus
kernel CPU accounting for the complete process tree remains the budget metric.
The application `pkg/wire` tree and service-local `wire/` trees have no unit lane. Their retained private behavior
witnesses are legacy integration coverage while behavior migrates to owners;
they are not templates for new tests. `make test-wiring-integration` discovers
all retained wiring packages; `make test-integration` includes that lane.
`wire-smoke` also retains the application wiring witnesses. New application
behavior uses public functional boundaries.

Once repeated linking has been reduced, prioritize the remaining measured
cost: unnecessary native binaries, repeated fixture construction, filesystem
and serialization work, excessive test output, and redundant cases. Use the
smallest fixture or snapshot that proves the behavior. Capacity-sized matrices,
large replay histories, and throughput loops belong in load/stress suites;
retain a representative unit case for each distinct behavior or boundary.
Moving cases **MUST** preserve their required gate in the destination layer.

## 3. Functional tests

Functional tests exist to protect customer experience. A valid functional test
names the actor, action, public entry point, and customer-observable result.
Customer-observable behavior includes successful output and state, documented
errors, cancellation, recovery, persistence, redaction, ordering, and lifecycle
behavior when a customer can see or depend on them. It does not include an
internal catalog merely because the catalog helps implement that behavior.

### Public execution model

- Functional application tests **MUST** construct the reusable application
  through `root.BuildProcess` and execute customer commands through
  `Process.Execute`.
- Scenarios **MUST** execute as Factory Sessions wherever the behavior admits a
  session. Each scenario owns its session, inputs, routes, streams, work
  directory, and cleanup.
- A package **MUST** consolidate `root.BuildProcess` construction into one
  shared process when the process is safe to reuse. Scenario isolation comes
  from sessions and test-owned boundaries, not repeated application builds.
- Functional tests **MUST NOT** compile, locate, or invoke the real `you`
  executable. Behavior requiring executable discovery, OS pipes, signals,
  process termination, or exit status belongs in integration testing.
- Invoking the Go test executable as a fixture process is still real executable
  and OS-process coverage. A build tag, long-test suffix, helper mode, or test
  name does not reclassify that behavior; move the smallest necessary proof to
  integration and retain only the controlled-boundary customer outcome in the
  functional lane.
- Ordinary customer flows **SHOULD** enter through the public CLI command
  contract. HTTP, MCP, or ACP entry is appropriate when that transport's
  customer contract or explicit parity is the behavior under test.

### Assertions and boundaries

Functional assertions **MUST** use public output, API responses, protocol
messages, session/work state, Factory Events, customer-visible persisted
artifacts, or observations made at an injected external-effect boundary.
Tests **MUST NOT** assert internal engine snapshots, constructor counts,
private event ordering, cleanup ledgers, route-allocation internals, registry
contents, package/file inventories, or other implementation topology.

Customer-visible persistence and replay guarantees **MUST** be asserted through
the public read, replay, export, or command surface. Directly decoding an
internal artifact is appropriate only in a unit or contract test owned by that
artifact's serializer; it is not a substitute for proving that a customer can
recover the value. Reusing a customer-selected destination, identity, or path
is a distinct behavior when reuse could merge, overwrite, or leak prior state.

External effects **MUST** be replaced at exact supported boundaries such as
`edges.Edges`. Use controlled provider command runners, clocks, filesystems,
network clients, and process runners rather than adding test-only access to
internal services. A filesystem scenario may write to a test-owned temporary
filesystem and assert the customer-visible result. It **MUST NOT** read or
mutate the user's real profile, home directory, configuration, daemon, or
workspace.

Values generated by a functional fixture **MUST** satisfy the same public
format and lifecycle rules as production values. Test-only session IDs, paths,
URLs, payloads, or timestamps that bypass validation can conceal a real
customer failure or create failures that production cannot produce. Prefer the
production generator unless deterministic identity is itself necessary for the
customer assertion; then use a deterministic value that is still contract
valid.

Mock the unavailable or unsafe edge, not the behavior being proved. If the
claim is specifically that a real remote service or published asset works, a
controlled substitute cannot prove it; use the authorized integration,
contract, asset-conformance, or release gate instead.

### Parallelism and determinism

Functional tests **MUST** run in parallel by default. Top-level independent
scenarios **MUST** call the language's parallel-test facility, and subtests
**SHOULD** do the same when their fixtures are independent.

The test framework **MUST** own subtest scheduling. In Go, register subtests
with `t.Run` from their parent and call `t.Parallel` inside each independent
subtest; do not call `t.Run` concurrently from a hand-built goroutine pool.
Manual scheduling can race the test runner itself and bypass its concurrency,
cleanup, and reporting guarantees.

Parallelism **MUST** remain bounded by the canonical functional-lane job
budget. "Run in parallel" means overlapping independently owned scenarios and
packages within that budget; it does not authorize unbounded application
construction. Broad verification **MUST** use the repository's functional-lane
runner (or the same explicit package-concurrency limit), not a raw package
wildcard whose tool default can start every package at once. Startup failures,
installation contention, or deadline expiry caused only by excessive package
fan-out are harness-capacity defects; increasing scenario timeouts does not fix
them.

Serialization is allowed only when overlap would change a customer-visible
contract that the test is explicitly proving. The test **MUST** document that
invariant and minimize the serialized region. Internal shared state, fixture
collisions, hard-coded ports, reused peer routes, global environment mutation,
or a harness that cannot isolate sessions are defects to fix, not standing
reasons to serialize a package.

When only some cells require local Current Factory or `~default` ownership,
the package **MUST** separate execution into ownership phases. Run the smallest
local cohort serially, then run independent explicit-session or hosted cells in
parallel. A local-only constraint on one customer journey does not justify
serializing API/session journeys that own their state.

Each parallel scenario **MUST** own unique identifiers, routes, ports,
directories, streams, and fake-edge state. Shared test support **MUST** be safe
under the race detector. Package setup may be shared, but mutable scenario
state may not leak through it.

A scenario **MUST NOT** test its shared fake, router, ledger, or selector table
by temporarily installing broad or ambiguous fixture state while customer
scenarios are active. Such fixture self-tests belong in focused unit tests for
the support boundary. Retain only the customer-visible failure or recovery
behavior in the functional suite, using scenario-owned fault injection.

A reusable application process does not make its invocation state reusable.
Concurrent `Process.Execute` calls **MUST NOT** share a customer home/profile
while either call can initialize, install, migrate, select, or clean resources
there. First-run package installation and profile migration are writes even
when the command under test appears read-only. Either complete and verify that
bootstrap before parallel work, or give each concurrent scenario a distinct
home/profile. Active installation-lock contention is an isolation defect, not
useful parallelism and not a reason to increase a timeout.

The scenario's explicit Factory Session identity **MUST** be allocated before
runtime construction and carried unchanged through runtime opening, routing,
events, external effects, and cleanup. Assigning a unique wrapper or response
identity after an internal runtime has already opened `~default` is not
isolation: concurrent scenarios have already shared authority at that point.

Functional commands **MUST** run with a test-owned home/profile environment.
Inheriting the developer or CI worker's real `HOME`, `USERPROFILE`,
`HOMEDRIVE`, or `HOMEPATH` creates cross-package installation locks, reads
ambient operator settings, and can mutate customer state. Prefer an
invocation-local environment on the session or command; do not mutate the test
process's global environment to obtain isolation.

Assertions in a shared fixture **MUST** also be scenario-scoped. Package-wide
active-call counters, aggregate route counts, global call order, and shared
"nothing is running" checks are invalid while independent scenarios overlap.
Observe the scenario's explicit Factory Session, route, stream, or fake edge
instead. If the harness cannot attribute an observation to its owning
scenario, fix that attribution boundary before using the observation as a
reason to serialize.

Calling the parallel-test facility proves only that a scenario is eligible to
be scheduled concurrently. It does not prove that the scenario executes
concurrently. A fixture **MUST NOT** hold a package-wide mutex, mutable edge
selector, Current Factory lease, output capture, or cleanup lock across an
entire `Process.Execute` call merely to make a shared process appear safe. Fix
the ownership boundary, give the invocation an explicit Factory Session and
scenario-owned edge state, or document the customer-visible invariant that
requires the smallest remaining serialized region.

Before declaring explicit-session conversion blocked on a default-session or
Current Factory limitation, auditors **MUST** check whether the public hosted
application can open the scenario's Factory Session directly and observe its
Work, events, responses, and terminal state through session-scoped endpoints.
The fixture's idle host Factory **MUST NOT** contain seeded customer Work or
consume a scenario-owned fake-edge outcome during application startup.

Tests **MUST** synchronize on observable readiness or completion signals. Fixed
sleeps and polling delays are prohibited as synchronization. Timeouts are
safety ceilings only: they **MUST** be generous enough for full-suite and race
execution, return immediately when the signal arrives, and never serve as a
performance assertion.

A hosted readiness clock **MUST NOT** include unrelated prerequisite fixture
setup. If a scenario does not test first-run initialization, complete its
scenario-owned customer bootstrap through the same reusable public process
before starting the host, then let the hosted invocation observe the prepared
state. If first-run initialization is the customer behavior, retain it inside
the clock and assert its public outcome explicitly. A race-only readiness
failure caused by packaged installation, fixture authoring, or other blocking
setup is fixed by separating that phase, not by increasing the timeout.

Tests **MUST NOT** open or await an optional event stream when the scenario does
not assert that stream's customer contract. Cancellation and failure paths must
synchronize on a terminal session, Work, or event guarantee that the product
actually promises; absence of an optional response frame is not a readiness or
completion signal.

### Scope and case selection

Functional suites **MUST** cover expanded customer behavior, not internal
implementation permutations. Select representative happy, failure, boundary,
and recovery cases from the customer contract. Do not duplicate pure
validation branches already proven by unit tests, and do not turn commands,
models, providers, routes, schemas, or files into an inventory matrix unless
each entry has distinct customer behavior.

When a customer input is transformed before an external effect, the functional
test **SHOULD** assert the expanded value at the testable public/external-effect
boundary (for example, the provider prompt or authored filesystem result), not
an internal submission record, token, runtime snapshot, or canonical storage
shape. Timed profiling captures, exhaustive diagnostics endpoint sweeps, and
every-profile matrices are inventory or load coverage unless each case has a
distinct documented customer outcome; functional coverage must retain a
representative customer journey instead.

Slow or flaky functional tests **MUST** be fixed, reclassified, or removed. A
flake fix addresses the race, shared ownership, readiness signal, cleanup, or
incorrect layer; increasing sleeps, retries, or serial execution is not a
general fix.

Functional coverage is evidence about retained customer journeys, not a reason
to preserve internal, inventory, topology, or compiled-executable scenarios in
the functional lane. Coverage gates **MUST NOT** require a functional test to
target an internal package directly. When removing or reclassifying an invalid
functional test intentionally lowers incidental package coverage, reviewers
**MUST** confirm that its customer guarantee is retained at the correct layer
and then reconcile the functional coverage floor to the measured behavioral
suite. A drop in coverage of a public customer surface still requires an
explicit behavior audit; the floor may not be lowered merely to make CI green.

Optimization **MUST NOT** replace a distinct customer guarantee with an easier
but weaker assertion. Before deleting or rewriting a cell, compare its actor,
action, public boundary, persisted identity, and observable result with the
replacement. If any customer-relevant dimension disappears, retain one focused
proof at the correct layer or explicitly record the unproven contract.

## 4. Contract and conformance tests

Contract tests protect externally meaningful shapes: authored OpenAPI,
protocol negotiation, public serialization, generated-client compatibility,
configuration schemas, and declared published assets. They **MUST** test the
contract owner and a property consumers rely on.

Checks that enumerate source files, packages, routes, constructors, docs links,
registrations, or generated file locations are lint/static checks unless the
enumerated shape is itself a documented public contract. Move such checks into
a named lint target with an actionable failure message. Do not cite a green
inventory test as customer-behavior evidence.

## 5. Integration and release tests

Integration tests **MUST** use an already compiled entity produced by the build
or release lane. The test **MUST NOT** run `go build`, rebuild per case, or hide
compilation inside setup. CI or the invoking target builds once and passes the
immutable artifact identity to the suite.

Integration coverage **MUST** be intentionally small: one or a few cases for
the production boundary properties that cannot be established below it. Good
examples include executable discovery, startup and shutdown, pipes and signals,
exit status, packaging, migration against a real store, and serialization
against a real compatible service. Exhaustive input, error, and provider
matrices belong at unit or functional layers with controlled boundaries.

Integration tests **SHOULD** reuse the compiled artifact, run independent cells
in parallel when safe, use isolated profiles and temporary directories, and
avoid real paid or mutating remote dependencies unless the plan declares
authority, budget, duration, and cleanup.

CI evidence produced by a compiled-artifact scenario—including executable
selection, signal delivery, process cleanup, and independently packaged client
compatibility—**MUST** be owned, required, and published by the integration or
release lane. Functional jobs **MUST NOT** retain environment switches or
artifact requirements for a scenario after it moves to integration.

Release smoke and end-to-end tests follow the same economy: prove a few critical
customer journeys through the delivered system, not every branch already
covered below.

## 6. Load, stress, race, and performance tests

Load and stress tests **MUST** live under `tests/load/`, `tests/stress/`, or a
more specific dedicated performance package. They **MUST NOT** live in unit,
functional, or integration packages and **MUST NOT** run as an accidental part
of their default suites. Each harness names its workload, environment,
duration, resource budget, success thresholds, and captured measurements.

Race detection is a correctness gate, not a load test. Changed concurrent code
and functional support **MUST** receive race coverage in hosted CI's race jobs;
shared harnesses **SHOULD** also receive a broad scheduled or PR race run.
Native `-race` does not work on the Windows factory host, so no workstation runs
it locally, and a race in code a PR does not change is a separate fix that
never blocks that PR. A race-detector
`DATA RACE` report is distinct from a timeout caused by slower instrumented
execution. Both must be addressed at their root cause.

Wall-clock assertions are allowed only for an explicit customer latency
contract in a controlled performance lane. Ordinary tests **MUST NOT** fail
because a shared host was busy. Optimize test topology by reducing builds,
processes, duplicated fixtures, real workers, and repeated setup, then use PR or
CI package timing as directional evidence.

A required coverage or correctness lane that already executes the complete
test corpus **SHOULD** own package and wall-time diagnostics for that execution.
CI **MUST NOT** add another required job that reruns the same corpus solely to
measure its latency. A separate performance lane is justified only when its
workload, environment, or customer latency contract is materially different;
otherwise reuse the existing lane's timing artifact and eliminate the duplicate
execution.

## 7. Location and naming

- Unit tests live beside the package they own or in a shared test package within
  the same service owner under the isolation rules in Section 2.
- Functional scenarios live under
  `tests/functional/<customer-domain>/<behavior>/...`.
- Integration scenarios live under `tests/integration/...` and consume a
  prebuilt artifact.
- Load and stress harnesses live under `tests/load/...` or `tests/stress/...`.
- Repository enforcement lives in a named lint/static-check tool or target.

Directories describe durable customer domains, not transports or implementation
layers, unless the transport itself is the customer contract. Test and subtest
names describe the observable behavior rather than the internal method or
fixture arrangement.

## 8. Required planning record

For every added or changed test, the planner **MUST** record:

1. the customer or component behavior being proved;
2. the selected layer and why a lower layer cannot prove it;
3. the public or testable boundary used;
4. real and controlled dependencies;
5. the parallel execution and isolation model;
6. build-artifact ownership when integration is selected;
7. the representative case matrix and deliberately omitted duplication; and
8. the exact command or gate and the property it proves.

Functional-test plans **MUST** explicitly state the Factory Session strategy,
shared `root.BuildProcess` ownership, and why any serialized cell represents a
customer-visible invariant. Integration-test plans **MUST** name the upstream
artifact build and cap the case set. Load-test plans **MUST** name their
dedicated package and resource budget.

## 9. Implementation and review enforcement

Implementers **MUST** reclassify a planned test when repository evidence shows
the chosen layer violates this standard; that is a plan delta, not permission
to disguise the test. They **MUST** run focused normal tests locally (race coverage comes from
hosted CI) and must not weaken assertions to gain parallelism.

Reviewers **MUST** reject:

- unit tests that assemble or simulate overall system behavior;
- functional tests without customer-observable assertions;
- functional tests that build or invoke a binary;
- functional tests that assert internals, inventories, or topology;
- per-scenario `root.BuildProcess` construction where safe reuse is available;
- unexplained serialization, fixed-sleep synchronization, shared mutable
  fixtures, or race-unsafe support;
- integration tests that compile their own artifact or carry an exhaustive
  case matrix; and
- load or stress behavior placed in another test layer.

Review evidence **MUST** identify the behavior proved, layer, dependency
fidelity, artifact identity where applicable, parallel/race result, and any
remaining unproven edge. A generic suite pass is not sufficient evidence by
itself.

## 10. Functional-test performance audit and optimization

A functional-test performance audit **MUST** measure the executed critical
path. Source-level construction counts and calls to the language's parallel
test facility are useful discovery signals, but they do not prove that setup is
shared or scenarios overlap.

### Audit procedure

For every slow package, the auditor **MUST**:

1. capture package and leaf-subtest timings in normal execution;
2. repeat the focused package enough times to distinguish a stable cost from
   host variance; race coverage comes from hosted CI;
3. count executable builds, subprocess launches, application construction
   sites, actual constructed process instances, and immutable edge shapes as
   separate quantities;
4. identify locks, gates, shared identifiers, Current Factory or `~default`
   ownership, environment mutation, ports, directories, streams, and cleanup
   that span a complete `Process.Execute` call;
5. trace the slowest leaf from readiness through its terminal signal instead of
   inferring its cost from the enclosing test name;
6. classify its elapsed time as fixture setup, scheduler or mutex queueing,
   profile initialization or installation-lock contention, active customer
   execution, readiness/completion waiting, timeout exhaustion, and cleanup.
   Report queued and blocked time separately from intrinsic scenario work;
7. inspect every wait to determine whether it completed from an observed signal
   or exhausted its timeout ceiling;
8. verify terminal outcomes by typed error, error identity, public status, or
   documented code. A substring that can also occur in harness or deadline
   text is not proof of cancellation, timeout, shutdown, or recovery; and
9. name the customer-observable behavior for every retained cell. Harness
   cleanup, fixture ledgers, constructor topology, and test-process recursion
   are not functional customer coverage; and
10. report the complete observed timing sample set or a representative range and
   median. Best-case or unloaded samples may be shown separately but **MUST NOT**
   headline the package result when slower valid samples were observed.

An aggregate test marked parallel may still serialize all useful work through
a fixture mutex. A table of subtests may hide one deadline-bound leaf. A large
safety timeout may hide a missing completion signal while the test remains
green. Reviewers **MUST** inspect these cases before declaring a delay an
unavoidable customer lifecycle boundary.

For audit purposes, a scenario is **non-blocking** only when it releases shared
fixture ownership while it waits and independent customer scenarios can make
progress through the same reusable application. A goroutine, asynchronous API,
or `t.Parallel` declaration is still **blocking** when it waits behind a lock
held for a complete invocation. The optimizer **MUST** verify overlap from
timestamped leaf evidence or instrumentation; source shape alone is
insufficient.

The auditor **MUST** compare the elapsed package critical path before and after
parallelization. Concurrent leaves that become individually slower through
shared-profile locks, repeated first-run installation, scheduler saturation,
or a hidden application gate have moved work rather than removed it. Keep
independent scenario homes where initialization is mutable, share only proven
immutable setup, and report both leaf inflation and package-wall improvement.

The audit **MUST** classify both the public operation and the fixture ownership
span. A public operation that starts work and returns before completion can
still be fixture-blocking when the harness retains a command gate, mutable edge
selection, output capture, Current Factory lease, or cleanup ownership until
the work terminates. Conversely, a scenario may synchronously await its own
result without blocking peers when all mutable state and completion signals are
session-owned. Reports **MUST NOT** label an API "non-blocking" unless both
properties have been verified independently.

Parallel subtests do not begin until their parent test body returns. Shared
servers, processes, listeners, temporary roots, and other parent-owned
fixtures used by parallel children **MUST** therefore use `t.Cleanup` (or an
equivalent lifetime owner that joins all children), not a parent `defer` that
runs before those children are released. Reviewers **MUST** verify fixture
lifetime separately from whether the child calls `t.Parallel`.

Scenario cleanup **MUST** assert only resources owned by that scenario. It
**MUST NOT** treat package-global route, session, stream, or active-call counts
as leaks while independent peers may still be running. Per-scenario handles
must be removed and drained locally; any package-wide quiescence assertion
belongs after every package scenario has joined. Fixture topology counts that
do not protect a customer-observable resource belong in fixture unit tests or
lint, not the functional lane.

A customer operation that intentionally observes a process-wide fleet,
catalog, or other global projection **MAY** require an isolated observation
window. Before serializing it, the test **MUST** prove that the public request
is genuinely global and cannot select scenario-owned identities. A flag or
helper name is not evidence of scope: the auditor must inspect the resulting
public request and response. Tests **MUST NOT** pass only because the package is
otherwise quiescent when their claimed selector is ignored or unsupported.

Long-running cancellation and recovery scenarios **MUST** expose distinct,
observable gates for startup readiness, cancellation acknowledgement,
terminal completion, and recovery readiness. They **MUST NOT** keep an
invocation-wide fixture lock while waiting at any of those gates. Pre-runtime
validation that cannot start customer work **SHOULD** execute without acquiring
runtime or Current Factory ownership; the fixture must not serialize a cheap
diagnostic behind an unrelated live invocation.

### Optimization order

Apply optimizations in this order so speed does not weaken the proof:

1. Remove tests with no customer observer, or move unit, contract, inventory,
   load, executable, and harness behavior to the correct layer.
2. Remove CLI compilation and test-binary subprocess recursion. Functional
   command behavior enters through `Process.Execute`; the deliberately small
   integration suite consumes one artifact built outside the tests.
3. Build one reusable process per compatible immutable edge shape. Consolidate
   repeated setup, package installation, service hosting, and fixture data.
   Keep a separate graph when reconstruction or a genuinely different
   immutable production edge is itself part of the customer behavior.
4. Give each scenario an explicit Factory Session and unique routes,
   identifiers, directories, streams, and fake-edge state. Preserve that
   session identity through every command and provider boundary so adapters do
   not fall back to process-global `~default` ownership.
5. Parallelize independent leaf scenarios, not only their aggregate parent.
   Minimize lock scope and verify from timing or instrumentation that complete
   `Process.Execute` calls actually overlap.
   Distinguish a scenario's own synchronous wait from a shared fixture block:
   only the latter prevents independent progress and must be removed from the
   critical path.
   Partition mixed packages into serialized local-ownership and parallel
   explicit-session phases instead of choosing one policy for the whole
   package.
6. Replace sleeps, polling cadence, and deadline-driven completion with
   observable readiness gates, event subscriptions, explicit cancellation, and
   deterministic completion signals. Retain timeouts only as failure ceilings.
7. Separate distinct public lifecycle controls in the scenario. For example,
   canceling a Factory Session and stopping a CLI invocation that owns a local
   server may be separate customer actions; proving one must not rely on the
   other timing out.
8. Drain already-retained public event or response heads when available rather
   than waiting for a new live event that may never arrive.
9. Re-run focused normal tests locally; hosted CI runs the race and broader functional lanes.
   Record before-and-after package and slowest-leaf timings, remaining
   serialization, and the customer invariant that requires it.

When one scenario submits multiple Works whose coexistence is part of the
claim, admit them atomically where the public contract permits or hold execution
behind a deterministic edge gate until admission completes. Do not depend on a
fast second request beating completion of the first.

Shared mutable edge switches are not a default convergence technique. Prefer
immutable fixtures and scenario-owned fakes; otherwise an optimization can
replace construction cost with cross-scenario coupling and flakes.

## 11. Delivery checklist

- Every test has one named behavior and observer.
- Unit tests remain inside the component boundary.
- Functional tests use public customer contracts, sessions where possible,
  controlled edges, a reusable process, and parallel isolation.
- No functional test builds or invokes the CLI binary.
- Integration tests consume one prebuilt artifact and contain only essential
  real-boundary cases.
- Inventory and topology enforcement is lint/static analysis.
- Load and stress coverage is isolated in a dedicated suite.
- Readiness is signal-driven; timeouts are ceilings and no fixed sleep hides a
  race or lifecycle defect.
- Focused tests pass locally; hosted CI's race coverage and broader gates pass.
