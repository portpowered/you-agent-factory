# Root Consolidation (L2) — Primary Dependencies for Worker Events

Status: proposed
Date: 2026-08-02
Lane: L2 in `docs/internal/projects/acp-program/README.md`
Audience: Workers, Factory Runtime, Providers, Recordings maintainers

Governing decisions live in the lane map. This plan cites `D1`–`D6` and does not
restate them.

## 1. Why this lane exists

ACP Worker Events (L4) needs a small number of service roots to actually mean
what they claim. That work is currently inside packaged-service-structure (L3),
which is slow because it is comprehensive. L2 extracts only the roots L4
consumes, ships them ahead of L3, and L3 then depends on L2's sealed surfaces
rather than re-deriving them.

L2 is also the only lane permitted to carry opportunistic cleanup (`D5`). Dead
API removal and duplicate-surface collapse land here so L1 and L4 stay
feature-shaped.

## 2. Scope fence

### L2 owns

| Area | Reason L4 needs it |
| --- | --- |
| Workers execution surface | Worker Sessions must start and cancel one supervised attempt through a sealed root |
| Workers panic/terminal semantics | Worker Session terminal states must carry a typed cause |
| Workers runner injection | Mutable reinjection prevents a single injected Workers root |
| Factory Runtime dispatch operations | Runtime must hand dispatch identity to Worker Sessions |
| Providers attempt control | Worker pause/cancel/terminate must reach a running provider attempt |
| Providers continuation | Worker resume must pass an exact typed provider session reference |

### L2 explicitly defers to L3

- **Factory Sessions root sealing.** All 45 methods stay as they are. L1 and L4
  read through thin shims (`D4`). L4 never calls Factory Sessions for Worker
  execution, so nothing in L4 blocks on it.
- **`runtimeOpener` / `RuntimeOpeningFactory` retirement.**
  `runtimeopening.NewFactory` takes **44 parameters**
  (`pkg/services/factory_sessions/internal/runtimeopening/factory.go:78-123`)
  and `RuntimeOpeningDependencies` has **44 exported fields**
  (`pkg/services/factory_sessions/wire/application_graph.go:145-188`). This is
  real debt, but no L4 behavior depends on it.
- **Recordings alias-surface reduction.** `pkg/services/recordings/contracts.go`
  is 402 lines, 383 of them pure `X = recordingcontracts.X` aliases, and
  `Service` embeds `recordingcontracts.Service`
  (`pkg/services/recordings/contracts.go:399-402`) over a 1388-line internal
  interface. L4 consumes Recordings only for dispatch/Worker-Session
  associations, which the current surface already supports. Defer.
- **Factory Definitions catalog sealing.** L1 needs additive catalog reads only.
- **Petri surface retirement, HTTP/MCP/CLI ownership splits, package-structure
  checkers.** Untouched.

### Correction to prior analysis

`models.Service` does **not** expose `ForRuntime`
(`pkg/services/models/service_contract.go:11`). It exposes
`OpenRuntimeScope`/`CloseRuntimeScope` returning opaque references, with the
explicit contract *"Implementations must not construct or return another
Service, host, runtime, puller, limiter, process, or storage handle while
opening the scope."* Models is **already sealed and is the exemplar pattern**
for this lane. `ForRuntime` survives only on the private implementation
(`pkg/services/models/internal/service/runtime_factory.go:123,554`) and in
legacy input projections (`runtime_config_contract.go:96,230`) — a `DEL-`
candidate, not a contract problem.

## 3. Evidence base

Verified against the working tree on 2026-08-02.

### E1 — `workers.Service.Execute` is dead in production

`Execute(context.Context, ExecuteRequest) (ExecuteResult, error)` is declared at
`pkg/services/workers/workstation_contracts.go:266` and documented as *"the
canonical request-scoped operation"* with pool lifecycle described as
*"temporary compatibility surfaces for later cutover."*

