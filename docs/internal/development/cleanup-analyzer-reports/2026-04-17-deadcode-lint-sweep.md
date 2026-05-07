# Cleanup Analyzer Report: Deadcode Lint Sweep

Date: 2026-04-17

## Scope

Deadcode analyzer pass over the Agent Factory Go module with test executables included.

## Evidence

Commands:

```bash
cd libraries/agent-factory
go run golang.org/x/tools/cmd/deadcode@v0.25.1 -test ./...
make deadcode
```

Findings:

- The initial `deadcode -test` report contained 86 unreachable functions.
- API cleanup had left unused route-test scaffolding for removed workflow, state, and stream surfaces in `pkg/api/server_test.go`.
- CLI dashboard formatting still carried two unused dispatch type helpers after event-derived dashboard state replaced older trace/snapshot flows.
- Trace reconstruction still carried one unused ordering helper after lineage sorting moved to the current depth/time comparator.

## Removed Symbols

- `newMockStoreWithWorkflows`
- `newStreamingMockFactory`
- `streamingMockFactory.Run`
- `streamingMockFactory.Pause`
- `streamingMockFactory.GetHistory`
- `streamingMockFactory.Submit`
- `streamingMockFactory.SubmitStreaming`
- `streamingMockFactory.SubscribeWorkEvents`
- `streamingMockFactory.GetState`
- `streamingMockFactory.GetUptime`
- `streamingMockFactory.GetMarking`
- `streamingMockFactory.GetTopology`
- `streamingMockFactory.GetRuntimeState`
- `streamingMockFactory.GetEngineStateSnapshot`
- `streamingMockFactory.GetFactoryEvents`
- `streamingMockFactory.WaitToComplete`
- `streamingMockFactory.setSnapshot`
- `streamingMockFactory.streamContext`
- `streamingMockFactory.engineStateSnapshotCalls`
- `sliceValue`
- `mapValue`
- `apiInitialStructureEvent`
- `apiWorkInputEvent`
- `apiWorkstationRequestEvent`
- `apiWorkstationResponseEvent`
- `dispatchTypesForEntry`
- `dispatchTypesForCompletedDispatch`
- `dispatchPrecedes`

## Outcome

Implemented in this sweep:

- Removed 28 dead symbols linked to deleted factory endpoint and trace/dashboard cleanup paths.
- Added `docs/development/deadcode-baseline.txt` as the accepted current deadcode report.
- Added `make deadcode` and wired it into the default `make lint` profile.
- Updated Agent Factory development guidance so future intentional deadcode drift updates the baseline in the same review.

The final baseline contains 58 accepted findings that are older public library or test-helper debt and not linked to the removed factory endpoint surfaces.
