# Runtime execution contract map

This map is the maintainer contract for provider-backed execution shared by
Petri workstation dispatch and JavaScript `agent.run` dispatch. It describes the
current implementation; it does not authorize package movement or change
runtime behavior.

## Classification rules

- **Shared** means both execution modes must preserve the same semantic fact at
  the named canonical boundary. Their orchestration-internal records and control
  flow may differ.
- **Intentionally different** means the modes have different orchestration
  owners or sequencing by design. A later convergence must preserve the stated
  rationale rather than force identical internals.
- **Missing** means one or both modes do not yet carry the fact through the
  canonical boundary. The bounded follow-up names the permitted next step; the
  absence is not an invitation to create another provider path.

“Public projection” below means the customer-facing Factory Session Dispatch,
Provider Session, artifact, failure, and replay facts. It does not mean that
Petri transition data and JavaScript script/checkpoint data must match.

## Contract matrix

| Behavior | Canonical owner | Petri workstation implementation | JavaScript `agent.run` implementation | Classification | Invariant or rationale | Evidence or bounded follow-up |
| --- | --- | --- | --- | --- | --- | --- |
| Provider request | `factorycontracts.ProviderInferenceRequest` is the provider-bound value; `pkg/workers.Provider` is the execution seam. | `pkg/workers/executor` renders a `WorkstationExecutionRequest`, derives the provider request, and invokes the configured provider without mutating `WorkDispatch`. | Live children use `pkg/factory/sessions/execution/livechild.ProviderChildExecutor`, which derives the same `ProviderInferenceRequest` and calls `workers.Provider.Infer`. Fake children remain in-process and do not claim to be provider calls. | **Shared** for the provider seam; request population is partly **missing** in the JavaScript adapter. | A production provider call accepts a detached provider-owned request containing dispatch correlation, rendered input, model/schema/runner data, and session context. It must not accept Petri tokens or JavaScript VM state as its contract. | `pkg/factory/contracts/work_dispatch.go`; `pkg/workers/provider_compat.go`; `pkg/factory/sessions/execution/livechild/provider.go`. Follow-up may fill semantically applicable request fields in that adapter only; it must not introduce a second provider interface. |
| Provider result | `factorycontracts.InferenceResponse` and normalized provider errors in `pkg/workers/provider`; orchestration translates those into its result record. | Provider content, Provider Session metadata, safe diagnostics, and normalized failures become `factorycontracts.WorkResult`; the factory consumes that result on a later tick. | Live child content becomes a `ChildExecutionResult` plus a terminal `ChildDispatchRecord`; provider errors become a safe `FailureDetail`. | **Shared** at provider response/error semantics; **intentionally different** after translation. | Provider content, stable session metadata, and normalized failure facts survive translation. Petri may route tokens/outcomes; JavaScript returns a script value and child record. Those result containers are not required to match. | `pkg/factory/contracts/work_execution.go`; `pkg/workers/provider/provider_behavior.go`; `pkg/factory/sessions/execution/livechild/provider.go`. |
| Provider Session identity | `factorycontracts.ProviderSessionMetadata {provider, kind, id}` and the public `ProviderSessionRef` shape. | `WorkResult.ProviderSession` is recorded with the workstation response and dispatch lifecycle facts. Canonical provider naming is applied at the provider boundary. | A terminal live-child record carries provider plus session id; projection emits `{provider, kind: "session_id", id}`. Fake execution uses explicit fake identity only as deterministic fixture data. | **Shared**. | When present, identity is provider + kind + id. Public readers must not infer identity from raw output, and absence remains absence. Provider-specific transcript or VM fields are not part of the shared shape. | `pkg/factory/contracts/work_execution.go`; `pkg/factory/sessions/execution/projection_consistency.go`; `pkg/factory/sessions/execution/service.go`; `pkg/workers/provider/inference_progress.go`. |
| Cancellation | The caller context is the provider-call cancellation boundary; the active orchestration owner decides when to cancel and what durable lifecycle fact to emit. | Factory pause/stop and worker-pool lifecycle control the workstation context; process/provider adapters propagate cancellation and the event loop observes the resulting completion/interruption. | Factory Session lifecycle and JavaScript runtime budgets control script/child contexts; `ProviderChildExecutor.Execute` checks and passes the context to `Infer`. Checkpoints can support session resume without becoming provider cancellation state. | **Intentionally different** orchestration; **shared** context propagation. | Provider implementations must honor the supplied context. Petri event-loop control and JavaScript script/checkpoint control retain ownership of cancellation timing and follow-up sequencing. Cancellation must not be encoded as provider-global mutable state. | `pkg/workers/interfaces.go`; `pkg/workers/process/doc.go`; `pkg/factory/sessions/execution/livechild/provider.go`; `pkg/factory/sessions/execution/control.go`. |
| Retry metadata and sequencing | Provider failure normalization belongs to `pkg/workers/provider`; retry policy belongs to the active orchestration owner; public usage/failure projections own replay-safe facts. | `WorkResult.Metrics.RetryCount` and `FailureMetadata` preserve the attempt outcome; the Petri runtime decides retry/continue/terminal routing on ticks and emits dispatch lifecycle retry facts. | Child summaries currently project `Attempt: 1`; JavaScript runtime/pipeline policy controls whether and when script work is re-executed or resumed. Provider-child multi-attempt metadata is not yet represented. | **Intentionally different** sequencing; JavaScript provider-attempt metadata is **missing**. | Do not centralize retry scheduling in the provider seam. A provider adapter may normalize failures and report attempts, but Petri transitions and JavaScript script/checkpoint policy decide retries independently. | `pkg/factory/contracts/work_execution.go`; `pkg/workers/provider/provider_behavior.go`; `pkg/factory/events/event_history_dispatch_lifecycle.go`; `pkg/factory/sessions/execution/service.go`. Bounded follow-up: carry attempt/retry facts through child records and public usage only when live-child retry policy is defined. |
| Safe diagnostics | `factorycontracts.WorkDiagnostics`, `factorycontracts.FailureDetail`, and canonical Factory event/public projection schemas. | Workstation results carry hashed/redacted prompt and invocation metadata, provider/command metadata, normalized failure detail, and metrics into event history. | Child records expose digests, selected execution metadata, and normalized failure detail; the raw error is retained only as an execution diagnostic, not promoted as Provider Session identity. | **Shared** safety boundary; JavaScript detail coverage is **missing**. | Public/replayable diagnostics contain safe structured facts and stable failure reason/message. They must not expose raw prompts, secrets, environment values, provider wire payloads, stack/VM internals, or unbounded stderr. | `pkg/factory/contracts/work_execution.go`; `pkg/factory/runtime/factory_event_history_test.go`; `pkg/orchestrators/javascript/runtime/records.go`; `pkg/factory/sessions/execution/livechild/provider.go`. Follow-up is limited to mapping already-sanitized provider diagnostics into child/public records. |
| Artifact references | Factory Session artifact projection and canonical artifact URI/reference format. | Completed workstation results and emitted work/artifact events associate output artifacts and lineage with the dispatch. | A child reserves a Factory Session artifact URI before execution; completed records project its artifact id as a dispatch output and an inspectable `CHILD_RESULT` artifact. | **Shared** association; creation timing is **intentionally different**. | Public artifacts have stable session-scoped identity, retrieval reference, dispatch association, kind/visibility, and safe metadata. A reserved JavaScript artifact is not publicly completed until the child completes. | `pkg/factory/events/event_history_dispatch_lifecycle.go`; `pkg/factory/sessions/execution/livechild/provider.go`; `pkg/factory/sessions/execution/service.go`; `pkg/orchestrators/javascript/result`. |
| Event append | Ordered Factory event history is the durable public owner; execution-mode records are inputs to that boundary. | The event loop records dispatch queued/started/completed/failed/interrupted/reconciled and workstation response facts through `FactoryEventHistory`. Worker output re-enters the loop rather than mutating canonical world state. | The runtime appends ordered typed records while running, updates the session execution projection, and Factory Session lifecycle/event surfaces publish canonical session/dispatch facts. Script records include phases, logs, artifacts, checkpoints, budgets, and child dispatches. | **Shared** event-first invariant; record production is **intentionally different**. | Every public state change derives from ordered append-only facts. A worker or child executor must not directly mutate canonical Factory world/session projections. Petri tick ordering and JavaScript record/checkpoint ordering remain distinct. | `pkg/factory/events/event_history.go`; `pkg/factory/events/event_history_dispatch_lifecycle.go`; `pkg/factory/sessions/execution/runtime_service.go`; `pkg/orchestrators/javascript/runtime/records.go`. |
| Dispatch projection | Canonical Factory Session Dispatch schemas and projection reducers; mode-specific extensions remain namespaced. | Factory world dispatch completion projects dispatch identity, workstation/result outcome, Provider Session identity, and safe diagnostics; inference-attempt projections carry provider/model facts. It does not expose the JavaScript lifecycle-status contract. | `ChildDispatchRecord` projects dispatch identity, lifecycle status/attempt, runner/model/provider, failure, Provider Session refs, and output artifact ids, plus the JavaScript-specific task/execution-mode projection. It does not expose safe provider response metadata. | **Shared** for dispatch identity, provider/model correlation, and Provider Session identity; compatible cross-mode lifecycle status/attempt and safe provider metadata are **missing**. Orchestration extensions are **intentionally different**. | Tests may compare only facts with compatible public semantics. Petri `WorkResult.Outcome` must not be relabeled as JavaScript `DispatchStatus`, and absent JavaScript diagnostics must not be manufactured by a fixture. Consumers must not compare Petri transition/work lineage with JavaScript task, execution mode, checkpoint, or script phase as though they were the same orchestration state. | `pkg/factory/runtime/factory_event_history_test.go` and `pkg/factory/sessions/execution/fixtures/runtime_live_child_test.go` exercise real injected provider paths and lock the currently shared facts through replay. A later explicitly authorized public-contract slice must define lifecycle and safe-metadata compatibility before cross-mode assertions include them. |
| Replay | Canonical ordered Factory events and Factory Session projection reducers. | `pkg/factory/projections` reconstructs world/session/dispatch state from dispatch lifecycle and workstation response events, including safe diagnostics and Provider Session facts. | Durable session events reconstruct lifecycle/result reads, while persisted ordered runtime records reconstruct child Dispatch, Provider Session, artifact, checkpoint, and progress projections for resume/inspection. | **Shared** observable invariant; replay mechanisms are **intentionally different**. | For each mode, replay of the same ordered facts is idempotent and reproduces its live public facts. Replay does not imply that fields classified as missing have a cross-mode equivalent, and it need not turn JavaScript records into Petri transitions or vice versa. | `pkg/factory/runtime/factory_event_history_test.go`; `pkg/factory/sessions/execution/fixtures/runtime_live_child_test.go`; `pkg/factory/sessions/execution.ReplaySessionProjection`; `pkg/factory/sessions/execution/runtimepersist`. The focused tests cover real provider-backed live/replay projections without claiming equality for missing fields. |