`workers.ExecuteRequest` has **zero non-test references outside
`pkg/services/workers`**. Every non-test occurrence is an implementation or
signature site: `internal/root.go:17,84`, `internal/service/provider.go:18`,
`internal/service/normalize.go:15`, `internal/service/execute.go:20,121`,
`internal/service/adapt.go:27`, `internal/runtime_service.go:273`.

The live path is the opposite of the documentation:

```
factory_runtime/internal/services/orchestration/runtime/factory.go:412
  → workers.NewWorkstationPoolBoundary(...)
  → WorkstationPoolBoundary.Publish            (workstation_pool_boundary_impl.go:116)
  → workers.Service.DispatchWorkstation        (internal/runtime_service.go:331)
  → workerExecutorRequestAdapter.Execute       (workstation_pool_boundary_impl.go:43)
  → WorkerExecutor.Execute(ctx, work.WorkDispatch)
```

### E2 — Cancellation is severed at the boundary

`pkg/services/workers/workstation_pool_boundary_impl.go:116-134`:

```go
execute := func() {
	result, err := b.service.DispatchWorkstation(context.WithoutCancel(ctx), request)
	accept(context.Background(), request, result, err)
}
if b.async { go execute(); return nil }
```

The caller's cancellation and deadline are deliberately dropped. Worker control
must route through `WorkstationExecutionService.CancelWorkstationDispatch`
(`workstation_pool_boundary_contracts.go:13`), never through context.

### E3 — Panics produce a nil error

`pkg/services/workers/workstation_pool_boundary_impl.go:43-55` recovers a panic
into `WorkResult{Outcome: OutcomeFailed, Error: fmt.Sprintf("executor panic: %v",
recovered)}` and sets `err = nil`. The failure string survives; the Go error does
not. A caller branching on `err != nil` sees success.

### E4 — Workers runner injection is mutable

`pkg/services/workers/runtime_service.go:34-46` declares `RuntimeService` with
`WithCommandRunners(CommandRunner, CommandRunner) (RuntimeService, error)` and
`WithProgressPublisher(...) (RuntimeService, error)` — copy-on-configure builders
on a service interface. Single production caller:
`pkg/services/factory_runtime/internal/runtime_build.go:329-357`
(`workerServiceWithProgress`). Implementations at
`pkg/services/workers/internal/runtime_construction.go:57,145`.

### E5 — Five Factory Runtime root methods are unimplemented

`pkg/services/factory_runtime/interfaces.go` declares `PlanDispatch` (:88),
`AcceptDispatchResult` (:96), `CaptureCheckpoint` (:103), `LoadCheckpoint`
(:110), and `RestoreCheckpoint` (:117), each returning
`ErrCapabilityUnavailable` (`composition_contracts.go:64`) *"until nested IMP-RUN
packets"* land. `ErrCapabilityUnavailable` is documented as not a successful
no-op.

Under `D1` the three checkpoint methods have no durable backing and no planned
one. The two dispatch methods are what L4 needs.

### E6 — Providers has no attempt control

`pkg/services/providers/service_contract.go:11-28` is three methods:
`ListProviders`, `GetProvider`, `Execute`. Continuation is half-built —
`ExecuteRequest.ResumeSession *SessionRef` exists
(`execute_contract.go:99`) and results *"may carry an optional detached
SessionRef"*. There is no pause, cancel, or terminate; cancellation is only
observable after the fact as `ErrExecuteCancelled`.

## 4. Decisions this lane makes

### DEC-L2-EXEC — Delete `Execute`, seal `WorkstationExecutionService`

**Decision:** remove `workers.Service.Execute`, `ExecuteRequest`, and
`ExecuteResult` from the root; promote the existing narrow port
`WorkstationExecutionService`
(`pkg/services/workers/workstation_pool_boundary_contracts.go:9-14`) to the
sealed Workers execution surface.

Rationale:

1. Deleting `Execute` removes code that has never run in production, not
   working behavior. Its supporting `internal/service/*.go` normalize/adapt/
   provider path goes with it.
