# Runtime Lookup Test Fixture Inventory

This inventory records the current test-owned runtime lookup doubles that mirror the layered production runtime lookup interfaces. It exists to define the narrow shared fixture seam before the helper extraction work starts.

The shared seam now lives in `pkg/testutil/runtimefixtures` instead of the root `pkg/testutil` package so package-internal tests can reuse the fixtures without importing the heavier harness helpers and creating import cycles.

## Generic Fixture Shapes

The repeated test doubles fall into three generic interface shapes that the shared helper should cover:

1. `interfaces.RuntimeWorkstationLookup`
2. `interfaces.RuntimeDefinitionLookup`
3. `interfaces.RuntimeConfigLookup`

## Inventory

| File | Local type | Shape | Current behavior | Disposition |
| --- | --- | --- | --- | --- |
| `pkg/factory/state/net_test.go` | `stubTransitionTopologyRuntimeConfig` | `RuntimeWorkstationLookup` | Map-backed workstation lookup | Migrate to shared workstation fixture |
| `pkg/factory/subsystems/circuitbreaker_test.go` | `circuitBreakerRuntimeConfig` | `RuntimeWorkstationLookup` | Map-backed workstation lookup | Migrate to shared workstation fixture |
| `pkg/factory/subsystems/history_transitioner_pipeline_test.go` | `historyTransitionerRuntimeConfig` | `RuntimeWorkstationLookup` | Map-backed workstation lookup | Migrate to shared workstation fixture |
| `pkg/factory/subsystems/subsystem_transitioner_test.go` | `transitionerRuntimeConfigStub` | `RuntimeWorkstationLookup` | Map-backed workstation lookup with nil-safe miss behavior | Migrate to shared workstation fixture |
| `pkg/factory/scheduler/work_queue_test.go` | `schedulerRuntimeConfig` | `RuntimeWorkstationLookup` | Map-backed workstation lookup | Migrate to shared workstation fixture |
| `pkg/factory/event_history_test.go` | `eventHistoryRuntimeConfig` | `RuntimeDefinitionLookup` | Map-backed worker and workstation lookup | Migrate to shared definition fixture |
| `pkg/factory/options_test.go` | `stubRuntimeConfig` | `RuntimeDefinitionLookup` | Fixed miss for both worker and workstation lookups | Migrate to shared definition fixture |
| `pkg/factory/projections/topology_projection_test.go` | `projectionRuntimeConfig` | `RuntimeDefinitionLookup` | Map-backed worker and workstation lookup | Migrate to shared definition fixture |
| `pkg/factory/runtime/factory_test.go` | `runtimeProjectionConfig` | `RuntimeDefinitionLookup` | Map-backed worker lookup plus fixed workstation miss | Migrate to shared definition fixture |
| `pkg/factory/runtime/factory_test.go` | `runtimeSchedulerConfig` | `RuntimeDefinitionLookup` | Fixed miss for both worker and workstation lookups | Migrate to shared definition fixture |
| `pkg/factory/subsystems/dispatcher_test.go` | `dispatcherRuntimeConfig` | `RuntimeDefinitionLookup` | Map-backed worker lookup plus fixed workstation miss | Migrate to shared definition fixture |
| `pkg/service/factory_test.go` | `serviceTestRuntimeConfig` | `RuntimeDefinitionLookup` | Map-backed worker and workstation lookup | Migrate to shared definition fixture |
| `pkg/workers/agent_test.go` | `staticRuntimeConfig` | `RuntimeConfigLookup` | Map-backed worker and workstation lookup plus explicit `FactoryDir` and `RuntimeBaseDir` fallback | Migrate to shared full runtime-config fixture |
| `tests/functional_test/logical_move_test.go` | `logicalMoveRuntimeConfig` | `RuntimeConfigLookup` | Fixed empty directory accessors, fixed worker miss, fixed logical workstation hit | Migrate to shared full runtime-config fixture |

## Scope Decision

The current inventory does not expose bespoke lookup doubles that need to remain local because of dynamic or behavior-specific lookup semantics. The custom behavior in these tests lives in adjacent schedulers, executors, planners, or returned worker/workstation data, not in the runtime lookup methods themselves.

That means the shared helper can stay narrow:

- a workstation-only fixture for `Workstation(name)`
- a definition fixture for `Worker(name)` plus `Workstation(name)`
- a full runtime-config fixture for `FactoryDir()`, `RuntimeBaseDir()`, `Worker(name)`, and `Workstation(name)`

## Initial Migration Targets

The highest-value initial migration set is the example slice already called out in the PRD because those files contain the clearest generic duplication:

- `pkg/workers/agent_test.go`
- `pkg/service/factory_test.go`
- `pkg/factory/event_history_test.go`
- `pkg/factory/projections/topology_projection_test.go`
- `pkg/factory/runtime/factory_test.go`
- `pkg/factory/subsystems/dispatcher_test.go`
- `tests/functional_test/logical_move_test.go`

The remaining workstation-only fixtures are also simple migration candidates, but they can follow after the shared seam lands without widening the first extraction step.
