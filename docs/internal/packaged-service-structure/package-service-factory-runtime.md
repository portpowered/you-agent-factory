Factory Runtime has completed much of the physical package folding, but contract convergence is still incomplete. The target should be a process-scoped Runtime service with opaque runtime identities; Factory Sessions owns session lifecycle decisions, while Runtime owns execution-state mechanics.

## Recommended authority boundary

Factory Runtime should own:

- Activating a resolved Factory definition into an executable runtime instance.
- Runtime control: pause, resume, terminate, wait.
- Accepting materialized Work into execution state.
- Scheduling runnable work and maintaining the dispatch outbox.
- Accepting correlated Workers results.
- Live, orchestration-neutral observation.
- Operator Work movement.
- Opaque checkpoint capture/load/restore.
- Private Petri and JavaScript execution.

It should not own:

- Definition loading, persistence, source validation, or packaging — Factory Definitions.
- Factory Session lifecycle, durable session status, or final session-result presentation — Factory Sessions.
- Work Request parsing, admission policy, content materialization, or Work queries — Work.
- Worker selection implementation, invocation, provider/model execution — Workers.
- Canonical event durability, replay history, event subscriptions — Recordings.
- HTTP/CLI/MCP result interpretation.

## Converged root interface

The current `Service` is a useful intermediate contract, but it is implicitly bound to one hosted runtime and omits submission while the legacy `APIFactory` remains necessary. See [interfaces.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/interfaces.go:30).

I recommend making the final service process-scoped and explicitly addressing every runtime:

```go
type Service interface {
    Activate(context.Context, ActivateRequest) (ActivateResult, error)

    SubmitWork(context.Context, SubmitWorkRequest) (SubmitWorkResult, error)

    Pause(context.Context, PauseRequest) (PauseResult, error)
    Resume(context.Context, ResumeRequest) (ResumeResult, error)
    Terminate(context.Context, TerminateRequest) (TerminateResult, error)
    Wait(context.Context, WaitRequest) (WaitResult, error)

    MoveWork(context.Context, MoveWorkRequest) (MoveWorkResult, error)
    Observe(context.Context, ObserveRequest) (ObserveResult, error)

    AcceptDispatchResult(
        context.Context,
        AcceptDispatchResultRequest,
    ) (AcceptDispatchResultResult, error)

    CaptureCheckpoint(
        context.Context,
        CaptureCheckpointRequest,
    ) (CaptureCheckpointResult, error)

    LoadCheckpoint(
        context.Context,
        LoadCheckpointRequest,
    ) (LoadCheckpointResult, error)

    RestoreCheckpoint(
        context.Context,
        RestoreCheckpointRequest,
    ) (RestoreCheckpointResult, error)
}
```

Every request should carry an opaque `RuntimeID` or a `{FactorySessionID, Generation}` reference. `Activate` receives a resolved, immutable definition contract—not a loader, filesystem, Petri net, or JavaScript service.

Two notable changes:

- `Wait` should honor `context.Context` and return normally, rather than returning a channel as [WaitToCompleteResult](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/work_move_errors.go:58) currently does.
- `PlanDispatch` should become an internal Runtime operation. Scheduling creates dispatch intents internally and publishes them through the Workers root. It should not remain an HTTP/MCP command that permits external callers to manufacture dispatch state. `AcceptDispatchResult` remains necessary as Workers-to-Runtime ingress.

Event subscription should move to Recordings or Factory Sessions. Runtime emits canonical facts; it should not also serve durable event history.

## Broken or transitional contracts

The main remaining contract failures are:

| Current contract | Problem | Destination |
|---|---|---|
| `APIFactory` | Keeps submission and event streaming outside `Service`; widely used through runtime type assertions | Submission into `Service`; subscriptions to Recordings/Sessions |
| `Factory`, `LegacySnapshotProvider`, `WorkMover` | Expose the old run loop and Petri snapshot access | Delete after caller migration |
| `HostedInstance`, `HostedHandle`, `Lifecycle`, `Sidecars`, `ReplacementBuilder` | Factory Sessions depends on Runtime’s construction graph | Opaque IDs through root `Service`; mechanics in `instance_host` |
| `OrchestrationCompilation` | Publicly returns `*state.Net` | Entirely private orchestration subservice |
| `OrchestrationJavaScriptExecution`, `JavaScriptWorkflows` | Strategy-specific public service surfaces | JavaScript implementation inside orchestration |
| `SubmissionHook`, `DispatchResultHook` | Root contracts import private Petri/state types | Orchestration/dispatch-planning internals |
| `FactoryStatusProjector` | Parallel observation service | Fold into `Service.Observe` |
| `SessionResultProjectionOperation` | Runtime root owns Factory Session presentation | Factory Sessions result-projection subservice |
| Filesystem, clock, logger, metrics interfaces | Dependency/effect ports inflate the public service contract | `wire`, `internal`, or platform contracts |
| Definition-owned dispatch aliases | Runtime vocabulary aliases Factory Definitions types | Move canonical dispatch types to Runtime |
| Petri/state aliases and helpers | Public root exposes `Net`, marking, state categories, place helpers | Orchestration internals |

The clearest leaks are:

