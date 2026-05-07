# WorkEvent Emitter Pipeline Cleanup

Date: 2026-04-18
Scope: Agent Factory legacy `WorkEvent` emitter pipeline removal for `US-006`.

## Summary
- Removed the legacy `WorkEvent` vocabulary from active Agent Factory runtime, hook, subsystem, and engine contracts.
- Deleted the no-op event-emitter subsystem and its sink implementations.
- Kept runtime history, streaming, replay artifacts, and dashboard projections on generated `FactoryEvent` values.
- Preserved worker-result wake-ups with plain signals and dispatch/result hook output instead of event payloads.

## Initial Inventory
Baseline: `origin/main` before the cleanup branch commits.

Command:

```sh
git grep -n -E "WorkEvent|WorkStream|EventEmitter|ChannelSink|BroadcasterSink|WithEventSink" origin/main -- libraries/agent-factory/pkg "*.go"
```

Combined result: 160 matching lines across 22 files.

| Symbol | Matching lines | Files | Primary locations |
| --- | ---: | ---: | --- |
| `WorkEvent` | 98 | 20 | `pkg/interfaces`, engine wake-up plumbing, subsystem execution signatures, hook results, runtime tests |
| `WorkStream` | 7 | 2 | `pkg/interfaces/factory_runtime.go`, event sink stream methods |
| `EventEmitter` | 36 | 6 | event-emitter subsystem, runtime registration, tick-group tests |
| `ChannelSink` | 16 | 2 | event-emitter subsystem and focused tests |
| `BroadcasterSink` | 11 | 2 | event-emitter subsystem and focused tests |
| `WithEventSink` | 3 | 2 | public factory option and runtime test registration |

## Final Inventory
Active package Go command:

```sh
rg -n "WorkEvent|WorkStream|EventEmitter|ChannelSink|BroadcasterSink|WithEventSink" libraries/agent-factory/pkg -g "*.go"
```

Expected result: exit code 1, 0 matching lines, 0 files.

Per-symbol verification:

| Symbol | Matching lines | Files | Classification |
| --- | ---: | ---: | --- |
| `WorkEvent` | 0 | 0 | Removed from active package code |
| `WorkStream` | 0 | 0 | Removed from active package code |
| `EventEmitter` | 0 | 0 | Removed from active package code |
| `ChannelSink` | 0 | 0 | Removed from active package code |
| `BroadcasterSink` | 0 | 0 | Removed from active package code |
| `WithEventSink` | 0 | 0 | Removed from active package code |

Repository text check, excluding analyzer reports because this report intentionally contains the searched terms:

```sh
rg -n "WorkEvent|WorkStream|EventEmitter|ChannelSink|BroadcasterSink|WithEventSink" libraries/agent-factory -g "!libraries/agent-factory/docs/development/cleanup-analyzer-reports/**"
```

Intentional non-code matches:

| Path | Classification |
| --- | --- |
| `libraries/agent-factory/tests/adhoc/factory/inputs/idea/default/factory-public-option-deadcode-cleanup.md` | Historical cleanup idea input that records why `WithEventSink` was deleted; not active runtime, test, or public documentation code |

The maintained record/replay design sketch previously included `WorkEvents` in a proposed hook result and was updated in this sweep so contributor guidance does not advertise the retired payload.

## Removed Symbols
- Deleted public `factory.EventSink` and `factory.WithEventSink`.
- Deleted exported `interfaces.WorkEventType`, `interfaces.WorkEvent`, and `interfaces.WorkStream`.
- Removed legacy event slices from `interfaces.TickResult`, `interfaces.SubmissionHookResult`, and `interfaces.DispatchResultHookResult`.
- Removed the `events []interfaces.WorkEvent` input from `subsystems.Subsystem.Execute`.
- Deleted `subsystems.EventEmitter`, `subsystems.ChannelSink`, `subsystems.BroadcasterSink`, `NewEventEmitter`, `NewChannelSink`, `NewBroadcasterSink`, and focused event-emitter tests.
- Replaced `FactoryEngine.resultCh chan interfaces.WorkEvent` and `NotifyResult(event interfaces.WorkEvent)` with plain wake signals.

## Retained Generated-Event Surfaces
- `factory.FactoryEventHistory` remains the runtime history owner.
- `Factory.GetFactoryEvents(ctx)` remains the snapshot read path for generated `pkg/api/generated.FactoryEvent` history.
- `Factory.SubscribeFactoryEvents(ctx)` remains the replay-then-stream path through `interfaces.FactoryEventStream`.
- Replay artifacts retain generated `ReplayArtifact.Events` rather than legacy top-level work-event arrays.
- Dashboard/world-state reducers continue to consume generated `FactoryEvent` values.

## Test Migration Evidence
- Engine tests now assert tick progress, subsystem ordering, hook output, and plain result wake-up signals instead of event payload propagation.
- Runtime tests assert generated factory history through `GetFactoryEvents` and `SubscribeFactoryEvents`.
- Subsystem tests assert mutations, dispatch records, histories, generated batches, and terminal state instead of `WorkEvent` delivery.
- Functional tests that provide test subsystems compile against `Execute(ctx, snapshot)` and assert runtime state or dashboard projections.

Focused behavioral coverage:

| Test | Behavior protected |
| --- | --- |
| `TestNew_CompletesWorkflowThroughActiveSubsystems` | Representative runtime work completes and generated dispatch history is observable without an event sink |
| `TestFactoryEventHistory_RecordsOrderedEventsWithStableIDs` | Canonical generated history records ordered runtime events |
| `TestFactoryEventHistory_SubscribeReplaysHistoryThenStreamsLiveEvents` | Canonical event stream replays history and streams live generated events |
| `TestNew_WorkerPoolDispatchResultHookRecordsCompletionAtObservedTick` | Worker-pool completion records generated dispatch completion at the observed tick |
| `TestNew_ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick` | Replay-delayed worker completion wakes at the planned logical tick |
| `TestNew_ServiceModeWorkerPoolResultSignalCompletesLateSubmission` | Service-mode late submission completes from a worker-result wake signal |

## Validation Commands
Commands run for this cleanup story:

```sh
cd libraries/agent-factory && go test ./pkg/interfaces ./pkg/factory/engine ./pkg/factory/runtime ./pkg/factory/subsystems -count=1
cd libraries/agent-factory && make lint
```

Commands run in the preceding worker-result wake-up cleanup and retained as evidence for this analyzer report:

```sh
cd libraries/agent-factory && go test ./pkg/factory/runtime -run "TestNew_(CompletesWorkflowThroughActiveSubsystems|WorkerPoolDispatchResultHookRecordsCompletionAtObservedTick|ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick|ServiceModeWorkerPoolResultSignalCompletesLateSubmission|ServiceModeWithoutInitialWork_AcceptsLateSubmission)$" -count=1 -v -timeout 30s
cd libraries/agent-factory && go test -short ./... -timeout 120s
```

## Intentional Exceptions
- Historical cleanup analyzer reports may keep the retired symbol names as audit evidence.
- Historical adhoc idea input may keep `WithEventSink` references because it documents the deleted public option that prompted the cleanup.
- Generated `FactoryEvent` names and APIs are retained intentionally; they are the canonical runtime history surface and are not part of the removed legacy `WorkEvent` pipeline.
