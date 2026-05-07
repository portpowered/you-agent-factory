# Safe Diagnostics Contract Consolidation Data Model

This artifact records the shared-model decisions for consolidating Agent
Factory safe diagnostics, provider-session metadata, and provider-failure
metadata onto one canonical internal safe boundary. It inventories the current
worker, event-history, replay, and selected-tick contracts so later cleanup
stories can retire the duplicate `Factory*` mirror family without changing the
generated public event contract.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch
  `ralph/agent-factory-safe-diagnostics-contract-consolidation`)
- Owner: Codex branch
  `ralph/agent-factory-safe-diagnostics-contract-consolidation`
- Packages or subsystems: `pkg/interfaces`, `pkg/factory`,
  `pkg/factory/projections`, `pkg/replay`, `pkg/service`,
  `pkg/cli/dashboard`, `pkg/api/generated`, and focused tests
- Canonical architecture document to update before completion: this file is the
  branch data-model construction artifact. Durable cleanup rules live in
  `docs/processes/agent-factory-development.md`.

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
| raw worker diagnostics | worker-internal contract | The full worker result diagnostics envelope, including command, panic, and provider or prompt metadata that may still contain sensitive values. | `pkg/interfaces/work_execution.go` `WorkDiagnostics` | `interfaces.WorkResult.Diagnostics`, `interfaces.InferenceResponse.Diagnostics` |
| canonical internal safe diagnostics | internal cross-package contract | The one safe subset that event history, replay, selected-tick world state, and workstation-request projections should share after allowlisting. | target `pkg/interfaces` family: `SafeWorkDiagnostics`, `SafeRenderedPromptDiagnostic`, `SafeProviderDiagnostic` | This cleanup PRD plus the generated `SafeWorkDiagnostics` event contract already used in `pkg/api/generated/server.gen.go` |
| generated safe event diagnostics | public generated contract | The existing customer-facing `DISPATCH_RESPONSE.payload.diagnostics` shape that omits raw prompts, stdin, and env values. | `api/openapi.yaml` and `pkg/api/generated/server.gen.go` `SafeWorkDiagnostics` | `DispatchResponseEventPayload.Diagnostics` |
| provider session metadata | shared metadata contract | Stable provider rollout or session identity attached to dispatch completions and replay artifacts. | `pkg/interfaces/work_execution.go` `ProviderSessionMetadata` | `WorkResult.ProviderSession`, generated `ProviderSessionMetadata` |
| provider failure metadata | shared metadata contract | Normalized provider failure family and type shared across retry, history, replay, and projections. | `pkg/interfaces/provider_failure.go` `ProviderFailureMetadata` | `WorkResult.ProviderFailure`, generated `ProviderFailureMetadata` |
| mirror `Factory*` diagnostics family | duplicate internal contract | Event-first read-model vocabulary that restates safe provider-session, provider-failure, and diagnostics shapes already represented elsewhere. | current `pkg/interfaces/factory_events.go` and `pkg/interfaces/factory_world_state.go` | `FactoryProviderSession`, `FactoryProviderFailure`, `FactoryWorkDiagnostics`, `FactoryRenderedPromptDiagnostic`, `FactoryProviderDiagnostic` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `dispatch_id` | runtime dispatch identifier string | worker result and generated event context | replay, selected-tick world state, workstation-request projection, CLI dashboard | `RecordWorkstationResponse(...)`, `applyDispatchCompleted(...)` |
| `provider_session_id` | provider session identifier string | worker result `ProviderSessionMetadata.ID` | generated safe event payload, replay artifact, selected-tick provider-session records | `generatedProviderSession(...)`, `replayProviderSessionFromGenerated(...)`, `providerSessionFromGenerated(...)` |
| `failure family/type` | normalized provider failure strings | worker result `ProviderFailureMetadata` | retry logic, generated event payload, replay artifact, selected-tick completion result | `generatedProviderFailure(...)`, `replayProviderFailureFromGenerated(...)`, `providerFailureFromGenerated(...)` |
| allowlisted prompt and provider metadata keys | fixed safe key set | safe adapter helpers | generated event payloads and replay | `safeRenderedPromptVariables(...)`, `safeDiagnosticMetadata(...)` in both `pkg/factory/event_history.go` and `pkg/replay/event_artifact.go` |

## Boundary Lifecycle

