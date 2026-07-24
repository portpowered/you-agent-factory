# What?
This defines the backend structure and how all components are set.
It defines the meta system, not necessarily deep internals of how each component works.

# overview
pkg/
    -> initializer
    -> root/
    -> wire/
    -> services
    -> transports
    -> platform


1. Wire instantiates all isntances of services for cross service interop,
2. services denote subservices of the overall system

## Normative conformance basis

This document defines the intended package shape. The following repository
standards are normative when this document is interpreted or enforced:

| Normative source | Rules applied here |
| --- | --- |
| [`docs/internal/standards/STANDARDS.md`](../internal/standards/STANDARDS.md) | Standards under `docs/internal/standards/` take precedence over architecture and process notes. |
| [`general-backend-standards.md`](../internal/standards/code/general-backend-standards.md) | Package families, dependency direction, service ownership, one composition root, lifecycle ownership, transport/domain separation, code shape, and test boundaries. |
| [`code-review-standards.md`](../internal/standards/code/code-review-standards.md) | Architecture violations, unexplained construction paths, hidden side effects, and missing required tests are review blockers. |
| [`wire-injection-full-blow.md`](../../wire-injection-full-blow.md) | Migration target for one canonical Wire graph, a reusable root-built process, exact injected roles, lifecycle-only Initializer behavior, and external-edge-only functional overrides. |

The conformance tables use these statuses:

| Status | Meaning |
| --- | --- |
| **Enforced** | A required lint or CI test rejects the violation generally. |
| **Partial** | A checker covers enumerated imports, symbols, or paths, but does not enforce the invariant generically. |
| **Missing** | The rule exists in prose but has no required mechanical gate. |
| **Failing** | A mechanical gate exists, but the current audited worktree does not pass it. |

Unless a table says otherwise, the expected merge gate is `make verify-lint`,
which runs the repository `lint` target in CI. Package tests are supplemental:
an architecture invariant should not depend only on a test being discovered in
a particular test lane when it can be checked statically.

## Enforcement implementation priorities

Implement new package-shape enforcement in this order:

1. **Unit tests do not depend on service internals.** A unit test outside
   `pkg/services/<owner>` must not import or construct anything below
   `pkg/services/<owner>/internal` or another implementation-classified
   service package. Owner-local unit tests are the deliberate exception. The
   check must be generic and owner-derived rather than an allowlist of known
   internal package paths.
2. **Service-root production files contain no implementation logic.** Files
   directly under `pkg/services/<service>` may declare public service
   interfaces, request and result structs, public value types, documented
   errors, constants, and explicitly approved pure contract helpers. They must
   not contain concrete service state, service method implementations, IO,
   goroutines, lifecycle behavior, or domain workflow implementations.
3. **Bespoke services own their protocol adapters and transformations.** HTTP
   adapter contracts, request/response transformation, and the service-specific
   HTTP adapter live under `pkg/services/<service>/transports/http`. The
   top-level `pkg/transports/http` package owns server and route composition and
   generated OpenAPI contracts, but does not implement service-specific
   transformation or service behavior. Apply the same ownership model to CLI
   and MCP adapters as those migrations are prioritized.
4. **Functional tests use the domain-mirrored customer-boundary layout.** New
   functional tests live under `tests/functional/<domain>/<subsection>/...` and
   use `root.BuildProcess`, `Process.Execute`, public HTTP, or public MCP.
   Domain nouns own domain proofs; `transport` owns transport mechanics only.
   There is no durable `features/` wrapper and no transport-first ownership for
   domain behavior. The catch-all `tests/functional/runtime_api` package is
   migration-only deletion debt: prohibit new files and scenarios there, record
   its current files as deletion-only debt, and migrate each scenario to its
   owning domain/subsection package. `tests/functional/internal/support` remains
   the only shared harness exception.

Each new check should initially compare findings with an exact, deletion-only
nonconformance ledger. New findings, increased occurrences, and stale ledger
entries are blocking; existing recorded findings may only remain unchanged or
be removed.

