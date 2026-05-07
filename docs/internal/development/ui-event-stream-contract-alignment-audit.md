# Agent Factory UI Event Stream Contract Alignment Audit

This audit records the current canonical `/events` contract used by the Agent
Factory backend and the UI-side assumptions that still need cleanup. It exists
to keep the parsing layer, timeline reducer, and stream-driven tests aligned on
one event-first vocabulary before narrower cleanup or regression stories land.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch
  `ralph/agent-factory-ui-event-stream-contract-alignment`)
- Owner: Codex branch
  `ralph/agent-factory-ui-event-stream-contract-alignment`
- Packages or subsystems: `libraries/agent-factory/ui/src/api/events`,
  `libraries/agent-factory/ui/src/state/factoryTimelineStore.ts`, and the
  App-level event-stream tests in `libraries/agent-factory/ui/src/App.test.tsx`
- Canonical backend contract source:
  `libraries/agent-factory/ui/src/api/generated/openapi.ts`

## Canonical Ownership Summary

The generated OpenAPI contract defines `FactoryEvent.context` as the canonical
owner for shared event identity:

- `requestId`
- `traceIds`
- `workIds`
- `dispatchId`

Payloads keep only facts first known at that boundary:

- `DISPATCH_REQUEST` owns `transitionId`, chaining-trace lineage, ordered
  consumed work refs in `inputs`, optional `resources`, and replay-only
  `metadata`.
- `DISPATCH_RESPONSE` owns `transitionId`, chaining-trace lineage, outcome, and
  completion facts such as output work and failure details.
- `INFERENCE_REQUEST` and `INFERENCE_RESPONSE` own provider-attempt facts only;
  they no longer canonically own `dispatchId` or `transitionId`.
- `SCRIPT_REQUEST` and `SCRIPT_RESPONSE` still own `dispatchId` and
  `transitionId` in payload because the public script contract retains those
  fields.

## UI Alignment Seams

- `libraries/agent-factory/ui/src/api/events/types.ts`
  Handwritten UI event interfaces currently mix generated payload types with
  compatibility aliases and legacy payload copies.
- `libraries/agent-factory/ui/src/state/factoryTimelineStore.ts`
  The reducer still carries compatibility reads for retired payload fields while
  reconstructing topology and world state.
- `libraries/agent-factory/ui/src/App.test.tsx`
  The stream-driven smoke fixtures still demonstrate some retired payload shapes
  and compatibility paths.

## Canonical Contract Delta vs Current UI Assumptions

### 1. Dispatch identity moved to `FactoryEvent.context`

Canonical contract:

- `DispatchRequestEventPayload` and `DispatchResponseEventPayload` do not own
  `dispatchId`.
- Consumers should read `dispatchId` from `FactoryEvent.context.dispatchId`.

Current UI assumptions:

- `types.ts` still declares `dispatchId` on handwritten dispatch request and
  dispatch response payload interfaces.
- `factoryTimelineStore.ts` still falls back to `event.payload.dispatchId` in
  `applyRequest(...)` and `applyResponse(...)`.

Compatibility status:

- Temporary compatibility remains in the reducer today, but it is legacy-only
  and should be removed once fixtures and event sources stop emitting the stale
  field.

### 2. Inference payload identity was thinned

Canonical contract:

- Inference payloads keep `inferenceRequestId`, `attempt`, prompt or response,
  duration, and provider-attempt details.
- They do not canonically own `dispatchId` or `transitionId`.

Current UI assumptions:

- `factoryTimelineStore.ts` still uses
  `legacyInferencePayloadDispatchID(...)` and
  `legacyInferencePayloadTransitionID(...)`.
- `types.test.ts` proves only the generated inference payload fields, but the
  reducer still preserves temporary reads for retired payload identity.

Compatibility status:

- Temporary compatibility is still required in the reducer until the existing
  UI fixtures and any retained recorded artifacts stop depending on the old
  payload shape.

### 3. Dispatch topology should be derived, not copied

Canonical contract:

- Dispatch payloads do not own `worker` or `workstation`.
- Reducers derive workstation and worker details from initial structure plus
  `transitionId`.

Current UI assumptions:

- `types.ts` still includes `worker` and `workstation` on dispatch request and
  dispatch response payloads.
- `factoryTimelineStore.ts` still reads those payload copies when projecting
  model/provider and workstation names, even though the same facts are
  reconstructible from topology.

Compatibility status:

- Compatibility is not required for the current canonical contract. These are
  stale payload aliases that should be retired in the alignment stories.

### 4. Dispatch inputs now keep work identity only

Canonical contract:

- `DispatchRequestEventPayload.inputs` is `DispatchConsumedWorkRef[]` and keeps
  only `workId`.
- Work names, trace lineage, and work types must be rebuilt from prior
  `WORK_REQUEST` history plus `FactoryEvent.context`.