| Layer | Owner | Allowed transition | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| raw worker result | worker boundary | worker or provider returns `WorkResult` or `InferenceResponse` with raw `WorkDiagnostics` and shared provider metadata | No | `pkg/interfaces/work_execution.go` |
| generated safe dispatch-completion payload | canonical event boundary | event history allowlists raw diagnostics into generated `SafeWorkDiagnostics`, `ProviderSessionMetadata`, and `ProviderFailureMetadata` | Yes | `pkg/factory/event_history.go` `RecordWorkstationResponse(...)` |
| replay artifact storage | canonical persistence boundary | replay stores and rehydrates the same generated safe payload | Yes | `pkg/replay/event_artifact.go` |
| selected-tick world-state reconstruction | event-first projection boundary | generated safe payload is mapped into selected-tick completion and provider-session records | Yes | `pkg/factory/projections/world_state.go` |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| raw worker result to generated event | `pkg/workers` and worker adapters produce `interfaces.WorkResult` | `pkg/factory/event_history.go` emits generated `DispatchResponseEventPayload` | worker result -> canonical event history | unsafe prompt, stdin, env, and panic data must be dropped before serialization | `generatedWorkDiagnostics(...)`, `generatedProviderSession(...)`, `generatedProviderFailure(...)` |
| generated event to replay artifact | `pkg/factory/event_history.go` or live event stream | `pkg/replay/event_artifact.go` persists and rehydrates generated events | canonical event -> replay artifact | duplicated allowlist logic can drift from live event emission | `generatedWorkDiagnostics(...)`, `replayWorkDiagnosticsFromGenerated(...)` |
| generated event to selected-tick world state | generated `FactoryEvent` log | `pkg/factory/projections/world_state.go` | canonical event -> event-first projection | duplicate generated-to-safe mirror helpers can drift from event or replay rules | `applyDispatchCompleted(...)`, `providerSessionFromGenerated(...)`, `providerFailureFromGenerated(...)`, `factoryWorkDiagnosticsFromGenerated(...)` |
| selected-tick world state to workstation-request and CLI views | `FactoryWorldState` canonical records | `pkg/factory/projections/world_view.go`, `pkg/factory/projections/world_view_workstation_requests.go`, `pkg/cli/dashboard` | canonical world state -> thin boundary adapters | current mirror-family clones and rewrapping widen the drift surface | `cloneFactoryWorkDiagnostics(...)`, `providerSessionMetadataFromFactoryRecord(...)` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  `ProviderSessionMetadata` and `ProviderFailureMetadata` stay canonical where
  their shape already matches the generated contract. The canonical internal
  safe diagnostics family for cross-package use should be
  `SafeWorkDiagnostics`, `SafeRenderedPromptDiagnostic`, and
  `SafeProviderDiagnostic` in `pkg/interfaces`, aligned with the existing
  generated public contract names.
- Package-local model selected: raw `WorkDiagnostics`,
  `RenderedPromptDiagnostic`, and `ProviderDiagnostic` remain worker-internal.
  `CommandDiagnostic`, `PanicDiagnostic`, and any raw prompt, stdin, env, or
  provider metadata values must never cross into event/history or selected-tick
  contracts.
- Reason: the worker boundary still needs the richer raw envelope for execution
  debugging, but event history, replay, selected-tick world state, and
  workstation-request projections all consume the same allowlisted subset. That
  subset should have one internal name and one adapter path instead of one
  worker vocabulary plus one `Factory*` mirror vocabulary.
- Translation boundary: one shared adapter should own raw-to-safe conversion
  and safe-to-generated / generated-to-safe conversion for diagnostics,
  provider session, and provider failure. `pkg/factory/event_history.go` and
  `pkg/replay/event_artifact.go` currently duplicate the allowlist helpers, and
  `pkg/factory/projections/world_state.go` adds a third generated-to-mirror
  path.
- Review evidence: `pkg/factory/event_history.go`,
  `pkg/replay/event_artifact.go`, `pkg/factory/projections/world_state.go`,
  `pkg/factory/projections/world_view.go`,
  `pkg/factory/projections/world_view_workstation_requests.go`, and
  `pkg/cli/dashboard/dashboard.go`.

## Contract Inventory

