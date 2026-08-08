# Worker Session Convergence

Status: the route is landed and the ACP gate is green; four durable-session
projection gaps remain (§6). The standalone `you run script.js` composition is
not converted.
Date: 2026-08-08
Branch: `worker-session-convergence`
Gate: `TestJavaScriptFactoryChildrenAreVisibleAsWorkers`
(`tests/functional/chat_sessions/root_composition/`), **green**

A JavaScript workflow's `agent.run` children and a Petri Factory's Workers are
the same thing: one provider execution with a prompt, a model, and an output.
They reached the provider through two separate stacks, and only one of them was
visible to a client. Now both enter Worker Sessions through one operation.

## 1. What landed

| Change | Where |
| --- | --- |
| `Start` → `InvokeSession`, `+Retry`, `+Attempts` | `worker_sessions/contracts.go` |
| The single retry loop | `worker_sessions/internal/service/control.go` |
| `ProviderInvocationRoute` + pool binding | `workers/workstation_contracts.go`, `workstation_pool_boundary_impl.go` |
| Direct-inference executor | `workers/internal/services/workstations/executor/provider_invocation.go` |
| `NewProviderInvocationExecutor` | `workers/wire/runtime_bridge.go` |
| `ProviderInvocationExecutorFactory` port | `factory_runtime/composition_contracts.go` |
| `InvokeWorker` root operation | `factory_runtime/invoke_worker_contract.go`, `internal/services/orchestration/runtime/invoke_worker.go` |
| Child adapter + late-bound invoker | `factory_sessions/internal/execution/child_worker_executor.go` |
| **Deleted**: `livechild/`, its retry loop, its QUEUED/RUNNING appends | `factory_sessions/internal/execution/livechild/` |
| **Deleted**: the `liveChildInvocation` edge through Wire | `pkg/wire/session_runtime_providers.go` |

Three properties worth not re-litigating:

- **No executor in the request.** `Execution.WorkstationName` is a route into
  the binding snapshot Workers already assembled. That is the whole of a
  caller's say in what runs, and it is what lets one operation serve both
  orchestrators without a second injection path.
- **Petri behaviour is unchanged.** `Retry`'s zero value is one attempt.
  Retryability was always *classified* on both paths; only `livechild` ever
  looped.
- **A retry is not a new Worker.** Attempts reuse the session identity and the
  open publication window, minting `.../attempt/N` as resume mints
  `.../resume/N`, so one Worker stays one tool call across its attempts.

## 2. The path a JavaScript child now takes

```
workflow agent.run
  -> childWorkerExecutor                  (factory_sessions/internal/execution)
       resolves the runner caller-side
  -> factoryruntime.Service.InvokeWorker  (the session's own runtime)
       Reserve -> RecordDispatchWorkerSessionAssociation -> InvokeSession
  -> workersessions.InvokeSession         (identity, window, controls, retry)
  -> WorkstationPoolBoundary              (admission, capacity, cancel)
       route: workers.ProviderInvocationRoute
  -> ProviderInvocationExecutor           (provider.Infer, no definition lookup)
```

The association recorded on the runtime's own `recordings.RuntimeLedger` is
what opens the ACP `tool_call`; the transport recognises no other event.

## 3. Wrong turns, recorded so they are not repeated

### 3.1 "JavaScript sessions compose no Workers runtime" — false

Disproved by instrumenting `responsebridge`: a JavaScript session already has a
live Factory Event stream with a real `APIFactory` behind it. `ensureEventHistory`
sits in the same constructor that calls `configureRuntimeDispatch`, so a
JavaScript session **already had** `cfg.workerSessions`, a pool boundary, and a
ledger. They were built and never reached the child path.

### 3.2 Emitting the association from the read model — wrong stream

`BuildCanonicalRuntimeSessionEvents` feeds the durable-session read model.
`responsebridge` subscribes through `SubscribeFactoryEventsForSession`, which
reads the session runtime's ledger. Adding `DISPATCH_WORKER_SESSION_ASSOC` to
the synthesis left the gate at 0 tool calls and was reverted.

### 3.3 `Service.BuildRuntime` never yields an executor

The first attempt obtained the provider-invocation executor by asking
`workers.Service.BuildRuntime` for a worker-role binding. That can never work:
the binding assembler at `runtime_composition.go:210` returns `RoleName`,
`RoleKind`, and `RunnerSelection` and **never sets `Executor`**. The lookup
returned nil unconditionally, and the runner-ID threading done to "fix" it was
irrelevant. `NewProviderInvocationExecutor` had zero production callers; the
missing piece was a construction, not a lookup.

### 3.4 Two claims made and retracted

- "`cfg.workerService` is not a `workers.Service`" — false; `workers.RuntimeService`
  embeds it, so the assertion succeeds.
- "A debug probe printed nothing, so the code never ran" — false; the ACP
  harness captures the served command's stderr into a discarded buffer. Probe
  to a file, not to stderr.

## 4. Two defects the convergence exposed