## Orchestration boundary

Petri workstation execution is driven by transition eligibility and deterministic
ticks. A worker result re-enters the event loop, where outcome arcs, guards,
retry policy, and later dispatches are selected. JavaScript execution is driven
by script order and host primitives such as `agent.run`, parallel/pipeline
composition, budgets, and checkpoints. Its orchestration owner decides child
order, concurrency, cancellation, retry/resume, and returned script values.

Those control flows are intentionally different. Convergence applies only below
them at the provider request/result seam and above them at canonical Factory
Session events and public projections. It must not make a provider schedule
Petri transitions, make the Petri loop interpret JavaScript checkpoints, or make
JavaScript orchestration synthesize Petri markings.

## Next live-provider slice

The only approved provider seam for the next slice is
`pkg/workers.Provider.Infer(context.Context, interfaces.ProviderInferenceRequest)`
(the compatibility alias of `pkg/workers/provider.Provider`). The slice may
improve the JavaScript live-child adapter in
`pkg/factory/sessions/execution/livechild` and shared request/result mapping around
that seam. It must:

1. preserve injected deterministic fake-child execution for tests, preview, and
   replay/resume fixtures;
2. reuse provider selection, normalization, Provider Session canonicalization,
   cancellation, and safe diagnostics from `pkg/workers`;