| Contract | Location | Classification | Canonical replacement or follow-up | Evidence |
| --- | --- | --- | --- | --- |
| `ProviderSessionMetadata` | `pkg/interfaces/work_execution.go` | canonical | Reuse directly across worker results, generated events, replay, and selected-tick projections | Generated `ProviderSessionMetadata` already matches the same fields |
| `ProviderFailureMetadata` | `pkg/interfaces/provider_failure.go` | canonical | Reuse directly across worker results, generated events, replay, and selected-tick projections | Generated `ProviderFailureMetadata` already matches the same fields |
| `WorkDiagnostics` | `pkg/interfaces/work_execution.go` | worker-internal-only | Keep raw worker diagnostics internal; map allowlisted fields into `SafeWorkDiagnostics` | Includes `Command`, `Panic`, and unconstrained metadata that must not leak |
| `RenderedPromptDiagnostic` | `pkg/interfaces/work_execution.go` | worker-internal-only | Safe subset maps into `SafeRenderedPromptDiagnostic` | Raw `Variables` can still carry prompt, stdin, and env-shaped values until allowlisted |
| `ProviderDiagnostic` | `pkg/interfaces/work_execution.go` | worker-internal-only | Safe subset maps into `SafeProviderDiagnostic` | Raw request and response metadata can still carry arbitrary provider values |
| `FactoryProviderSession` | `pkg/interfaces/factory_events.go`, `pkg/interfaces/factory_world_state.go` | delete | Merge into `ProviderSessionMetadata` | Same `provider`, `kind`, and `id` fields with extra conversion churn |
| `FactoryProviderFailure` | `pkg/interfaces/factory_events.go`, `pkg/factory/projections/world_state.go` | delete | Merge into `ProviderFailureMetadata` | Same `family` and `type` meaning with event-first renaming only |
| `FactoryWorkDiagnostics` | `pkg/interfaces/factory_events.go`, `pkg/interfaces/factory_world_state.go` | delete | Merge into canonical `SafeWorkDiagnostics` | Exists only to mirror the generated safe payload inside internal projections |
| `FactoryRenderedPromptDiagnostic` | `pkg/interfaces/factory_events.go` | delete | Merge into canonical `SafeRenderedPromptDiagnostic` | Same safe hash-plus-variables shape as the generated contract |
| `FactoryProviderDiagnostic` | `pkg/interfaces/factory_events.go` | delete | Merge into canonical `SafeProviderDiagnostic` | Same safe provider or model plus allowlisted metadata shape as the generated contract |

## Representative `DISPATCH_RESPONSE` Flow

| Stage | Current path | What is dropped | What survives |
| --- | --- | --- | --- |
| worker result | `interfaces.WorkResult.Diagnostics`, `ProviderSessionMetadata`, `ProviderFailureMetadata` enter `RecordWorkstationResponse(...)` | nothing yet; raw command, panic, stdin, env, and prompt-body values may still be present | full raw worker diagnostics plus shared provider session and provider failure metadata |
| generated event emission | `pkg/factory/event_history.go` `generatedWorkDiagnostics(...)`, `generatedRenderedPromptDiagnostic(...)`, `generatedProviderDiagnostic(...)`, `generatedProviderSession(...)`, `generatedProviderFailure(...)` | raw prompt variables, prompt bodies, `stdin`, `env`, `command`, `panic`, and non-allowlisted provider metadata keys | prompt hashes, allowlisted prompt variables, provider/model, allowlisted provider metadata, provider session metadata, provider failure metadata |
| replay artifact | `pkg/replay/event_artifact.go` repeats the same allowlist and generated mapping helpers | same raw fields remain absent because replay only stores generated safe payloads | the same generated `SafeWorkDiagnostics`, `ProviderSessionMetadata`, and `ProviderFailureMetadata` payload shape |
| selected-tick world state | `pkg/factory/projections/world_state.go` `applyDispatchCompleted(...)` plus `providerSessionFromGenerated(...)`, `providerFailureFromGenerated(...)`, `factoryWorkDiagnosticsFromGenerated(...)` | nothing additional; raw fields are already gone before reconstruction | safe diagnostics, provider session, and provider failure survive on `FactoryWorldDispatchCompletion` and `FactoryWorldProviderSessionRecord` |
| workstation-request and CLI projection | `pkg/factory/projections/world_view.go`, `pkg/factory/projections/world_view_workstation_requests.go`, and `pkg/cli/dashboard/dashboard.go` clone or rewrap the mirror family | nothing additional; this is mostly contract churn | selected-tick safe diagnostics and provider-session data still reach workstation-request and CLI consumers |

The current drift points are the duplicate allowlist helpers in
`pkg/factory/event_history.go` and `pkg/replay/event_artifact.go`, plus the
generated-to-mirror reconstruction helpers in
`pkg/factory/projections/world_state.go`. Later cleanup stories should replace
those three paths with one shared adapter and one canonical internal safe
diagnostics family.

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Relevant interaction patterns: API contract, event or message contract, and
  shared-library contract. The generated public `FactoryEvent` payload remains
  the external schema source of truth; the cleanup is about consolidating the
  internal safe boundary behind that contract.
- Approved exceptions: none for this inventory story. The current duplicate
  `Factory*` mirror family is the cleanup target for later stories, not an
  approved durable boundary.