## Overall package-family conformance

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Direct children of `pkg/` use only the approved package families. | `make pkg-boundary` / `cmd/pkgboundarycheck` root-family allowlist and retired-root checks. | **Partial** | Align the executable allowlist with the normative standard: `pkg/config` is still approved by the checker even though the standard retires it. |
| Hand-maintained package directories contain no more than 15 Go files. | `make pkg-file-count`, included in `make lint`, blocks unrecorded oversized packages and exactly ratchets audited existing counts. | **Enforced** | Burn down `backend-package-file-count.json`; every reduction must lower or remove its entry. |
| No package grows through broad permanent architectural exceptions. | Exact deletion-only ledgers in `pkgboundarycheck` and `backend-package-file-count.json`; `backend-exemption-budget.json` covers size/complexity directives. | **Partial** | Consolidate the remaining structural ledgers into one architecture conformance ledger. |

# intializer

intiializer is the function that is responsible for initializing the appropriate bundle roots and services during invocation.

i.e. the CLI may want to activate a http server mode. the initializer is responsible for getting the pointer instances of the services for
1. worker daemons
2. crons and automations

and triggering start/stop as appropriate.

type Initializer struct{
    Start(ctx context.Context, StartRequest) (StartResponse, error)
    Stop(ctx context.Context, StopRequest) (StopResponse, error)
}

func NewInitializer(sessionService, workerService, etc) Initializer {

}

In general the initializer is very thing, and does mostly nothing but initilize/lifecycle operations.

## Initializer conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Initializer starts, stops, cancels, joins, and unwinds already-constructed lifecycle roles only. | `make pkg-boundary` initializer-behavior scan and `make ownership-boundary-check`. | **Partial** | The scanners recognize selected constructor names, imports, edge bags, filesystem calls, and lifecycle behavior; they cannot yet prove semantically that every operation is lifecycle-only. |
| Initializer does not construct product services or transports. | `pkgboundarycheck`, `ownershipboundarycheck`, and `pkg/initializer/application/boundary_test.go`. | **Partial** | Replace enumerated constructor/symbol lists with a generic dependency and constructor-ownership rule. |
| Initializer receives exact roles rather than `bundle.Bundle`, `edges.Edges`, a service locator, or a runtime scope. | `pkgboundarycheck` initializer-behavior rules and Initializer boundary tests. | **Partial** | Enforce forbidden broad dependency types through resolved Go type information rather than selected type/symbol names. |
| Initializer does not import service implementation subpackages or transport packages. | `ownershipboundarycheck`. | **Enforced** | Keep exact leaf lifecycle contracts explicitly classified when a legitimate external-effect port is needed. |

# platform

platform are utility functions that are generally used across services

i.e.

platform
    /filesystem
    /observability
        /logging
        /metrics
    /clock
    /random

These represent base structures and utilities that are injected generally across services that have no meaningful functionality.

Generally, platform functions are to be mockable and are to be injected as necessary.
i.e. the platform may construct a clock, but a mock may be injected for functional tests to test system behavior.

## Platform conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Platform contains policy-free cross-cutting adapters and utilities, not product-domain policy. | `make ownership-boundary-check` rejects most Platform imports of service packages. | **Partial** | The allowlist is explicit rather than capability-based and currently conflicts with `pkgboundarycheck` for `pkg/platform/pty -> pkg/services/workers/agypty`. |
| Platform implementations are replaceable through exact injected ports. | Production-default selection checks in `pkgboundarycheck`; functional process-edge checks cover selected edge types. | **Partial** | Define every external effect and its owning port in one machine-readable inventory and verify every Platform adapter implements one of those ports. |
| Platform does not choose Factory, Factory Session, worker, model, Work, or transport policy. | Import restrictions plus selected default-selection and behavior scans. | **Partial** | Add generic dependency rules and owner tests for policy selection; current symbol-prefix checks can miss newly named policy. |
| Time, filesystem, process, randomness, metrics, and logging effects are injected where behavior must be deterministic. | Logging boundary, production-default, transport-behavior, and ownership checks cover portions of this rule. | **Partial** | Add a complete external-effect inventory; randomness and some filesystem/process paths are not uniformly classified. |


