# Cleanup Analyzer Report: Primitive Factory Observability Getters

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for primitive factory-boundary observability getters.
The retained service-facing observability surface is `GetEngineStateSnapshot`
for aggregate runtime state and `GetFactoryEvents` for canonical event history.
The cleanup did not change service/API status, work, or event behavior.

## Analyzer Commands

Starting inventory command shape for the retired getter names, run from an
`origin/main` checkout:

```bash
rg -n "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"
rg --count-matches "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"
```

This worktree recorded the same branch-base inventory with `git grep` so the
report could inspect `origin/main` without replacing the current checkout:

```bash
git grep -n -E "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" origin/main -- libraries/agent-factory/pkg libraries/agent-factory/tests
git grep -c -E "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" origin/main -- libraries/agent-factory/pkg libraries/agent-factory/tests
```

Current active inventory across package and functional-test Go code:

```bash
rg -n "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"
rg --count-matches "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg libraries/agent-factory/tests -g "*.go"
```

Final required active inventory for the factory boundary, service, listeners,
and test utilities:

```bash
rg -n "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg/factory libraries/agent-factory/pkg/service libraries/agent-factory/pkg/listeners libraries/agent-factory/pkg/testutil -g "*.go"
```

## Branch-Base Inventory

The branch-base command observed 86 matches across 16 files.

| File | Matches | Classification |
| --- | ---: | --- |
| `pkg/factory/interfaces.go` | 5 | Retired public factory-boundary methods |
| `pkg/factory/runtime/factory.go` | 6 | Retired runtime factory implementations and engine delegation |
| `pkg/factory/runtime/factory_test.go` | 6 | Runtime assertions migrated to aggregate snapshots |
| `pkg/listeners/filewatcher_test.go` | 5 | Listener test-double stubs removed |
| `pkg/service/factory_test.go` | 6 | Service test assertions and test-double stubs migrated |
| `pkg/testutil/mock_factory.go` | 5 | Testutil primitive getter methods removed |
| `pkg/testutil/mock_factory_test.go` | 25 | Primitive getter counter/guard tests removed or replaced |
| `pkg/factory/engine/engine.go` | 1 | Retained engine-owned raw marking method |
| `pkg/factory/engine/engine_test.go` | 10 | Retained engine-owned raw marking assertions |
| `tests/functional_test/cron_smoke_test.go` | 1 | HTTP status helper, not factory boundary |
| `tests/functional_test/functional_server_override_regression_test.go` | 1 | HTTP status helper, not factory boundary |
| `tests/functional_test/functional_server_test.go` | 3 | Functional server `/status` helper and callers |
| `tests/functional_test/integration_smoke_test.go` | 6 | HTTP status helper callers |
| `tests/functional_test/ootb_experience_test.go` | 1 | HTTP status helper caller |
| `tests/functional_test/runtime_state_test.go` | 3 | Retained engine-owned raw marking assertions |
| `tests/functional_test/service_config_override_alignment_test.go` | 2 | HTTP status helper callers |

## Removed Symbols

- `factory.Factory.GetState`
- `factory.Factory.GetUptime`
- `factory.Factory.GetMarking`
- `factory.Factory.GetTopology`
- `factory.Factory.GetRuntimeState`
- `runtime.factoryImpl.GetState`
- `runtime.factoryImpl.GetUptime`
- `runtime.factoryImpl.GetMarking`
- `runtime.factoryImpl.GetTopology`
- `runtime.factoryImpl.GetRuntimeState`
- Listener and service test-double primitive getter stubs
- `testutil.MockFactory.GetState`
- `testutil.MockFactory.GetUptime`
- `testutil.MockFactory.GetMarking`
- `testutil.MockFactory.GetTopology`
- `testutil.MockFactory.GetRuntimeState`
- `testutil.MockFactoryPrimitiveGetter`
- `testutil.MockFactoryForbiddenPrimitiveGetterBehavior`
- `testutil.MockFactory` primitive getter counters and forbidden-getter fields
- `testutil.MockFactory` fallback runtime-state construction separate from
  `GetEngineStateSnapshot`

## Migrated Assertions

- Runtime lifecycle, uptime, topology, marking, dispatch, and runtime status
  assertions now read `GetEngineStateSnapshot`.
- Runtime tests that intentionally inspect raw Petri-net behavior remain scoped
  to package-local `FactoryEngine` state.
- Service tests read the aggregate service snapshot through
  `FactoryService.GetEngineStateSnapshot`.
- Listener, service, and testutil factory doubles now satisfy the retained
  observability contract with `GetEngineStateSnapshot`, `GetFactoryEvents`, and
  `SubscribeFactoryEvents`.
- A service smoke test runs a dry-run batch factory and verifies completed
  status, topology, marking, completed work, aggregate runtime activity
  through tick count and dispatch history, and canonical factory events without
  primitive factory getters.

## Final Inventory

The final required active inventory command observed 11 matches across 2 files:

- `pkg/factory/engine/engine.go`: `FactoryEngine.GetMarking`
- `pkg/factory/engine/engine_test.go`: 10 engine-owned raw marking assertions

These matches are intentional engine-level exceptions. They are not methods on
`factory.Factory`, the runtime factory implementation, service code, listener
code, or testutil doubles.

The broader current package and functional-test inventory observed 28 matches
across 9 files. Outside the engine-owned exceptions, the remaining matches are
functional server `/status` helper calls named `GetState`; those calls exercise
the HTTP status API and do not expose or depend on the retired factory-boundary
methods.

## Lint And Deadcode Baseline

`make lint` remained green during the cleanup stories. No deadcode-baseline
drift was accepted or recorded for this primitive getter cleanup.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/service -run TestFactoryService_RunPreservesSnapshotAndFactoryEventObservability -count=1
rg -n "GetState\(|GetUptime\(|GetMarking\(|GetTopology\(|GetRuntimeState\(" libraries/agent-factory/pkg/factory libraries/agent-factory/pkg/service libraries/agent-factory/pkg/listeners libraries/agent-factory/pkg/testutil -g "*.go"
go test ./pkg/factory/runtime ./pkg/service ./pkg/listeners ./pkg/testutil -count=1
make lint
```

## Out Of Scope

- No API route, service response, event vocabulary, dashboard projection, or
  functional-test server status helper was removed.
- No unrelated cleanup scope was included.