- [projection_contracts.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/projection_contracts.go:1) aliases private `state.Net`, `WorkType`, `ResourceDef`, and state functions into the Runtime root.
- [execution_contracts.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/execution_contracts.go:8) imports Petri and orchestration state directly into public hook contracts.
- [orchestration_contract.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/orchestration_contract.go:39) exposes `CompilePetriNet`.
- [hosting.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/hosting.go:23) publishes the complete hosted-instance lifecycle graph.

## Private subservices

The four existing private subservice names are correct:

1. `orchestration`
2. `dispatch_planning`
3. `instance_host`
4. `checkpoint_recovery`

Their shapes are not all converged yet.

- `dispatch_planning` is closest. Its single [Service](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/service.go:198) owns the outbox and Workers publication boundary.
- `instance_host` still embeds the public `Lifecycle` contract and uses public hosted handles. Its contract should use private instance records and opaque IDs.
- `orchestration` has one root service, but it still exposes JavaScript-specific methods and has roughly twenty direct implementation directories. Petri, JavaScript, scheduler, engine, tokens, metrics, replay hooks, and tooling should all move beneath its `internal/`.
- `checkpoint_recovery` currently defines `CheckpointStore`, not a singular `Service`. `Capture`, `Load`, `Compatibility`, and `Restore` should be methods of its private `Service`; the store becomes an internal dependency.

## Logic still misplaced in transports/mappings

The largest confirmed violation is clean CLI invocation result policy.

[run_clean_invocation.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/cli/run/run_clean_invocation.go:30) currently:

- Type-asserts `LegacySnapshotProvider`.
- Reads `GetEngineStateSnapshot`.
- Inspects Petri topology and markings.
- Searches terminal/failed tokens.
- Interprets dispatch history and Workers failure metadata.
- Selects the successful output.
- Determines timeout/failure semantics.

That is domain projection logic, not CLI adaptation. The CLI should receive a plain terminal Work or invocation result from Work/Factory Sessions. It should only select JSON/plain formatting and map typed errors.

Additional leakage:

- Runtime’s CLI adapter exposes `CountTokenStates(*PetriMarkingSnapshot)` in [service.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_runtime/transports/cli/service.go:21). Token counting belongs in Runtime observation projection.
- `pkg/transports/mapping/composition` contains legacy `APIFactory` fallbacks. Those should disappear when callers consume the singular root contracts.
- `factory_status.go` representation mapping itself is acceptable: converting a detached Runtime observation/status into generated API shapes is legitimate mapping work.
- HTTP and MCP Runtime handlers are otherwise mostly thin. The concern there is the decision to expose `PlanDispatch`, rather than substantial hidden business logic.

## Final tree

```text
pkg/services/factory_runtime/
├── service.go
├── contracts.go
├── control.go
├── observation.go
├── dispatch.go
├── checkpoint.go
├── errors.go
│
├── wire/
│   └── wire.go
│
├── transports/
│   ├── cli/
│   ├── http/
│   └── mcp/
│
└── internal/
    ├── service/
    │   ├── service.go
    │   ├── activation.go
    │   ├── control.go
    │   ├── submission.go
    │   └── observation.go
    │
    ├── services/
    │   ├── orchestration/
    │   │   ├── service.go
    │   │   ├── wire/
    │   │   └── internal/
    │   │       ├── petri/
    │   │       ├── javascript/
    │   │       ├── scheduler/
    │   │       ├── engine/
    │   │       ├── state/
    │   │       ├── token/
    │   │       ├── replay/
    │   │       └── tooling/
    │   │
    │   ├── dispatch_planning/
    │   │   ├── service.go
    │   │   └── internal/service/
    │   │
    │   ├── instance_host/
    │   │   ├── service.go
    │   │   └── internal/service/
    │   │
    │   └── checkpoint_recovery/
    │       ├── service.go
    │       └── internal/
    │           ├── service/
    │           ├── store/
    │           └── javascript/
    │
    └── testkit/
```

The current direct `testdata/` should move beneath `internal/` or the owning subservice. The remaining `factorystatus`, `host`, `legacysnapshot`, `orchestrators`, `rootobservation`, and transitional `service` directories under `internal/` should be absorbed into the destinations above.

## Recommended migration order

1. Decide process-scoped service plus opaque Runtime identity.
2. Add `Activate`, `SubmitWork`, and context-aware `Wait` to the root contract.
3. Migrate Factory Sessions off `Hosted*`, `Lifecycle`, `Sidecars`, and `ReplacementBuilder`.
4. Migrate Work submission off `APIFactory`.
5. Move event history/subscription consumers to Recordings or Factory Sessions.
6. Remove `PlanDispatch` from HTTP/MCP and route scheduling internally to `dispatch_planning`.
7. Move clean-invocation result projection out of CLI.
8. Internalize every Petri/state/token and JavaScript execution contract.
9. Move JavaScript source validation/preview to Factory Definitions; keep execution in Runtime orchestration.
10. Move session-result projection to Factory Sessions.
11. Collapse each private subservice to one `Service` plus `wire/internal/transports`.
12. Delete migration contracts and remove the Runtime baseline exceptions.

The highest-value next cut is steps 2–5 together: eliminate `APIFactory` and the public hosting graph. Until those are gone, the new Runtime `Service` remains an optional façade rather than the actual authority boundary.