# World-View Contract Cleanup Data Model

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-world-view-boundary-reduction`)
- Owner: Codex branch `ralph/agent-factory-world-view-boundary-reduction`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `pkg/interfaces`, `pkg/factory/projections`, `pkg/service`, `pkg/cli/dashboard`, `api/openapi.yaml`, `ui/src/api/generated`, `ui/src/api/dashboard`
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

## Inventory Review Procedure

1. Run `rg -n --glob '!world_view_contract_guard_test.go' '^type (FactoryWorld.*View|FactoryEnabledTransitionView|FactoryFiringDecisionView)\b' pkg/interfaces`.
   Expected result on this branch: 3 live `FactoryWorld*View` definitions, all in `pkg/interfaces/factory_world_view.go`, and zero live selected-tick hook-mirror definitions.
2. Run `rg -n "FactoryEnabledTransitionView|FactoryFiringDecisionView" pkg/interfaces`.
   Expected result on this branch: only the guard-test allowlist and forbidden-name hits in `pkg/interfaces/world_view_contract_guard_test.go`.
3. Run `rg -n "FactoryWorldWorkstationRequestView|FactoryWorldWorkstationRequestCountView|FactoryWorldWorkstationRequestRequestView|FactoryWorldWorkstationRequestResponseView|FactoryWorldTokenView|FactoryWorldMutationView" pkg/interfaces -g "*.go"`.
   Expected result on this branch: only the focused guard notes in `pkg/interfaces/world_view_contract_guard_test.go`.
4. Run `rg -n "FactoryEnabledTransitionView|FactoryFiringDecisionView|FactoryWorldDispatchView|FactoryWorldProviderSessionView|FactoryWorldInferenceAttemptView|FactoryProviderSession|FactoryProviderFailure|FactoryWorkDiagnostics|FactoryRenderedPromptDiagnostic|FactoryProviderDiagnostic" pkg -g "*.go"`.
   Expected result on this branch: only the focused guard notes in `pkg/interfaces/world_view_contract_guard_test.go`.
5. Compare each live definition and each retired hook-mirror name from those commands against the inventory table below before approving new boundary cleanup work.

## Verified Inventory Snapshot

- Verified on `2026-04-25` against branch `ralph/agent-factory-world-view-boundary-reduction`.
- Live `pkg/interfaces` `FactoryWorld*View` allowlist:
  `FactoryWorldView`, `FactoryWorldTopologyView`, and
  `FactoryWorldRuntimeView`.
- Retired selected-tick mirror names still deleted from live `pkg/**/*.go`:
  `FactoryWorldWorkstationRequestView`,
  `FactoryWorldWorkstationRequestCountView`,
  `FactoryWorldWorkstationRequestRequestView`,
  `FactoryWorldWorkstationRequestResponseView`, `FactoryWorldTokenView`,
  `FactoryWorldMutationView`, `FactoryEnabledTransitionView`,
  `FactoryFiringDecisionView`, `FactoryWorldDispatchView`,
  `FactoryWorldProviderSessionView`, `FactoryWorldInferenceAttemptView`,
  `FactoryProviderSession`, `FactoryProviderFailure`,
  `FactoryWorkDiagnostics`, `FactoryRenderedPromptDiagnostic`, and
  `FactoryProviderDiagnostic`.
- Guard evidence: `pkg/interfaces/world_view_contract_guard_test.go` is the only
  live `pkg/interfaces` file that references the retired boundary-only names,
  and the same guard file is the only live package file that references the
  retired canonical mirror names. It carries the same three-type allowlist
  documented here.

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `selected-tick world state` | noun | Canonical event-first reconstruction of topology, place occupancy, active dispatches, completed dispatches, inference attempts, provider sessions, and failed-work details for one tick. | `pkg/interfaces/factory_world_state.go` | `FactoryWorldState`, `ReconstructFactoryWorldState(...)` |
| `world-view boundary adapter` | noun | Thin compatibility layer that maps canonical selected-tick state into API, CLI, or UI-facing shapes without becoming a second runtime model. | `pkg/factory/projections/world_view.go` plus focused topology/runtime mapper files in the same package | `BuildFactoryWorldView(...)` |
| `boundary-only contract` | classification | A contract that still exists because a CLI, API, or UI surface needs a transport or presentation shape, not because the runtime needs a second canonical model. | this artifact | PRD cleanup requirement plus current API/CLI/UI consumers |
| `dead hook mirror` | classification | An `interfaces` type that restates an already-owned engine-runtime contract and has no first-party consumer. | `pkg/interfaces/engine_runtime.go` | `FactoryEnabledTransitionView` and `FactoryFiringDecisionView` only appear in `pkg/interfaces/factory_hooks.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `dispatch_id` | runtime dispatch identifier string | canonical `DISPATCH_REQUEST`, `DISPATCH_RESPONSE`, and inference events | selected-tick state, world-view adapter, CLI dashboard, workstation-request API/UI projection | projection tests and dashboard tests keyed by `dispatch_id` |
| `transition_id` | workstation or transition identifier string | topology plus dispatch events | selected-tick state, CLI dashboard, workstation-request projection | `world_state.go`, `world_view.go`, CLI dashboard render tests |
| `inference_request_id` | provider-attempt identifier string | `INFERENCE_REQUEST` and `INFERENCE_RESPONSE` events | inference-attempt history and workstation-request counters | `world_state_test.go`, `world_view_test.go` |
| `work_id` | work-item identifier string | `WORK_REQUEST`, dispatch, and terminal-work events | place occupancy, dispatch history, provider-session history, workstation-request projection | `FactoryWorkItem`, `FactoryTerminalWork`, functional selected-tick tests |