2. `Execute` is synchronous and returns no handle. L4 needs start-then-cancel
   supervision. `WorkstationExecutionService` already provides
   `DispatchWorkstation` + `CancelWorkstationDispatch` and is the proven path.
3. A method documented as "canonical" that nothing calls is a trap; the next
   implementer builds on it and discovers E1 the hard way.

Rejected alternative — finishing the `Execute` cutover — means migrating the
live boundary onto an untested surface to satisfy a comment.

### DEC-L2-CKPT — Checkpoint methods are removed, not implemented

Under `D1` there is no durable store and none is planned, so `CaptureCheckpoint`,
`LoadCheckpoint`, and `RestoreCheckpoint` cannot be honestly implemented. They
are deleted from the root. The process-local `CheckpointStore` under
`factory_runtime/internal/services/checkpoint_recovery` remains private and
permanent. Requires an amendment note in
`packaged-service-structure` recording that the `IMP-RUN-04` follow-on is closed.

## 5. Task catalog

Packets follow the packaged-service-structure vocabulary. Each is independently
dispatchable unless a dependency is named.

### Workers

| Packet | Scope | Depends on |
| --- | --- | --- |
| `CTR-WRK-EXEC` | Publish `WorkstationExecutionService` as the sealed Workers execution contract with documented start/dispatch/cancel semantics, including that cancellation is boundary-routed not context-routed (E2). | — |
| `DEL-WRK-EXECUTE` | Delete `Service.Execute`, `ExecuteRequest`, `ExecuteResult`, and their `internal/service` normalize/adapt/provider path (E1). | `CTR-WRK-EXEC` |
| `IMP-WRK-PANIC` | Panic recovery returns a typed non-nil error alongside `OutcomeFailed` (E3). Existing `WorkResult.Error` string retained. | — |
| `CLN-WRK-RUNNERS` | Inject command runners and progress publisher once at construction; delete `WithCommandRunners`/`WithProgressPublisher` from `RuntimeService` (E4). | — |
| `CUT-RUN-WRK-RUNNERS` | Migrate `runtime_build.go:329-357` onto constructor injection. | `CLN-WRK-RUNNERS` |

### Factory Runtime

| Packet | Scope | Depends on |
| --- | --- | --- |
| `IMP-RUN-DISPATCH` | Implement `PlanDispatch` and `AcceptDispatchResult` against the sealed Workers execution contract, returning dispatch identity L4 associates with a Worker Session (E5). | `CTR-WRK-EXEC` |
| `DEL-RUN-CKPT` | Delete `CaptureCheckpoint`, `LoadCheckpoint`, `RestoreCheckpoint` from the root per `DEC-L2-CKPT`. | — |
| `CLN-RUN-CAPUNAVAIL` | Remove `ErrCapabilityUnavailable` returns that no longer exist; keep the sentinel only if a declared-unimplemented method remains. | `IMP-RUN-DISPATCH`, `DEL-RUN-CKPT` |

### Providers

| Packet | Scope | Depends on |
| --- | --- | --- |
| `CTR-PRV-CONTROL` | Add typed attempt pause/cancel/terminate to the root, each returning an explicit capability result including a typed *unsupported* outcome so callers branch rather than guess (E6). | — |
| `IMP-PRV-CONTROL` | Implement control for providers that support it; return unsupported for the rest. | `CTR-PRV-CONTROL` |
| `CTR-PRV-CONT` | Promote continuation from an optional `ExecuteRequest` field to an explicit typed root input with defined failure when the reference is stale or foreign. | — |

### Opportunistic cleanup (`D5`)

| Packet | Scope |
| --- | --- |
| `DEL-MOD-FORRUNTIME` | Remove the private `ForRuntime` leftovers and legacy input projections in Models (`internal/service/runtime_factory.go:123,554`, `runtime_config_contract.go:96,230`). Root is already sealed; this is dead-path removal. |
| `CLN-L2-SHIMS` | Maintain the shim deletion register (§6). One packet, updated as L1/L4 create shims. |

## 6. Outputs and shim register

### L4 consumes from L2

