# Retire Deprecated Workstation Join Config Data Model

This artifact records the shared model decisions for retiring the deprecated
workstation-level `join` configuration surface from Agent Factory. It covers the
public config contract, generated OpenAPI types, config mapper/validator
behavior, replay/generated-factory boundaries, and guard-backed fan-in model
that remains canonical.

## Change

- PRD, design, or issue: `prd.json` for Retire Deprecated Workstation Join
  Config
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory config/API/replay/runtime reviewers
- Packages or subsystems: `pkg/interfaces`, `pkg/config`, `pkg/api`,
  `pkg/replay`, `pkg/factory/projections`, functional tests, stress tests, and
  Agent Factory UI generated API types
- Canonical architecture document to update before completion: this artifact is
  the branch data-model construction artifact. Durable fan-in authoring rules
  are captured in `docs/processes/agent-factory-development.md` and
  `libraries/agent-factory/docs/development/cleanup-analyzer-reports/2026-04-19-retire-deprecated-workstation-join-config.md`.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [x] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| Per-input guard | public config contract | Supported fan-in authoring surface on workstation inputs. Guard entries define child-completion and child-failure dependency behavior. | `interfaces.IOConfig.Guard`, `interfaces.WorkstationIOGuardConfig`, and generated `WorkstationIO.guards` | `pkg/interfaces/factory_config.go`, `libraries/agent-factory/api/openapi.yaml`, `pkg/config/openapi_factory_test.go` |