## Lifecycle States

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| active dispatch | `FactoryWorldDispatch` | request accepted -> dispatch in flight -> completion recorded | No | `FactoryWorldState.ActiveDispatches` |
| completed dispatch | `FactoryWorldDispatchCompletion` | active dispatch -> accepted or rejected completion | Yes | `FactoryWorldState.CompletedDispatches`, `world_view_test.go` |
| failed dispatch | `FactoryWorldDispatchCompletion` plus `FactoryWorldFailureDetail` | active dispatch -> failed completion | Yes | `FactoryWorldState.FailedDispatches`, `FailureDetailsByWorkID` |
| provider session record | `FactoryWorldProviderSessionRecord` | created only when a completion exposes provider-session metadata | Yes | `FactoryWorldState.ProviderSessions` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| canonical selected-tick reconstruction | `pkg/factory/projections/world_state.go` | `pkg/service`, `pkg/factory/runtime`, `tests/functional_test`, `pkg/factory/projections/world_view.go` | canonical event log -> selected-tick state -> outer adapters | reconstruction can fail on malformed event payloads | `ReconstructFactoryWorldState(...)` call sites and tests |
| internal world-view shell | `pkg/factory/projections/world_view.go` plus focused topology/runtime mapper files | `pkg/service.factoryWorldView`, `pkg/cli/dashboard` | selected-tick state -> internal CLI/dashboard adapter | adapter now carries canonical dispatch, provider-session, and inference-attempt records; display-only shaping happens at the CLI/UI boundary | `BuildFactoryWorldView(...)`, CLI dashboard tests |
| workstation-request public contract | `api/openapi.yaml` `FactoryWorldWorkstationRequestProjectionSlice` | generated Go/TS models and dashboard typed surfaces | OpenAPI schema -> generated models -> UI/request-selection consumers | schema drift breaks generated consumers | `pkg/api/openapi_contract_test.go`, `ui/src/api/dashboard/types.ts` |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected: `FactoryWorldState` and its canonical member types in `pkg/interfaces/factory_world_state.go`
- Package-local model selected: `FactoryWorldView` may remain a thin boundary adapter, but only for compatibility shells and explicitly justified boundary-only contracts
- Reason: selected-tick history is already reconstructed from canonical events in `ReconstructFactoryWorldState(...)`; runtime duplication in `factory_world_view.go` should collapse onto canonical state types or small boundary-only helpers instead of keeping a second runtime vocabulary
- Translation boundary: `BuildFactoryWorldView(...)` for the thin selected-tick shell plus `pkg/api.BuildFactoryWorldWorkstationRequestProjectionSlice(...)` for the published workstation-request contract; work-item, token, and mutation shaping should stay inside the owning projection or API package instead of reintroducing shared `pkg/interfaces` presentation helpers, API JSON serialization handles timestamp formatting, and CLI/UI boundary mappers own compatibility-only labels such as `time:expire`
- Review evidence: `world_view.go`, `pkg/service/factory.go`, `pkg/cli/dashboard/dashboard.go`, `api/openapi.yaml`, `ui/src/api/dashboard/types.ts`

## Contract Inventory

