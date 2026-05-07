# Workstation Request Projection Data Model

## Change

- PRD, design, or issue: `prd.json` workstation-request projection slice (`US-003`)
- Owner: Codex branch `ralph/agent-factory-world-view-boundary-consolidation`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `api/openapi.yaml`, `pkg/interfaces`, `pkg/factory/projections`, `pkg/service`, `ui/src/api/generated`, `ui/src/state`, `ui/src/api/dashboard`
- Canonical architecture document to update before completion: `docs/processes/agent-factory-development.md`

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
| `workstation request projection` | noun | One request-scoped dashboard read-model entry keyed by dispatch ID with request, response, and inference-count details. | `api/openapi.yaml` plus `pkg/api/workstation_request_projection.go` | `FactoryWorldWorkstationRequestProjectionSlice` and generated Go/UI projection tests |
| `latest inference attempt` | noun | Highest-precedence request/response attempt chosen for prompt, worktree, response text, and error-class projection. | `pkg/factory/projections/world_view.go` and `ui/src/state/factoryTimelineStore.ts` | `latestWorkstationInferenceAttempt(...)` / `latestWorkstationAttempt(...)` |
| `safe provider metadata` | noun | Allowlisted provider request/response metadata copied from dashboard-safe diagnostics, not raw command diagnostics. | `pkg/interfaces/factory_events.go` | `FactoryProviderDiagnostic`, event-history safe-key tests, request/response metadata projection fields |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `dispatch_id` | runtime dispatch identifier string | canonical `DISPATCH_CREATED` / `DISPATCH_COMPLETED` / inference events | API projection builder, UI replay store, later selector work | projection tests keyed by `dispatch_id` |
| `inference_request_id` | `dispatch-id/inference/...` string | canonical inference request/response events | latest-attempt selection and count reducers | inference-attempt projection tests |

## Lifecycle States

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| request-only | event-first world state | dispatch created, no matching dispatch completed yet | No | workstation-request projection tests |
| responded | dispatch completed with non-error inference outcome or accepted/rejected completion | request-only -> responded | Yes | success projection tests |
| errored | dispatch completed with failed inference outcome or failed completion | request-only -> errored | Yes | error projection tests |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | n/a | n/a | n/a | n/a | This slice only projects existing runtime data. |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| `FactoryWorldState.ActiveDispatches` carries request-time provider/model fallback for in-flight workstation requests | `pkg/factory/projections/world_state.go` | `pkg/api/workstation_request_projection.go` | reducer -> API boundary adapter inside `libraries/agent-factory` | missing worker metadata leaves provider/model omitted | reducer mapping plus request-only tests |
| `FactoryWorldWorkstationRequestProjectionSlice` is mirrored by `DashboardWorkstationRequest` selectors in the UI replay store | generated Go/OpenAPI contract plus API boundary adapter | `ui/src/api/dashboard/types.ts`, `ui/src/state/factoryTimelineStore.ts` | generated API contract -> UI mirror | drift if either side changes alone | focused Go/UI workstation-request projection tests |
| `FactoryWorldWorkstationRequestProjectionSlice` publishes the request projection through generated contract surfaces without restoring removed `/dashboard` routes | `api/openapi.yaml` | `pkg/api/generated/server.gen.go`, `ui/src/api/generated/openapi.ts`, `ui/src/api/dashboard/types.ts` | OpenAPI schema -> generated Go/TS models -> dashboard aliases | schema drift leaves generated consumers on handwritten mirrors | OpenAPI contract test plus generated typecheck |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected: OpenAPI `FactoryWorldWorkstationRequestProjectionSlice` plus generated Go/TS consumers
- Package-local model selected: UI replay `WorldDispatch` and `WorldCompletion` keep local reducer fields before projection
- Reason: reducer-local fields stay package-private, while the workstation-request projection itself remains a stable API contract that should not be re-owned by `pkg/interfaces`
- Translation boundary: `pkg/api.BuildFactoryWorldWorkstationRequestProjectionSlice(...)` and `buildFactoryTimelineSnapshot(...)`
- Review evidence: Go API projection tests, OpenAPI contract tests, generated UI alias typecheck, process-note update documenting precedence

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| provider/model/prompt/session details previously split across active execution, inference-attempt, provider-session, and diagnostics views | API projection builder and UI replay store | Unify core request-inspection fields on the workstation-request projection; retain lower-level views for drill-down and retry history | current branch | US-004 will add the selector/accessor that consumes this projection directly |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical model: UI `WorldDispatch` / `WorldCompletion` remain replay-store internals and are not published contracts; the generated workstation-request slice remains the only supported API contract for this surface.
- Approved exceptions with owner, reason, scope, expiration, and removal condition: none.
- Follow-up cleanup tasks: none required for this slice.
