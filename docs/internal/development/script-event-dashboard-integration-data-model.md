# Script Event Dashboard Integration Data Model

This artifact records the shared script-execution request and response model
used by the Agent Factory public event contract, canonical selected-tick world
state, workstation-request projection, and dashboard replay/detail UI.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory Script Event Dashboard
  Integration (`US-001` through `US-005`)
- Owner: Codex branch `ralph/agent-factory-script-event-dashboard-integration`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `libraries/agent-factory/api/components/schemas/events`,
  `libraries/agent-factory/api/openapi-main.yaml`,
  `libraries/agent-factory/pkg/interfaces`,
  `libraries/agent-factory/pkg/factory/projections`,
  `libraries/agent-factory/pkg/api`,
  `libraries/agent-factory/ui/src/api/generated`,
  `libraries/agent-factory/ui/src/state`,
  `libraries/agent-factory/ui/src/features/current-selection`
- Canonical architecture document to update before completion:
  `docs/processes/agent-factory-development.md`
- Canonical package responsibility artifact:
  `docs/architecture/package-responsibilities.md` plus the Agent Factory
  authored OpenAPI source in `libraries/agent-factory/api/openapi-main.yaml`.
  Public script event payloads and workstation-request view schemas are owned by
  the API contract; `pkg/interfaces.FactoryWorldState` owns the canonical
  reduced in-memory model; `pkg/api` owns translation into the generated
  workstation-request projection surface.
- Canonical package interaction artifact:
  `docs/architecture/package-interactions.md`. Generated
  `pkg/api/generated.FactoryEvent`,
  `pkg/api/generated.FactoryWorldWorkstationRequest*`, and
  `ui/src/api/generated/openapi.ts` are the reviewed cross-package boundary from
  event stream to Go projections to UI consumers.

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
| `script request` | noun | Canonical request-side execution boundary emitted when a dispatch invokes a script executor. | `api/components/schemas/events/payloads` and generated `FactoryEvent` payloads | OpenAPI contract tests plus `ReconstructFactoryWorldState(...)` reducer coverage |
| `script response` | noun | Canonical response-side execution boundary emitted for the matching script request outcome, output, duration, and failure detail. | `api/components/schemas/events/payloads` and generated `FactoryEvent` payloads | OpenAPI contract tests plus reducer/projection coverage |
| `script-backed workstation request` | noun | Dispatch-keyed request-history row that carries `script_request` and `script_response` detail without coercing script activity into inference attempts or provider sessions. | `pkg/api/workstation_request_projection.go` plus generated `FactoryWorldWorkstationRequest*` schemas | workstation-request projection tests and selected-work UI tests |
| `workstation-request selection` | noun | Dashboard selected-work/request-detail identity that resolves script-backed dispatch history by `dispatch_id`. | `ui/src/features/current-selection` | `useCurrentSelection.ts`, `workstation-request-detail.tsx`, `App.test.tsx`, Storybook smoke coverage |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `dispatch_id` | runtime dispatch identifier string | canonical dispatch and script boundary events | canonical world-state reducer, API projection builder, dashboard timeline store, selected-work UI | Go projection tests and UI timeline/current-selection tests keyed by `dispatch_id` |
| `script_request_id` | `dispatch-id/script-request/<attempt>` string | `SCRIPT_REQUEST` and `SCRIPT_RESPONSE` payloads | reducer correlation, latest-request/latest-response selection, UI detail rendering | reducer tests and `factoryTimelineStore` correlation tests |
| `attempt` | positive integer attempt counter per script request | script request/response payloads | projection ordering, latest-attempt fallback, detail rendering | projection tests and timeline-store tests |
| `work_id` | requested or produced work item identifier string | dispatch request or response payloads | selected-work history resolution for script-backed workstations | `useCurrentSelection.ts` coverage and mixed fixture replay smoke |