# wire

Wire is the primary dependenct injection functiont hat wires all the dpeendencies together.
its largely just about grabbing and providing data, it has no functionality implemented on its own. There are no real constructors, it delegates constructors to downstreams.

It has a single function in wire.InjectBundle, and nothing else is exposed in the wire package.

## Wire conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| `pkg/wire` is the sole product composition root. | `make pkg-boundary` rejects imports of `pkg/wire` outside `pkg/root` and `pkg/wire`; `pkg/wire/boundary_test.go` rejects selected alternate builders. | **Enforced** | Keep all new construction paths reachable from the generated canonical injector. |
| Root `pkg/wire` composes service-local `pkg/services/<service>/wire` providers rather than importing service internals directly. | Go rejects direct imports of service `internal` packages, but the service-local Wire package convention is not yet required. | **Partial** | Add an import-shape check and migrate root-owned service construction into the corresponding service-local Wire packages. |
| `pkg/root.BuildProcess` calls the single `wire.InjectBundle` entrypoint. | Root/Wire tests and application-graph import restrictions provide indirect coverage. | **Partial** | Add a static call-graph assertion for exactly one production `BuildProcess -> InjectBundle` edge. |
| `InjectBundle` constructs the complete inert dependency graph and performs no lifecycle start. | Wire generation tests and focused construction/lifecycle tests. | **Partial** | Add a generic check preventing start/listen/watch/run calls during injection, rather than relying on selected tests. |
| Wire delegates behavior to owner constructors and contains no domain policy. | Review standard and package-size/complexity checks only. | **Missing** | Add a Wire behavior rule that permits provider selection/binding but rejects domain computation, IO, lifecycle, and request handling. |
| `InjectBundle` is the only exported production declaration in `pkg/wire`; Wire subpackages are private implementation details. | `TestWirePackageExposesOnlyCanonicalApplicationInjector` checks only exported functions whose names start with `Inject`. | **Partial** | Reject every exported production function, type, variable, and constant except an exact allowlist. `BundleSet` and exported Wire-subpackage providers are not covered by the current test. |
| Generated `wire_gen.go` remains synchronized and directly reaches product constructors. | `make wire-smoke` regenerates twice, checks drift, and runs Wire tests; `make verify-api` runs it. | **Enforced** | Consider moving the no-secondary-builder assertions from package tests into the central architecture checker. |

# transports

transports compose of

transports/
    http
    mcp
    cli

## http

The http server constructs a http.handler that handles requests, it wires all the dependent handlers from all the services together. It does not itself implement any handlesr.

type Handler struct {
    sessionHandler sessionservicehttp.Handler
}

func NewHandler(sessionHandler sessionservicehttp.Handler) {

}

The HTTP server handles the wiring of the routes to the handlers manually, but that's basically it.

## MCP/CLI

The same functional wrapping logic holds for the MCP/CLI.

i.e. the CLI is responsible for flags and what not, but the actual run functions that are used to perform execution and transformation between the interface presentation and the internal system interfaces are done bespoke to each service implementation's transports package for the function.

