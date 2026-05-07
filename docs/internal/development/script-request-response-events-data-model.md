# Script Request Response Events Data Model

This artifact records the shared public contract and runtime boundary for the
`SCRIPT_REQUEST` and `SCRIPT_RESPONSE` event family added by the Agent Factory
script request/response work. It exists so review can compare the public schema,
generated models, runtime emission, canonical event history, and replay parity
against one stated model instead of inferring that contract from code spread
across packages.

## Change

- PRD, design, or issue: `prd.json` for `agent-factory-script-request-response-events`
- Owner: Agent Factory maintainers
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `api/components/schemas/events`, `api/openapi.yaml`,
  `pkg/api/generated`, `ui/src/api/generated`, `pkg/factory`, `pkg/service`,
  `pkg/workers`, `tests/functional_test`
- Canonical architecture document to update before completion:
  `docs/processes/agent-factory-development.md`
- Canonical package responsibility artifact:
  `docs/architecture/package-responsibilities.md`
- Canonical package interaction artifact:
  `docs/architecture/package-interactions.md`

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
| `SCRIPT_REQUEST` | public event type | Canonical pre-run public event for a resolved script command boundary | `libraries/agent-factory/api/components/schemas/events/FactoryEventType.yaml` | `pkg/api/openapi_contract_test.go`, `pkg/api/generated_contract_test.go` |
| `SCRIPT_RESPONSE` | public event type | Canonical post-run public event for a resolved script command boundary | `libraries/agent-factory/api/components/schemas/events/FactoryEventType.yaml` | `pkg/api/openapi_contract_test.go`, `pkg/api/generated_contract_test.go` |
| `ScriptRequestEventPayload` | public payload | Request-side schema with command name, resolved args, attempt, and dispatch correlation | `libraries/agent-factory/api/components/schemas/events/payloads/ScriptRequestEventPayload.yaml` | `api/openapi.yaml`, `pkg/workers/script_test.go` |
| `ScriptResponseEventPayload` | public payload | Response-side schema with outcome, stdout, stderr, duration, exit code, and stable failure classification | `libraries/agent-factory/api/components/schemas/events/payloads/ScriptResponseEventPayload.yaml` | `api/openapi.yaml`, `pkg/workers/script_test.go`, `tests/functional_test/script_events_test.go` |
| `ScriptExecutionOutcome` | public enum | Stable public outcome surface for script execution | `libraries/agent-factory/api/components/schemas/events/ScriptExecutionOutcome.yaml` | `pkg/workers/script.go`, `pkg/workers/script_test.go` |
| `ScriptFailureType` | public enum | Stable public failure classification when no normal exit code exists | `libraries/agent-factory/api/components/schemas/events/ScriptFailureType.yaml` | `pkg/workers/script.go`, `pkg/workers/script_test.go` |

## Identifiers

| Identifier | Format | Producer | Consumer | Validation evidence |
| --- | --- | --- | --- | --- |
| `scriptRequestId` | Stable event-family correlation ID derived from the dispatch attempt boundary | `pkg/workers/script.go` | `SCRIPT_REQUEST`, `SCRIPT_RESPONSE`, canonical event history readers, replay readers | `pkg/factory/event_history_test.go`, `tests/functional_test/script_events_test.go` |
| `dispatchId` | Existing dispatch correlation field on `FactoryEvent.context` | runtime dispatch history / event recorder | public event consumers, replay readers, projections | `pkg/factory/event_history_test.go`, `tests/functional_test/script_events_test.go` |
| `attempt` | One-based integer for the concrete command attempt | `pkg/workers/script.go` | public event consumers and replay readers | `pkg/workers/script_test.go`, `tests/functional_test/script_events_test.go` |

## Lifecycle States

