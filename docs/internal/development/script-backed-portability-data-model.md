# Script-Backed Portability Data Model

This artifact records the shared-model decisions for the script-backed Agent
Factory portability PRD. It covers the public `copyReferencedScripts`
workstation contract, inline script-backed workstation runtime definitions,
and the expand-layout script-copy boundary that loads the copied layout through
the normal runtime config path.

## Change

- PRD, design, or issue: `prd.json` for
  `agent-factory-flatten-script-definition-support`
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory API, config, worker-runtime, replay, and docs
  reviewers
- Packages or subsystems: `libraries/agent-factory/api`,
  `pkg/api/generated`, `ui/src/api/generated`, `pkg/config`, `pkg/workers`,
  functional tests, and Agent Factory docs/process guidance
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
| `copyReferencedScripts` | public config field | Workstation-level public flag that opts `config expand` into copying supported referenced script files into the expanded layout. | `libraries/agent-factory/api/openapi.yaml` and generated artifacts derived from it | `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, `ui/src/api/generated/openapi.ts`, `pkg/api/factory_config_contract_audit_test.go` |
| Inline script-backed workstation runtime definition | public/runtime config shape | A workstation whose supported runtime fields are authored directly in `factory.json`, allowing flatten/load without a split workstation `AGENTS.md`. | `pkg/config` runtime config loading and canonical serialization | `pkg/config/runtime_config.go`, `pkg/config/layout.go`, `pkg/config/flatten_script_definition_test.go` |
| Supported referenced script path | config-expand portability contract | A static bundle-relative `SCRIPT_WORKER` script reference copied during expand when the owning workstation opts in. Supported sources are a relative command path or the actual interpreter script path after value-bearing flags are skipped. | `pkg/config` expand-layout writer | `pkg/config/layout.go`, `pkg/config/layout_script_copy_test.go`, `pkg/config/layout_script_copy_integration_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `workstations[].copyReferencedScripts` | camelCase JSON/OpenAPI field name | config authors and generated public clients | OpenAPI-generated models, config mapper, expand layout writer, runtime loader | `api/openapi.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/config/factory_config_mapping_test.go`, `pkg/api/factory_config_contract_audit_test.go` |
| `SCRIPT_WORKER` relative command path | worker command string | config authors and split worker `AGENTS.md` | `pkg/config` expand layout writer and runtime loader | `pkg/config/layout.go`, `pkg/config/layout_script_copy_test.go` |
| Interpreter script arg after flag values | worker args entry | config authors and split worker `AGENTS.md` | `pkg/config` expand layout writer and runtime loader | `pkg/config/layout.go`, `pkg/config/layout_script_copy_test.go`, `pkg/config/layout_script_copy_integration_test.go` |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public `Workstation` schema | `libraries/agent-factory/api/openapi.yaml` and generated models | canonical workstation fields plus optional `copyReferencedScripts` | omitted `copyReferencedScripts` defaults to `false` | generated Go models, generated UI models, config mapper, docs/examples | `api/openapi.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/config/factory_config_mapping_test.go` |
| Canonical `interfaces.FactoryWorkstationConfig` | `pkg/interfaces` and `pkg/config` mapper boundary | topology plus resolved runtime fields and `CopyReferencedScripts` | inline script-backed workstations remain loadable without split workstation `AGENTS.md` when supported runtime fields are present | runtime loader, expand layout writer, workers, replay helpers | `pkg/interfaces/factory_config.go`, `pkg/config/runtime_config.go`, `pkg/config/layout.go` |
| Expanded layout script-copy boundary | `pkg/config.writeExpandedFactoryLayout` | expanded `factory.json`, split worker/workstation files, and copied supported script files when opted in | no script copying unless `copyReferencedScripts` is explicitly `true` | CLI `config expand`, functional tests, runtime loader | `pkg/config/layout.go`, `pkg/config/layout_script_copy_test.go`, `pkg/config/layout_script_copy_integration_test.go`, `tests/functional_test/fat_factory_config_test.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| OpenAPI-owned factory-config contract | `api/openapi.yaml` | generated Go models, generated UI models, config docs, config mapper tests | OpenAPI source owns the public field names and generated consumers must not redefine or alias the field locally. | Reintroducing a snake_case alias or omitting the generated field is a public-contract regression. | `pkg/api/factory_config_contract_audit_test.go`, generated artifacts, `make generate-api` |
| Config-owned flatten/load/expand boundary | `pkg/config` | CLI config helpers, runtime loader, workers, replay helpers, tests | Consumers load or flatten runtime config through `pkg/config`; they do not reconstruct inline runtime fallback or expand-layout script-copy rules locally. | Missing inline workstation runtime fields still fail with a targeted path error; unsupported absolute or escaping script paths fail instead of being rewritten. | `pkg/config/runtime_config.go`, `pkg/config/layout.go`, `pkg/config/flatten_script_definition_test.go`, `pkg/config/layout_script_copy_test.go` |
| Runtime script execution contract | `pkg/config` plus script-worker config | `pkg/workers.WorkstationExecutor` and script executor | Config owns worker command/arg resolution; worker execution consumes the loaded command/arg contract without special expand-only rewrites. | Expand must copy the actual script path, not a preceding flag value, or the expanded layout fails before runtime load. | `pkg/workers/workstation_executor.go`, `pkg/config/layout_script_copy_integration_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  OpenAPI `Factory` / `Workstation` schemas plus generated Go/UI models remain
  the canonical public config contract.
- Package-local model selected: `interfaces.FactoryConfig`,
  `interfaces.FactoryWorkstationConfig`, and worker runtime structs remain the
  internal runtime representation behind explicit config mapper and loader
  boundaries.
- Reason: config authors, generated clients, flatten/load/expand helpers, and
  runtime worker execution need one durable public contract owner while keeping
  runtime-only loading behavior in the config package.
- Translation boundary: `FactoryConfigMapper.Expand`,
  `FactoryConfigMapper.Flatten`, `LoadRuntimeConfig(...)`, and
  `writeExpandedFactoryLayout(...)`.
- Review evidence: contract audit tests, config mapper tests, flatten/load
  regressions, expand-layout tests, and the functional flatten-expand smoke.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Public copy flag naming | OpenAPI schema, generated Go/UI models, canonical JSON serialization | Keep one canonical camelCase field `copyReferencedScripts`; do not advertise a snake_case alias. | Agent Factory API/config | Complete in this PRD slice. |
| Script-backed workstation runtime definition | inline `factory.json` fields vs split workstation `AGENTS.md` | Keep one loaded workstation meaning in `pkg/config`; split files remain optional input, not a second runtime contract. | Agent Factory config | Complete in this PRD slice. |
| Script-copy script-path resolution | expand-layout helper vs worker execution | Keep copy-path detection in `pkg/config` and keep worker execution on the unchanged command/arg contract. | Agent Factory config/workers | Complete in this PRD slice. |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
  The public contract starts in the OpenAPI source, while `pkg/config` owns
  runtime config loading, canonical serialization, and expand-layout script
  copying inside the shared library boundary.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
  This slice uses the documented API-contract, shared-library, and
  configuration-owner interaction patterns.
- Interop structs or config models that intentionally differ from the canonical
  model: internal `interfaces.FactoryConfig` and worker runtime structs remain
  package-local and are translated only at explicit config boundaries.
- Approved exceptions: none in this slice.