Both were real bugs in Workers, not in the new code, and both are now pinned by
red-first tests.

- **The pool rejected the binding.** `routesFromBindings` rejects any binding
  whose `RoleKind != RuntimeBuildRoleKindWorkstation`, and rejects the *whole*
  start request — so a worker-kind provider-invocation binding made every route
  in the session unusable. Every pool route is a workstation route; what a
  provider-invocation Worker lacks is an authored definition, and that lives in
  the executor. Pinned by
  `TestWorkstationPoolBoundaryBindsProviderInvocationAsWorkstationRole`.
- **The pool blanked the runner.** `Pool.dispatch` overwrote
  `execution.RunnerID` from the route's `RunnerSelection` unconditionally. A
  route that pins no runner blanked the only selection that existed, and the
  provider rejected every such Worker as `permanent_bad_request`. It now
  overwrites only when the route actually resolved one. Pinned by
  `TestPoolDispatchKeepsRequestRunnerWhenRouteResolvesNone`.

## 5. The one composition not converted

`you run script.js` (`factory_sessions/internal/executionopening`) opens a
JavaScript execution service **directly**. It builds no Factory Runtime, so it
has no Worker Sessions service and no ledger, and its children cannot be
Workers. It keeps a `workers.InvocationExecutor` and a slim
`directChildExecutor`.

That is not the second injection path the convergence removed. Nothing selects
between them at dispatch time: the two executors are chosen once, by which
composition is being built — `childWorkerExecutor` where a runtime exists,
`directChildExecutor` where none does. What the direct executor no longer
carries is the duplicated retry loop, which was the actual defect.

Converting it means giving that composition a Factory Runtime. That is a
separate project, and until it happens `you run script.js` children remain
invisible to a client: there is no Factory whose tool call they could be
content inside.

## 6. What is still red, and why

Nine functional cells still fail. None of them are about the route -- children
reach the provider, run concurrently, and open tool calls. All four are facts
the durable-session projection used to get from the deleted executor and now
gets from nobody.

| Failing cells | Missing fact | Where it has to come from |
| --- | --- | --- |
| `TestJavaScriptParallelPartialFailureUsesDocumentedPolicy` | `failureDetail`, `failureClassification`, `retryable` on a failed child record | `InvokeWorkerResult` narrows to `Diagnostic` alone. It needs the closed-vocabulary `FailureDetail.Reason` and the retryable flag as well -- both are safe to cross; `InvocationResult.Message` still must not. |
| `TestJavaScriptChildProgressPublishesCanonicalResponseEvents`, `TestJavaScriptTerminalResultFollowsFinalResponseEvent` | child provider progress on the Factory Session response-event surface | Progress now flows Workers -> `ProviderSessionObservationPublisher` -> the runtime progress stream. It no longer reaches the durable session's own response-event store, which the JavaScript service used to write through `sessionProgressPublisher`. |
| `TestJavaScriptInterruptedSessionResumes...`, `TestJavaScriptResumeRestoresCheckpoint...`, `TestJavaScriptDurabilityDoesNotPersistSnapshotsByDefault`, `TestFactorySessionResumeDoesNotRepeatCompletedDispatch`, `TestFactorySessionHistoryIsNotPersistedByDefault`, `TestAPIPartialResultIsAvailableBeforeTerminalCompletion` | workflow cancellation reaching the running child | `InvokeWorker` deliberately calls `InvokeSession` under `context.WithoutCancel`, because Worker Sessions owns cancellation. Nothing yet translates a canceled workflow into `Cancel` on that Worker Session. |
| `TestJavaScriptMockWorkersReplaceOnlyNamedChildren` | mock-worker substitution for a named child | The provider-invocation route is built from the session command runner; per-child mock matching is not applied to it. |

The first three are the load-bearing ones. Cancellation is the only one that is
a behaviour regression rather than a projection gap.

## 7. Behaviour changes worth knowing

- A child's queued/running/terminal dispatch records survive the convergence.
  They are the durable session's own projection -- what its progress counts and
  dispatch inspection read -- and the Worker Session's lifecycle records, which
  live on the Worker topic and feed the transport, are a different thing.
  Removing them silently zeroed every session's progress counts.
- Live-child construction no longer validates that an invocation executor
  exists. A runtime-backed session's invoker is bound *after* construction, so
  there is nothing a constructor could check. What is still rejected is an
  unsupported child-executor mode.
- The durable response-event store is now provisioned for sessions with a bound
  worker invoker, rather than for sessions with a live-child invocation factory.

## 8. Not in scope

- Merging `ProviderInferenceRequest` and `WorkstationExecutionRequest` (~85%
  duplicate; should converge, but not needed to make children Workers)
- Moving `ChildIndex` into Worker Sessions — it is a workflow fan-out position
  and Worker Sessions has no sibling concept
- Homing the child output map — `TerminalResult` carries no payload today
- Plan reporting — see `docs/internal/projects/acp-plan-reporting/`
