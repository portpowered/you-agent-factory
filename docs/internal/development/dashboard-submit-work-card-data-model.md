# Dashboard Submit Work Card Data Model

This artifact records the shared dashboard submit-work metadata and mutation
contract used by the Agent Factory backend world-view projection, generated UI
dashboard types, timeline store, and submit-work widget.

## Change

- PRD, design, or issue: `prd.json` for Agent Factory Dashboard Submit Work Card
  (`US-001` through `US-005`)
- Owner: Codex branch `ralph/agent-factory-dashboard-submit-work-card`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `libraries/agent-factory/pkg/interfaces`,
  `libraries/agent-factory/pkg/factory/projections`, `libraries/agent-factory/pkg/api`,
  `libraries/agent-factory/ui/src/api/dashboard`,
  `libraries/agent-factory/ui/src/api/work`,
  `libraries/agent-factory/ui/src/state`,
  `libraries/agent-factory/ui/src/features/submit-work`
- Canonical architecture document to update before completion:
  `docs/processes/agent-factory-development.md`
- Canonical package responsibility artifact:
  `docs/architecture/package-responsibilities.md`. The canonical dashboard
  world-view contract is owned by `libraries/agent-factory/pkg/interfaces`,
  backend projection assembly lives in `libraries/agent-factory/pkg/factory/projections`,
  and generated REST request/response ownership stays with `libraries/agent-factory/pkg/api`
  plus the authored OpenAPI source.
- Canonical package interaction artifact:
  `docs/architecture/package-interactions.md`. Allowed interaction direction is
  backend projection -> generated/dashboard UI types -> timeline store and widget,
  while submit mutation traffic uses the existing generated REST contract rather
  than a dashboard-only backend seam.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [ ] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `submit work type` | noun | Customer-authored work type that can be queued directly from the dashboard because it has an initial state and is not an internal system-time work type. | `pkg/interfaces.FactoryWorldSubmitWorkType` projected by `pkg/factory/projections/world_view.go` | `world_view_test.go`, `selected_tick_cross_boundary_smoke_test.go`, `factoryTimelineStore.test.ts` |
| `submit work card` | noun | Dashboard widget that exposes one work-type selector, one request textarea, and one submit action inside the existing bento layout. | `ui/src/features/submit-work` | `submit-work-widget.test.tsx`, `App.test.tsx`, `submit-work-card.stories.tsx` |
| `submit work request payload` | noun | Existing single-work REST request body where the dashboard sends the textarea content as `payload`. | generated `/work` contract consumed by `ui/src/api/work/api.ts` | `api.test.ts`, widget tests, App smoke |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `work_type_name` | customer-authored work type name string | backend world-view projection and UI timeline snapshot | submit selector options, `/work` request body | Go projection tests, `factoryTimelineStore.test.ts`, widget/App tests |
| `payload` | freeform request string | submit-work widget hook | existing `POST /work` endpoint | `ui/src/api/work/api.test.ts`, `submit-work-widget.test.tsx`, App smoke |
| `work_id` | runtime work item identifier string | `POST /work` response | inline success state in submit card | widget/App tests and Storybook integration smoke |

## Lifecycle States

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| None | n/a | n/a | n/a | This branch reuses the existing `/work` mutation lifecycle and models UI-only loading/success/failure states locally in the widget hook rather than adding a new shared lifecycle enum. |

## Configuration Shapes

| Config shape | Owner | Required fields | Defaults | Consumers | Evidence |
| --- | --- | --- | --- | --- | --- |
| Active factory `WorkTypes` projection | `interfaces.InitialStructurePayload` | work type ID/name plus at least one `INITIAL` state to qualify as submit-eligible | none | backend world view, UI timeline snapshot, submit selector | `world_view.go`, `factoryTimelineStore.ts`, focused tests |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| `FactoryWorldTopologyView.SubmitWorkTypes` | backend world-view projection in `pkg/factory/projections` | generated dashboard types, UI timeline store, submit-work widget | interfaces -> projection -> UI contract consumer | missing projection would leave selector empty or make backend/UI dashboard paths disagree | `world_view_test.go`, `selected_tick_cross_boundary_smoke_test.go`, `factoryTimelineStore.test.ts` |
| Dashboard timeline `submit_work_types` mirror | UI timeline reducer in `ui/src/state/factoryTimelineStore.ts` | App dashboard snapshot and submit selector | generated dashboard payload -> reducer -> widget | drift would desynchronize initial snapshot and selected-tick replay views | `factoryTimelineStore.test.ts`, `App.test.tsx` |
| Existing `POST /work` single-work contract | generated REST API and `ui/src/api/work/api.ts` wrapper | submit-work hook and card | generated API -> typed UI wrapper -> widget hook | server validation or transport failures must surface inline without clearing draft state | `api.test.ts`, `submit-work-widget.test.tsx`, `App.test.tsx`, Storybook browser smoke |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `pkg/interfaces.FactoryWorldTopologyView` for dashboard metadata and the existing
  generated `/work` REST contract for submission.
- Package-local model selected:
  `ui/src/features/submit-work/use-submit-work-widget.ts` owns local draft and
  mutation UI state, and `ui/src/state/factoryTimelineStore.ts` keeps a reducer-local
  dashboard snapshot mirror.
- Reason:
  submit-eligible work types are stable cross-package dashboard contract data, so
  they belong on the canonical world-view boundary. Form drafts and inline mutation
  states are widget-local behavior and do not belong on a shared contract.
- Translation boundary:
  `BuildFactoryWorldView(...)`, `buildFactoryTimelineSnapshot(...)`, and
  `submitWork(...)`.
- Review evidence:
  Go projection tests, contract guard tests, timeline reducer tests, widget/App tests,
  and Storybook browser checks for submit success and retryable failure.

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Submit-eligible work type list derived in both backend projection and UI timeline reducer | `pkg/factory/projections/world_view.go`, `ui/src/state/factoryTimelineStore.ts` | Justify | current branch | Both paths project the same canonical topology data because the dashboard supports both current snapshot and event-backed selected-tick views; focused Go and UI tests guard drift |
| Submit mutation form state could have been lifted into App-level dashboard state | `ui/src/features/submit-work` | Justify | current branch | Keep draft and inline mutation status local to the widget hook; no follow-up needed for this branch |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical
  model:
  `DashboardTopology["submit_work_types"]` and widget-local option state mirror
  the canonical world-view contract for UI ergonomics, but they do not redefine
  the backend meaning.
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition:
  none.
- Follow-up cleanup tasks:
  none required by this branch.
