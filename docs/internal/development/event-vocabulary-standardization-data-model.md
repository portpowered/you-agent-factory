# Event Vocabulary Standardization Data Model

This artifact records the approved public Agent Factory event vocabulary before
runtime emitters, generated models, reducers, and fixtures are migrated in
later stories. It keeps one canonical old-to-new mapping across the OpenAPI
contract, generated clients, runtime emitters, replay consumers, fixtures, and
docs.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory Event Vocabulary Standardization
- Owner: Agent Factory maintainers
- Packages or subsystems: `api/openapi.yaml`, `pkg/api/openapi_contract_test.go`,
  `pkg/api/testdata`, `docs`, generated consumers in later stories
- Canonical architecture document to update before completion:
  `libraries/agent-factory/docs/record-replay.md` and
  `libraries/agent-factory/docs/run-timeline.md` in the migration/documentation
  stories
- Canonical package responsibility artifact: `api/openapi.yaml` owns the public
  event vocabulary; generated `pkg/api/generated` and `ui/src/api/generated`
  mirror it; runtime emitters and replay consumers translate only by using that
  canonical contract.
- Canonical package interaction artifact: generated `FactoryEvent` is the
  shared event boundary across runtime emission, replay, projections, API
  streaming, and checked-in fixtures.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Lifecycle state or status value
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Boundary Rules

- `WORK_REQUEST`, `INFERENCE_REQUEST`, and `INFERENCE_RESPONSE` stay unchanged
  because they already communicate canonical request/response semantics.
- This story approves the vocabulary and publishes the canonical enum list plus
  a canonical raw JSON fixture. Generated payload schema renames and runtime
  emission changes follow in later stories.
- Retired names such as `*_STARTED`, `*_FINISHED`, `*_CREATED`,
  `*_COMPLETED`, and bare `*_CHANGE` are removed from the published enum list
  as soon as the approved mapping is adopted.

## Shared Vocabulary

| Current event name | Canonical event name | Classification | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `RUN_STARTED` | `RUN_REQUEST` | request-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `INITIAL_STRUCTURE` | `INITIAL_STRUCTURE_REQUEST` | request-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `WORK_REQUEST` | `WORK_REQUEST` | intentionally unchanged | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | Already canonical request-style vocabulary |
| `RELATIONSHIP_CHANGE` | `RELATIONSHIP_CHANGE_REQUEST` | request-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `DISPATCH_CREATED` | `DISPATCH_REQUEST` | request-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `INFERENCE_REQUEST` | `INFERENCE_REQUEST` | intentionally unchanged | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | Already canonical request-style vocabulary |
| `INFERENCE_RESPONSE` | `INFERENCE_RESPONSE` | intentionally unchanged | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | Already canonical response-style vocabulary |
| `DISPATCH_COMPLETED` | `DISPATCH_RESPONSE` | response-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `FACTORY_STATE_CHANGE` | `FACTORY_STATE_RESPONSE` | response-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |
| `RUN_FINISHED` | `RUN_RESPONSE` | response-style | `libraries/agent-factory/api/openapi.yaml#/components/schemas/FactoryEventType` | PRD approved baseline mapping |

## Payload Schema Mapping

| Current payload schema | Canonical payload schema | Classification | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `RunStartedEventPayload` | `RunRequestEventPayload` | request-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `InitialStructureEventPayload` | `InitialStructureRequestEventPayload` | request-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `WorkRequestEventPayload` | `WorkRequestEventPayload` | intentionally unchanged | OpenAPI payload schema family | Already canonical request-style vocabulary |
| `RelationshipChangeEventPayload` | `RelationshipChangeRequestEventPayload` | request-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `DispatchCreatedEventPayload` | `DispatchRequestEventPayload` | request-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `InferenceRequestEventPayload` | `InferenceRequestEventPayload` | intentionally unchanged | OpenAPI payload schema family | Already canonical request-style vocabulary |
| `InferenceResponseEventPayload` | `InferenceResponseEventPayload` | intentionally unchanged | OpenAPI payload schema family | Already canonical response-style vocabulary |
| `DispatchCompletedEventPayload` | `DispatchResponseEventPayload` | response-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `FactoryStateChangeEventPayload` | `FactoryStateResponseEventPayload` | response-style | OpenAPI payload schema family | PRD payload-schema mapping |
| `RunFinishedEventPayload` | `RunResponseEventPayload` | response-style | OpenAPI payload schema family | PRD payload-schema mapping |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| `FactoryEventType` enum list | OpenAPI contract | Generated Go/UI clients, runtime emitters, replay reducers, fixtures | `api/openapi.yaml` -> generated/models/tests/docs | Reintroducing retired names creates contract drift across public surfaces | `pkg/api/openapi_contract_test.go` canonical vocabulary assertions |
| Canonical raw event-stream fixture | `pkg/api/testdata/canonical-event-vocabulary-stream.json` | OpenAPI contract tests and future migration stories | Test fixture -> contract guard only | Fixture drift can advertise retired names before runtime is migrated | `pkg/api/openapi_contract_test.go` fixture validation |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  generated `FactoryEvent` contract rooted in `api/openapi.yaml`
- Package-local model selected: none for the approved vocabulary itself
- Reason: event names are customer-visible and stable across API, replay,
  runtime emission, and generated consumers, so the vocabulary must live on one
  shared generated contract instead of package-local copies
- Translation boundary: later runtime and replay stories will translate old
  emitter/consumer usage directly to the canonical generated enum names
- Review evidence: `pkg/api/openapi_contract_test.go` asserts the published enum
  list and validates the canonical fixture

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Legacy public event names in historical docs, fixtures, and runtime code | runtime emitters, replay fixtures, docs | follow-up | Agent Factory maintainers | Covered by US-002 through US-006 in `prd.json` |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical
  model: generated payload schema type names still use legacy identifiers until
  US-002 renames them together with regenerated clients.
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition: none in this story.
- Follow-up cleanup tasks: OpenAPI payload schema renames, generated client
  refresh, runtime emission updates, replay reducer updates, and migration docs
  remain in the existing PRD stories.
