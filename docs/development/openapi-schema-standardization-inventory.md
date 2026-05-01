# OpenAPI Schema Standardization Inventory

This inventory records the authored OpenAPI component-schema surface before the
schema-file cleanup stories move the remaining inline definitions out of
`api/openapi-main.yaml`.

## Scope

- Authored entrypoint: `api/openapi-main.yaml`
- Bundled published artifact: `api/openapi.yaml`
- Existing checked-in fragment family:
  `api/components/schemas/events/` and
  `api/components/schemas/events/payloads/`
- Generated downstream surfaces:
  `pkg/api/generated/server.gen.go` and `ui/src/api/generated/openapi.ts`

## Current Authored Component Layout

### Already authored as checked-in fragment files

The event contract family already uses one-file-per-schema fragments under
`api/components/schemas/events/`:

- Envelope and shared event helpers:
  `FactoryEvent`, `FactoryEventType`, `FactoryEventContext`,
  `DispatchConsumedWorkRef`, `DispatchRequestEventMetadata`, `FactoryState`,
  `InferenceOutcome`, `ProviderDiagnostic`, `ProviderFailureMetadata`,
  `ProviderSessionMetadata`, `RenderedPromptDiagnostic`,
  `SafeWorkDiagnostics`, `ScriptExecutionOutcome`, `ScriptFailureType`,
  `WallClock`, `WorkDiagnostics`, `WorkMetrics`, `WorkOutcome`, `Diagnostics`
- Event payload fragments:
  `RunRequestEventPayload`, `InitialStructureRequestEventPayload`,
  `WorkRequestEventPayload`, `RelationshipChangeRequestEventPayload`,
  `DispatchRequestEventPayload`, `InferenceRequestEventPayload`,
  `InferenceResponseEventPayload`, `ScriptRequestEventPayload`,
  `ScriptResponseEventPayload`, `DispatchResponseEventPayload`,
  `FactoryStateResponseEventPayload`, `RunResponseEventPayload`

`api/openapi-main.yaml` already references these schemas instead of redefining
their bodies inline.

### Still authored inline in `api/openapi-main.yaml`

Every published component schema below is still defined inline today and is in
scope for the follow-on extraction stories.

#### Shared runtime helpers

- `ResourceUsage`
- `ResourceRequirement`
- `StringMap`
- `IntegerMap`
- `CommandDiagnostic`
- `PanicDiagnostic`

#### Request, response, and error schemas

- `SubmitWorkRequest`
- `SubmitWorkResponse`
- `UpsertWorkRequestResponse`
- `ListWorkResponse`
- `PaginationContext`
- `TokenResponse`
- `TokenHistory`
- `StatusCategories`
- `StatusResponse`
- `ErrorFamily`
- `ErrorResponse`
- `FactoryName`
- `NamedFactory`
- `WorkRequest`
- `WorkRequestType`
- `Work`
- `Relation`
- `RelationType`

#### Additive factory-world and dashboard-facing read models

- `FactoryWorldWorkstationRequestProjectionSlice`
- `FactoryWorldRenderedPromptDiagnostic`
- `FactoryWorldProviderDiagnostic`
- `FactoryWorldWorkDiagnostics`
- `FactoryWorldWorkItemRef`
- `FactoryWorldTokenView`
- `FactoryWorldMutationView`
- `FactoryWorldScriptRequestView`
- `FactoryWorldScriptResponseView`
- `FactoryWorldWorkstationRequestCountView`
- `FactoryWorldWorkstationRequestRequestView`
- `FactoryWorldWorkstationRequestResponseView`
- `FactoryWorldWorkstationRequestView`

#### Factory-config and named-factory public contract

- `Factory`
- `ResourceManifest`
- `RequiredTool`
- `BundledFile`
- `BundledFileContent`
- `InputType`
- `InputKind`
- `WorkType`
- `WorkState`
- `WorkStateType`
- `Resource`
- `Worker`
- `WorkerType`
- `WorkerModelProvider`
- `WorkerProvider`
- `Workstation`
- `WorkstationLimits`
- `WorkstationKind`
- `WorkstationType`
- `WorkstationCron`
- `WorkstationGuardType`
- `WorkstationGuard`
- `WorkstationGuardMatchConfig`
- `WorkstationIO`
- `InputGuard`
- `InputGuardType`
- `Transition`

## Target Fragment Layout

The cleanup stories should keep the existing one-schema-per-file event pattern
and extend it to the remaining published component families:

```text
api/components/schemas/
  events/
    *.yaml
    payloads/*.yaml
  runtime/
    SubmitWorkRequest.yaml
    SubmitWorkResponse.yaml
    UpsertWorkRequestResponse.yaml
    ListWorkResponse.yaml
    PaginationContext.yaml
    TokenResponse.yaml
    TokenHistory.yaml
    StatusCategories.yaml
    StatusResponse.yaml
    ErrorFamily.yaml
    ErrorResponse.yaml
    WorkRequest.yaml
    WorkRequestType.yaml
    Work.yaml
    Relation.yaml
    RelationType.yaml
  factory-config/
    Factory.yaml
    ResourceManifest.yaml
    RequiredTool.yaml
    BundledFile.yaml
    BundledFileContent.yaml
    InputType.yaml
    InputKind.yaml
    WorkType.yaml
    WorkState.yaml
    WorkStateType.yaml
    Resource.yaml
    Worker.yaml
    WorkerType.yaml
    WorkerModelProvider.yaml
    WorkerProvider.yaml
    Workstation.yaml
    WorkstationLimits.yaml
    WorkstationKind.yaml
    WorkstationType.yaml
    WorkstationCron.yaml
    WorkstationGuardType.yaml
    WorkstationGuard.yaml
    WorkstationGuardMatchConfig.yaml
    WorkstationIO.yaml
    InputGuard.yaml
    InputGuardType.yaml
    Transition.yaml
    FactoryName.yaml
    NamedFactory.yaml
  factory-world/
    FactoryWorld*.yaml
  shared/
    ResourceUsage.yaml
    ResourceRequirement.yaml
    StringMap.yaml
    IntegerMap.yaml
    CommandDiagnostic.yaml
    PanicDiagnostic.yaml
```

## Naming Convention

- One published component schema per file.
- File names stay `PascalCase.yaml` and match the OpenAPI component key exactly.
- `api/openapi-main.yaml` remains the authored entrypoint and should reference
  fragment files with relative `$ref`s.
- Generated artifacts remain derived outputs only. Authors should never
  hand-edit `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, or
  `ui/src/api/generated/openapi.ts`.

## Verification Surfaces That Must Stay Aligned

- Bundle step: `make bundle-api`
- Regeneration step: `make generate-api`
- Canonical OpenAPI verification path: `make api-smoke`
- Bundled contract tests in `pkg/api/openapi_contract_test.go`
- Generated contract coverage in `pkg/api/generated_contract_test.go`
- Live generated/runtime smoke coverage in
  `tests/functional_test/generated_api_smoke_test.go`

## Notes For Follow-On Stories

- The `/events` route already points `x-event-schema` at the canonical
  `FactoryEvent` schema. Later stories should preserve that reference while
  moving any remaining non-event inline schemas into fragment files.
- The extraction story should move structure first and preserve current field
  names, enums, descriptions, examples, and validation rules before the
  camelCase rename story intentionally changes public property names.
