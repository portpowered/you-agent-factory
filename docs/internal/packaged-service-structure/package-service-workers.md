Workers has completed most of its physical top-level package cleanup, but its production execution path is still shaped around persistent runtime-bound Worker instances and Workstation pools. That is not the desired end state.

The implementation-grade contract, dependency-direction, migration, and verification plan lives in [workers-stateless-execution-composition-plan.md](docs/temp/projects/packaged-service-structure/workers-stateless-execution-composition-plan.md).

The converged Workers service should be a stateless, request-scoped execution authority. Factory Runtime owns dispatch lifecycle, scheduling, concurrency, retries, and application of results. Workers executes one logical attempt using an immutable request, returns one normalized result, and retains no Factory Session or Workstation instance after the call completes.

## Recommended convergence

The final Workers root should expose one operation:

```go
package workers

type Service interface {
	Execute(
		context.Context,
		ExecuteRequest,
	) (ExecuteResult, error)
}
```

This replaces the current collection of runtime construction, pool lifecycle, executor, runner, direct invocation, and provider-shaped interfaces.

The service may use immutable process-scoped runner registries and request-scoped execution objects internally. It must not publish or retain runtime-bound Worker objects, Workstation pools, executable bindings, or Current Factory references.

## 1. Authority boundary

Workers should own:

- Executing one logical Worker attempt.
- Selecting a private runner from an already resolved execution specification.
- Agent, script, inference, and test/mock runner implementations.
- Prompt rendering from resolved inputs and policy.
- Invocation-time environment and Worktree preparation.
- Calling Providers and Models through their root services.
- Request-scoped progress reporting.
- Normalizing diagnostics, metrics, provider references, failures, and proposed output.
- Propagating context cancellation through subprocess, provider, and model calls.
- Cleaning up request-scoped processes, temporary files, Worktree leases, and other effects.

Workers should not own:

- Factory Session lifecycle or Current Factory selection — Factory Sessions.
- Persistent Worker or Workstation instances.
- Dispatch identity allocation, correlation, outbox state, scheduling, concurrency, or Workstation capacity — Factory Runtime.
- Work-level retry, transition routing, token movement, or result application — Factory Runtime.
- Authored Worker/Workstation configuration interpretation or persistence — Factory Definitions.
- Work Request admission, canonical Work materialization, or Work lineage policy — Work.
- Provider catalog/alias authority, provider throttling, or Provider Session persistence — Providers and Provider Sessions.
- Local model lifecycle, readiness, leases, or model capacity — Models.
- Canonical event durability, replay, or historical projection — Recordings.
- Hosted-source polling, filesystem watching, webhooks, or reconciliation loops — Automations.

## 2. What stateless means

Stateless does not mean an execution call has no temporary state. During `Execute`, Workers may hold:

- A running subprocess, provider call, or model call.
- Request-scoped prompt and environment materialization.
- Progress fragments for the active attempt.
- A temporary directory or Worktree lease.
- Cancellation and cleanup bookkeeping.
- Safe diagnostic material for the returned result.

That state ends with the call. Workers should not retain:

- A map of Worker instances keyed by Factory Session.
- A Workstation pool or route snapshot.
- A current-runtime pointer.
- Durable dispatch, retry, or outbox state.
- Persistent provider conversation objects.
- Runtime configuration or workflow context recovered from Factory Sessions.
- Executable bindings returned to Factory Runtime.

Persistent provider conversations are represented as opaque references. The underlying state remains owned by Provider Sessions or the external provider:

```go
type ProviderContinuationRef struct {
	ProviderSessionID string
	Provider          string
}
```

An execution request may carry a resume reference, and its result may return a continuation reference. Workers never owns or exposes the provider's underlying session object.

## 3. Converged request and result

A representative request is:

```go
type ExecuteRequest struct {
	FactorySessionID string
	DispatchID       string
	AttemptID        string

	Worker      ResolvedWorker
	Workstation ResolvedWorkstation
	Inputs      []WorkInput

	PreviousAttempts []AttemptSummary
	Resume            *ProviderContinuationRef
}
```

The request should be immutable and complete. Workers must not back-query Factory Sessions, Factory Definitions, or Factory Runtime to discover execution context.

`ResolvedWorker` and `ResolvedWorkstation` are detached execution values produced before execution. They contain only the policy Workers needs, such as:

