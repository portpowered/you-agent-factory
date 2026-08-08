# Worker Session Convergence

Status: half implemented — contract and Workers route landed, JavaScript caller
not yet converted
Date: 2026-08-08
Branch: `worker-session-convergence`
Gate: `TestJavaScriptFactoryChildrenAreVisibleAsWorkers`
(`tests/functional/chat_sessions/root_composition/`), currently red at 0 tool
calls

A JavaScript workflow's `agent.run` children and a Petri Factory's Workers are
the same thing: one provider execution with a prompt, a model, and an output.
They reach the provider through two separate stacks, and only one of them is
visible to a client. This records what converging them requires, including the
two wrong turns taken on the way, so the remaining work does not re-derive
either.

## 1. What is landed

| Change | Where |
| --- | --- |
| `Start` → `InvokeSession`, `+Retry`, `+Attempts` | `worker_sessions/contracts.go` |
| The single retry loop | `worker_sessions/internal/service/control.go` |
| `ProviderInvocationRoute` + worker-role binding | `workers/workstation_contracts.go`, `workstation_pool_boundary_impl.go` |
| Direct-inference executor | `workers/internal/services/workstations/executor/provider_invocation.go` |
| Standalone workstation pool | `workers/wire/workstation_pool.go` |
| `InvokeWorker` root operation — the bridge | `factory_runtime/invoke_worker_contract.go`, `internal/services/orchestration/runtime/invoke_worker.go` |

Three properties worth not re-litigating:

- **No executor in the request.** `Execution.WorkstationName` is a route into
  the binding snapshot Workers already assembled. That is the whole of a
  caller's say in what runs, and it is what lets one operation serve both
  orchestrators without a second injection path.
- **Petri behaviour is unchanged.** `Retry`'s zero value is one attempt.
  Retryability was always *classified* on both paths; only `livechild` ever
  looped. Converging must not silently give every Petri Worker a retry it
  never had.
- **A retry is not a new Worker.** Attempts reuse the session identity and the
  open publication window, minting `.../attempt/N` as resume already mints
  `.../resume/N`, so one Worker stays one tool call across its attempts.

`workers/wire/workstation_pool.go` is **unused and should be re-examined
before it is kept** — see §3.

## 2. Two wrong turns, recorded so they are not repeated

### 2.1 "JavaScript sessions compose no Workers runtime" — false

Inferred from the Factory Sessions durable-execution path, which builds only an
`InvocationExecutor`. Disproved by instrumenting `responsebridge` and running
the gate: a JavaScript session has a live Factory Event stream with a real
`APIFactory` behind it, carrying `RUN_REQUEST`, `INITIAL_STRUCTURE_REQUEST`,
`SESSION_STARTED`, `FACTORY_STATE_RESPONSE`.

Those come from `ensureEventHistory`, which sits in the same constructor that
calls `configureRuntimeDispatch` (`orchestration/runtime/factory.go:204`). So a
JavaScript session **already has** `cfg.workerSessions`, a pool boundary, and a
`recordings.RuntimeLedger`. They are built and never reach the child path.

### 2.2 Emitting the association from the read model — wrong stream

`BuildCanonicalRuntimeSessionEvents` feeds the durable-session read model.
`responsebridge` subscribes through `SubscribeFactoryEventsForSession`, which
resolves the session's runtime and reads its ledger. Adding
`DISPATCH_WORKER_SESSION_ASSOC` to the synthesis left the gate at 0 tool calls
and was reverted: it would assert an association for a Worker Session that does
not exist.

**The association must be recorded on the per-session `recordings.RuntimeLedger`,
by whatever actually invokes the Worker.**

## 3. The real remaining problem

For a JavaScript Factory, two objects exist for one session:

- a `factoryImpl` holding `cfg.workerSessions`, the pool boundary, and
  `eventHistory` — it records session lifecycle events but does **not** run the
  workflow (`RunJavaScript` appears nowhere in `orchestration/runtime`)
- a `JavaScriptRuntimeService` in Factory Sessions that **does** run the
  workflow, via the stateless `OrchestrationJavaScriptExecution`, and holds no
  runtime handle

That split is the defect. The child executor is composed on the side that has
no Worker Sessions and no ledger, which is why it grew a private path rather
than using the route `workers.Service`'s own doc comment already declares is
"the sole production execution route".