## Transport conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Top-level `pkg/transports/{http,cli,mcp}` owns protocol composition, flags/routes/tools, and generated boundary contracts only. | `pkgboundarycheck` transport-behavior scan rejects selected IO, lifecycle, concurrency, default selection, and domain-policy operations. | **Partial** | Enforce permitted dependency direction and declaration roles generically; symbol-name and prefix inventories can miss newly named behavior. |
| Domain services do not import transports. | `make pkg-boundary` domain-transport import scan. | **Enforced** | Generated contract exceptions must remain exact and generated-only. |
| Transports use service-root contracts and do not import service implementations. | `pkgboundarycheck` transport-private implementation checks. | **Partial** | Replace the explicit `transportPrivateServiceSubpackages` list with a deny-by-default classification for every service subpackage. |
| Service-specific HTTP, CLI, and MCP adapters are service-owned under `pkg/services/<service>/transports/<protocol>`. | `make pkg-structure` permits the named `transports` container while rejecting unrelated direct service-root directories. | **Partial** | Move service-specific protocol behavior and mapping out of generic transport grab bags into the owning service's named protocol adapter. |
| Transport mapping performs representation conversion only; it does not perform IO, lifecycle, or asynchronous work. | `ownershipboundarycheck` scans `pkg/transports/mapping`; `pkgboundarycheck` has additional transport-behavior rules. | **Partial** | Merge the overlapping rules and enforce calls using resolved import/type information. |

# service

each service declares an interface and an implmememntation of an interface.

pkg/services/<factorysessions>
    -> service.go (an interface declaration)
    /internal
        -> internal implementation content that goes to implementing that service
    /wire
        -> service-local providers/injector that may import the service's internal implementation
    /transports
        /http -> service-owned HTTP adapter contract and transformation
        /mcp -> service-owned MCP adapter contract and transformation
        /cli -> service-owned CLI adapter contract and transformation
    /services
        /<subservice> -> another subservice root with the same wire/internal/transports/services shape

## root package (services/factorysessions)

root package has no implementation logic, its just a
```
interface Blah {
    GetX()
    ListX()
    DoX()
    DeleteX()
}
```

implementation details are only inside of the service's `internal` tree.
`pkg/services/<service>/wire` is the public construction bridge: it can import
the service's internal implementation and exposes only the focused providers or
injector required by the root `pkg/wire` composition graph.

## Service package conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Every durable product service is a direct child of `pkg/services`. | Root-family and retired-root rules in `make pkg-boundary`. | **Enforced** | Add an explicit approved service-owner inventory so arbitrary new service names require ownership review. |
| Every service and nested subservice root declares exactly one named interface. | `make pkg-structure` scans direct service roots and recursively scans `services/<subservice>` roots; current interface sets are exact deletion-only debt. | **Enforced** | Burn the baseline down until every root has exactly one interface. |
| A service root exposes the singular interface and mostly plain request/result structs without loose exported functions. | `make pkg-structure` rejects every new exported package-level function and ratchets existing functions by exact file/symbol. | **Partial** | Add a later plain-struct check for function/interface fields, mutable implementation state, and non-contract declarations; those are intentionally outside the first light gate. |
| Service-root production files do not contain implementation logic. | `make pkg-structure` covers exported package-level functions; selected construction, transport-behavior, size, and complexity checks cover other portions. | **Partial** | Extend the AST rule later to concrete methods, IO, goroutines, lifecycle behavior, and domain workflow bodies after the root-function debt is reduced. |
| Direct service-root directories are limited to `wire`, `internal`, `transports`, and `services`; the same rule applies recursively to subservices. | `make pkg-structure` applies the path rule recursively and records every current unexpected directory as exact deletion-only debt. | **Enforced** | Burn down the current directory inventory; new directory names are immediately blocking. |
| Service-root imports are limited to the standard library, neutral contract packages, and explicitly approved peer service-root contracts. | Peer-service implementation import rules and domain-transport rules. | **Partial** | Add an explicit import policy for service roots. Current enforcement allows many imports as long as they are not on an enumerated private list. |
| Cross-service consumers use only the peer service root, never its implementation or subservices. | `pkgboundarycheck` peer-service, external-implementation, test-service, and support-service scans. | **Partial** | Convert `convergedServiceSubpackageRoots` and other explicit maps into a generic owner-boundary rule. |
| Product service constructors are selected only by Wire; other packages receive an already-constructed role. | `pkgboundarycheck` scans construction-shaped calls outside the owner and Wire, with exact value-constructor exceptions. | **Partial** | Constructor detection is name-prefix based (`New`, `Build`, `Create`, and similar); resolved return types or explicit constructor metadata would be safer. |

## internal