| State | Owner | Allowed transitions | Terminal? | Evidence |
| --- | --- | --- | --- | --- |
| `SCRIPT_REQUEST` emitted | script worker runtime | `SCRIPT_REQUEST` -> `SCRIPT_RESPONSE` for the same `scriptRequestId` once a concrete command request exists | No | `pkg/workers/script.go`, `tests/functional_test/script_events_test.go` |
| `SUCCEEDED` | public response payload outcome | terminal response outcome | Yes | `pkg/workers/script.go`, `pkg/workers/script_test.go` |
| `FAILED_EXIT_CODE` | public response payload outcome | terminal response outcome | Yes | `pkg/workers/script.go`, `pkg/workers/script_test.go` |
| `TIMED_OUT` | public response payload outcome | terminal response outcome | Yes | `pkg/workers/script.go`, `pkg/workers/script_test.go` |
| `PROCESS_ERROR` | public response payload outcome | terminal response outcome | Yes | `pkg/workers/script.go`, `pkg/workers/script_test.go` |

## Configuration Shapes

No new shared configuration shape is introduced by this change. Script request
and response events are derived from the existing worker runtime boundary and
the existing public `FactoryEvent` union.

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| Public script-event schema family (`FactoryEventType`, `FactoryEvent.payload.oneOf`, payload schemas, enums) | OpenAPI source under `api/components/schemas/events/` | bundled OpenAPI, generated Go models, generated UI models, contract tests | `api/` source -> generated consumers and tests | Schema drift would create mismatched public event unions or generated payloads | `api/openapi.yaml`, `pkg/api/openapi_contract_test.go`, `pkg/api/generated_contract_test.go`, `ui/src/api/events/types.test.ts` |
| Typed script-event recorder on `FactoryEventHistory` | `pkg/factory/event_history.go` | `pkg/service/factory.go`, `pkg/workers/script.go`, replay and live event readers | `pkg/factory` owns canonical history; workers emit through injected recorder | non-script workers must not publish script events; event IDs and correlation must remain stable | `pkg/factory/event_history_test.go`, `pkg/service/factory_test.go` |
| Script runtime emission boundary | `pkg/workers/script.go` | canonical event history, `GetFactoryEvents(...)`, replay artifacts | worker runtime -> shared history contract -> public readers | timeout and process-launch failures must collapse to stable public classification without leaking raw env or stdin | `pkg/workers/script_test.go`, `tests/functional_test/script_events_test.go` |

## Shared Package or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent selected:
  generated `FactoryEvent` contract rooted in the split OpenAPI event schemas
- Package-local model selected:
  raw command diagnostics remain package-local on worker diagnostics and
  `WorkResult`
- Reason:
  the public event pair is customer-visible and used across API, replay, UI,
  and runtime history, while raw command environment values and stdin remain
  internal-only implementation data
- Translation boundary:
  `pkg/workers/script.go` projects worker-internal command diagnostics into the
  public generated payloads before recording them through `FactoryEventHistory`
- Review evidence:
  `pkg/workers/script_test.go` and
  `tests/functional_test/script_events_test.go` assert the public payload keeps
  command/args/stdout/stderr/duration/outcome fields while omitting raw env and
  stdin

## Consolidation Review

| Duplicate or near-duplicate model | Location | Decision | Owner | Follow-up |
| --- | --- | --- | --- | --- |
| Worker-internal command diagnostics versus public script-event payloads | `pkg/interfaces`, `pkg/workers`, generated `pkg/api/generated` payloads | justify | Agent Factory maintainers | Keep the public payload projected from the internal diagnostics boundary; do not share raw env/stdin-bearing structs with the public contract |
| Script event history emission versus worker-local event channels | `pkg/factory`, `pkg/service`, `pkg/workers` | unify | Agent Factory maintainers | Resolved in this branch by reusing the shared `FactoryEventHistory` recorder rather than introducing a second history channel |

## Reviewer Notes

- Interop structs or config models that intentionally differ from the canonical
  model:
  raw `WorkResult` and command diagnostics remain richer than the public
  payload because the public contract intentionally excludes raw stdin and raw
  environment values
- Approved exceptions with owner, reason, scope, expiration, and removal
  condition:
  none
- Follow-up cleanup tasks:
  none required for this event family beyond the code and process updates
  already on this branch