| L4 need | L2 output |
| --- | --- |
| Start one supervised Worker attempt | `CTR-WRK-EXEC` |
| Cancel/terminate a running attempt | `CTR-WRK-EXEC` + `CTR-PRV-CONTROL` |
| Typed terminal cause on panic | `IMP-WRK-PANIC` |
| Dispatch identity for Worker Session association | `IMP-RUN-DISPATCH` |
| Exact provider session continuation on resume | `CTR-PRV-CONT` |
| Single injected Workers root | `CLN-WRK-RUNNERS` + `CUT-RUN-WRK-RUNNERS` |

### Shims registered for deletion

Created by L1/L4 under `D4`, owned for removal by `CLN-L2-SHIMS`. Each entry
records the consumer package, the provider root it adapts, and the L2 or L3
packet whose completion retires it.

| Shim | Adapts | Retired by |
| --- | --- | --- |
| `chat_sessions/internal/factorysessionsshim` | `factory_sessions.Service` (45 methods) | L3 Factory Sessions sealing |
| `worker_sessions/internal/workersshim` | Workers execution | `CTR-WRK-EXEC` (may retire at creation) |
| `worker_sessions/internal/providersshim` | `providers.Service` | `CTR-PRV-CONTROL` + `CTR-PRV-CONT` |

A shim added without a register entry is a review defect.

## 7. Acceptance criteria

Behavioral outcomes a reviewer can verify. Architecture properties are design
constraints, not acceptance evidence.

1. A Worker attempt cancelled through the sealed execution contract stops the
   underlying provider process, and the attempt reports a terminal cancelled
   outcome. Cancelling an already-terminal attempt is a typed no-op, not an
   error.
2. A Worker executor that panics produces a terminal failed outcome **and** a
   non-nil typed error carrying the panic cause. A caller branching only on
   `err != nil` observes the failure.
3. A Factory dispatch returns a dispatch identity before any worker output is
   published, and that identity resolves to the same dispatch on later lookup.
4. Requesting pause on a provider that cannot pause returns an explicit
   unsupported outcome; the attempt continues running and is still cancellable.
5. Resuming a Worker attempt passes the exact provider/kind/id continuation
   reference. A stale or foreign reference fails with a typed error and does not
   silently start a fresh provider session.
6. Building the application no longer constructs a second Workers service view;
   command runners and the progress publisher are supplied once and observable
   in a single constructed instance.
7. `workers.Service.Execute` and the three Factory Runtime checkpoint methods are
   absent from the public surface, and no consumer references them.
8. Existing Factory execution behavior is unchanged: a Factory run that
   succeeded before this lane succeeds after it, with the same terminal result
   and event sequence.

## 8. Verification

Narrowest useful tier first, broadened where the surface is shared.

- Focused package tests for each changed service (`go test ./pkg/services/workers/...`,
  `./pkg/services/factory_runtime/...`, `./pkg/services/providers/...`).
- `make test` for the short Go suite.
- `make lint`.
- `make pkg-boundary` and `make pkg-structure` — these already gate ownership;
  extend their fixtures to the changed roots rather than adding bespoke
  import-graph tests.
- `make verify-fast` before review; `make verify-pr` before merge.
- Cancellation, panic, and dispatch-identity paths run under the race detector
  with repeat/stress mode, per `planning-standards.md` regulation 7.
- `make test-functional` for the dispatch cutover packets, since Factory
  execution is shared surface.

Deleted surfaces additionally require: `rg` evidence of zero remaining
references, and a green build with the declaration removed rather than
deprecated.

## 9. Delivery boundary

Per `planning-standards.md` regulation 9, a packet is complete only when required
CI is terminal and passing, blocking review conversations are explicitly
addressed, conflicts with concurrent L1/L3/L4 work are resolved against current
`main`, and the pull request is **merged**. Opening a PR, obtaining approval, or
reaching green CI without merge is not completion.

Composition edits under `pkg/wire/`, `pkg/root/`, and `pkg/initializer/` are
resolved by normal rebase and are not phase gates (`D3`).