3. keep JavaScript retry and cancellation sequencing in the JavaScript/Factory
   Session orchestration owner; and
4. avoid a new provider client, provider interface, subprocess path, or direct
   SDK integration under `pkg/orchestrators/javascript` or
   `pkg/factory/sessions/execution`.

If the seam lacks a required semantic field, extend the shared request/result
contract and both adapters deliberately; do not tunnel provider payloads through
JavaScript output maps or Petri tokens.

## Approved composition destination

The approved future startup flow is:

`cmd/factory -> pkg/root -> pkg/wire -> pkg/initializer`

These names describe destination ownership, not the current package topology:

| Destination | Responsibility | Dependency rule |
| --- | --- | --- |
| `cmd/factory` | Remain the thin executable entrypoint. Parse process-boundary inputs and hand control to `pkg/root`; do not assemble the application graph or construct transport-specific services. | May import `pkg/root`; domain packages must never import `cmd/factory`. |
| `pkg/root` | Normalize process arguments and environment input, then select the process mode and top-level behavior, such as API hosting, local CLI execution, or sidecar startup. | May request an application graph from `pkg/wire` and pass it to `pkg/initializer`; it must not construct domain dependencies or execute component lifecycle, and domain packages must not import `pkg/root`. |
| `pkg/wire` | Construct one explicit application dependency graph from normalized configuration and injected filesystem, environment, time, process, persistence, runtime, and provider dependencies. It owns construction, not domain policy or process startup. | May import inward domain/platform owners to assemble them. It must not be imported by domain packages or transport packages, and it must not start transports. |
| `pkg/initializer` | Start, stop, cancel, join, and unwind API, CLI, MCP, sidecars, and other already-built process adapters. | Must receive constructed core collaborators; it must not lazily construct domain services or rebuild Factory Session runtimes, providers, persistence, model hosts, or other core dependencies. |