- Runner identity.
- Provider/model selection.
- Model operation bindings.
- System and user prompt templates or already resolved prompt policy.
- Tool-execution policy.
- Output contract.
- Timeout.
- Working-directory and Worktree policy.
- Environment additions.
- Permission policy.

They must not contain service interfaces, executors, runner objects, provider implementations, filesystem implementations, or mutable definition lookups.

The result should describe execution facts, not Runtime mutations:

```go
type ExecuteResult struct {
	DispatchID string
	AttemptID  string
	Outcome    ExecutionOutcome

	Output      ProposedOutput
	Diagnostics *SafeDiagnostics
	Metrics     ExecutionMetrics

	Continuation *ProviderContinuationRef
}
```

`ProposedOutput` may include normalized content, feedback, classification, and proposed follow-up Work. Workers should not materialize canonical Work, create Runtime tokens, select output arcs, or mutate Factory state.

## 4. Cancellation, concurrency, retries, and idempotency

### Cancellation

Cancellation should flow through `context.Context`. The caller cancels the active `Execute` call; Workers propagates cancellation into Providers, Models, subprocesses, and cleanup.

A separate public `CancelDispatch` method is unnecessary unless an external execution protocol demonstrably cannot be controlled through the call context. Any such adapter-specific cancellation remains behind the runner or peer-service implementation.

### Concurrency and capacity

Factory Runtime owns semantic Workstation/resource admission and the number of concurrent dispatches it starts. Models and Providers own their own leases, throttles, and integration capacity.

Workers may retain an internal process-safety limiter, but it is not a customer-visible Workstation pool and has no start/stop lifecycle. Saturation from Models or Providers should return typed peer failures that Workers normalizes.

### Retries

`Execute` represents one logical Worker attempt. Factory Runtime owns Work-level retry and transition policy. Providers may own transport-level retry within one provider operation. Workers must not maintain durable retry state across calls.

### Idempotency

`DispatchID` and `AttemptID` provide correlation and may be forwarded as provider idempotency keys. Factory Runtime must prevent concurrent duplicate attempts and owns outbox redelivery policy. A stateless Workers service cannot promise process-durable exactly-once execution.

## 5. The live dual-stack problem

The repository currently contains two competing Workers paths:

1. `workers/wire.NewService` constructs the newer packaged `Service` backed by `runtime_assembly` and `workstations`.
2. Live Factory Runtime construction calls `workers/wire.NewRuntimeWithSelection`, which constructs the larger `RuntimeService` and builds concrete executor maps.

The second path remains the effective production authority. It:

- Back-queries Factory Sessions through `CurrentRuntimeResolver`.
- Reads the Current Runtime configuration and workflow context.
- Accepts command runners and provider overrides after construction.
- Builds maps of concrete `WorkerExecutor` values.
- Rebinds provider registries for session-specific command logging.
- Exposes direct model invocation alongside Runtime dispatch execution.
- Participates in Workstation pool construction.

The newer `Service` is not the final answer either. Its `BuildRuntime`, `StartWorkstationPool`, `DispatchWorkstation`, and related methods preserve the persistent-instance assumptions of the legacy implementation.

Both paths must converge on request-scoped `Execute` rather than attempting to move the legacy pool into a cleaner package.

## 6. Contracts to remove or relocate

The live Workers root contains many named interfaces and exported helper functions. The principal dispositions are:

| Current contract | Problem | Final disposition |
|---|---|---|
| `RuntimeService` | Persistent session-bound Workers runtime and parallel root | Delete |
| `BuildRuntime` | Constructs executable runtime bindings | Delete; resolve immutable values before `Execute` |
| `AssembledRuntimeBinding` | Publishes `WorkstationRequestExecutor` objects | Delete |
| `StartWorkstationPool` / `StopWorkstationPool` | Introduces Worker pool lifecycle | Delete |
| `WorkstationRoute` | Publishes live route/pool state | Delete; Runtime validates resolved Workstation routes |
| `DispatchWorkstation` | Pool-oriented synonym for execution | Replace with `Execute` |
| `WorkstationExecutionService` | Duplicate narrow root | Delete |
| `WorkstationPoolBoundary` | Runtime wrapper owns pool startup and asynchronous callbacks | Delete |
| `ModelInvoker` | Creates a direct Models-shaped execution path | Delete; use Models root internally during `Execute` |
| `InvocationExecutor` | Direct provider invocation bypasses canonical execution | Private runner implementation |
| `WorkerExecutor` | Executable object crosses into Factory Runtime | Private implementation |
| `WorkstationRequestExecutor` | Executable object crosses assembly boundaries | Private implementation |
| `Runner` | Private strategy contract published at Workers root | `internal/services/runners` |
| `Provider` | Duplicates Providers inference authority | Providers root |
| `ProviderRegistry` | Reverses provider catalog ownership | Providers root |
| `PromptTemplates` | Prompt implementation service at public root | Private prompting implementation; authored validation in Definitions |
| `FactoryWorktreePreparer` | Request execution implementation role | Private Worktree implementation |
| Command runner, PTY, filesystem, Git, and clock interfaces | External effects published as Workers peer contracts | `wire`, `edges`, platform, or private runner packages |
| Hosted poller clock/HTTP/path contracts | Automations effects published by Workers | Automations/platform |

