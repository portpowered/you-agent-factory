# @you/goal API Contract Audit (P11)

Implementation-ready contract boundary for the `@you/goal` slice. This audit
maps each relevant surface to ownership, public vs internal scope, reuse vs
change posture, and follow-on verification expectations.

**Status:** complete (all stories `you-goal-p11-api-contract-audit-001` through
`you-goal-p11-api-contract-audit-003`)

**Last updated:** 2026-07-02 UTC

## Context

The `@you/goal` planning direction is intentionally narrow: reuse existing
public Factory Session invocation, lifecycle, and dispatch contracts; keep
response-stream progress internal to runtime/session models; and treat
`Workstation.workPropagation` as the landed public factory-configuration delta
for this slice. This document is the single reviewer-verifiable artifact
maintainers should cite before widening API scope in follow-on PRs.

Canonical public vocabulary: `docs/architecture/data-model.md` (`Factory`,
`Factory Session`, `Work`, `Work Request`, dispatch lifecycle, invocation
result). Invocation equivalence rules: `docs/architecture/invocation-contract.md`.

## Audit sections

| Section | Story | Status |
|---------|-------|--------|
| [Invocation and adapter reuse boundaries](#invocation-and-adapter-reuse-boundaries) | `you-goal-p11-api-contract-audit-001` | complete |
| [Response streams vs lifecycle contracts](#response-streams-vs-lifecycle-contracts) | `you-goal-p11-api-contract-audit-002` | complete |
| [Public OpenAPI delta and generated-client expectations](#public-openapi-delta-and-generated-client-expectations) | `you-goal-p11-api-contract-audit-003` | complete |

---

## Invocation and adapter reuse boundaries

`@you/goal` is a packaged named factory (`@you/goal`, project
`builtin-goal`) resolved through the same live-session invocation path as other
authored factories. Goal-mode work must not introduce a parallel invocation or
result transport.

### Boundary matrix

| Surface | Ownership | Public / internal | @you/goal posture | Follow-on verification |
|---------|-----------|-------------------|--------------------|------------------------|
| `POST /factory-sessions/{session_id}/invocations` | Shared live-session invocation API (`invokeFactorySessionBySessionId`) | **Public** | **Reuse as-is** — canonical API entrypoint for `@you/goal` invocations against an open live Factory Session | API regression in `tests/functional/runtime_api/api_session_invocation_test.go`; contract description in `api/openapi-main.yaml` |
| `InvocationRequest` / `InvocationResponse` | OpenAPI work schemas + `pkg/transports/mapping` projection | **Public** | **Reuse as-is** — text-first input via `WorkContent`; terminal status and `primaryResult` on `InvocationResponse` | OpenAPI contract tests in `pkg/transports/http/contracttests/`; generated client sync via `make generate-api` only when these schemas change |
| `Factory.invocationReturn` | Factory configuration on the active session factory | **Public** | **Reuse as-is** — sole public selector for the final primary result; default `SUBMITTED_WORK_TERMINAL` when omitted | Factory validation before runtime; primary-result selection tests in `pkg/invocations/primary_result_test.go` |
| `you run --factory` / `you run --named @you/goal` | CLI transport adapter in `pkg/transports/cli/run/` | **Public CLI** | **Reuse as-is** — must call the same shared resolver and primary-result selector as the API; no goal-specific transport contract | CLI tests in `pkg/transports/cli/run/run_invocation_test.go`, `pkg/transports/cli/root_run_factory_prompt_test.go`, and `pkg/transports/cli/root_run_test.go` (`run --named @you/goal`) |
| Shared invocation resolver (`pkg/invocations`) | Backend-owned input and return-policy logic | **Internal shared contract** | **Reuse as-is** — `ResolveTextInput` / `ResolveAPITextInputContent` for input; `ResolvePrimaryResult` for return policy | Unit tests in `pkg/invocations/input_test.go` and `pkg/invocations/primary_result_test.go` |
| Goal-only result endpoint | — | — | **Out of scope** — this slice does **not** add `/goal/...`, `/results` variants, or any goal-named primary-result route | N/A — reject proposals that bypass `InvocationResponse.primaryResult` |

### Canonical public API entrypoint

`POST /factory-sessions/{session_id}/invocations` is the canonical public API
entrypoint for `@you/goal`. The route:

- Accepts one text-first `InvocationRequest` against an already-open live
  Factory Session (`session_id`, typically `~default` for CLI-started sessions).
- Submits work into the session runtime and blocks until terminal
  primary-result selection completes (subject to `timeoutMillis`).
- Returns `InvocationResponse` with `status` and, on success,
  `primaryResult` as `WorkContent`.

Durable JavaScript workflow execution routes (`POST /factory-sessions/async`,
`POST /factory-sessions/sync`, `GET /factory-sessions/{session_id}/results`)
are separate public surfaces for orchestrator-backed durable sessions. They are
**not** the `@you/goal` invocation entrypoint for this slice. Goal follow-on
work that needs one-shot live invocation against the packaged factory must use
the existing invocation route, not durable start/result routes.

**Authoritative references:**

- OpenAPI: `api/openapi-main.yaml` (`/factory-sessions/{session_id}/invocations`)
- Handler: `pkg/transports/http/handlers_work_write.go` → `Server.InvokeFactorySessionBySessionId`
  (delegates to session runtime `InvokeFactorySession`)
- Service: `pkg/service/model_catalog.go` (`InvokeFactorySession`,
  `resolveSessionInvocationInput`, session wait + primary-result projection)

### `Factory.invocationReturn` as the public result selector

`Factory.invocationReturn` on the active session factory remains the **only**
public configuration for selecting the invocation's final primary result:

| Policy | Behavior |
|--------|----------|
| *(omitted)* | `SUBMITTED_WORK_TERMINAL` — follow submitted work to terminal output and return that content as `InvocationResponse.primaryResult` |
| `EXPLICIT` | Select by configured `workTypeName`, `terminalState`, and optional `workName` within the invocation submit scope |

This slice does **not** add:

- A goal-only result endpoint or response wrapper.
- Per-invocation return-policy overrides on `InvocationRequest`.
- A separate public selector for streaming or partial progress (those belong to
  internal response-stream models; see story 002).

Primary-result selection is implemented once in `pkg/invocations/primary_result.go`
(`ResolvePrimaryResult`) and consumed by `pkg/service/model_catalog.go` after
the session invocation wait completes. Unresolved selection surfaces as
`InvocationResponse` status `FAILED` with code
`INVOCATION_PRIMARY_RESULT_UNRESOLVED` and no `primaryResult`, matching the
documented OpenAPI contract.

The built-in `@you/goal` factory payload (`pkg/config/builtingoal/factory.json`,
embedded through `pkg/config/layout.go` as `BuiltInGoalFactoryJSON`) already
declares an explicit `invocationReturn` policy:

| Field | Built-in value |
|-------|----------------|
| `policy` | `EXPLICIT` |
| `workTypeName` | `goal` |
| `terminalState` | `complete` |

Terminal `goal:complete` work content is therefore the configured
`InvocationResponse.primaryResult` for `@you/goal` invocations. Maintainer
follow-on work must preserve this authored policy through normal factory
configuration and materialization — not through new API routes.

### CLI adapter reuse requirements

CLI transports must remain thin adapters over the shared invocation contract:

| CLI path | Adapter responsibility | Must not |
|----------|------------------------|----------|
| `you run --factory <factory.json> <text>` | Resolve positional/stdin text via `ResolveFactoryInvocationInput` → build `InvocationRequest` → `InvokeFactorySession` on `~default` | Invent separate conflict, empty-input, or primary-output rules |
| `you run --named @you/goal <text>` | Resolve named factory to the same live-session invocation flow as `--factory` | Add goal-specific flags, headers, or response shapes |
| Success output | Format `InvocationResponse.primaryResult` (e.g. text extraction in `writeInvocationSuccess`) | Return a goal-specific JSON envelope not derived from `InvocationResponse` |

**Shared resolver chain (CLI and API must converge here):**

1. **Input:** `pkg/invocations.ResolveTextInput` (CLI positional/stdin) and
   `pkg/invocations.ResolveAPITextInputContent` (API `WorkContent`) — same
   conflict and empty-input errors (`INVOCATION_INPUT_SOURCE_CONFLICT`,
   `INVOCATION_INPUT_EMPTY`).
2. **Submit + wait:** `FactoryService.InvokeFactorySession` with session
   `invocationReturn` from runtime factory config.
3. **Primary result:** `invocations.ResolvePrimaryResult` against selected-tick
   world state — same policy semantics for CLI and API callers.

Transport files: `pkg/transports/cli/run/factory_invocation_input.go`,
`pkg/transports/cli/run/run.go`. Equivalence requirement is normative in
`docs/architecture/invocation-contract.md`.

### Reviewer evidence for this boundary

Maintainers reviewing `@you/goal` invocation follow-on PRs should verify:

| Evidence | What it proves |
|----------|----------------|
| `docs/architecture/invocation-contract.md` | CLI/API equivalence and return-policy ownership are documented public contract |
| `pkg/invocations/input.go`, `pkg/invocations/primary_result.go` | Single backend-owned resolver and selector — no transport-specific forks |
| `pkg/service/model_catalog.go` (`InvokeFactorySession`, `resolveSessionInvocationInput`) | API path uses shared resolver; `invocationReturn` read from session factory config |
| `pkg/transports/cli/run/factory_invocation_input.go` | CLI path uses `invocations.ResolveTextInput` and `InvokeFactorySession` |
| `pkg/invocations/input_test.go`, `pkg/invocations/primary_result_test.go` | Unit coverage for input conflicts and both return policies |
| `pkg/transports/cli/run/run_invocation_test.go` | CLI request construction from positional/stdin sources |
| `pkg/transports/cli/run/run_invocation_test.go` (`TestResolveFactoryInvocationRequest_NamedFactory*`) | Named factory positional/stdin text selects standard invocation mode and builds `InvocationRequest` through the shared resolver (same code path as `@you/goal`) |
| `tests/functional/runtime_api/api_session_invocation_test.go` | End-to-end API invocation returns `primaryResult`; explicit `invocationReturn` policy honored |
| `pkg/transports/cli/root_run_test.go` (`TestRunCommand_NamedFactoryResolutionMetadataFlowsForBuiltInGoal`) | `run --named @you/goal` resolves built-in goal factory metadata into `RunConfig` at the CLI flag layer |

**Follow-on implementation PRs** that touch invocation behavior should extend
the evidence above (API functional tests, CLI run tests, and
`pkg/invocations` unit tests) rather than adding goal-specific routes or
adapters. Public OpenAPI or generated-client changes are **not** required for
invocation reuse in this slice.

---

## Response streams vs lifecycle contracts

`@you/goal` goal-mode progress must not widen the public OpenAPI surface through
ephemeral provider output, partial token streams, or goal-specific control
routes. This section separates **durable public history** (canonical
`FactoryEvent` SSE streams and existing lifecycle routes) from **ephemeral
internal progress** (response-stream plumbing inside the session runtime).

### Durable public history vs ephemeral internal progress

| Concern | Durable public history | Ephemeral internal progress |
|---------|------------------------|-----------------------------|
| Purpose | Replay-safe, customer-visible factory and session facts | Live provider/model output chunks, partial assistant text, and in-flight response assembly before terminal selection |
| Transport | `GET /factory-sessions/{session_id}/events` (canonical SSE `FactoryEvent`); `GET /events` (compatibility-only process-global SSE) | In-process session/runtime subscribers only |
| Persistence | Canonical event history (`pkg/factory/events/`, durable session replay in `pkg/factorysessionexecution/`) | Session-scoped runtime state; not a public REST resource |
| `@you/goal` posture | **Reuse as-is** — observe progress through existing event types and lifecycle vocabulary | **Internal-only** — implement as `SessionResponseStream` / `SessionResponseStreamEvent`; never publish to OpenAPI |

Existing public events such as `MODEL_RESPONSE`, `INFERENCE_RESPONSE`,
`DISPATCH_RESPONSE`, and `WORK_STATE_CHANGE` already record terminal or
checkpoint facts on the canonical stream. They are **not** substitutes for
internal response streams and must not be extended with per-chunk streaming
payloads for goal-mode partial output.

### Boundary matrix

| Surface | Ownership | Public / internal | @you/goal posture | Follow-on verification |
|---------|-----------|-------------------|--------------------|------------------------|
| `GET /events` | Runtime SSE (`getEvents`) | **Public** | **Compatibility-only** — process-wide `FactoryEvent` history + live tail retained for legacy tooling and diagnostics; new session-aware consumers should use `GET /factory-sessions/{session_id}/events` | `pkg/transports/http/servertests/server_dashboard_events_test.go`; reconnect cursors in `pkg/factory/events/event_reconnect_test.go` |
| `GET /factory-sessions/{session_id}/events` | Session-scoped SSE (`getEventsBySessionId`) | **Public** | **Canonical** — same `FactoryEvent` vocabulary filtered to one live or durable session | `pkg/transports/http/servertests/server_durable_session_events_test.go`; `FilterEventsAfterReconnect` in `pkg/factorysessionexecution/listing.go` |
| `FactoryEventType` / event payloads | OpenAPI `api/components/schemas/events/` | **Public** | **Reuse as-is** — no new types for response-stream chunks | `pkg/transports/http/contracttests/openapi_contract_common_test.go`; `ui/src/api/events/types.test.ts` |
| `after_event_id` / `after_sequence` reconnect filters | OpenAPI parameters + `pkg/transports/http/handlers_events.go` | **Public** | **Reuse as-is** — cursor filters apply only to canonical `FactoryEvent` replay | `pkg/factory/projections/projectiontests/session_reconnect_replay_test.go` |
| `POST /factory-sessions/{session_id}/pause` / `/resume` | Durable lifecycle control API | **Public** | **Reuse routes** — behavioral repair may extend live `~default` sessions through the same routes; no `/goal/.../pause` family | `pkg/transports/http/servertests/server_durable_session_lifecycle_control_test.go`; `pkg/factorysessionexecution/control.go` |
| `POST /factory-sessions/{session_id}/cancel` / `/terminate` / `/retry-dispatch` | Durable lifecycle + dispatch recovery | **Public** | **Reuse as-is** — graceful cancel and forced terminate for control; `retry-dispatch` for recoverable interruptions | Same lifecycle servertests; `DISPATCH_RECONCILED` replay in dispatch projection tests |
| Dispatch lifecycle events (`DISPATCH_QUEUED`, `DISPATCH_INTERRUPTED`, `DISPATCH_RECONCILED`) | `pkg/factory/events/event_history_dispatch_lifecycle.go` | **Public** (on canonical stream) | **Reuse vocabulary** — interrupt and recovery facts surface here; no parallel goal run/interrupt API | `pkg/factory/projections/projectiontests/dispatch_lifecycle_event_replay_test.go` |
| `SessionResponseStream` | Session runtime read model (follow-on internal package) | **Internal** | **New internal model only** — aggregates ephemeral provider/model output for subscribers inside the runtime | Runtime/session unit tests near the implementing package; **no** OpenAPI or generated-client changes |
| `SessionResponseStreamEvent` | Per-chunk or per-phase internal stream record | **Internal** | **New internal model only** — names follow-on partial-progress events; must not become `FactoryEventType` values | Same as above |
| Public response-stream endpoint | — | — | **Out of scope** — no `GET .../response-stream`, SSE alias, or WebSocket surface | N/A — reject proposals that expose internal streams on REST |

### Public factory event stream (durable history)

The canonical factory event stream is the **only** normal progress and lifecycle
history transport for new dashboard and Factory Session consumers:

- **Session-scoped (canonical):** `GET /factory-sessions/{session_id}/events`
  streams the same vocabulary for one session (`~default` live sessions and
  `dur-sess-*` durable sessions). Reconnect with `after_event_id` or
  `after_sequence` on this route.
- **Process-wide (compatibility-only):** `GET /events` streams historical then
  live `FactoryEvent` records across the current runtime process. Retained for
  legacy tooling and operator diagnostics—not for new session-aware integrations.

Reconnect clients use `after_event_id` or `after_sequence` (preferring
`FactoryEvent.context.sessionSequence` for session-scoped lifecycle events) to
resume after an acknowledged point. Durable session replays synthesize canonical
events through `pkg/factorysessionexecution/listing.go`
(`BuildCanonicalSessionEvents`, `BuildCanonicalRuntimeSessionEvents`).

**Authoritative references:**

- OpenAPI: `api/openapi-main.yaml` (`/events`,
  `/factory-sessions/{session_id}/events`)
- Handlers: `pkg/transports/http/handlers_events.go`
- Event vocabulary: `api/components/schemas/events/FactoryEventType.yaml`
- UI consumer: `ui/src/features/dashboard/hooks/event-stream/useFactoryEventStream.ts`

Dashboard and timeline features derive lifecycle banners and dispatch state from
these public events (`replayDispatchLifecycle.ts`,
`replaySessionLifecycle.ts`). Goal follow-on work must extend client projections
from existing `FactoryEvent` types, not from internal response-stream models.

### Response streams are internal-only

For `@you/goal`, **response streams are internal runtime/session constructs** in
this slice. They carry ephemeral partial output while work is in flight. They
are **not** public OpenAPI resources.

Follow-on internal implementation must use these stable names:

| Internal name | Role |
|---------------|------|
| `SessionResponseStream` | Session-scoped aggregate of in-flight response output (subscribers, cursors, and flush-to-terminal behavior stay inside the runtime) |
| `SessionResponseStreamEvent` | One internal stream record (chunk, phase marker, or provider-specific progress fact) |

This slice **must not**:

- Add `FactoryEventType` enum values (for example `RESPONSE_STREAM_CHUNK` or
  goal-prefixed variants).
- Add new public event payload schemas or variants under
  `api/components/schemas/events/payloads/` for streaming chunks.
- Extend `GET /events` or `GET /factory-sessions/{session_id}/events` with
  response-stream filters, alternate SSE schemas, or secondary stream URLs.
- Add public response-stream REST or SSE endpoints.
- Regenerate Go or TypeScript public clients for internal response-stream model
  changes (see story 003 for the public-contract generation rule).

Internal streams may inform terminal facts that **later** appear on the canonical
stream (`DISPATCH_RESPONSE`, `WORK_STATE_CHANGE`, invocation
`primaryResult`), but partial chunks themselves stay out of public history.

### Session pause/resume reuse

Existing Factory Session lifecycle control routes are the public pause/resume
surface for this slice:

| Route | Current scope | @you/goal posture |
|-------|---------------|-------------------|
| `POST /factory-sessions/{session_id}/pause` | Durable `dur-sess-*` sessions (runtime-backed and fixture-backed) | **Reuse route** — goal-mode pause must target this route, not a goal-named variant |
| `POST /factory-sessions/{session_id}/resume` | Durable sessions | **Reuse route** — same as pause |

Responses use `FactorySessionLifecycleControlResponse` with typed outcomes
(`ACCEPTED`, `NO_OP`, `INVALID_STATE`, `TERMINAL_SESSION`, `CONFLICT`) and
`FactorySessionLifecycleControlLinks` for post-control inspection of results,
dispatches, and artifacts.

**Behavioral repair without new route families:** Live Factory Sessions opened
through `POST /factory-sessions` or CLI `you run` (session `~default`) may
receive pause/resume **behavior** on the existing `/pause` and `/resume` routes
when follow-on work needs goal-mode control during an open live session. That
repair extends handlers and runtime coordination (`pkg/transports/http/handlers_factory.go`,
`pkg/service/runtime_sessions.go`) — it does **not** justify
`/factory-sessions/{session_id}/goal-pause`, MCP goal tools, or CLI-only pause
commands.

Today, non-durable session IDs may still return `501 NotImplemented` on pause
(see `TestPauseFactorySession_NonDurableSessionPreservesLiveStub`). Treat that
as a known behavioral gap repairable on the existing public routes, not as
evidence that goal mode needs a separate control API.

### Interrupt behavior and dispatch lifecycle vocabulary

Goal-mode interrupt and recovery must reuse **existing dispatch lifecycle
vocabulary** on the canonical event stream instead of inventing a parallel run
or interrupt API:

| Concept | Public vocabulary | Must not add |
|---------|-------------------|--------------|
| Dispatch interrupted | `DISPATCH_INTERRUPTED` (`DispatchInterruptedEventPayload`: `reason`, `observedStatus`, `interruptedAt`, `retryPlanned`, optional refs) | `POST /goal/interrupt`, per-goal cancel routes, or invocation-scoped interrupt endpoints |
| Dispatch queued | `DISPATCH_QUEUED` | Goal-specific queue event types |
| Dispatch reconciled | `DISPATCH_RECONCILED` | Separate recovery/rollback API families |
| Session stop | `POST /factory-sessions/{session_id}/cancel` or `/terminate` (durable) | Goal-named terminate routes |

Runtime emission lives in `pkg/factory/events/event_history_dispatch_lifecycle.go`;
replay projection in `pkg/factory/projections/world_state_dispatch.go`. CLI and
dashboard consumers already map `DISPATCH_INTERRUPTED` in
`ui/src/api/events/types.ts`.

For live `@you/goal` invocations, user-initiated interrupt during an in-flight
`POST /factory-sessions/{session_id}/invocations` should be modeled through
session/runtime cancellation that ultimately surfaces as dispatch lifecycle facts
(`DISPATCH_INTERRUPTED` and related session bracket events), not through a new
public "stop goal run" endpoint.

### Reviewer evidence for this boundary

Maintainers reviewing `@you/goal` streaming and control follow-on PRs should
verify:

| Evidence | What it proves |
|----------|----------------|
| This section + landed public OpenAPI delta | Response streams stay internal; `Workstation.workPropagation` is the landed public factory-configuration delta |
| `api/components/schemas/events/FactoryEventType.yaml` | No new public event types for streaming chunks |
| `pkg/transports/http/handlers_events.go` | Public SSE exposes only `FactoryEvent`; no response-stream branch |
| `pkg/factory/events/event_history_dispatch_lifecycle.go` | Interrupt facts use `DISPATCH_INTERRUPTED` emission path |
| `pkg/factory/projections/projectiontests/dispatch_lifecycle_event_replay_test.go` | Dispatch lifecycle replay reconstructs interrupted/recovered state |
| `pkg/transports/http/servertests/server_durable_session_lifecycle_control_test.go` | Pause/resume/cancel/terminate reuse existing lifecycle routes |
| `pkg/factorysessionexecution/control.go` | Lifecycle control semantics and idempotent replay for durable sessions |
| `ui/src/api/events/types.ts` | Dashboard event mapping includes `DISPATCH_INTERRUPTED` without response-stream types |

**Follow-on implementation PRs** for internal `SessionResponseStream` work
should add runtime/session unit and integration tests near the implementing
packages. They must **not** require `make generate-api`, OpenAPI fragment
changes, or contract smoke updates for stream-only diffs.

## Merged packaged goal factory contract

The built-in `@you/goal` factory (`pkg/config/builtingoal/factory.json`) and
its materialized on-disk layout already encode the merged goal slice. Maintainer
docs must describe this behavior as present — not as follow-on work.

### Decision-envelope parsing is explicit

Workstations that need structured reviewer/checker JSON must set
`outcomeFormat: "decision-envelope"` in factory JSON. The runtime routes agent
output through the envelope parser only when that field is authored; it does
**not** infer decision envelopes from arbitrary JSON in worker output.

| Workstation | Parsing mode | Routing |
|-------------|--------------|---------|
| `structured-review-goal` | `outcomeFormat: "decision-envelope"` | `classificationRoutes` on goal-routing labels such as `accepted`, `needs_changes`, `blocked`, `needs_human`, and `interrupted` |
| `review-goal` | stop-token / classifier routing (no `outcomeFormat`) | `classificationRoutes` on plain classifier labels from reviewer output |

Authoritative envelope shape and vocabulary:
`factory/docs/decision-envelope.md`, `pkg/packagedfactories/goal/decision_envelope.go`,
and `pkg/workers/executor/agent.go`.

### Decision routing, non-success states, and recovery

The packaged goal topology routes review outcomes to authored goal states
without goal-specific public endpoints:

| Routed state | Meaning | Public recovery surface |
|--------------|---------|-------------------------|
| `goal:complete` | Terminal success; `invocationReturn` selects this as `primaryResult` | Successful `InvocationResponse` |
| `goal:blocked` | Review routed to blocked | `INVOCATION_BLOCKED` plus existing session/work inspection |
| `goal:needs-human` | Review routed to human escalation | `INVOCATION_NEEDS_HUMAN` plus existing session/work inspection |
| `goal:interrupted` | Review or dispatch interruption routed stop | `INVOCATION_INTERRUPTED` plus dispatch/session inspection |
| `goal:failed` | Workstation failure or guard exhaustion | `INVOCATION_RUNTIME_FAILURE` or unresolved primary result |

Pause, resume, inspect, and batch/headless behavior reuse existing Factory
Session, work, and dispatch surfaces:

- **Batch/headless:** default `you run --named @you/goal` batch mode completes
  without browser or dashboard interaction for the normal success path; see
  `docs/reference/packaged-goal.md`.
- **Pause/resume:** `POST /factory-sessions/{session_id}/pause` and `/resume`
  plus `you session resume <session-id>`; no `/goal/...` control routes.
- **Inspect-first recovery:** `you session show`, `you work show`, and matching
  REST reads before applying existing work/session controls.
- **Interrupt:** dispatch lifecycle facts (`DISPATCH_INTERRUPTED`) and shared
  invocation non-success codes; no goal-named interrupt endpoint.

Customer-facing wording for these flows lives in `docs/reference/packaged-goal.md`.
Maintainer process maps: `docs/internal/processes/invocation-relevant-files.md`
and `docs/internal/processes/api-relevant-files.md`.


`@you/goal` landed its expected public factory-configuration delta on the
existing `Workstation` schema through the merged `you-goal-p07-define-work-propagation-config`
work. Everything else in this slice — invocation routes, event vocabulary,
lifecycle routes, internal response streams, and CLI invocation adapters —
reuses or repairs existing public surfaces without new OpenAPI route families or
generated-client churn.

### Landed public OpenAPI delta

| Surface | Current state | @you/goal posture | Follow-on verification |
|---------|---------------|-------------------|------------------------|
| `Workstation.workPropagation` | **Present in OpenAPI** — optional object field on `api/components/schemas/data-models/Workstation.yaml` referencing `WorkPropagation.yaml` | **Reuse as-is** — landed public contract for this slice | `make generate-api`; `make api-smoke`; factory config load/save round-trip tests; dashboard factory-definition decode when the field is authorable in UI |
| `WorkPropagationMode` | **Present in OpenAPI** — enum on `api/components/schemas/data-models/WorkPropagationMode.yaml` with values `OUTPUT_AS_PAYLOAD` and `PRESERVE_INPUT` only | **Reuse as-is** — companion enum for `workPropagation.mode` | Same as `workPropagation`; enum normalization in `pkg/interfaces/public_factory_enums.go` when runtime projection changes |
| All other `Workstation` fields | Present in `Workstation.yaml` | **Reuse as-is** — no goal-prefixed duplicates | Existing factory validation and topology tests |
| `Factory`, `FactorySession`, invocation, event, lifecycle schemas | Present | **Reuse as-is** — stories 001–002 lock reuse posture | Existing contract and servertests only |
| `SessionResponseStream` / `SessionResponseStreamEvent` | Internal runtime models (not in OpenAPI) | **Internal-only** — must not appear in authored OpenAPI fragments | Runtime/session tests near implementing packages; **no** `make generate-api` |

Factory JSON uses the structured object syntax:

```json
"workPropagation": { "mode": "PRESERVE_INPUT" }
```

Omit `workPropagation` to use the default `OUTPUT_AS_PAYLOAD` behavior. Only
`OUTPUT_AS_PAYLOAD` and `PRESERVE_INPUT` are supported modes.

No other public OpenAPI additions are in scope for `@you/goal` in this slice:
no new routes, no new `FactoryEventType` values, no response-stream endpoints,
and no goal-prefixed configuration objects parallel to `Workstation`.

### Stable follow-on terminology

Later `@you/goal` PRs must use **exactly** these names across docs, OpenAPI
fragments, backend config mapping, CLI/API adapters, and generated clients:

| Name | Layer | Usage rule |
|------|-------|------------|
| `Workstation.workPropagation` | Public OpenAPI + factory JSON | JSON property name on each `Workstation` object in factory configuration; OpenAPI field on `Workstation.yaml` |
| `WorkPropagationMode` | Public OpenAPI enum | Enum type for `workPropagation` values; fragment file `WorkPropagationMode.yaml` (or equivalent) registered in `api/openapi-main.yaml` |
| `SessionResponseStream` | Internal runtime/session | In-process aggregate for ephemeral provider output; **never** a public schema, route, or generated-client type |
| `SessionResponseStreamEvent` | Internal runtime/session | One internal stream record; **never** a `FactoryEventType`, SSE payload, or public client export |

**Naming consistency requirements:**

- Docs and planning artifacts use `Workstation.workPropagation` (schema-qualified)
  when referring to the public field; use `workPropagation` only when describing
  JSON/YAML on disk.
- OpenAPI `x-enum-varnames` for `WorkPropagationMode` follow the landed enum
  members `WorkPropagationModeOutputAsPayload` and
  `WorkPropagationModePreserveInput`.
- Go internal structs may use idiomatic Go field names (`WorkPropagation`) but
  JSON tags and OpenAPI property names must remain `workPropagation`.
- TypeScript factory-definition decoders must read the generated OpenAPI
  `Workstation` shape — no parallel `goalWorkPropagation` or UI-only property
  names.
- CLI factory validate/save paths must accept the same factory JSON the API
  exposes; no goal-only flags that bypass `Workstation.workPropagation`.

### Generated-client and artifact rules

Public generated artifacts are derived from authored OpenAPI only. Internal
response-stream work must not touch them.

| Change class | Regenerate? | Required verification |
|--------------|-------------|----------------------|
| Add or modify `Workstation.workPropagation` / `WorkPropagationMode` in authored OpenAPI | **Yes** — run `make generate-api` | `api/openapi.yaml`, `pkg/transports/http/generated/server.gen.go`, `pkg/transports/http/client/client.gen.go`, `ui/src/api/generated/openapi.ts` must match authored fragments; run `make api-smoke` when feasible |
| Internal `SessionResponseStream` / `SessionResponseStreamEvent` implementation | **No** | Package-local runtime/session tests; reject PRs that regenerate public clients for stream-only diffs |
| Invocation reuse, lifecycle behavioral repair, dispatch vocabulary reuse | **No** — unless an unrelated public schema also changes | Extend existing API/CLI/servertests from stories 001–002 |
| Dashboard factory editor exposing `workPropagation` | **Yes** — when authored OpenAPI or editor behavior changes | UI decoders in `ui/src/api/factory-definition/` aligned with generated `Workstation`; `make ui-test` / `make verify-fast` for affected modules |

**Generation pipeline (public contract changes only):**

1. Author fragments under `api/components/schemas/data-models/` and register in
   `api/openapi-main.yaml`.
2. Run `make generate-api` (bundles `api/openapi.yaml`, regenerates Go server
   and client, regenerates `ui/src/api/generated/openapi.ts`).
3. Wire config load/save through `pkg/config/factory_config_mapping*.go` and
   validation through `pkg/factory/validation/` if the field affects runtime
   topology or dispatch behavior.
4. Map API read/write surfaces through `pkg/transports/mapping/` when factory
   configuration crosses the HTTP boundary.
5. Run `make api-smoke` to prove bundled contract, generated drift checks, and
   contract integration smoke stay aligned.

**Explicitly not required** for internal-only response-stream PRs:

- `make generate-api` or edits to generated files listed above.
- `make api-smoke` solely because stream subscribers changed.
- Meta tests that scan the repository for forbidden route names or file
  inventories — observable API, CLI, runtime, and event behavior tests are
  sufficient.

### Boundary matrix (public vs internal artifacts)

| Artifact | Role | @you/goal posture |
|----------|------|-------------------|
| `api/openapi-main.yaml` + `api/components/` | Authored public contract | `Workstation.workPropagation` already present; avoid unrelated goal OpenAPI churn |
| `api/openapi.yaml` | Bundled contract | Regenerated; never hand-edited |
| `pkg/transports/http/generated/server.gen.go` | Go server types | Regenerated only on public OpenAPI change |
| `pkg/transports/http/client/client.gen.go` | Go HTTP client | Regenerated only on public OpenAPI change |
| `ui/src/api/generated/openapi.ts` | TypeScript API types | Regenerated only on public OpenAPI change |
| `pkg/transports/http/contracttests/` | Contract smoke and authoring guards | Extend when `Workstation` schema changes; no new inventory tests for streams |
| `pkg/config/builtingoal/factory.json` (`BuiltInGoalFactoryJSON`) | Built-in factory payload | Carries explicit `invocationReturn` and goal routing; may author `workPropagation` through normal factory JSON without new routes |
| Internal response-stream packages | Runtime/session plumbing | New Go packages only; zero OpenAPI footprint |

### Reviewer evidence for this boundary

Maintainers reviewing `@you/goal` public-contract follow-on PRs should verify:

| Evidence | What it proves |
|----------|----------------|
| This section + stories 001–002 | `Workstation.workPropagation` is the landed public delta; streams stay internal |
| `api/components/schemas/data-models/Workstation.yaml` | Field and enum fragments use `workPropagation` / `WorkPropagationMode` naming |
| `make generate-api` diff | Generated artifacts change only when authored OpenAPI changes |
| `make api-smoke` | Bundled contract, drift checks, and integration smoke pass after public delta |
| `pkg/transports/http/contracttests/openapi_contract_surface_test.go` | Factory data-model schemas remain contract-complete when `Workstation` grows |
| `pkg/transports/mapping/factoryconfig/mappingtests/` or factory validation tests | Factory JSON round-trips `workPropagation` through config mapping |
| Absence of `SessionResponseStream*` in `api/components/` | Internal stream models did not leak into public contract |
| Stories 001–002 reviewer tables | Invocation, events, and lifecycle reuse unchanged by `workPropagation` work |

**Follow-on implementation PRs** that add only internal `SessionResponseStream`
behavior should cite stories 001–002 and prove runtime behavior with focused
tests. They must **not** include generated OpenAPI client diffs. PRs that change
authored `Workstation.workPropagation` or companion enum fragments must include
the full generation and `make api-smoke` verification path above and use the
locked terminology table in this section.

---

## Final integration verification evidence (P25)

The `you-goal-06` final verification work records behavioral proof that the
packaged `@you/goal` experience matches this audit without widening public
contracts. Customer wording lives in `docs/reference/packaged-goal.md`
(`you docs packaged-goal`); maintainer contract wording stays in this artifact.

| Verification lane | Evidence | What it proves |
|-------------------|----------|----------------|
| Fresh packaged run | `tests/functional/smoke/cli_named_goal_run_smoke_test.go` | Fresh-home materialization, successful `primaryResult` stdout, customer-edit preservation, and legacy prompt-template upgrade on reuse |
| CLI/API invocation parity | `tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go` | Positional, stdin, and API invocation paths share `InvocationResponse` semantics for success and representative failures |
| Decision routing | `tests/functional/smoke/cli_named_goal_routing_smoke_test.go` | Accepted, blocked, needs-human, failed, interrupted, rework, and structured unknown decisions surface predictable outcomes |
| Operator controls, replay, inspection | `tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go` | Pause buffers work, resume drains buffered goals in submission order (plan-goal `StartTime` ordering), interrupted inspect summaries stay on shared session/work surfaces, and `SESSION_LIFECYCLE_CONTROL` replay events remain durable |
| Response-stream boundary | `tests/functional/smoke/cli_named_goal_response_stream_smoke_test.go` plus `pkg/transports/http/contracttests/` | CLI `--output response-stream` still returns `primaryResult`; internal `SessionResponseStream` data stays out of public OpenAPI and durable `FactoryEvent` contracts |
| Customer docs and vocabulary | `docs/reference/packaged-goal.md`, `pkg/transports/cli/docs/docs_packaged_reference_test.go`, `tests/functional/smoke/cli_docs_smoke_test.go` | Packaged goal docs describe shipped invocation, routing, operator controls, and recovery without goal-specific public routes or internal stream contracts |
| Generated artifact alignment | `make api-smoke` and `pkg/transports/http/contracttests/openapi_contract_surface_test.go` | Public generated artifacts remain aligned with the internal-only response-stream boundary |

### Final verification commands (`you-goal-06-007`, 2026-07-02 UTC)

All commands ran from the `you-goal-06` worktree unless noted.

| Command | Result | Notes |
|---------|--------|-------|
| `make typecheck` | pass | Dashboard TypeScript build check |
| `make test` | pass | Short Go suite including functional smoke packages |
| `make api-smoke` | pass | OpenAPI validate/bundle, generated drift check, contract integration smoke |
| `go test ./tests/functional/smoke/ -run 'TestNamedGoal\|TestDocsCommandSmoke_' -count=1 -timeout 600s` | pass | Focused `@you/goal` functional smoke and packaged-docs CLI smoke |
| `go test ./pkg/transports/cli/docs/... -count=1` | pass | Packaged reference topic markers |
| `go test ./pkg/transports/cli -run TestDocsCommand_ -count=1` | pass | `you docs` command coverage |
| `go test ./tests/functional/smoke -run TestDocsCommandSmoke_ -count=1` | pass | Functional docs smoke |
| `make verify-fast` | **narrower** | `make typecheck` and `make test` pass; `make ui-test` fails on pre-existing `factory-graph-layout-performance.test.ts` 500/1000-node budget regressions unrelated to `@you/goal` (no UI graph-editor or layout code changed in this lane) |
| `make docs-reference-smoke` | **skipped** | See docs verification note below |

Focused smoke fixtures exercised end-to-end:

- `TestNamedGoalRun_RealCLIMaterializesFreshFactoryAndPreservesCustomerEditsOnRerun`
- `TestNamedGoalInvocationParity_*`
- `TestNamedGoalRouting_*`
- `TestNamedGoalOperatorControls_*`
- `TestNamedGoalResponseStream_*`
- `TestDocsCommandSmoke_PackagedGoalTopic_*`

### Skipped or narrower verification

| Skipped lane | Concrete reason | Risk left behind |
|--------------|-----------------|------------------|
| `make docs-reference-smoke` | `docs-reference-check` shells into `docs/` and runs `go run ../markdown-linter/cmd/markdown-linter`, which is absent from nested worktrees (`stat .../markdown-linter: directory not found`) | Markdown lint drift in `docs/reference/` is not enforced by the Make target in worktrees; in-repo docs `go test` lanes above still prove topic content and CLI wiring |
| `make verify-fast` (full green) | Inherited UI graph-layout performance budget failures in `ui/src/features/factory-graph-editor/lib/layout/performance/factory-graph-layout-performance.test.ts`; `@you/goal` verification did not touch graph-editor layout code | Large-factory dashboard layout performance regressions may land without this lane catching them until UI perf budgets are repaired on `main` |

### Remaining follow-up gaps

| Gap | Owning surface | Expected observable behavior | Suggested verification |
|-----|----------------|------------------------------|------------------------|
| Worktree-safe `make docs-reference-smoke` | `docs/` + CI Make targets | `make docs-reference-smoke` passes from nested worktrees and CI without a sibling `markdown-linter` checkout | `make docs-reference-smoke` from a `.claude/worktrees/...` checkout; tracked in `tasks/ideas-to-review/docs/fix-docs-reference-smoke-worktree-path.md` |
| Factory graph layout performance budgets | `ui` graph-editor layout | 500-node and 1000-node canonical layout fixtures stay within documented median budgets | `make ui-test` / `make verify-fast` green on `factory-graph-layout-performance.test.ts` after budget repair or fixture tuning |

### Scope confirmation

Final verification for `@you/goal` added focused functional smoke tests, packaged/customer docs updates, a CLI `--output response-stream` flag wiring fix, and maintainer verification records. It did **not** introduce new public OpenAPI routes, `FactoryEvent` response-stream contracts, goal-specific control APIs, or broad unrelated cleanup outside the `@you/goal` verification surfaces listed above.

**Docs verification note:** `make docs-reference-smoke` currently fails in nested
worktrees because `docs-reference-check` shells into `docs/` and invokes a
sibling `../markdown-linter` path that is not present in this repository layout.
For final verification, run the in-repo docs proof instead:

- `go test ./pkg/transports/cli/docs/... -count=1`
- `go test ./pkg/transports/cli -run TestDocsCommand_ -count=1`
- `go test ./tests/functional/smoke -run TestDocsCommandSmoke_ -count=1`

**Ownership for remaining gaps:** follow-up planning artifacts such as
`docs/internal/development/you-goal-active-pr-refresh.md` track merge ordering
for older open `@you/goal` PR lanes; they are not customer-facing contracts and
should not be cited as public API surface.