`internal` contains the private implementation of the service. Go's internal
package rule prevents packages outside `pkg/services/<service>` from importing
it directly. Owner-local packages and tests can use it when appropriate. Root
`pkg/wire` reaches the implementation only through the service-local
`pkg/services/<service>/wire` construction package.

### Internal implementation conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Implementation code is located in the service-owned `internal` area rather than the service root. | Explicit converged/private subpackage maps in `pkgboundarycheck`, plus Go's compiler-enforced internal import rule once code is placed there. | **Partial** | Standardize on `pkg/services/<service>/internal` and migrate existing implementation packages with exact deletion-only debt. |
| Other services, transports, external tests, and shared test support cannot import the implementation area. | Go enforces the parent-tree boundary for `internal`; existing production, test, support, and transport scans cover current non-`internal` implementation paths. | **Partial** | Add a path-shape rule requiring implementation packages to move under the owning service's `internal` tree. |
| Root `pkg/wire` does not import a service's `internal` packages directly. | Go rejects that import once the implementation uses the reserved `internal` path. | **Enforced** | Root Wire must import `pkg/services/<service>/wire`, which is inside the allowed parent subtree and can construct the internal implementation. |
| Internal packages do not become alternate composition roots. | Application-graph import and product-service construction scans. | **Partial** | Add a generic rule preventing an implementation package from constructing peer services or returning a broad dependency graph. |

## service-local wire

Each service has a focused construction package at
`pkg/services/<service>/wire`. This package exists so the canonical root
`pkg/wire` graph can construct the service while the implementation remains in
the Go-protected service `internal` tree.

The service-local Wire package may:

- import its own service root and internal implementation;
- declare focused Wire provider sets, bindings, or an injector for that service;
- accept peer service-root contracts and Platform/external-effect ports needed
  to construct the service; and
- return the owning service's public root contracts to root `pkg/wire`.

It must not start lifecycle components, select customer operations, construct a
second application graph, import peer service internals, or expose its own
internal implementation types as cross-service contracts.

### Service-local Wire conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Every service implementation is constructed through `pkg/services/<service>/wire`. | Current construction is primarily centralized in root `pkg/wire`; there is no required service-local package shape. | **Missing** | Add the service-local Wire directory convention and baseline current root-Wire construction that has not migrated. |
| A service-local Wire package imports only its own root/internal packages, peer service-root contracts, and approved Platform/effect ports. | Peer implementation-import and construction scans cover portions. | **Partial** | Add an owner-derived import rule specifically for `pkg/services/<service>/wire`. |
| Root `pkg/wire` may import service-local Wire packages but no service `internal` package. | Go enforces the `internal` half; application composition rules recognize root Wire as the composition owner. | **Partial** | Add a positive/negative static import rule requiring service-local construction packages and rejecting non-Wire service implementation imports. |
| Service-local Wire returns public service-root contracts and does not expose implementation types. | No generic checker. | **Missing** | Resolve exported function return types and reject types owned below the service's `internal` tree. |
| Service-local Wire performs construction only and owns no lifecycle or domain behavior. | General review rules and selected constructor/lifecycle scanners. | **Partial** | Extend Wire behavior checks to every service-local Wire package. |

## service-owned transports (`services/factorysession/transports/http`)
This just declares a handler that handles a request. A root server gets injected an instance of the transport then uses that to handle certain requests.
This transport is responsible for interpreting between the request interface and the internal details of the interface.


```
type Transport struct {
    func Handle(req, Resp)
}

func (t Transport) Handle (req, Resp) {

}
func NewTransport(service factorysessions.Service) {

}
```


### other transports

The other transports
do similar thigns