The canonical inert root currently implements `InvokeModel` by returning “factory service runtime is not available.” A method that necessarily fails on the canonical root is evidence that it does not belong on the contract.

## 7. Broken value contracts

### Runtime tokens

Workers currently publishes `Token`, `Color`, `PlaceID`, visit history, and invocation arguments as Worker execution vocabulary. These are Factory Runtime orchestration structures.

Factory Runtime should project its token/marking state into detached `WorkInput` values before calling Workers. Workers may receive:

- Work identity and type.
- Canonical Work content.
- Tags and lineage references.
- Prior-attempt summaries.
- Invocation arguments already admitted for the Work.

Workers should not receive place IDs, markings, transition topology, or resource tokens.

### Canonical Work output

`WorkResult.RecordedOutputWork` currently contains canonical `work.FactoryWorkItem` values. Workers should return proposed output content. Work validates and materializes canonical Work; Runtime applies the resulting execution outcome.

### Runtime routing outcomes

Workers may report normalized execution or business facts such as completed, failed, canceled, accepted, rejected, or continued. It must not decide which Runtime arc fires or how tokens move. Runtime interprets the result against resolved Factory policy.

### Definition policy services

Workers currently receives `InvocationInterpolationService`, `WorkstationExecutionPolicyService`, and `DecisionEnvelopeService` from Factory Definitions.

Factory Definitions should interpret authored configuration and return an immutable execution projection. Workers consumes that projection. It should not call Definition policy services while executing a dispatch.

### Provider and Provider Session types

Provider alias/catalog resolution and prerequisite validation belong to Providers. Provider Session lifecycle and metadata belong to Provider Sessions. Workers should expose only the minimal opaque continuation reference and normalized execution facts its caller needs.

### Factory event payloads

Workers emits normalized execution facts. Contracts explicitly described as Factory event payloads belong to the canonical Factory event vocabulary or Recordings mapping, not the Workers root.

## 8. Revised private decomposition

The previously accepted `runtime_assembly`, `workstations`, and `runners` packages are useful migration waypoints, but they should not all survive as services.

### Runtime assembly

`runtime_assembly` exists because the old architecture creates persistent executor bindings for a Factory Runtime. That responsibility disappears.

Disposition:

- Move authored-policy interpretation to Factory Definitions.
- Move request projection to Factory Runtime.
- Move runner selection and request-scoped construction into Workers execution.
- Delete `runtime_assembly` after callers no longer request executor bindings.

Any remaining pure conversion helpers become ordinary private packages, not a service.

### Workstations

Workstations remain public Factory definition concepts and Runtime scheduling routes, but they should not be live Workers pools.

Workers retains only request-scoped Workstation execution mechanics:

- Prompt rendering.
- Environment and working-directory resolution.
- Worktree preparation.
- Output parsing.
- Permission enforcement.
- Safe diagnostics.

The current `workstations.Service` pool contract should be deleted. Its implementation packages should fold under ordinary Workers internal packages such as `execution`, `prompting`, `worktree`, `diagnostics`, and `policy`.

### Runners

`runners` is the one durable private subservice. It owns an immutable process-scoped registry and request-scoped invocation of strategies:

- Agent.
- Script.
- Inference.
- Test/mock.

The private runner service should expose one operation to the Workers root implementation:

```go
type Service interface {
	Execute(
		context.Context,
		ExecuteRequest,
	) (ExecuteResult, error)
}
```

Concrete runner instances are immutable process-scoped adapters or request-scoped values. They are not Factory Session Worker instances.

## 9. Hosted and poller Workers

The current runtime builder detects poller Worker types and assigns `NoopExecutor`, which returns success without performing hosted work. This should not be replaced by a persistent hosted Workers runner.

