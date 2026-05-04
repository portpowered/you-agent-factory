# Retire Legacy Unary SubmitRequest Ingress Data Model

This artifact records the shared model decisions for retiring the legacy unary
`SubmitRequest` ingress. It covers the canonical public submit batch, generated
batch output, retained internal token-construction item, and cross-package
contracts changed by the cleanup.

## Change

- PRD, design, or issue: `prd.json` for Retire Legacy Unary SubmitRequest
  Ingress
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory runtime/API/CLI/replay reviewers
- Packages or subsystems: `pkg/interfaces`, `pkg/factory`, `pkg/service`,
  `pkg/api`, `pkg/cli`, `pkg/listeners`, `pkg/replay`, `pkg/timework`,
  functional and stress tests
- Canonical architecture document to update before completion: this artifact is
  the branch data-model construction artifact. Durable submit-boundary rules
  are captured in `docs/processes/agent-factory-development.md`,
  `libraries/agent-factory/docs/work.md`, and the cleanup report under
  `libraries/agent-factory/docs/development/cleanup-analyzer-reports/`.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| Work request | public batch contract | One accepted submit request containing one or more work items plus optional intra-batch relations. | `interfaces.WorkRequest` and generated `factoryapi.WorkRequest` | `pkg/interfaces/factory_runtime.go`, `libraries/agent-factory/api/openapi.yaml`, `pkg/api/openapi_contract_test.go` |
