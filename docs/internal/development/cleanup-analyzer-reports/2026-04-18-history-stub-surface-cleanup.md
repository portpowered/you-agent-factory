# Cleanup Analyzer Report: History Stub Surface Cleanup

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for the legacy `GetHistory` contract and the unused history model family. Runtime history remains canonical through generated factory events and event projections.

## Analyzer Commands

Initial caller inventory from the branch base:

```bash
git grep -n -e "GetHistory" origin/main -- libraries/agent-factory
git grep -n -e "FactoryHistory" -e "WorkstationHistory" -e "ResourceHistory" -e "WorkItemHistory" origin/main -- libraries/agent-factory
git grep -n -e "GetFactoryEvents" -e "SubscribeFactoryEvents" -e "ReconstructFactoryWorldState" -e "BuildFactoryWorldView" origin/main -- libraries/agent-factory
```

Final active Go inventory:

```bash
rg -n "GetHistory" libraries/agent-factory -g "*.go"
rg -n "FactoryHistory|WorkstationHistory|ResourceHistory|WorkItemHistory" libraries/agent-factory -g "*.go"
rg -n "GetFactoryEvents|SubscribeFactoryEvents|ReconstructFactoryWorldState|BuildFactoryWorldView" libraries/agent-factory -g "*.go"
```

## Findings

- The branch base exposed `Factory.GetHistory(ctx)` in `pkg/factory/interfaces.go`.
- The runtime implementation in `pkg/factory/runtime/factory.go` returned `nil, nil`, so the public method advertised a non-functional history path.
- Test-only satisfiers in `pkg/testutil/mock_factory.go`, `pkg/listeners/filewatcher_test.go`, and `pkg/service/factory_test.go` carried stub-only `GetHistory` methods.
- `pkg/interfaces/factory_runtime.go` defined the unused `FactoryHistory`, `WorkstationHistory`, `ResourceHistory`, and `WorkItemHistory` model family.
- Canonical event-history paths already existed through `GetFactoryEvents`, `SubscribeFactoryEvents`, `ReconstructFactoryWorldState`, and `BuildFactoryWorldView`.

## Removed Symbols

- `factory.Factory.GetHistory`
- `factoryImpl.GetHistory`
- `MockFactory.GetHistory`
- `mockFactory.GetHistory`
- `aggregateSnapshotFactory.GetHistory`
- `interfaces.FactoryHistory`
- `interfaces.WorkstationHistory`
- `interfaces.ResourceHistory`
- `interfaces.WorkItemHistory`
- `interfaces.WorkstationTimings`
- `interfaces.ResourceTimings`
- `interfaces.WorkItemTimings`
- `interfaces.StateTiming`

## Outcome

- Removed the legacy public `GetHistory` contract, runtime stub, and test-only interface satisfier methods.
- Removed the legacy history DTO family and timing structs after active Go caller analysis found no remaining consumers.
- Preserved the runtime and dashboard read path on generated events and event projections.
- Updated runtime coverage so representative work is submitted, generated events are read with `GetFactoryEvents`, selected-tick world state is reconstructed with `ReconstructFactoryWorldState`, and a world view is built with `BuildFactoryWorldView`.

## Final Match Expectations

- `rg -n "GetHistory" libraries/agent-factory -g "*.go"` returns 0 active Go matches.
- `rg -n "FactoryHistory|WorkstationHistory|ResourceHistory|WorkItemHistory" libraries/agent-factory -g "*.go"` returns 0 active Go matches.
- `rg -n "GetFactoryEvents|SubscribeFactoryEvents|ReconstructFactoryWorldState|BuildFactoryWorldView" libraries/agent-factory -g "*.go"` returns active Go matches for the canonical event-history read path; the cleanup run observed 127 matches.

## Retained Exceptions

- Historical cleanup report files may mention the removed names as audit evidence.
- No active Go exceptions are retained for `GetHistory`, `FactoryHistory`, `WorkstationHistory`, `ResourceHistory`, or `WorkItemHistory`.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/factory ./pkg/factory/runtime ./pkg/testutil ./pkg/listeners ./pkg/service -count=1
go test ./pkg/interfaces ./pkg/factory/projections -count=1
go test ./pkg/factory/runtime ./pkg/factory/projections -count=1
make test
make lint
```
