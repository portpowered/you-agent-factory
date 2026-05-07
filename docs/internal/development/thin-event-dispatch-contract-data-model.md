# Thin Event Dispatch Contract Data Model

This artifact records the canonical ownership rules for the Agent Factory thin
dispatch and inference event cleanup. It names which identifiers now belong to
`FactoryEvent.context`, which facts remain owned by each payload, and which
views downstream reducers must derive from earlier events or the initial
structure.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch
  `ralph/agent-factory-thin-event-dispatch-contract-cleanup`)
- Owner: Codex branch
  `ralph/agent-factory-thin-event-dispatch-contract-cleanup`
- Packages or subsystems: `libraries/agent-factory/api`,
  `libraries/agent-factory/pkg/api`, `pkg/factory`, `pkg/replay`,
  `pkg/workers`, and Agent Factory replay or event-history fixtures
- Canonical architecture document to update before completion: this file is the
  branch data-model construction artifact. Durable workflow rules live in
  `docs/processes/agent-factory-development.md`.

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Ownership Table

| Fact | Canonical owner | Allowed copies | Notes |
| --- | --- | --- | --- |
| `dispatchId` | `FactoryEvent.context.dispatchId` | none in dispatch or inference payloads | Dispatch and inference readers must correlate through event context. |
| `requestId` | `FactoryEvent.context.requestId` | none in dispatch payload metadata | Request identity follows the shared event context boundary. |
| `traceIds` | `FactoryEvent.context.traceIds` | none in dispatch or inference payloads | Read models join traces from context rather than payload duplication. |
| `workIds` | `FactoryEvent.context.workIds` | none in dispatch or inference payloads | Work correlation stays in the shared event context. |
| dispatch consumed work details | prior `WORK_REQUEST` events keyed by `DispatchRequestEventPayload.inputs[].workId` | no copied work names, tags, types, or traces on dispatch payload refs | Dispatch payload refs preserve ordered consumed-work identity only; reducers and tests must join back to prior work-request history for detailed work facts. |
| dispatch transition id | `DispatchRequestEventPayload.transitionId` and `DispatchResponseEventPayload.transitionId` | none on inference payloads | Inference events derive transition ownership from the matching dispatch request. |
| workstation topology | initial structure plus `transitionId` | none on dispatch request or response payloads | Readers derive workstation name, routes, and configured worker from topology. |
| worker topology | initial structure plus workstation binding | none on dispatch request or response payloads | Provider or model details come from topology-owned worker definitions. |
| inference attempt id | `InferenceRequestEventPayload.inferenceRequestId` and matching response | none elsewhere | This remains the provider-boundary correlation key. |
| provider-attempt facts | inference request or response payloads | not restated in dispatch payload metadata | Prompt and provider results stay on inference-boundary events. |

## Representative Canonical Event Shapes

| Event type | Context-owned fields | Payload-owned fields | Derived fields |
| --- | --- | --- | --- |
| `DISPATCH_REQUEST` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `transitionId`, ordered consumed-work refs in `inputs`, `resources`, replay-only `metadata` | workstation, worker, provider, model, consumed work names or tags |
| `INFERENCE_REQUEST` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `inferenceRequestId`, `attempt`, `workingDirectory`, `worktree`, `prompt` | transition id |
| `INFERENCE_RESPONSE` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `inferenceRequestId`, `attempt`, `outcome`, `response`, `durationMillis`, `exitCode`, `errorClass` | transition id |
| `DISPATCH_RESPONSE` | `dispatchId`, `traceIds`, `workIds` | `completionId`, `transitionId`, `outcome`, completion diagnostics, output work or resources | workstation, worker |

## Inter-Package Contracts

| Contract | Producer | Consumer | Allowed dependency direction | Error cases | Evidence |
| --- | --- | --- | --- | --- | --- |
| OpenAPI thin-event ownership contract | `api/components/schemas/events/` | generated Go or UI models, runtime writers, replay reducers, contract fixtures | OpenAPI authoring -> bundled artifact -> generated consumers | reintroducing duplicate identity or topology fields widens the public contract and drifts reducers | `api/openapi-main.yaml`, `api/openapi.yaml`, `pkg/api/openapi_contract_test.go` |
| Runtime event writers | `pkg/factory/event_history.go`, `pkg/workers/recording_provider.go` | replay reducer, world-state reducer, live `/events` consumers | runtime writers -> canonical event log | payload copies can disagree with `FactoryEvent.context` or topology state | writer tests plus reducer tests |
| Reducer derivation boundary | `pkg/factory/projections`, `pkg/replay` | dashboard views, replay hydration, selected-tick API | canonical event log -> derived read models | reducer code that still depends on retired payload copies breaks as schemas thin | projection tests and replay tests |

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact: `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Canonical owner note: `FactoryEvent.context` owns shared identity and
  correlation. Dispatch and inference payloads own only first-known
  event-specific facts that cannot be reconstructed from prior events or the
  initial structure.
- Public-contract drift note: this change intentionally narrows the published
  event payload schemas. Contract tests and the canonical fixture must fail if
  retired dispatch or inference duplicate fields return.