Current UI assumptions:

- `types.ts` still models dispatch inputs as `Array<FactoryWork | { workId: string }>`
  instead of the thinner canonical reference list.
- `factoryTimelineStore.ts` still supports richer input objects through
  `factoryWorkToItem(...)`.

Compatibility status:

- Compatibility may still be needed for older fixtures or recordings, but the
  canonical UI type layer should move to the thin work-ref contract and keep
  any fallback logic scoped to reducer migration only.

### 5. Dispatch completion provider facts belong on inference responses

Canonical contract:

- `DispatchResponseEventPayload` does not canonically own provider-session or
  safe-diagnostics copies; those stay on inference responses.

Current UI assumptions:

- `types.ts` still declares `diagnostics`, `providerSession`, `worker`, and
  `workstation` on dispatch responses.
- `factoryTimelineStore.ts` still projects provider-session and diagnostics from
  dispatch completion payload first.

Compatibility status:

- Compatibility is not required for the current canonical contract. These
  completion-level copies are stale and should be retired as the reducer and UI
  request views align to inference-boundary ownership.

### 6. Factory shape aliases remain in the UI type layer

Canonical contract:

- Generated event payloads use camelCase field names such as `factoryDir`,
  `sourceDirectory`, `workflowId`, `workTypes`, `inputTypes`, `modelProvider`,
  `sessionId`, `onFailure`, `onRejection`, and
  `currentChainingTraceId` / `previousChainingTraceIds`.

Current UI assumptions:

- `types.ts` still defines broad camelCase plus snake_case compatibility pairs
  across `FactoryDefinition`, `FactoryWorker`, `FactoryWorkstation`,
  `FactoryWork`, and related helpers.
- `factoryTimelineStore.ts` still reads those aliases through helpers such as
  `factoryWorkTypes(...)`, `ioWorkType(...)`, `workerModelProvider(...)`,
  `workstationFailureIO(...)`, and `workstationRejectionIO(...)`.

Compatibility status:

- Compatibility is not required for the current canonical `/events` contract.
  These aliases are stale UI read-model assumptions that should be removed
  where they are only supporting older event shapes.

## Affected Field Ownership Inventory

| Event type | Context-owned fields consumed by UI | Payload-owned fields still needed by UI | UI stale payload assumptions |
| --- | --- | --- | --- |
| `WORK_REQUEST` | `requestId`, `traceIds`, `workIds` | `works`, `relations`, `source`, `parentLineage`, `type` | `FactoryWork` still keeps legacy `work_type_id` alias alongside canonical `work_type_name` |
| `RELATIONSHIP_CHANGE_REQUEST` | `requestId`, `traceIds`, `workIds` | `relation` | none beyond relation field aliases already used in UI read models |
| `DISPATCH_REQUEST` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `transitionId`, chaining-trace lineage, `inputs`, `resources`, replay `metadata` | payload `dispatchId`, `worker`, `workstation`, rich input objects |
| `INFERENCE_REQUEST` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `inferenceRequestId`, `attempt`, `workingDirectory`, `worktree`, `prompt` | payload `dispatchId`, payload `transitionId` fallback |
| `INFERENCE_RESPONSE` | `dispatchId`, `requestId`, `traceIds`, `workIds` | `inferenceRequestId`, `attempt`, `outcome`, `response`, `durationMillis`, provider-attempt details | payload `dispatchId`, payload `transitionId` fallback |
| `SCRIPT_REQUEST` | none required beyond event ordering | `dispatchId`, `transitionId`, `scriptRequestId`, `attempt`, `command`, `args` | none; script payload identity is still canonical |
| `SCRIPT_RESPONSE` | none required beyond event ordering | `dispatchId`, `transitionId`, `scriptRequestId`, `attempt`, `outcome`, `stdout`, `stderr`, `durationMillis`, `exitCode`, `failureType` | none; script payload identity is still canonical |
| `DISPATCH_RESPONSE` | `dispatchId`, `traceIds`, `workIds` | `transitionId`, chaining-trace lineage, outcome, output work, failure details | payload `dispatchId`, `worker`, `workstation`, completion-level `providerSession`, `diagnostics` |

## Implementation Notes For Follow-on Stories

- Start the cleanup in `ui/src/api/events/types.ts` by replacing handwritten
  payload interfaces with generated payload types where the generated contract
  already matches the canonical backend schema.
- Keep reducer migration narrow in
  `ui/src/state/factoryTimelineStore.ts`: derive topology from initial
  structure plus `transitionId`, derive work facts from prior `WORK_REQUEST`
  history, and keep any temporary compatibility fallbacks local to the reducer.
- Use one focused event-stream fixture path in `ui/src/App.test.tsx` or the
  existing timeline-store seam to prove canonical event reduction, then retire
  fixtures that keep the old payload copies alive.