This also means §1's standalone workstation pool is probably unnecessary: the
session already registers a full `workers.RuntimeService` through
`runtimeopening`, and a second pool would publish Worker topics correctly while
recording its association on a ledger nothing reads — the §2.2 failure again,
one layer down. **Delete it unless the chosen approach genuinely needs it.**

## 4. The approach, and the handle that makes it work

**Make the runtime own it.** Add one additive root operation to Factory
Runtime — an `InvokeWorker`-shaped call that reserves the Worker Session,
records the association on its own `eventHistory`, and calls
`workerSessions.InvokeSession` with `ProviderInvocationRoute`. The JavaScript
child executor then calls that and nothing else.

This was chosen over threading the ledger and Worker Sessions service through
`runtimeopening` into `JavaScriptRuntimeService`. Threading is more mechanical,
but it adds collaborators to a composition already carrying seventeen and
leaves the workflow running outside the runtime that owns its Workers — the
same split this project exists to remove. Factory Runtime **already** exposes
Worker Session controls to peers (`worker_session_control.go`), so a Worker
Session operation on that root is established rather than novel, and
CLAUDE.md/D4 explicitly prefers "one deliberate operation added to a root over
a shim that reaches around it".

The apparent blocker was that Factory Sessions holds no runtime handle. It
does. The chain is:

```
live_runtime.Resolve(sessionID)        // factory_sessions/internal/services/live_runtime
  -> livesession.LiveSession.Runtime   // *factorysessions.LiveRuntime
  -> LiveRuntime.Factory               // the per-session Factory Runtime
                                       // (holds cfg.workerSessions + eventHistory)
```

`JavaScriptRuntimeService` lives in Factory Sessions alongside that resolver,
so the child executor can reach the exact runtime whose ledger the response
bridge reads. No new threading through `runtimeopening` is required — which is
also why §1's standalone workstation pool should be deleted rather than kept.

## 4a. The one connection still missing

`InvokeWorker` is implemented and reachable on the runtime. What remains is
handing `JavaScriptRuntimeService` a way to reach the runtime for its session,
so `childExecutorHooks` can build a child executor that calls it.

The obstacle is ordering, not availability. The live-session registry that can
`Resolve(sessionID)` lives on the Factory Sessions service, and
`FactorySessionExecutionFactory` is a **dependency of** that service — so the
executor factory cannot depend on it directly without a cycle. The resolver must
therefore be late-bound: a `func(sessionID string) factory.Service` closure
supplied after the sessions service exists, or a small lazy provider, rather
than a constructor argument.

Resolve that and the remaining steps are mechanical.

## 5. Then, in order

1. Build the chosen bridge; confirm the gate reaches **5 tool calls** before
   touching anything else.
2. Child adapter: child spec → `WorkstationExecutionRequest`;
   `InvokeSessionResult` → `JavaScriptChildExecutionResult`;
   `Retry{MaxAttempts: policy.MaxRetries + 1}`.
3. **Only then** delete: `factory_sessions/internal/execution/livechild/`
   (one production caller, `runtime_service.go:926`), `executeWithRetry` /
   `sleepWithContext`, the `AppendChildDispatch` QUEUED/RUNNING calls, the
   `liveChildInvocation` closures in `pkg/wire`, and every
   `workers.InvocationExecutor` consumer outside `pkg/services/workers` —
   `factory_sessions/wire/wire.go`, `durable_execution/wire/wire.go`,
   `durable_execution/internal/service/construction.go`.

   Durable execution threads the same executor, so persisted and live
   JavaScript workflows convert together or not at all.

Deleting before step 1 is verified removes the only working child path and
yields a tree that compiles, passes `make test`, and silently produces no
Worker output.

## 6. Not in scope

- Merging `ProviderInferenceRequest` and `WorkstationExecutionRequest` (~85%
  duplicate; should converge, but not needed to make children Workers)
- Moving `ChildIndex` into Worker Sessions — it is a workflow fan-out position
  and Worker Sessions has no sibling concept
- Homing the child output map — `TerminalResult` carries no payload today
- Plan reporting — see `docs/internal/projects/acp-plan-reporting/`
