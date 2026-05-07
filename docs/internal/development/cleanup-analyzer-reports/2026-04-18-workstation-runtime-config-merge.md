# Workstation Runtime Config Merge Cleanup

Date: 2026-04-18
Scope: Agent Factory workstation runtime configuration merge for `US-008`.

## Analyzer Commands

Initial and final inventory commands:

```bash
rg -n "workstation_configs|WorkstationDefs|WorkstationCfgs" libraries/agent-factory/pkg libraries/agent-factory/docs libraries/agent-factory/api libraries/agent-factory/examples
rg -n "HasInlineRuntimeConfig|InlineRuntimeConfig|SetInlineRuntimeConfig|workstationDefFromInlineConfig|cloneWorkstationConfigPtr" libraries/agent-factory/pkg libraries/agent-factory/docs libraries/agent-factory/api libraries/agent-factory/examples
rg -n "runtime_type|runtimeType" libraries/agent-factory/pkg libraries/agent-factory/docs libraries/agent-factory/api libraries/agent-factory/examples
rg -n "\.WorkstationConfig\(" libraries/agent-factory/pkg libraries/agent-factory/tests
rg -n "\bWorkstationConfig\b" libraries/agent-factory/pkg/interfaces libraries/agent-factory/pkg/config libraries/agent-factory/pkg/workers libraries/agent-factory/pkg/factory/projections libraries/agent-factory/pkg/replay
```

## Initial Findings

- The branch started with overlapping workstation contracts: `interfaces.WorkstationConfig` for runtime executor fields and `interfaces.FactoryWorkstationConfig` for topology and scheduling fields.
- Runtime config loading carried parallel workstation maps and lookup methods, including `WorkstationDefs`, `WorkstationCfgs`, `RuntimeConfig.Workstation(...)`, and `RuntimeConfig.WorkstationConfig(...)`.
- Replay effective config serialized historical split-map workstation metadata through `workstation_configs`.
- Inline runtime compatibility helpers let callers copy between the split shapes: `HasInlineRuntimeConfig`, `InlineRuntimeConfig`, `SetInlineRuntimeConfig`, `workstationDefFromInlineConfig`, and `cloneWorkstationConfigPtr`.
- Public docs, examples, generated schema, and fixtures still advertised scheduling `type` and runtime `runtime_type` as normal authoring fields.

## Cleanup Applied

- Deleted the overlapping `interfaces.WorkstationConfig` runtime contract and moved runtime metadata onto `interfaces.FactoryWorkstationConfig`.
- Collapsed runtime config loading to one `Workstations` map keyed by workstation name and one `RuntimeConfig.Workstation(name)` lookup.
- Removed the active inline runtime conversion helper family.
- Removed replay artifact storage of `workstation_configs`; replay effective config now stores runtime workstation metadata in canonical `workstations`.
- Updated workers, topology projection, replay reconstruction, schema, docs, examples, fixtures, and tests to use `kind` for scheduling and `type` for runtime executor selection.
- Added `TestWorkstationRuntimeConfigSmoke_SingleLookupDrivesDispatchTopologyAndReplay`, which loads split workstation config, resolves the canonical workstation, dispatches model work through `WorkstationExecutor`, projects topology, saves and loads replay effective config, and resolves the replay workstation through the same lookup.

## Final Inventory

- `workstation_configs|WorkstationDefs|WorkstationCfgs`: 4 matches.
  - Retained matches are two record/replay docs documenting the intentional artifact compatibility break and two tests that fail if saved effective config or replay artifact JSON reintroduces `workstation_configs`.
- Inline conversion helpers: 0 matches.
- `.WorkstationConfig(` runtime lookup calls: 0 matches.
- `\bWorkstationConfig\b` in active runtime config, worker, replay, and topology packages: 0 matches.
- `runtime_type|runtimeType`: 19 matches.
  - Retained matches are mapper-boundary compatibility normalization in `pkg/config/openapi_factory.go`, the documented legacy input alias field in `interfaces.FactoryWorkstationConfig`, tests proving the alias is not advertised or emitted canonically, and a local `runtimeType` variable in projection code that writes canonical `type`.

## Verification Added

- `go test ./tests/functional_test -run TestWorkstationRuntimeConfigSmoke_SingleLookupDrivesDispatchTopologyAndReplay -count=1`