### Service-owned transport conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Each service-owned adapter lives under `transports/<protocol>` and depends on the owning service root contract. | `make pkg-structure` permits the `transports` container for service roots; existing peer-implementation rules still apply. | **Partial** | Require `pkg/services/<owner>/transports/<protocol>` to import its owner root and replace aggregate mapping seams. |
| The adapter performs protocol decoding, service invocation, error mapping, and response encoding only. | Generic transport-behavior scans would cover the directory if it is under `pkg/transports`, but not necessarily when nested under a service. | **Missing** | Extend transport behavior classification to service-owned transport directories. |
| Top-level protocol packages compose these adapters without implementing service behavior. | Wire injects the Models HTTP representation adapter into the service-owned handler, then injects that handler into the application HTTP constructor. For CLI, Wire constructs one Models CLI service with its HTTP protocol and bootstrap invocation dependencies, and the Cobra constructor receives that service instead of separate command functions. `pkgboundarycheck` limits adapter imports to the matching protocol composer. | **Partial** | Add positive handler-ownership checks that connect every route, command, or tool to a service-owned adapter. |

## subservices

a service may have subservices under `services/<subservice>`. They follow the
same root, `wire`, `internal`, `transports`, and `services` rules recursively, with the
additional constraint that other programs cannot call subservices directly.

## Subservice conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| A subservice root has only `wire`, `internal`, `transports`, and `services` child directories and exactly one interface. | `make pkg-structure` discovers subservices recursively from `services/<subservice>`. | **Enforced** | Existing nonconforming subservice roots remain exact deletion-only debt. |
| A subservice is private to its owning service. | Peer-service and converged-subpackage checks cover registered paths. | **Partial** | Classify subservices from their path and reject every external import by default. |
| A subservice does not import or construct peer service implementations. | Peer-service and product-service construction scans. | **Partial** | Replace constructor-name and registered-path heuristics with resolved owner/dependency checks. |
| Cross-service behavior is exposed through the owning service root. | Normative standard plus registered cross-owner import checks. | **Partial** | Add an API reachability/ownership review check or required owner-level contract test for each externally consumed capability. |

# tests

the system has tests in the general form as follows:
- unit tests
- functional tests
- integration tests
- stress tests

## Test-layout conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Tests are classified as package unit, functional, integration, or stress tests with defined dependency permissions. | Unit and functional lanes discover tests by package/path; several boundary scanners use path-specific rules. | **Partial** | Define how `tests/adhoc`, `tests/factory`, `tests/functional_test`, and `tests/release` fit the taxonomy, then reject unclassified test roots. |
| Shared test support is not an application composition root. | `pkgboundarycheck` scans `internal/testutil` and `tests/functional/internal/support`. | **Enforced** | Keep any new reusable support root in the same policy inventory. |
| Architecture checks run in required CI rather than an optional local target. | `pkg-boundary` runs through `make verify-lint`. | **Partial** | Add `functional-boundary-check` directly to `LINT_TARGETS` or incorporate all of its rules into `pkg-boundary`. |

## unit tests