| Workstation guard | public config contract | Workstation-level guard surface retained only for visit-count behavior. It must not carry parent-aware child fan-in. | `interfaces.FactoryWorkstationConfig.Guards` and generated `Workstation.guards` | `pkg/config/config_validator.go`, `pkg/config/config_mapper.go`, `docs/processes/agent-factory-development.md` |
| Retired workstation join | removed config contract | Former workstation-level fan-in object that duplicated per-input guard behavior. It is no longer representable in generated types or active config structs. | Removed from `interfaces.FactoryWorkstationConfig`, OpenAPI schemas, and generated API models | cleanup analyzer report, `rg` US-007 inventory |
| Dynamic fanout count | Petri-net runtime behavior | Count token and guard behavior used when a per-input child guard specifies `spawned_by`. | `pkg/config` mapper and Petri guard types | `pkg/config/config_mapper.go`, `pkg/config/config_mapper_test.go`, `pkg/replay/effective_config_test.go` |
| Parent-aware child failure | Petri-net runtime behavior | `any_child_failed` per-input guard route that observes failed children for the same parent context. | `pkg/config` mapper and Petri guard types | `pkg/config/config_mapper_test.go`, `tests/functional_test/config_driven_test.go`, `tests/stress/barrier_limits_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `parent_input` / `parentInput` | input name string | config authors, OpenAPI clients, UI generated clients | config mapper, config validator, replay generated-factory conversion | `pkg/config/config_validator.go`, `pkg/config/openapi_factory_test.go`, `pkg/replay/effective_config_test.go` |
| `spawned_by` / `spawnedBy` | workstation name string | config authors, generated-factory replay payloads | config mapper and validator for dynamic fanout count behavior | `pkg/config/config_mapper_test.go`, `pkg/config/config_validator_test.go`, `pkg/replay/effective_config_test.go` |
| `all_children_complete` | per-input guard type | config authors and generated Factory payloads | config mapper, Petri guard construction, replay conversion | `pkg/config/config_mapper_test.go`, `tests/functional_test/multi_input_guard_test.go` |
| `any_child_failed` | per-input guard type | config authors and generated Factory payloads | config mapper, Petri guard construction, replay conversion | `pkg/config/config_mapper_test.go`, `tests/functional_test/config_driven_test.go`, `tests/stress/barrier_limits_test.go` |
| `visit_count` | workstation guard type | config authors | workstation-level guard mapper and validator | `pkg/config/config_mapper.go`, `pkg/config/config_validator.go` |

## Lifecycle States

This cleanup does not add or rename lifecycle states. Existing work states
remain authored under `work_types[].states`, and guard behavior observes those
states without introducing new runtime lifecycle values.

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| `workstations[*].inputs[*].guards[]` | `interfaces.IOConfig.Guard` and OpenAPI `WorkstationIO.guards` | `type`, `work_type`, `state`; parent-aware guards also use `parent_input`; dynamic fanout uses `spawned_by` | No implicit join default; absent guards behave as normal input requirements | `pkg/config`, `pkg/replay`, generated UI/API clients | `api/openapi.yaml`, `pkg/config/config_mapper.go`, `pkg/replay/generated_factory_config_conversion.go` |
| `workstations[*].guards[]` | `interfaces.FactoryWorkstationConfig.Guards` and OpenAPI `Workstation.guards` | `type: visit_count` and visit-count fields | No parent-aware fan-in defaults | `pkg/config` mapper and validator | `pkg/config/config_validator.go`, `pkg/config/config_mapper.go` |
| Removed workstation join object | none | none | none | none | cleanup analyzer report and US-007 zero-match inventory |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public factory config fan-in | config authors, OpenAPI clients, UI generated clients | `pkg/config` loader, mapper, validator | Public schemas expose per-input guards; mapper translates those guards into Petri-net arcs. | Raw retired `join` payloads fail at the boundary because the generated/internal public shape cannot represent them. | `api/openapi.yaml`, `pkg/config/factory_config_mapping_test.go`, `pkg/config/config_mapper_test.go` |
| Generated Factory replay fan-in | recorded generated Factory artifacts | `pkg/replay` hydration and config mapper | Replay uses explicit generated API to internal config mappers, preserving `WorkstationIO.guards`. | Generated artifacts cannot reintroduce `join`; unsupported guard shapes fail validation or mapping. | `pkg/replay/generated_factory_config_conversion.go`, `pkg/replay/effective_config_test.go` |
| Petri-net guard fan-in | config mapper | Petri runtime and functional/stress tests | Mapper creates observation/count arcs and parent-aware guards from per-input guard config. | Invalid `spawned_by`, parent input, or unsupported workstation-level child fan-in guard types fail validation. | `pkg/config/config_mapper.go`, `pkg/config/config_validator.go`, `tests/functional_test/config_driven_test.go`, `tests/stress/barrier_limits_test.go` |
| Topology projection fan-in display | loaded runtime config | `pkg/factory/projections` and dashboard generated contracts | Projections read canonical runtime config guards and do not preserve retired join config. | Removed fields cannot appear in generated projection/API payloads. | `pkg/factory/projections/topology_projection.go`, generated API zero-match inventory |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `interfaces.IOConfig.Guard`, `interfaces.WorkstationIOGuardConfig`, OpenAPI
  `WorkstationIO.guards`, generated Go API models, and generated UI API models.
- Package-local model selected: Petri guard structs such as
  `FanoutCountGuard` and `AnyWithParentGuard` remain runtime implementation
  details owned by the config/Petri mapping boundary.
- Reason: config authors, replay artifacts, generated API clients, mapper
  tests, functional tests, and stress tests need one stable public fan-in
  meaning, while Petri guard structs are execution details.
- Translation boundary: OpenAPI/generated Factory models map explicitly to
  internal config structs; config mapping translates per-input guards into
  Petri-net arcs and guards.
- Review evidence: cleanup analyzer report, US-007 retired-symbol inventories,
  API smoke, full Agent Factory config/replay/functional/stress tests, UI
  typecheck, and lint.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Workstation-level join object | retired config field, OpenAPI property, and generated join type family | Removed from active code and generated contracts. | Agent Factory config/API | Complete in this PRD. |
| Join-specific mapper and validator paths | retired mapper helper, validation rule, replay merge, and clone preservation paths | Removed; per-input guards own fan-in behavior. | Agent Factory config/replay | Complete in this PRD. |
| Workstation-level child fan-in guards | top-level `workstations[*].guards` with child-completion or child-failure semantics | Rejected; workstation guards remain limited to `visit_count`. | Agent Factory config | Complete in this PRD. |
| Generated API model vs internal config model | OpenAPI generated structs and `interfaces` structs | Kept with explicit boundary mappers so public schema names and internal runtime fields can intentionally differ. | Agent Factory API/config/replay | No follow-up. |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
  Agent Factory is a reusable library/API/CLI module under `libraries/`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
  This cleanup uses the shared-library, API-contract, generated-contract,
  config-mapping, replay, and scheduler/dispatcher interaction patterns; it
  does not introduce an undocumented special-case interaction.
- Interop structs or config models that intentionally differ from the canonical
  model: generated OpenAPI models and internal `interfaces` structs differ only
  at explicit config/API/replay mapper boundaries.
- Approved exceptions: none.
- Follow-up cleanup tasks: none from this artifact.