| Contract | Current role | Classification | Owning boundary or canonical owner | First-party consumers | Why it cannot yet collapse into `FactoryWorldState` or canonical replacement | Removal condition or next owner | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `FactoryWorldView` | Aggregate selected-tick shell that groups topology and runtime boundary payloads | keep | shared projection adapter in `pkg/factory/projections/world_view.go` for API, CLI, and UI selected-tick responses | `pkg/service`, `pkg/cli/dashboard`, selected-tick regression tests | `FactoryWorldState` is the canonical runtime model, but supported callers still consume one aggregate topology-plus-runtime response shell instead of stitching two separate payload roots | remove when supported API, CLI, and UI selected-tick readers can consume canonical state plus boundary-local topology/request adapters without the extra shell | `BuildFactoryWorldView(...)` and `pkg/service/factory.go` still publish one selected-tick world-view payload |
| `FactoryWorldTopologyView` | Stable graph topology shell for workstation nodes and edges | keep | shared projection adapter in `pkg/factory/projections/world_view.go` | `pkg/service`, `pkg/cli/dashboard`, UI dashboard graph consumers | `FactoryWorldState` owns runtime/session state, but selected-tick graph rendering still needs a boundary-friendly topology shell that groups node and edge lists for supported readers | remove when topology payloads move to a separate boundary-owned graph contract or when supported readers consume canonical topology data directly | `FactoryWorldView.Topology` plus dashboard graph consumers still expect node and edge groupings |
| `FactoryWorldRuntimeView` | Selected-tick runtime shell that carries counts, active execution maps, activity maps, place occupancy, and canonical session members | keep | shared projection adapter in `pkg/factory/projections/world_view.go` | `pkg/service`, `pkg/cli/dashboard`, UI dashboard runtime consumers | `FactoryWorldState` does not yet expose the supported selected-tick boundary shape for activity maps, place counts, and session aggregates, so one thin runtime shell still groups boundary-local adapters around canonical state members | remove when supported readers can assemble these runtime maps from canonical state or from boundary-local adapters without a shared shell | `FactoryWorldView.Runtime` now carries canonical activity and session members only; completed/failed work label summaries plus failed-work detail rows are derived in CLI/UI boundary mappers from place occupancy and canonical dispatch history, and the API-owned workstation-request slice remains outside `pkg/interfaces` |
| `FactoryWorldWorkstationRequestView` | retired shared workstation-request mirror; the additive request-selection contract now lives only in generated API models | move | API boundary published through `FactoryWorldWorkstationRequestProjectionSlice` and built in `pkg/api/workstation_request_projection.go` | `api/openapi.yaml`, generated Go models, generated TypeScript models, `ui/src/api/dashboard/types.ts` | the supported request-selection API still publishes this contract, but `pkg/interfaces` no longer owns the transport shape and `FactoryWorldState` intentionally does not own transport-only request/response grouping | keep the contract only in generated API surfaces until the published request-selection shape is versioned or replaced | `FactoryWorldWorkstationRequestProjectionSlice` remains the additive public contract while `pkg/interfaces/world_view_contract_guard_test.go` now rejects the retired shared mirror |
| `FactoryWorldWorkstationRequestCountView` | retired shared nested request counter mirror | move | API boundary under `FactoryWorldWorkstationRequestView` | generated Go/TS models, UI request-selection consumers | counts are transport-only request-selection summary fields, not canonical runtime state | keep the counter only in generated API surfaces until the request-selection contract is replaced | generated schema plus `pkg/api/workstation_request_projection.go` own the field now |
| `FactoryWorldWorkstationRequestRequestView` | retired shared nested request payload mirror | move | API boundary under `FactoryWorldWorkstationRequestView` | generated Go/TS models, UI request-selection consumers | request-local transport fields such as prompt, request metadata, and grouping do not belong on `FactoryWorldState` or `pkg/interfaces` | keep the payload only in generated API surfaces until the request-selection contract is replaced | generated schema plus `pkg/api/workstation_request_projection.go` own the field now |
| `FactoryWorldWorkstationRequestResponseView` | retired shared nested response payload mirror | move | API boundary under `FactoryWorldWorkstationRequestView` | generated Go/TS models, UI request-selection consumers | response-local transport grouping and additive response metadata do not belong on `FactoryWorldState` or `pkg/interfaces` | keep the payload only in generated API surfaces until the request-selection contract is replaced | generated schema plus `pkg/api/workstation_request_projection.go` own the field now |
| `FactoryWorldTokenView` | retired shared token mirror for workstation-request payloads and active execution debug projection | move | API boundary generated schema plus API-local mapping helpers | generated Go/TS workstation-request consumers | token projection remains supported at the API boundary, but the shared `pkg/interfaces` helper duplicated transport-only shaping instead of keeping active execution data canonical | keep only in generated API surfaces and API-local mapping code; active executions now carry canonical `ConsumedInputs` | `pkg/api/workstation_request_projection.go` now owns generated token shaping and `FactoryWorldActiveExecution.ConsumedInputs` keeps runtime data canonical |
| `FactoryWorldMutationView` | retired shared mutation mirror for workstation-request responses | move | API boundary generated schema plus API-local mapping helpers | generated Go/TS workstation-request consumers | mutation projection remains supported at the API boundary, but the shared `pkg/interfaces` helper duplicated transport-only shaping | keep only in generated API surfaces and API-local mapping code | `pkg/api/workstation_request_projection.go` now owns generated mutation shaping from canonical dispatch completions |
| `FactoryWorldDispatchView` | retired session dispatch-history mirror DTO | merge | canonical owner `FactoryWorldDispatchCompletion` in `pkg/interfaces/factory_world_state.go` | none in live code; only retired-name guard coverage remains | replaced by canonical dispatch-completion members plus CLI/API/UI boundary mappers for display-only fields | already retired; guard must keep the name deleted | forbidden in `pkg/interfaces/world_view_contract_guard_test.go` and absent from live package files |
| `FactoryWorldProviderSessionView` | retired provider-session mirror DTO | merge | canonical owner `FactoryWorldProviderSessionRecord` in `pkg/interfaces/factory_world_state.go` | none in live code; only retired-name guard coverage remains | replaced by canonical provider-session records plus CLI/UI boundary mappers | already retired; guard must keep the name deleted | forbidden in `pkg/interfaces/world_view_contract_guard_test.go` and absent from live package files |
| `FactoryWorldInferenceAttemptView` | retired inference-attempt mirror DTO | merge | canonical owner `FactoryWorldInferenceAttempt` in `pkg/interfaces/factory_world_state.go` | none in live code; only retired-name guard coverage remains | replaced by canonical inference attempts plus outer-boundary timestamp formatting | already retired; guard must keep the name deleted | forbidden in `pkg/interfaces/world_view_contract_guard_test.go` and absent from live package files |
| `FactoryEnabledTransitionView` | retired selected-tick hook mirror for engine enabled-transition data | delete | canonical owner `interfaces.EnabledTransition` in `pkg/interfaces/engine_runtime.go` | none in live code; only retired-name guard coverage remains | no boundary owner remains because the mirror duplicated the engine-runtime contract | already retired; guard must keep the name deleted | `rg -n "FactoryEnabledTransitionView|FactoryFiringDecisionView" pkg/interfaces` only hits the guard test |
| `FactoryFiringDecisionView` | retired selected-tick hook mirror for engine firing-decision data | delete | canonical owner `interfaces.FiringDecision` in `pkg/interfaces/engine_runtime.go` | none in live code; only retired-name guard coverage remains | no boundary owner remains because the mirror duplicated the engine-runtime contract | already retired; guard must keep the name deleted | `rg -n "FactoryEnabledTransitionView|FactoryFiringDecisionView" pkg/interfaces` only hits the guard test |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical model: only `FactoryWorldView`, `FactoryWorldTopologyView`, and `FactoryWorldRuntimeView` remain in `pkg/interfaces` as the shared selected-tick shell. The published workstation-request contract family now lives only in generated API models plus `pkg/api/workstation_request_projection.go`.
- Approved exceptions with owner, reason, scope, expiration, and removal condition: the retained `FactoryWorld*View` allowlist in `pkg/interfaces` is now limited to the shared topology/runtime shell. Reason: supported selected-tick surfaces still need one aggregate shell, but transport-only token and mutation shaping moved to the API owner while active execution debug data stayed canonical on `ConsumedInputs`. Removal condition: later cleanup stories move supported readers onto canonical state plus boundary-local adapters so the shared `pkg/interfaces` allowlist can shrink again. Guard enforcement now has two layers: the `pkg/interfaces` allowlist for live boundary types and a package-tree retired-name scan so shared projections cannot silently reintroduce the deleted mirror vocabulary.
- Follow-up cleanup tasks: none added by this story; later PRD stories already cover contract consolidation, projection slimming, and regression guards.