pkg/<my-package>/*_test.go

These tests instantiate a class and test the internal logic.

## constraints
Generally, these are not allowed to call across services. i.e. they don't perform initialization or call new on the children.
They are supposed to be fast.

We don't for example create the entire subservices here as we're not mean to, that's a functional tests job.

## density
we keep these plentiful as necessary but light, because we want them to run fast.

## Unit-test conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Unit tests remain package-local and construct only the narrowly owned component. | Test service-import and test-behavior scans in `pkgboundarycheck`. | **Partial** | There is no generic rule proving a unit test does not instantiate cross-service dependencies. Current checks cover registered implementation imports and selected policy/construction operations only. |
| Unit tests do not call constructors in peer services or assemble a multi-service graph. | Product-service construction scan and test-behavior scan. | **Partial** | Resolve constructor calls by package and return type; reject any peer-owned constructor regardless of naming. |
| Unit tests may fake peer service-root interfaces but may not import peer implementation/subservice packages. | Test service-import baseline and converged-subpackage checks. | **Partial** | Make all non-root service packages private by convention rather than relying on an inventory. |
| Unit tests do not call `root.BuildProcess`; customer-scale behavior belongs in functional tests. | `pkgboundarycheck` test-behavior rule rejects `BuildProcess` inside service tests. | **Enforced** | Keep reviewed command-inventory exceptions exact and outside service unit tests. |
| Unit tests remain fast and deterministic without live filesystem, process, time, or network dependencies unless the package owns that adapter. | General backend standard and review requirements. | **Missing** | Add effect-import/call checks for unit tests or require explicit integration build tags/classification for real effects. |

# functional tests

tests/functional/**.go

## logic
The functional tests are intended to test customer facing expected behaviors.

the functioanl tests call nito the root.go and instantiate the entire internal bundle and test it as an aggregate, we mock the edges.

## constraints
- they can't call into the internal service logic
- they generall operate blackbox so don't have systems internals except those that can be observed at the edges.

## structure

Functional scenario sources **MUST** live under domain nouns that match the
product and code:

```text
tests/functional/<domain>/<subsection>/...
```

There is no durable `features/` wrapper and no transport-first ownership for
domain behavior. `transport` owns transport mechanics only (CLI/HTTP/MCP
process, routing, content types, protocol errors, and thin wiring). Domain
proofs live under their domain nouns even when the scenario enters through a
transport surface.

Intended domain tree:

```text
tests/functional/
  transport/
    cli/
    http/
    mcp/
  workers/
    script/
    inference/
      <provider>/
    mock/
  orchestration/
    javascript/
    petri/
  workstations/
    execution/
    cron/
    repeater/
    poller/
    watcher/
  work/
    submission/
    relationships/
    routing/
    recovery/
    visualization/
  sessions/
    lifecycle/
    controls/
    execution/
    restart/
  factory/
    definitions/
    packaged/
    current/
  provider_sessions/
    details/
    association/
  events/
    factory_events/
    response_events/
    replay/
  models/
  guards/
  resources/
  observability/
    logging/
    metrics/
    coverage/            # Make/CI functional coverage + viz contract smokes
  product/
    docs/
    dashboard/
  resilience/
    process/
    batch/
    platform/
  internal/
    support/             # only shared harness exception
    # other internal/* roots (e.g. restclient) are deletion-only debt
```
Examples:

```text
tests/functional/workers/script/execution_failure_test.go
tests/functional/orchestration/javascript/composition_run_test.go
tests/functional/sessions/controls/pause_resume_test.go
tests/functional/transport/cli/parameters/flag_parsing_test.go
```

Historical catch-alls such as `smoke`, `workflow`, and other non-domain roots
are not durable owners for new scenarios. Place new coverage under an approved
domain/subsection path instead.

### deprecated `runtime_api` layout

`tests/functional/runtime_api` is migration-only deletion debt and is not a
durable domain owner. Do not add new test files, helpers, or scenarios there.
Move each existing scenario to:

```text
tests/functional/<domain>/<subsection>/<behavior>_test.go
```

When a scenario truly spans domains and has no primary owner, place it under
the smallest primary domain and name the file for the secondary concern, or use
a local `cross/` subsection only when necessary. Do not create a generic parity
bucket. Shared functional process/edge helpers remain under the approved
`tests/functional/internal/support` boundary—the only shared harness
exception—and must not construct product services.

The enforcement check must record the current `runtime_api` file/scenario set
as exact deletion-only debt. A new path or scenario is blocking; removed or
moved entries are stale and must be deleted from the ledger. Renaming a test
inside `runtime_api` is not conformance—the destination must have a durable
domain/subsection owner.

## density

we have these as many as possible since they test system flows the best.

## Functional-test conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Functional tests construct the customer process through `root.BuildProcess`. | `cmd/functionalboundarycheck`, `pkgboundarycheck` functional process-edge and alternate-composition rules. | **Partial** | Prohibit every alternate graph constructor generically and require the root-built process when a scenario needs application construction. |
| Functional actions enter through `Process.Execute`, public HTTP, or public MCP. | Selected forbidden imports, calls, fields, and handwritten transport-construction rules. | **Partial** | Add a generic rule preventing direct service method calls in functional test files, except exact edge fakes and generated public clients. |
| Functional tests override external effects only through `edges.Edges`. | Functional process-edge scans and forbidden configuration-field checks. | **Partial** | Maintain a complete typed edge inventory and reject newly added internal service/factory overrides. |
| Functional tests do not import service implementations, Wire, Initializer, runtime scopes, replay projections, or internal orchestration. | `functionalboundarycheck` plus `pkgboundarycheck` test/import rules. | **Partial** | The forbidden import lists are explicit. Apply the generic service-owner rule and permit only root contracts, edges, and generated public clients. |
| Functional tests use `tests/functional/<domain>/<subsection>/...` under approved domain nouns and have a durable domain owner. | `make pkg-structure` rejects new shallow, catch-all, and unclassified Go sources and records existing nonconforming paths as exact deletion-only debt. | **Enforced** | Burn down the baseline by moving each source to its owning domain/subsection; `internal/support` is the only shared harness exception. |
| `tests/functional/runtime_api` receives no new files or scenarios and converges to deletion. | `make pkg-structure` records both the exact Go-file inventory and exact `Test*` scenario inventory. | **Enforced** | Move files and scenarios to domain/subsection owners and delete stale baseline entries. |
| `functional-boundary-check` is merge-blocking. | `make test-functional` and `make test-functional-coverage` both run it. Required CI Backend Functional Coverage invokes `make functional-test-viz`, which nests `test-functional-coverage`, so the lane cannot succeed without the boundary check. | **Enforced** | Keep the prerequisite on `test-functional-coverage`; do not let CI call bare `gocoveragecheck` for the functional lane. |

# integration tests

tests/integration/**.go

## logic

the integration tests are used to test the system behavior with the real filesystem and behavior and integrations and see it really works e2e.

## constraints
- these are full white box
- we try to keep these as low as possible
## density
- we keep one when possible for happy case on each integration path.

## Integration-test conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Integration tests are located in a declared integration-test root or owner-specific integration-test package. | `make test-integration` lists a few exact package paths. | **Partial** | Define and enforce the complete allowed layout; there is no generic `tests/integration/**` discovery today. |
| Integration tests may use real external adapters but do not create an alternate product composition graph. | Application-graph import restrictions catch direct Wire imports under `pkg`; coverage outside `pkg` is inconsistent. | **Partial** | Apply the single-composition-root and constructor-ownership rules to every integration test root. |
| White-box access is limited to the owning integration boundary and does not bypass service ownership casually. | Review standard only. | **Missing** | Define permitted dependency depth and require explicit metadata for any cross-owner white-box integration. |
| Integration tests remain few and focused on real adapter paths. | Manual review and exact Make target package list. | **Missing** | Density is a review policy; optionally require a manifest naming the integration path and owner rather than attempting a numeric lint. |

# stress tests

tests/stress/*.go

## logic
these are like integration tests but they're heavier since they're more expensive

we use these generally to validate system peformance at load

## Stress-test conformance checklist

| Invariant | Current enforcement | Status | Missing or required closure |
| --- | --- | --- | --- |
| Stress tests live under declared stress packages and are separated from the fast unit lane. | `make test-stress` uses configured stress packages; unit-lane package classification supplies partial separation. | **Partial** | Add a path/build-tag rule so an expensive test cannot enter the unit lane accidentally. |
| Stress tests use the canonical process or a narrowly owned component appropriate to the load target. | No stress-specific composition checker. | **Missing** | Apply root-process rules to customer-scale stress tests and owner-boundary rules to component stress tests. |
| Stress tests declare resource/time expectations and remain opt-in where infrastructure is expensive. | Make timeouts and selected long-test tags provide partial operational control. | **Partial** | Define required build tags or manifest metadata for expensive external infrastructure. |
| Stress tests do not introduce cross-service construction exceptions that production and functional tests forbid. | General package-boundary checks cover some imports under `pkg`, not a complete stress-test rule. | **Missing** | Scan every stress root with the same constructor, implementation-import, and alternate-composition policies. |
