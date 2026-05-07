# Same-Name Input Guard Data Model

This artifact records the shared model decisions for the Agent Factory
same-name workstation input guard. It covers the public config contract,
generated OpenAPI types, config-boundary validation and mapping, Petri runtime
evaluation, and the representative smoke fixture used to prove the feature
across those boundaries.

## Change

- PRD, design, or issue: `prd.json` for `agent-factory-same-name-guard`
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory API, config, runtime, and functional-test reviewers
- Packages or subsystems: `api/openapi-main.yaml`, `pkg/api/generated`,
  `pkg/interfaces`, `pkg/config`, `pkg/petri`, `pkg/factory/scheduler`,
  `tests/functional_test`, generated UI API types, and authoring docs
- Canonical architecture document to update before completion: this artifact is
  the branch data-model construction artifact. Durable authoring and smoke-test
  guidance is captured in `docs/processes/agent-factory-development.md` plus
  the user-facing guard guides under `libraries/agent-factory/docs/`.

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
| Same-name input guard | public config contract | Per-input authored guard that compares a candidate token's authored work name against a peer input token from the same workstation. | `interfaces.InputGuardConfig`, OpenAPI `InputGuard`, and generated `InputGuardType` | `pkg/interfaces/factory_config.go`, `api/openapi-main.yaml`, `pkg/config/openapi_factory_test.go` |
| Peer input binding | runtime guard dependency | The referenced workstation input whose bound token supplies the comparison name. | `pkg/config` mapper and Petri guard wiring | `pkg/config/config_mapper.go`, `pkg/config/config_mapper_test.go` |
| Authored work name equality | runtime behavior | Exact string equality on the two compared authored work names; missing names fail closed. | `pkg/petri` same-name guard evaluation | `pkg/petri/guard.go`, `pkg/petri/guard_test.go`, `pkg/factory/scheduler/enablement_test.go` |
| Same-name smoke fixture | cross-boundary fixture | Checked-in factory fixture reused for generated-boundary decode, runtime load, and functional execution. | `tests/functional_test/testdata/same_name_guard_dir` | `tests/functional_test/same_name_guard_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `SAME_NAME` / `same_name` | input-guard type enum | config authors, OpenAPI clients, generated Go/UI models | config boundary, mapper, replay/runtime load, Petri evaluation | `api/openapi-main.yaml`, `pkg/config/public_factory_enums.go`, `pkg/config/openapi_factory_test.go` |
| `matchInput` / `match_input` | peer input name string | config authors, split `AGENTS.md` frontmatter, generated clients | config validator, config mapper, runtime guard binding | `pkg/config/layout.go`, `pkg/config/agents_config.go`, `pkg/config/config_validator_test.go` |
| Workstation input name | intra-workstation reference key | workstation input declarations | same-name config validator and mapper | `pkg/config/config_validator.go`, `pkg/config/config_mapper_test.go` |

## Lifecycle States

This change does not add or rename lifecycle states. The same-name guard only
changes whether a multi-input transition becomes enabled for the existing input
and output states already authored in the workstation config.

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| `workstations[*].inputs[*].guard` with `type: SAME_NAME` | `interfaces.InputGuardConfig` and OpenAPI `InputGuard` | `type`, `matchInput` | No default peer input; missing `matchInput` is invalid | OpenAPI/generated boundary, split `AGENTS.md` loaders, config validator, config mapper | `api/openapi-main.yaml`, `pkg/config/factory_config_mapping.go`, `pkg/config/layout.go`, `pkg/config/agents_config.go` |
| Internal same-name guard config | `interfaces.InputGuardConfig` canonical internal form | `type: same_name`, `match_input` | Canonical lowercase internal enum after alias normalization | config validator, runtime config load, mapper | `pkg/interfaces/factory_config.go`, `pkg/config/public_factory_enums.go`, `pkg/config/config_validator.go` |
| Same-name smoke fixture | checked-in `factory.json` plus split workstation and worker `AGENTS.md` files | one guarded workstation with two named inputs and a representative worker | no programmatic defaults; fixture is fully authored | generated-boundary decode, `LoadRuntimeConfig(...)`, `ServiceTestHarness` | `tests/functional_test/testdata/same_name_guard_dir/factory.json`, `tests/functional_test/testdata/same_name_guard_dir/workstations/match-items/AGENTS.md` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public same-name config contract | OpenAPI/authored config | generated Go/UI clients and `pkg/config` | Public contract starts in OpenAPI; generated clients and config loaders consume that schema | Unsupported enum spellings, missing `matchInput`, or retired join fields fail at the boundary | `api/openapi-main.yaml`, `pkg/config/openapi_factory_test.go`, `pkg/config/factory_config_mapping_test.go` |
| Config validation and mapping | `pkg/config` | Petri runtime and scheduler enablement | `pkg/config` owns validation, alias normalization, and runtime guard binding | Missing, unknown, or self-referential peer inputs fail validation; wrong peer binding fails mapper tests | `pkg/config/config_validator.go`, `pkg/config/config_mapper.go`, `pkg/config/config_mapper_test.go` |
| Runtime same-name evaluation | Petri guard evaluation | scheduler enablement and functional runtime behavior | runtime consumes the mapped guard binding and compares token names at enablement time | Missing binding or missing authored names fail closed; mismatched names block the transition | `pkg/petri/guard.go`, `pkg/petri/guard_test.go`, `pkg/factory/scheduler/enablement_test.go` |
| Cross-boundary smoke path | checked-in functional fixture | config boundary, runtime load, and service harness | one fixture is reused across all three boundaries instead of mixing authored and programmatic setup | fixture drift fails decode, runtime load, or functional execution in one place | `tests/functional_test/same_name_guard_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  OpenAPI `InputGuard`, generated Go/UI API types, and
  `interfaces.InputGuardConfig` as the shared config meaning.
- Package-local model selected: Petri runtime guard structs and scheduler
  bindings remain package-local execution details behind config mapping.
- Reason: authors, generated clients, split config loaders, runtime config
  load, replay-friendly config boundaries, and functional tests need one stable
  authored guard contract, while Petri guard structs are implementation
  details.
- Translation boundary: `pkg/config` normalizes aliases, validates the authored
  peer-input reference, and maps the public/internal guard into the runtime
  binding consumed by Petri evaluation.
- Review evidence: focused OpenAPI/config/mapper/Petri/scheduler tests, the
  checked-in same-name smoke fixture, and package-level verification commands.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Retired workstation-level join fields | prior public config surface | Keep removed; same-name remains only a per-input guard and does not revive join config | Agent Factory config/API | None |
| Generated API model vs internal config model | generated OpenAPI structs and `interfaces` structs | Keep explicit translation at the config boundary because public names and internal lowercase enums intentionally differ | Agent Factory API/config | None |
| Programmatic same-name smoke setup | older temp-dir smoke scaffolding | Replaced with one checked-in fixture shared across boundary and runtime checks | Agent Factory functional tests | None |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
  The public contract starts in authored OpenAPI, while `pkg/config` owns
  validation and runtime mapping inside the reusable Agent Factory library.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
  This change uses the documented API-contract, shared-library, configuration-
  owner, and scheduler/dispatcher interaction patterns.
- Interop structs or config models that intentionally differ from the canonical
  model: generated OpenAPI structs and internal `interfaces` structs differ
  only at explicit config-mapper boundaries.
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition: none.
- Follow-up cleanup tasks: none.
