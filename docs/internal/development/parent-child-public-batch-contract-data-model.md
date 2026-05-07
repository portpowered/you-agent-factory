# Parent-Child Public Batch Contract Data Model

This artifact records the shared-model decisions for the first public
`FACTORY_REQUEST_BATCH` parent-child contract slice. It covers the public batch
request fields, boundary validation rules, canonical event payloads, and the
world-state reconstruction contract touched by this branch.

## Change

- PRD, design, or issue: `prd.json` for `batch-parent-child-public-contract`
- Owner: Agent Factory maintainers
- Packages or subsystems: `pkg/interfaces`, `pkg/factory`, `pkg/api`,
  `pkg/listeners`, `pkg/replay`, `pkg/service`, `ui`, docs, and functional or
  contract tests
- Canonical architecture document to update before completion: this file is the
  branch data-model construction artifact. Durable submit-boundary rules live
  in `docs/processes/agent-factory-development.md` and the customer-facing
  contract guide lives in `libraries/agent-factory/docs/guides/batch-inputs.md`.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [x] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| Work request | public batch contract | One accepted `FACTORY_REQUEST_BATCH` containing submitted work items plus optional intra-batch relations. | `interfaces.WorkRequest` and generated `factoryapi.WorkRequest` | `pkg/interfaces/factory_runtime.go`, `api/openapi.yaml`, `pkg/api/openapi_contract_test.go` |
| Work item state | public initial-state field | Optional public `works[].state` that selects the submitted work item's initial placement when valid for that work type. | generated `factoryapi.Work`, `interfaces.Work`, boundary normalization in `factory.NormalizeWorkRequest(...)` | `api/openapi.yaml`, `pkg/factory/work_request.go`, `pkg/api/server_test.go` |
| Parent-child relation | public batch relation | `PARENT_CHILD` relation from `source_work_name` child to `target_work_name` parent within the same batch request. | `interfaces.WorkRelation`, generated `factoryapi.Relation` | `pkg/interfaces/factory_runtime.go`, `pkg/factory/work_request.go`, `pkg/api/generated_contract_test.go` |
| Boundary validation | submit safety rule | Invalid relation names, duplicate work names, self-parenting, duplicate edges, retired aliases, and invalid state values reject the full request before work creation. | `factory.NormalizeWorkRequest(...)` | `pkg/factory/work_request.go`, `pkg/factory/work_request_test.go`, `tests/functional_test/factory_request_batch_test.go` |
| Canonical request history | event payload | `WORK_REQUEST` and `RELATIONSHIP_CHANGE_REQUEST` events preserve batch work membership and relation meaning for replay and reporting. | generated `factoryapi.WorkRequestEventPayload` and `factoryapi.RelationshipChangeRequestEventPayload` | `pkg/factory/event_history.go`, `pkg/api/generated/server.gen.go`, `pkg/replay/event_artifact.go` |
| World-state relation replay | projection contract | Reconstructed relation source IDs resolve from the recorded request work-item names for the same request before any event-context fallback. | `factory.ReconstructFactoryWorldState(...)` | `pkg/factory/projections/world_state.go`, `pkg/factory/projections/world_state_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `request_id` | stable batch request string | API handler, watched-file loader, service helpers, replay artifacts | submit normalization, event history, replay, world-state projection | `pkg/factory/work_request.go`, `pkg/api/handlers.go`, `pkg/factory/projections/world_state.go` |
| `work_id` | stable per-work item ID | batch normalization or explicit input | token construction, event history, replay, projections | `pkg/factory/work_request.go`, `pkg/factory/event_history.go`, `pkg/factory/projections/world_state.go` |
| `source_work_name` | submitted work-item name | public batch relations | batch normalization and world-state replay | `pkg/factory/work_request.go`, `pkg/factory/projections/world_state.go` |
| `target_work_name` | submitted work-item name | public batch relations | batch normalization and world-state replay | `pkg/factory/work_request.go`, `pkg/factory/projections/world_state.go` |
| `state` | configured work-type state name | public batch work item | batch normalization, engine submission, replay and event history | `api/openapi.yaml`, `pkg/factory/work_request.go`, `pkg/factory/event_history.go` |

## Lifecycle States

This slice adds customer-facing initial-state input but does not add new
runtime lifecycle states. Submitted `state` values must match the selected
work type's configured states before work creation.

| State concept | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| Submitted initial state | `interfaces.Work.TargetState` after normalization | Public boundary may seed a valid configured state before token creation; later runtime transitions remain owned by the Petri net and workstation definitions. | No | `pkg/factory/work_request.go`, `pkg/factory/engine/engine.go`, `pkg/replay/event_artifact.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public batch API and watched-file input | HTTP handler and file watcher | service, runtime, engine | Public boundaries decode canonical `FACTORY_REQUEST_BATCH` and submit `interfaces.WorkRequest` through `SubmitWorkRequest`. | Invalid relation names, retired aliases, unsupported relation types, self-parenting, and invalid `state` values reject the whole batch before work creation. | `pkg/api/handlers.go`, `pkg/listeners/filewatcher.go`, `pkg/service/factory.go`, focused tests |
| Runtime normalization | `interfaces.WorkRequest` boundary payload | engine token construction and history | `factory.NormalizeWorkRequest(...)` owns canonical validation and flattening into runtime submission records. | Errors are atomic and prevent partial request creation. | `pkg/factory/work_request.go`, `pkg/factory/engine/engine_test.go`, `tests/functional_test/factory_request_batch_test.go` |
| Canonical event history | runtime event recorder | replay, projections, dashboard consumers | Event history records `WORK_REQUEST` before relation changes or work-input details; replay and reporting consume generated event payloads. | Missing or malformed relation endpoints are ignored only when canonical payload data is absent; normal submitted requests must replay deterministically. | `pkg/factory/event_history.go`, `pkg/replay/delivery_test.go`, `pkg/factory/projections/world_state_test.go` |
| Public docs and generated contract | OpenAPI source and guide docs | API clients, watched-file authors, UI generated types, reviewers | `api/openapi.yaml` remains the public schema source, with one canonical guide payload shared across API and watched-file examples. | Contract or doc drift fails generated or focused contract checks. | `api/openapi.yaml`, `ui/src/api/generated/openapi.ts`, `docs/guides/batch-inputs.md`, `pkg/api/generated_contract_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  generated `factoryapi.WorkRequest` and `factoryapi.Relation` for the public
  request shape; `interfaces.WorkRequest`, `interfaces.Work`, and
  `interfaces.WorkRelation` for runtime normalization and service boundaries;
  generated factory event payloads for canonical replay and reporting.
- Package-local model selected: flattened runtime `interfaces.SubmitRequest`
  records remain private to token construction after normalization.
- Reason: the public contract is shared across API, watched-file, replay,
  generated UI types, and docs, while token construction still benefits from a
  private normalized item representation.
- Translation boundary: API and file-watcher boundaries map public request
  fields into runtime interfaces explicitly, and projections reconstruct batch
  relations from canonical request history rather than inventing a second
  relation model.

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact:
  `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Relevant interaction patterns: API contract, shared-library contract, and
  event or message contract. The branch keeps public field names in generated
  schemas and performs explicit boundary translation into runtime interfaces.
- Approved exceptions: none.