| Work item | public batch item | One requested unit of work inside a work request. Public JSON uses `work_type_name`; runtime stores the value in `WorkTypeID` for token construction. | `interfaces.Work`, generated `factoryapi.Work` | `pkg/interfaces/factory_runtime.go`, `pkg/api/handlers.go`, `pkg/factory/work_request.go` |
| Direct submit request | API convenience input | `POST /work` single-work convenience payload that handlers immediately wrap into a one-item `WorkRequest`. | generated `factoryapi.SubmitWorkRequest`; API handler mapper | `libraries/agent-factory/api/openapi.yaml`, `pkg/api/handlers.go`, `pkg/api/server_test.go` |
| Generated submission batch | internal engine/hook output | Canonical generated request emitted by hooks, replay, or worker fanout, with metadata for source, relation context, and parent lineage. | `interfaces.GeneratedSubmissionBatch` | `pkg/interfaces/factory_runtime.go`, `pkg/interfaces/factory_hooks.go`, `pkg/factory/engine/engine.go` |
| Internal submit item | private token-construction item | Flat normalized item used after batch normalization so token construction can keep byte payloads, target state, execution ID, and runtime relations. Not a public JSON contract. | `interfaces.SubmitRequest` normalized through `pkg/factory.WorkRequestFromSubmitRequests(...)` | `pkg/interfaces/factory_runtime.go`, `pkg/factory/work_request.go`, cleanup report retained exceptions |
| Work request record | event-history request observation | Batch-level record emitted before per-work input/token history so consumers can reconstruct accepted request membership and ordering. | `interfaces.WorkRequestRecord`; runtime event history | `pkg/interfaces/factory_runtime.go`, `pkg/factory/runtime/factory.go`, `pkg/factory/engine/engine.go` |
| Work relation | public dependency edge | Named work-item dependency inside a batch. Normalization resolves it into runtime relations on concrete work IDs. | `interfaces.WorkRelation` and generated `factoryapi.Relation` | `pkg/interfaces/factory_runtime.go`, `pkg/api/handlers.go`, `pkg/factory/work_request.go` |
| Work request event payload | replay and audit event payload | Canonical `WORK_REQUEST` event payload persisted in generated event logs before related input events. | generated `factoryapi.WorkRequestEventPayload` | `pkg/api/generated/server.gen.go`, `pkg/replay/event_reducer.go`, `pkg/api/generated_contract_test.go` |
| Cron time work request | internal producer batch | One-item `FACTORY_REQUEST_BATCH` for internal `__system_time` work submitted by cron sidecars through `SubmitWorkRequest`. | `pkg/timework.CronTimeWorkRequest` | `pkg/timework/cron.go`, `pkg/service/cron_watcher.go`, `pkg/timework/cron_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `request_id` | stable string, explicit on idempotent/batch paths and generated when absent on convenience paths | API, CLI, file watcher, startup work file, cron, replay, generated batches | factory engine, runtime event history, replay reducer | `pkg/factory/work_request.go`, `pkg/api/server_test.go`, `pkg/factory/engine/engine_test.go` |
| `work_id` | stable string per work item; generated when absent | work-request normalization and generated batch producers | token construction, dispatch history, event history | `pkg/factory/work_request.go`, `pkg/factory/runtime/factory_test.go` |
| `work_type_name` | configured `work_types[].name` public string | public API/CLI/file watcher/startup files, generated API events | API mappers, file watcher, normalizer, token construction | `pkg/api/openapi_contract_test.go`, `pkg/listeners/filewatcher_test.go`, `pkg/cli/submit/submit_test.go` |
| `work_type_id` | internal token/read-model field only | token construction and read-model projections | runtime internals, dashboard/read-model consumers, negative alias tests | cleanup report final inventory and retained exceptions |
| `trace_id` | stable trace string on request or item | submit callers, handler mappers, generated batch metadata | accepted metadata, event history, dispatch diagnostics | `pkg/api/server_test.go`, `pkg/factory/runtime/factory_test.go` |
| `__system_time` | internal work type name for cron coordination | `pkg/timework.CronTimeWorkRequest` | service cron watcher, Petri guards, replay reducer, public read-model filters | `pkg/timework/cron.go`, `pkg/replay/delivery_test.go`, functional retirement smoke |

## Lifecycle States

This PR does not add customer-facing lifecycle states. Existing factory work
states still come from `factory.json` work-type state definitions. The only
state-like submit data retained by this cleanup is private runtime target state
on internal normalized items, used for cron and replay reconstruction.

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| Internal submit target state | `interfaces.SubmitRequest.TargetState`, `interfaces.Work.TargetState` | Boundary mappers may set the initial or pending target state before token creation; Petri runtime state transitions remain owned by workstation definitions. | No | `pkg/interfaces/factory_runtime.go`, `pkg/timework/cron.go`, `pkg/replay/event_reducer.go` |

## Configuration Shapes

This PR does not introduce a new shared configuration shape. `FactoryServiceConfig.WorkFile`
and `agent-factory run --work` remain configuration inputs that point to a file,
but the file content is now the canonical `WorkRequest` payload instead of a
flat `SubmitRequest`.

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Startup work file payload | `interfaces.WorkRequest` | `request_id`, `type: FACTORY_REQUEST_BATCH`, non-empty `works` | None at the file-content contract; callers must provide canonical batch JSON. | `pkg/service`, `pkg/cli/run` | `pkg/service/factory.go`, `pkg/cli/run/run.go`, functional retirement smoke |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Factory submit ingress | API, CLI, listeners, service startup, cron, replay, tests | `pkg/service`, `pkg/factory/runtime`, `pkg/factory/engine` | Consumers call `SubmitWorkRequest(ctx, interfaces.WorkRequest)`; public unary `Submit(ctx, []SubmitRequest)` is removed. | Validation errors reject the whole batch before token creation; duplicate `request_id` returns accepted metadata without duplicate history. | `pkg/factory/interfaces.go`, `pkg/service/factory.go`, `pkg/factory/engine/engine.go`, runtime/API idempotency tests |
| Public direct submit API | HTTP clients and CLI submit command | `pkg/api` handler and generated API model | Generated OpenAPI model uses `work_type_name`; handler maps to domain `WorkRequest` before calling service/factory. | `work_type_id` is rejected with a clear validation error. | `libraries/agent-factory/api/openapi.yaml`, `pkg/api/handlers.go`, `pkg/api/server_test.go`, `pkg/cli/submit/submit_test.go` |
| Public batch API and file input | HTTP `PUT /work-requests/{request_id}`, `agent-factory run --work`, file watcher structured JSON | API handler, CLI run loader, service, file watcher | Boundary code decodes `FACTORY_REQUEST_BATCH` into `interfaces.WorkRequest` and submits through `SubmitWorkRequest`. | Missing type/request/work fields, rejected public alias fields such as `work_type_id` and `works[].target_state`, invalid relation targets, and watched-folder conflicts fail before partial submission. | `pkg/api/handlers.go`, `pkg/cli/run/run.go`, `pkg/service/factory.go`, `pkg/listeners/filewatcher.go` |
| Generated hook output | submission hooks, replay delivery, worker fanout transitioner | factory engine | Hooks and tick results return `GeneratedSubmissionBatch`; engine records canonical request/input history. | Invalid generated batches fail at normalization; legacy mixed generated/unary suppression is removed. | `pkg/interfaces/factory_hooks.go`, `pkg/interfaces/engine_runtime.go`, `pkg/factory/engine/engine.go`, engine/replay tests |
| Replay event reconstruction | generated `WORK_REQUEST` event log | `pkg/replay` submission hook | Replay reduces generated `FactoryEvent` logs into `interfaces.WorkRequest` batches and re-enters through generated batch processing. | Missing older-artifact fields are enriched at the replay boundary where compatibility is intentional. | `pkg/replay/event_reducer.go`, `pkg/replay/delivery_test.go` |
| Cron internal time submission | `pkg/timework` and service cron watcher | `pkg/service`, factory ingress, replay/event history | Cron builds a one-item canonical batch and calls `SubmitWorkRequest`; dispatch readiness stays in Petri guards. | Cron builder rejects invalid workstation/time-work metadata before submission. | `pkg/timework/cron.go`, `pkg/service/cron_watcher.go`, `pkg/timework/cron_test.go` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `interfaces.WorkRequest`, `interfaces.Work`, `interfaces.WorkRelation`,
  generated OpenAPI `WorkRequest`, generated `SubmitWorkRequest` for the direct
  API convenience payload, `interfaces.GeneratedSubmissionBatch`, and generated
  `WorkRequestEventPayload`.
- Package-local model selected: `interfaces.SubmitRequest` remains as an
  internal normalized item for token construction and generated-batch
  enrichment. It is not exposed as a factory/service ingress or accepted as a
  public file/API/CLI schema.
- Reason: runtime, service, API, CLI, file watcher, replay, cron, hooks, and
  tests need one stable batch meaning, while token construction still benefits
  from a flat private item with byte payloads and runtime-only fields.
- Translation boundary: API handlers map generated public models to domain
  `interfaces.WorkRequest`; file watcher/startup loaders decode canonical
  batch JSON; internal helpers flatten normalized batch works into
  `SubmitRequest` only after public validation.
- Review evidence: cleanup analyzer report, API/CLI/listener/engine/replay/cron
  behavioral tests, and
  `TestLegacyUnaryRetirementSmoke_CanonicalSubmitPathsStayBatchOnly`.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Public unary factory/service submit ingress | `Factory.Submit(ctx, []SubmitRequest)` and service wrappers | Removed; all active submit callers use `SubmitWorkRequest`. | Agent Factory runtime/service | Complete in US-001. |
| Public `SubmitRequest` JSON/file shape | API/CLI/file watcher/startup files | Removed from public boundaries; canonical `FACTORY_REQUEST_BATCH` remains. | Agent Factory API/CLI/listener/service | Complete in US-002, US-005, US-007, and PR review follow-up. |
| Hook unary output | `HookSubmission`, `SubmissionHookResult.Submissions` | Removed; hooks return generated batches. | Agent Factory engine/hooks | Complete in US-003. |
| Tick-result legacy request/input records | `TickResult.WorkRequests`, `TickResult.WorkInputs` | Removed; tick results emit generated batches only. | Agent Factory engine/subsystems | Complete in US-004. |
| `work_type_id` public aliases | OpenAPI/API/CLI/docs/examples | Removed or converted to negative tests; retained matches are internal token/read-model fields. | Agent Factory public boundaries | Complete in US-007 and cleanup report. |
| Generated API model vs domain model | `factoryapi.WorkRequest` and `interfaces.WorkRequest` | Kept as explicit boundary translation so OpenAPI remains the public schema and runtime interfaces remain library-local. | Agent Factory API/runtime | No follow-up; mapper tests guard drift. |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
  Agent Factory lives under the `libraries/` responsibility because it is a
  reusable Go module with exported library/API/CLI contracts.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
  The submit cleanup uses the shared-library, API-contract, event-contract, and
  scheduler/dispatcher interaction patterns; it does not add an undocumented
  special-case interaction.
- Interop structs or config models that intentionally differ from the canonical
  model: generated OpenAPI models differ from runtime `interfaces` structs only
  at explicit API mappers; `interfaces.SubmitRequest` remains private to token
  construction and generated-batch enrichment.
- Approved exceptions: none open-ended. The retained internal `work_type_id`
  and `SubmitRequest` fields are not public ingress contracts and are bounded
  by the cleanup report inventories plus boundary rejection tests.
- Follow-up cleanup tasks: none from this artifact.
