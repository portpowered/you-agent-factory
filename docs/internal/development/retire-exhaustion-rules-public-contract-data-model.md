# Retire Exhaustion Rules Public Contract Data Model

This artifact records the shared-model decisions for the first public-contract
retirement slice of Agent Factory `exhaustion_rules`. The broader design source
for the full retirement remains
`tasks/ideas-fleshing-out/prd-agent-factory-retire-exhaustion-rules.md`; this
slice removes the retired field from the public schema and generated models,
rejects new raw input that still uses it, and keeps guarded
`LOGICAL_MOVE` workstations with `visit_count` guards as the only supported
public replacement surface.

## Change

- PRD, design, or issue: `prd.json` for guarded-loop-breaker-public-contract;
  broader retirement source:
  `tasks/ideas-fleshing-out/prd-agent-factory-retire-exhaustion-rules.md`
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory API, config, replay, runtime, and generated-contract
  reviewers
- Packages or subsystems: `libraries/agent-factory/api`,
  `pkg/api/generated`, `ui/src/api/generated`, `pkg/config`, `pkg/replay`,
  runtime tests, and docs/process guidance
- Canonical architecture document to update before completion:
  `docs/processes/agent-factory-development.md`

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
| `exhaustion_rules` | retired public config field | Former top-level loop-breaker contract that is no longer part of the public schema or generated public models. | `libraries/agent-factory/api/openapi.yaml` and generated artifacts derived from it | `api/openapi.yaml`, `pkg/api/openapi_contract_test.go`, generated Go/UI outputs |
| Guarded logical loop breaker | public config contract | Supported replacement authored as a workstation with `type: LOGICAL_MOVE` and a workstation-level `visit_count` guard. | OpenAPI `Factory` / `Workstation` schemas and internal `interfaces.FactoryWorkstationConfig.Guards` | `api/openapi.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/api/openapi_contract_test.go` |
| Migration-oriented rejection | boundary validation behavior | Raw input that still contains `exhaustion_rules` fails with guidance toward the guarded workstation replacement instead of being remapped into the canonical public contract. | `pkg/config/FactoryConfigMapper.Expand` | `pkg/config/factory_config_mapping.go`, `pkg/config/factory_config_mapping_test.go`, `pkg/config/runtime_config_test.go` |
| Scheduler-dispatched guarded loop breaker | internal runtime behavior | Guarded logical loop breakers remain normal scheduler-dispatched workstations so the documented loop-breaker workstation appears in runtime dispatch history while `TransitionExhaustion` remains reserved for legacy or system circuit-breaker paths. | Agent Factory config, scheduler, runtime, and service wiring | `pkg/config/config_mapper.go`, `pkg/config/config_mapper_test.go`, `pkg/factory/scheduler/work_queue.go`, `pkg/factory/runtime/factory.go`, `pkg/service/factory.go`, `docs/processes/agent-factory-development.md` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `exhaustion_rules` | top-level JSON/OpenAPI field name | legacy config authors and old fixtures | `FactoryConfigMapper.Expand` boundary rejection only | `pkg/config/factory_config_mapping.go`, `pkg/config/runtime_config_test.go` |
| `type: LOGICAL_MOVE` | workstation runtime type string | config authors and generated public clients | config mapper, runtime executor wiring, replay conversion | `api/openapi.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/replay/effective_config_test.go` |
| `guards[].type: visit_count` | workstation guard enum value | config authors and generated public clients | config validator, config mapper, topology projection | `api/openapi.yaml`, `pkg/config/config_mapper_test.go`, `pkg/factory/projections/topology_projection.go` |
| `guards[].workstation` | workstation name string | config authors and generated public clients | visit-count guard validator and runtime scheduler eligibility | `pkg/config/config_validator.go`, `pkg/config/config_mapper_test.go` |

## Lifecycle States

