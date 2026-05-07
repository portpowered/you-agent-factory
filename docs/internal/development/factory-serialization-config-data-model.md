# Factory Serialization Config Data Model

This artifact records the shared model decisions for the Agent Factory factory
serialization config PRD. It covers the move to generated Factory as the only
serialized config contract for loading, run-request events, replay artifacts,
dashboard consumers, and replay hydration.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory Factory Serialization Config
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory API, replay, runtime config, and dashboard reviewers
- Packages or subsystems: `api/openapi.yaml`, `pkg/api/generated`,
  `pkg/config`, `pkg/replay`, `pkg/service`, `tests/functional_test`, `ui`
- Canonical architecture document to update before completion:
  durable rules moved into `docs/processes/agent-factory-development.md` and
  Agent Factory record/replay docs.

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
| Generated Factory config | shared configuration shape | The self-contained factory payload serialized at record, API event, replay artifact, and dashboard boundaries. | `libraries/agent-factory/api/openapi.yaml` generated as `pkg/api/generated.Factory` | `pkg/replay/generated_factory.go`, `pkg/replay/generated_factory_runtime.go`, `pkg/api/openapi_contract_test.go` |
| Run-request factory payload | event payload | The config seed carried by `RUN_REQUEST.payload.factory`. | Agent Factory OpenAPI event schema | `pkg/replay/event_artifact.go`, `pkg/api/server_test.go`, `ui/src/state/factoryTimelineStore.ts` |
| Replay runtime config | package-local hydration view | Runtime config reconstructed from generated Factory after original files are unavailable. | `pkg/replay.EmbeddedRuntimeConfig` implementing `config.RuntimeConfig` | `pkg/replay/generated_factory_runtime.go`, `pkg/service/factory.go`, `tests/functional_test/factory_only_serialization_smoke_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `factory_dir` / `source_directory` | filesystem path string from the recording run | `pkg/replay.GeneratedFactoryFromLoadedConfig` | replay metadata warnings and artifact diagnostics | `pkg/service/factory_test.go`, `pkg/replay/generated_factory_test.go` |
| `workflow_id` | caller-provided workflow identifier string | `pkg/service` record setup | replay metadata comparison and event consumers | `pkg/replay/generated_factory_test.go`, `pkg/service/factory.go` |
| workstation and worker names | names from generated Factory arrays | `pkg/config` and generated Factory serialization | replay hydration, topology projection, dashboard reducers | `pkg/replay/effective_config_test.go`, `ui/src/state/factoryTimelineStore.test.ts` |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| `pkg/api/generated.Factory` | OpenAPI schema | Work types, workers, workstations, resources, and replay metadata fields when present; guarded loop breakers are authored through workstation guards instead of a top-level exhaustion-rules contract | Runtime worker/workstation fields are embedded in generated worker/workstation entries; workstation stop handling serializes through one canonical `stopWords` array, and worker/workstation runtime resource declarations serialize through the shared `resources[{name,capacity}]` contract. | `pkg/replay`, `pkg/service`, `pkg/api`, dashboard UI, fixtures | `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, `pkg/replay/generated_factory_test.go` |
| `replay.EmbeddedRuntimeConfig` | `pkg/replay` | Factory, worker configs, workstation configs, lookup maps | Built entirely from `RUN_REQUEST.payload.factory`; no filesystem fallback in replay mode. | `pkg/service`, workers, topology projection | `pkg/replay/generated_factory_runtime.go`, `pkg/service/factory.go` |
| Replay artifact JSON | `pkg/replay` | schema version, recorded time, generated Factory event log | Stored artifacts canonicalize current run-request config to `payload.factory`. | replay loader, fixture tests, functional tests | `pkg/replay/artifact.go`, `pkg/replay/fixture_safety_test.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| OpenAPI generated Factory | `api/openapi.yaml` | Go API server, replay package, dashboard TypeScript types, fixtures | Consumers depend on generated schema fields and translate at package boundaries. | Missing factory payload rejects replay hydration and contract tests reject legacy-only config. | `pkg/api/openapi_contract_test.go`, `pkg/replay/artifact_test.go` |
| Record artifact creation | `pkg/service` and `pkg/replay` | replay loader, event stream, dashboard reducers | Service passes loaded runtime config to replay serialization; replay emits generated event payloads. | Serialization errors abort recording setup with context. | `pkg/service/factory.go`, `pkg/service/factory_test.go` |
| Replay hydration | `pkg/replay` | `pkg/service` replay mode, topology projection, functional harness | Replay package owns config hydration and returns the `config.RuntimeConfig` interface. | Empty factory payload returns a replay artifact config error. | `pkg/replay/generated_factory_runtime.go`, `tests/functional_test/factory_only_serialization_smoke_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  generated `pkg/api/generated.Factory` from `api/openapi.yaml`.
- Package-local model selected: `replay.EmbeddedRuntimeConfig` remains
  package-local because it adapts generated Factory into the runtime config
  interface and is not a serialized contract.
- Reason: API events, replay artifacts, fixtures, and dashboard consumers need
  one stable serialized config meaning; runtime packages still need a local
  lookup shape for execution.
- Translation boundary: only `pkg/replay` converts generated Factory into
  `config.RuntimeConfig`; service and UI consumers use generated Factory payloads.
- Review evidence: generated contract tests, replay unit tests, dashboard
  reducer tests, fixture guards, and factory-only functional smoke.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Retired replay config wrapper | prior replay/event payload path | Removed from generated schema, current artifacts, and active runtime code. | Agent Factory replay | No follow-up; fixture guards reject reintroduction. |
| Hidden runtime config side map | prior artifact payload extension | Removed; runtime definitions live in generated worker/workstation entries. | Agent Factory replay | No follow-up; serialization tests assert forbidden keys are absent. |
| Dashboard config seed | run-started event and initial-structure event | Both consume generated Factory projection helpers. | Agent Factory UI | No follow-up; reducer tests cover both event shapes. |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical
  model: `replay.EmbeddedRuntimeConfig` is local hydration state and is excluded
  from artifact JSON.
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition: none.
- Follow-up cleanup tasks: none discovered during this merge iteration.