A poller, filesystem watcher, webhook listener, or reconciliation loop is an Automation hosted source:

```text
External system
    ↓ poll, webhook, or watch
Automations
    ↓ Work Request
Work
    ↓ admitted Work
Factory Runtime
    ↓ one execution attempt
Workers
```

Disposition:

- Move hosted-source lifecycle and polling effects to Automations.
- Have Automations submit Work through the Work service.
- Remove poller Worker types from Workers execution routing.
- Delete the poller `NoopExecutor` fallback.
- Reframe IMP-WRK-07 as an ownership cut/removal, not implementation of a hosted Worker runtime.

## 10. Remaining runner conversion work

### Agent

A private Agent Runner exists, but legacy agent executor, conductor, provider construction, and direct invocation paths remain alongside it. Converge all agent attempts on the private runner and Providers root.

### Script

A private Script Runner exists, but legacy script factories, Workstation executors, command-runner decorators, and direct construction helpers remain active. The private runner should execute one request and own subprocess cleanup.

### Inference

A private Inference Runner exists, but direct model invocation and model-recording decorators remain in `RuntimeService`. Converge model attempts on the private runner and Models root.

### Mock

Mock behavior exists in the runner tree, but root parsing helpers and command-runner wrapping create a parallel construction path. Treat mock selection as an execution dependency chosen at composition time; keep parsing and implementation private or in a Workers-owned CLI adapter.

## 11. Logic still misplaced in transports and mappings

### CLI mock-worker loading

The top-level run transport selects, loads, and passes `MockWorkersConfig` through root loader contracts. CLI flag interpretation belongs in CLI. JSON parsing, validation, and conversion into a test runner selection should live in a Workers-owned adapter or private configuration package. The Workers root should not export loader constructors and parsers.

### Dashboard projection fallbacks

`pkg/transports/cli/dashboard` reconstructs completed and failed Work lanes by examining Worker outcomes, dispatch inputs, terminal Work, and output Work. Formatting is presentation behavior; reconstructing canonical world state is Recordings or Factory Visualization projection policy.

### Worker diagnostics mapping

`pkg/transports/mapping/workerdiagnostics` is legitimate when it only converts canonical safe diagnostics to and from generated OpenAPI shapes. Redaction, safety classification, event decoding, and failure normalization remain service behavior.

### Factory configuration mapping

Factory configuration mappers call Workers runner normalization and enforce hosted-worker compatibility. Representation conversion may remain mapping. Runner capability policy and hosted-source ownership validation belong to Factory Definitions, Providers, Workers preflight, and Automations as appropriate.

### Runtime and CLI result interpretation

Factory Runtime should consume one stable `ExecuteResult` and apply Runtime routing policy. CLI should consume completed Work or Factory Session results. Neither should inspect runner implementations, provider output formats, or Workers-internal diagnostics to determine success.

## 12. Final tree

```text
pkg/services/workers/
├── service.go
├── execution.go
├── diagnostics.go
├── progress.go
├── errors.go
│
├── wire/
│   └── wire.go
│
├── transports/
│   └── cli/                 # only if mock configuration remains service-owned
│
└── internal/
    ├── service/
    │   ├── service.go
    │   └── execute.go
    │
    ├── services/
    │   └── runners/
    │       ├── service.go
    │       ├── wire/
    │       └── internal/
    │           ├── service/
    │           ├── agent/
    │           ├── script/
    │           ├── inference/
    │           ├── process/
    │           └── testing/
    │
    ├── execution/
    ├── prompting/
    ├── worktree/
    ├── diagnostics/
    ├── policy/
    └── testkit/
```

The public root contains only:

- The singular `Service` interface.
- Detached execution request/result/error/value contracts.
- `wire`.
- Genuine Workers-owned transports, if any.

It contains no exported constructors, clone helpers, normalizers, command/process adapters, executable interfaces, runtime builders, pools, or pool implementations.

## 13. Revised migration plan

### Phase 1: freeze the stateless contract

1. Define `ExecuteRequest`, `ExecuteResult`, typed failures, diagnostics, progress, and opaque provider continuation references.
2. Add the singular `Service.Execute` contract.
3. Characterize current agent, script, inference, mock, cancellation, timeout, progress, and failure behavior against that contract.
4. Add boundary tests prohibiting Runtime tokens, executable objects, Factory Session gateways, Definition policy interfaces, Provider implementations, and Models request/result types from the Workers root.