Current composition remains under `pkg/wire`, `pkg/runtimehost`, broad
`pkg/service` construction paths, and the existing
`pkg/initializer` adapters. Existing architecture notes call the future graph
builder `pkg/inject`; for the foundation batch, that provisional name is
superseded by the approved `pkg/wire` destination above. This story does not
rename or move any implementation.

## Approved collapsed package families

The following are future ownership families. They are not aliases for every
similarly named current package, and existing packages remain canonical until a
separately reviewed migration moves behavior and callers.

| Collapsed target family | Ownership purpose | Current implementations and migration treatment |
| --- | --- | --- |
| `pkg/transports` | HTTP, CLI, MCP, generated transport contracts and clients, and boundary mapping. Transport code translates requests and responses and invokes injected application/domain services; it does not own domain policy or canonical runtime state. | The migration-only roots are `pkg/api`, `pkg/apisurface`, `pkg/cli`, and `pkg/mcp` under **Batch 006 — Transport family move**. Remove each exception when its behavior and callers have moved to `pkg/transports`; the target must not become a parallel transport implementation. |
| `pkg/work` | Work and Work Request content, query/selection, graph/lineage, pure invocation input and return policy, materialization, and cron/time-work concepts. Factory Session orchestration, worker/provider execution, and generic platform clocks are excluded. | Work content, materialization, query/selection, graph/lineage, cron/time-work behavior, and pure invocation policy live in `pkg/work`. Stateful invocation orchestration lives in `pkg/factory/sessions/invocation`; provider-neutral inference binding/output shaping and worker permission policy live under `pkg/workers`. |
| `pkg/platform` | Cross-cutting logging, replay artifact filesystem infrastructure, metrics, cursor storage, and non-domain clocks. It may implement domain-owned interfaces, but it does not choose Factory, Factory Session, worker, model, scheduling, or Work policy. | Logging, policy-free replay reads and atomic replacement, runtime metrics, cursor mechanics, and generic clocks are canonical under `pkg/platform/logging`, `pkg/platform/replay`, `pkg/platform/metrics`, `pkg/platform/cursors`, and `pkg/platform/clock`; Factory replay policy is canonical under `pkg/factory/replay`, `pkg/config` remains a durable family, and domain time-work belongs in `pkg/work`. |

The dependency direction remains edge-to-domain: `pkg/root`, `pkg/wire`,
`pkg/initializer`, and `pkg/transports` may depend inward on application,
domain, and platform contracts. Domain packages must not import `pkg/root`,
`pkg/wire`, or transport packages. `pkg/platform` may implement domain-owned
interfaces but must not import orchestration owners merely to choose domain
policy.

## Foundation batch policy prerequisites

Before any destination family is created, the foundation batch must update the
package-boundary policy deliberately. `pkg/root`, `pkg/wire`,
`pkg/transports`, `pkg/work`, and `pkg/platform` each require a product-owned
package-family allowlist entry naming its owner and rationale. `pkg/initializer`
is already approved and remains the startup owner. This story changes neither
the allowlist nor the production package tree.

The required policy surfaces for that later batch are:

1. the normative package-boundary rules in
   `docs/internal/standards/code/general-backend-standards.md`, including the
   approved-family list and dependency direction;
2. `cmd/pkgboundarycheck/main.go`, where durable
   `approvedProductPackageFamilies`, metadata-bearing migration exceptions, and
   generated-code exceptions are separate executable policy classes;
3. `cmd/pkgboundarycheck/main_test.go`, which locks allowlist validation,
   generated-code exceptions, blocking diagnostics, deterministic reporting,
   and the `make pkg-boundary`/lint entrypoints; and
4. `Makefile`'s `pkg-boundary` target and the `lint` target list, which must stay
   blocking during and after migration.

The foundation batch must update documentation, guard policy, diagnostics, and
tests together before adding directories. It must not weaken generated-code or
removed migration-shim checks, and it must prove the five new product-owned
families are accepted while an unknown root family still fails with actionable
diagnostics.