## Lifecycle States

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| pending script request | canonical selected-tick reducer | request arrives, no matching response at selected tick | No | `world_state_test.go` pending script coverage and request-detail pending UI tests |
| successful script response | canonical event payload plus reducer/projection | pending script request -> success response | Yes | workstation-request projection tests, `workstation-request-detail.test.tsx`, mixed replay smoke |
| failed script response | canonical event payload plus reducer/projection | pending script request -> failed response | Yes | workstation-request projection tests, `workstation-request-detail.test.tsx`, mixed replay smoke |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | n/a | n/a | n/a | n/a | This branch projects existing event/runtime state and does not add a new customer-authored configuration shape. |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| `SCRIPT_REQUEST` and `SCRIPT_RESPONSE` payload schemas | authored OpenAPI event schemas and generated `FactoryEvent` models | `pkg/factory/projections.ReconstructFactoryWorldState(...)` and UI `reconstructWorldState(...)` | OpenAPI contract -> generated Go/TS models -> reducer consumers | schema drift or field mismatch would desynchronize replay, API, and UI event readers | `pkg/api/openapi_contract_test.go`, `world_state_test.go`, `factoryTimelineStore.test.ts` |
| `FactoryWorldState.ScriptRequestsByDispatchID` and `ScriptResponsesByDispatchID` | canonical Go selected-tick reducer | API workstation-request projection builder | reducer -> API boundary adapter inside `libraries/agent-factory` | missing request/response correlation would drop pending or completed script detail from dashboard request history | `world_state.go` plus `workstation_request_projection_test.go` |
| `FactoryWorldWorkstationRequestProjectionSlice` with `script_request` and `script_response` views | `pkg/api.BuildFactoryWorldWorkstationRequestProjectionSlice(...)` and generated OpenAPI schemas | UI dashboard timeline, selected-work request history, and request-detail card | API schema -> generated Go/TS models -> UI dashboard aliases and renderers | projection drift would make script-backed dispatches appear empty or inference-shaped in the dashboard | Go projection tests, `factoryTimelineStore.test.ts`, `App.test.tsx`, Storybook smoke |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  generated `FactoryEvent` payloads plus generated
  `FactoryWorldWorkstationRequest*` schemas rooted in
  `libraries/agent-factory/api/openapi-main.yaml`
- Package-local model selected:
  Go reducer-local accumulation inside `pkg/factory/projections/world_state.go`
  and UI replay-store normalization inside `ui/src/state/factoryTimelineStore.ts`
- Reason:
  script request and response semantics are stable public and cross-package
  concepts, so the event payloads and workstation-request projection belong on
  one shared generated contract. The reducer and UI store keep local additive
  maps because they are internal implementation details that translate into the
  shared projection at the package boundary.
- Translation boundary:
  `ReconstructFactoryWorldState(...)`,
  `BuildFactoryWorldWorkstationRequestProjectionSlice(...)`, and
  `buildFactoryTimelineSnapshot(...)`
- Review evidence:
  OpenAPI contract tests, focused reducer/projection tests, selected-work UI
  tests, App replay smoke, and Storybook browser-backed request-detail smoke

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Script execution detail could have been mirrored into provider-session metadata or inference-attempt arrays | `pkg/api`, `ui/src/state`, `ui/src/features/current-selection` | Justify | current branch | Keep script detail on dedicated `script_request` and `script_response` fields; no follow-up needed for this branch |
| Selected-work history could have kept provider-session-only or work-item-only script drill-down paths | `ui/src/features/current-selection` | Unify | current branch | Use dispatch-keyed workstation-request selection as the primary script history surface and retain provider-session fallback only for legacy non-projected history |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical
  model:
  UI `DashboardScriptRequest` and `DashboardScriptResponse` are local aliases of
  the generated/public projection contract for replay-store ergonomics; they do
  not redefine the public meaning.
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition:
  none.
- Follow-up cleanup tasks:
  none required by this branch; the branch already closes the projection,
  reducer, selection, and UI integration path for script-backed dispatch
  history.