### Phase 2: establish the canonical execution path

5. Implement `Service.Execute` by adapting the existing canonical execution path without creating pools or persistent Worker instances.
6. Make Factory Runtime project detached Work inputs and resolved execution policy into `ExecuteRequest`.
7. Make Factory Runtime call `Execute` under its own bounded scheduling and dispatch lifecycle.
8. Propagate cancellation exclusively through the execution context.
9. Return normalized proposed output; have Work materialize canonical Work and Runtime apply routing/mutations.

### Phase 3: remove session and runtime coupling

10. Remove `CurrentRuntimeResolver` and every Workers back-query into Factory Sessions.
11. Pass Factory Session ID, definition version, execution policy, and provider continuation references explicitly in requests.
12. Remove workflow-context recovery from hosted Runtime objects.
13. Remove `RuntimeService`, `NewRuntimeWithSelection`, and post-construction command-runner/progress decorators.

### Phase 4: delete pool and assembly architecture

14. Stop Factory Runtime from building executor maps.
15. Delete `BuildRuntimeExecutors` and concrete executor bindings crossing into Runtime.
16. Delete `BuildRuntime`, `AssembledRuntimeBinding`, and `runtime_assembly` after its pure helpers are relocated.
17. Delete `StartWorkstationPool`, `StopWorkstationPool`, `WorkstationRoute`, and the `workstations.Service` pool contract.
18. Delete `WorkstationExecutionService`, `WorkstationPoolBoundary`, and Runtime-owned async pool callback wiring.
19. Move Workstation execution mechanics into request-scoped internal packages.

### Phase 5: converge runner implementations

20. Move `Runner` and its registry entirely under `internal/services/runners`.
21. Converge agent execution on the private Agent Runner and Providers root.
22. Converge script execution on the private Script Runner and injected process effects.
23. Converge inference execution on the private Inference Runner and Models root.
24. Converge mock behavior on the private test runner without command-runner wrapping as a second production path.
25. Delete legacy agent/script/inference executors, factories, decorators, and direct invocation façades after parity proof.

### Phase 6: correct adjacent ownership

26. Move provider registry and provider identity authority to Providers root contracts.
27. Replace Factory Definitions policy service injection with one resolved immutable execution projection.
28. Move hosted pollers and their HTTP/clock/secret/runtime-path effects to Automations.
29. Remove poller Worker routing and the `NoopExecutor` fallback.
30. Move canonical Work materialization out of Worker results and into Work.
31. Move Factory event payload construction into Runtime/Recordings canonical vocabulary.

### Phase 7: close root and package debt

32. Move command runner, PTY, filesystem, Git, clock, Worktree, prompt, diagnostics, and failure helpers behind `wire`, platform edges, or private packages.
33. Remove root exported constructors, parsers, clone helpers, normalizers, and compatibility aliases.
34. Collapse `runners` to `service.go` plus `wire/internal/transports` as applicable.
35. Delete obsolete `runtime_assembly` and `workstations` service packages.
36. Lower package-structure, ownership, package-target, and coverage baselines.
37. Add functional proof that the process constructs Workers inertly and each `Execute` call is isolated, cancelable, and leaves no session-bound Worker state.

## 14. Completion invariants

The Workers decomposition is complete only when:

- `workers.Service` contains one request-scoped `Execute` operation.
- No Factory Runtime or Factory Sessions caller receives an executor, runner, binding, or pool object.
- Workers does not import or back-query Factory Sessions.
- Workers retains no Factory Session, Current Factory, Workstation pool, or persistent Worker instance state.
- Runtime owns semantic concurrency, dispatch correlation, retries, cancellation initiation, and result application.
- Providers and Models own their persistent sessions, leases, throttles, and runtime capacity.
- Provider continuation is represented by opaque references.
- Runtime tokens and place IDs do not appear in Workers root contracts.
- Workers returns proposed output rather than canonical Work or Runtime mutations.
- Hosted polling is owned by Automations rather than a Workers runner.
- Agent, script, inference, and mock execution share one private runner path.
- Cancellation and cleanup are proven for subprocess, provider, and model execution.
- Construction through `root.BuildProcess` remains inert until Runtime initiates an execution call.

The highest-value first implementation slice is phases 1 through 3: establish `Execute`, move Runtime onto it, and eliminate the Factory Sessions back-query and `RuntimeService`. Pool and package deletion should follow once that stateless path is behaviorally proven.
