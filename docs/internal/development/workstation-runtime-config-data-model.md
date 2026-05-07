# Workstation Runtime Config Data Model

This artifact records the shared model decisions for the Collapse Workstation
Runtime Config PRD. It covers the transition from split workstation topology and
runtime maps to one canonical loaded workstation config.

## Change

- PRD, design, or issue: `prd.json` for Collapse Workstation Runtime Config
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory runtime/config reviewers
- Packages or subsystems: `pkg/config`, `pkg/workers`, `pkg/replay`,
  `pkg/service`, `pkg/factory/projections`
- Canonical architecture document to update before completion:
  this artifact is the branch data-model artifact; durable rules should move
  into `libraries/agent-factory/docs/development/development.md` or a dedicated
  Agent Factory architecture document if they outlive the cleanup PRD.

## Trigger Check

- [x] Shared noun or domain concept
- [ ] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [ ] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| Loaded workstation config | configuration shape | The effective workstation object used for topology fields and runtime execution fields after loading `factory.json` plus optional split `AGENTS.md` data. | `libraries/agent-factory/pkg/config.LoadedFactoryConfig.Workstations` | `pkg/config/runtime_config.go`, `pkg/config/runtime_config_test.go` |
| Workstation topology kind | field | The Petri-net execution semantics from `factory.json` such as `standard`, `repeater`, or `cron`. | `interfaces.FactoryWorkstationConfig.Type` | `pkg/interfaces/factory_config.go`, `pkg/config/config_mapper.go` |
| Workstation runtime type | field | The worker execution behavior from workstation runtime config, such as `MODEL_WORKSTATION` or `LOGICAL_MOVE`. | `interfaces.FactoryWorkstationConfig.Type` | `pkg/interfaces/factory_config.go`, `pkg/workers/workstation_executor.go` |
| Workstation execution timeout | field | The per-dispatch runtime limit for a workstation. Canonical loaded configs store it only under `limits.maxExecutionTime`; top-level workstation `timeout` is a load-boundary alias that is normalized away. | `interfaces.FactoryWorkstationConfig.Limits.MaxExecutionTime` | `pkg/config/workstation_execution_limits.go`, `pkg/workers/workstation_executor.go`, `pkg/service/cron_watcher.go` |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| `LoadedFactoryConfig.Workstations` | `pkg/config` | Workstation name key and one `interfaces.FactoryWorkstationConfig` value containing topology and resolved runtime fields. | Split `AGENTS.md` data is merged into the topology entry when present; topology-only script-worker dispatch defaults to `MODEL_WORKSTATION` at worker execution. | `pkg/service`, `pkg/workers`, `pkg/factory/projections`, `pkg/replay` | `pkg/config/runtime_config.go`, `pkg/workers/workstation_executor_test.go` |
| `RUN_REQUEST.payload.factory.workstations` | `pkg/replay` and generated API schema | Generated Factory workstation array containing the replayable topology and runtime fields used by the recorded run. | Workstation stop handling serializes through one canonical ordered `stopWords` array; current artifacts must not contain legacy split workstation stop-word fields or maps. | `pkg/service`, `pkg/workers`, `pkg/factory/projections` | `pkg/replay/generated_factory.go`, `pkg/replay/generated_factory_runtime.go`, `pkg/replay/artifact_test.go` |
| Split workstation layout | `pkg/config` | `factory.json` workstation entry and optional `workstations/<name>/AGENTS.md`. | Existing split files are preserved during expand when no inline runtime fields exist. | Config CLI, runtime loader, tests | `pkg/config/layout.go`, `pkg/config/runtime_config_test.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Runtime config lookup | `pkg/config` and `pkg/replay` | `pkg/workers`, `pkg/service`, `pkg/factory/projections` | Consumers call the canonical `interfaces.RuntimeConfigLookup` path-aware contract; consumers do not rebuild loaded workstation fallback order or invent a second execution-only lookup family. | Missing workstation runtime remains a failed dispatch for model workers; topology-only script workers keep the existing default model workstation behavior. | `pkg/interfaces/runtime_lookup.go`, `pkg/workers/workstation_executor.go`, `pkg/workers/workstation_executor_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `interfaces.FactoryWorkstationConfig` remains the shared workstation shape.
- Package-local model selected: no new package-local workstation runtime model.
- Reason: config loading, worker execution, replay, service cron setup, and
  topology projections all need the same workstation meaning.
- Translation boundary: OpenAPI and replay compatibility paths remain temporary
  PRD follow-ups until their stories collapse them.
- Review evidence: focused tests in `pkg/config`, `pkg/workers`, and
  `pkg/replay`.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Loaded config split maps | `LoadedFactoryConfig.WorkstationDefs` and `LoadedFactoryConfig.WorkstationCfgs` | Unified into `LoadedFactoryConfig.Workstations`. | Agent Factory config | Complete in US-002. |
| Replay split maps | Legacy replay workstation side maps and generated Factory hydration maps | Unified into `RUN_REQUEST.payload.factory.workstations` and `EmbeddedRuntimeConfig.Workstations`. | Agent Factory replay | Complete in the factory-only serialization PRD; current artifacts use generated Factory config only. |
| Runtime config dual lookup methods | `Workstation` and `WorkstationConfig` | Unified on `Workstation`. | Agent Factory workers/runtime | Complete in US-004; workers, replay, and topology projections read canonical workstation fields from one lookup. |

## Reviewer Notes

- Runtime config interfaces now expose only `Workstation`; the old
  `WorkstationConfig` method was removed at the worker, config, replay, and
  projection boundaries.
- The canonical public path-aware runtime lookup seam stays on
  `interfaces.RuntimeConfigLookup`; it owns both `FactoryDir()` for authored
  source reads and `RuntimeBaseDir()` for relative runtime execution paths.
- `petri.Transition` is topology-only. Workstation scheduling kind, retry
  limits, and stop-word execution metadata are derived from runtime config via
  workstation name lookup instead of being copied onto Petri transitions or
  mutated after mapping.
- Replay artifacts now store one canonical generated Factory workstation array
  on `RUN_REQUEST.payload.factory`; current checked-in artifacts must not carry
  legacy replay split maps.