This retirement slice does not add or rename work lifecycle states. Existing
`work_types[].states` values remain canonical, and the guarded replacement only
changes how loop breakers are authored at the public config boundary.

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public `Factory` schema | `libraries/agent-factory/api/openapi.yaml` and generated models | canonical factory fields without `exhaustion_rules` | no retired-field compatibility alias | generated Go models, generated UI models, config examples, replay payloads | `api/openapi.yaml`, `pkg/api/openapi_contract_test.go`, `pkg/api/generated/server.gen.go`, `ui/src/api/generated/openapi.ts` |
| Guarded loop-breaker workstation | OpenAPI `Workstation` and internal `interfaces.FactoryWorkstationConfig` | `type: LOGICAL_MOVE`, inputs, outputs, and a workstation-level `visit_count` guard | normal workstation defaults; no top-level loop-breaker object | config loader, mapper, validator, replay conversion, runtime tests | `api/openapi.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/config/config_mapper_test.go` |
| Raw input rejection path | `pkg/config/FactoryConfigMapper.Expand` | top-level `exhaustion_rules` present in raw JSON | no remap; fail fast with migration guidance | runtime config loader, CLI/config helpers, tests | `pkg/config/factory_config_mapping.go`, `pkg/config/runtime_config_test.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| OpenAPI-owned public config contract | `api/openapi.yaml` | generated Go models, generated UI models, config examples, tests | OpenAPI source owns the public contract; generated consumers must not redefine retired fields locally. | Reintroducing `exhaustion_rules` or `ExhaustionRule` is a contract regression. | `pkg/api/openapi_contract_test.go`, generated artifacts |
| Raw config boundary validation | config authors and checked-in `factory.json` inputs | `pkg/config` loader and runtime config helpers | raw input enters through `FactoryConfigMapper.Expand`, which owns retirement rejection behavior | Top-level `exhaustion_rules` fails with migration guidance; guarded workstation input continues. | `pkg/config/factory_config_mapping.go`, `pkg/config/factory_config_mapping_test.go`, `pkg/config/runtime_config_test.go` |
| Guarded replacement dispatch path | public config mapper and runtime wiring | Petri runtime, scheduler, dispatcher history, and service wiring | public guarded workstations stay explicit logical workstations through config mapping and dispatch so the supported public route remains observable in runtime history | Invalid or missing visit-count guard details fail validation; the retired public field is never remapped. | `pkg/config/config_mapper.go`, `pkg/config/config_mapper_test.go`, `pkg/factory/scheduler/work_queue.go`, `pkg/factory/runtime/factory.go`, `pkg/service/factory.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  OpenAPI `Factory` and `Workstation` schemas plus generated Go/UI public models
  remain the canonical public contract surface for authored config.
- Package-local model selected: internal `interfaces.FactoryConfig`,
  guarded `interfaces.FactoryWorkstationConfig` loop breakers, and Petri
  transition structs remain runtime implementation details that are allowed to
  differ from the public contract while retirement cleanup is still in
  progress.
- Reason: public authors, generated clients, replay/config serialization, and
  config validation need one durable contract owner, while runtime scheduling
  and dispatcher-history implementation details remain package-local.
- Translation boundary: `FactoryConfigMapper.Expand`,
  `replay.RuntimeConfigFromGeneratedFactory(...)`, and
  `replay.GeneratedFactoryFromLoadedConfig(...)`.
- Review evidence: OpenAPI contract tests, generated-model regression coverage,
  config boundary tests, runtime config tests, and `make api-smoke`.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Top-level public loop-breaker contract | OpenAPI `Factory`, generated Go/UI public models, examples | Retired from the public contract and replaced by guarded workstations. | Agent Factory API/config | Further doc/example cleanup remains with the broader retirement PRD. |
| Public guarded replacement vs legacy exhaustion path | OpenAPI/generated/public config vs scheduler and circuit-breaker runtime behavior | Guarded loop breakers stay explicit workstations; `TransitionExhaustion` remains limited to legacy or system circuit-breaker paths. | Agent Factory config/runtime | Keep the public guarded-workstation contract and the legacy exhaustion path documented as separate behaviors. |
| Legacy raw-input compatibility | old raw `exhaustion_rules` input vs canonical public contract | Rejected at the boundary instead of normalized into the public model. | Agent Factory config | No follow-up in this slice. |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
  The public contract starts in the OpenAPI source; guarded-loop-breaker
  scheduling and dispatch-history behavior remains in Agent Factory
  config/runtime packages.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
  This slice uses the documented API-contract, shared-library, generated-model,
  and configuration-owner interaction patterns.
- Interop structs or config models that intentionally differ from the canonical
  model: generated public models omit `exhaustion_rules`, while internal
  runtime/config structs keep only guarded workstation loop-breaker authoring;
  `TransitionExhaustion` remains a separate runtime-only concept behind
  explicit mapper and scheduler boundaries.
- Approved exceptions: none in this slice.
- Follow-up cleanup tasks: broader docs/example/runtime cleanup stays tracked by
  `tasks/ideas-fleshing-out/prd-agent-factory-retire-exhaustion-rules.md`.
