# Consolidated Agent Response Stream — Program Plan

**Status:** proposed  
**Last updated:** 2026-07-11  
**Owners:** Factory Session runtime, worker providers, API, CLI  
**Target:** provider-neutral response events over internal subscriptions, REST SSE,
and `you run` NDJSON, including providers that expose only a final result

## 1. Executive summary

The repository currently has an internal, dispatch-scoped
`SessionResponseStream` that flattens provider activity into progress fragments,
response fragments, terminal markers, and compaction signals. `you run --output
response-stream` can attach to those internal streams, while the public OpenAPI
contract deliberately rejects response-stream types as internal implementation
detail.

That boundary no longer meets the product need. Provider CLIs expose materially
different protocols:

- Codex exposes typed thread, turn, and item JSONL events.
- Cursor exposes session initialization, assistant output, tool-call lifecycle,
  and final result NDJSON.
- Claude exposes complete messages and, optionally, raw message/content-block
  deltas including tool-input deltas.
- Pi exposes explicit agent, turn, message, and tool-execution lifecycles.
- Current OpenCode versions document raw JSON events, but older or configured
  installations may only provide final output.
- Agy does not expose a semantic stream through normal subprocess IO. The
  `agy-headless-bridge` obtains TTY output, cleans it, and returns a final or
  partial-on-timeout textual result.

The program will introduce one versioned `FactoryResponseEvent` contract that
normalizes semantic resources rather than provider syntax. The same event shape
will flow through:

1. provider-specific parsers into the session runtime;
2. bounded internal subscriptions used by local consumers;
3. a public session-scoped OpenAPI SSE endpoint;
4. newline-delimited JSON from `you run --output response-stream --json`;
5. human progress rendering from `you run --output response-stream`.

Providers that do not stream remain first-class. Their adapters synthesize only
run lifecycle events and emit an authoritative completed message snapshot when
the invocation finishes. They do not fabricate token deltas, reasoning, or tool
events.

## 2. Goals

- Define a stable, provider-neutral response-event contract for messages,
  reasoning summaries, tools, file changes, plans, progress, usage, errors, and
  stream gaps.
- Make completed snapshots authoritative and deltas optional so consumers can
  work with full streams, partial streams, or final-only providers.
- Expose response events through OpenAPI without merging them into canonical
  replayable `FactoryEvent` history.
- Make CLI NDJSON and API SSE carry the same event JSON object.
- Preserve the existing invocation contract: the factory-selected
  `InvocationResponse.primaryResult` remains the authoritative invocation
  result.
- Integrate Agy headless execution into provider execution with an owned,
  testable PTY/cleaning boundary and final-only response events.
- Move stream parsing and provider behavior out of shared monolithic files into
  provider-owned packages or focused files.
- Extend provider-specific error classification to structured stream failures,
  throttling, retry notifications, transport disconnects, timeouts, and process
  exits.
- Bound memory without mutating already-published semantic lifecycle records.
- Preserve unknown provider events safely for diagnostics without exposing
  prompts, raw reasoning, tool secrets, or unbounded payloads.

## 3. Non-goals

- Response events do not replace durable `FactoryEvent` history or factory state
  projections.
- Response events are not required to be durable across process restarts in the
  first release.
- The first release does not promise replay older than the configured completed
  dispatch/session retention window.
- The contract does not expose private chain-of-thought. Only provider-supported
  reasoning summaries or opaque reasoning activity may be represented.
- The program does not force every provider to expose identical granularity.
- Provider-native payloads do not become the public contract.
- The program does not add a dashboard transcript UI. Generated UI types and API
  adapters must remain compatible, but a customer-facing viewer is a separate
  plan.
- The Agy integration does not vendor an entire third-party repository without
  license, security, maintenance, and platform review.

## 4. Architectural boundaries

### 4.1 Canonical sources of truth

| Concern | Canonical owner |
|---|---|
| Factory lifecycle, replay, work/dispatch state | `FactoryEvent` and factory projections |
| Invocation input and primary result | shared invocation resolver and `InvocationResponse` |
| Transient agent activity | `FactoryResponseEvent` stream |
| Provider-native protocol | provider adapter/parser package |
| Final provider response | `interfaces.InferenceResponse`, then canonical work/invocation result |
| Stream retention and subscription cursors | session runtime response-stream store |
| Public response-event schema | authored OpenAPI fragments under `api/components/` |

Response events are ephemeral observation records. They may refer to a Factory
Session, dispatch, provider session, turn, or semantic item, but they must not be
used to derive canonical work state after replay.

### 4.2 Dependency direction

```text
provider CLI / bridge
        |
        v
provider-specific decoder and error classifier
        |
        v
provider-neutral FactoryResponseEvent publisher
        |
        v
session-owned bounded response-event store
        |
        +-------------------+--------------------+
        |                   |                    |
        v                   v                    v
internal subscriber     REST SSE            you run renderer
                                            human / NDJSON
```

Provider packages may depend on the provider-neutral event contract. The event
contract and stream store must not import provider packages.

## 5. Canonical response-event contract

### 5.1 Envelope

The public and internal semantic envelope should be named
`FactoryResponseEvent` to distinguish it from canonical `FactoryEvent`.

```yaml
FactoryResponseEvent:
  required:
    - schemaVersion
    - eventId
    - sequence
    - recordedAt
    - factorySessionId
    - runId
    - kind
    - phase
    - provenance
    - payload
  properties:
    schemaVersion: { type: string, example: "1" }
    eventId: { type: string }
    sequence: { type: integer, format: int64, minimum: 1 }
    recordedAt: { type: string, format: date-time }
    factorySessionId: { type: string }
    runId: { type: string }
    dispatchId: { type: string }
    turnId: { type: string }
    itemId: { type: string }
    parentItemId: { type: string }
    providerSessionRef: { $ref: ProviderSessionMetadata }
    kind: { $ref: FactoryResponseEventKind }
    phase: { $ref: FactoryResponseEventPhase }
    payload: { $ref: FactoryResponseEventPayload }
    provenance: { $ref: FactoryResponseEventProvenance }
```

Identity rules:

- `eventId` identifies one immutable emitted record.
- `sequence` orders records within one Factory Session response stream.
- `itemId` identifies the semantic object updated across multiple events.
- `dispatchId` correlates the provider execution with the factory dispatch.
- `turnId` represents a provider turn when available; adapters may synthesize a
  stable turn ID for a one-shot invocation.
- Provider-native item IDs should be retained when safe. Otherwise the adapter
  creates a stable scoped ID and records the native ID only in internal
  diagnostic metadata.

### 5.2 Kinds and phases

```text
FactoryResponseEventKind:
  SESSION
  RUN
  TURN
  MESSAGE
  REASONING
  TOOL
  FILE_CHANGE
  PLAN
  PROGRESS
  USAGE
  ERROR
  STREAM_GAP

FactoryResponseEventPhase:
  STARTED
  DELTA
  UPDATED
  COMPLETED
  FAILED
  CANCELED
```

Allowed combinations must be validated centrally. Examples:

| Kind | Allowed phases |
|---|---|
| SESSION | STARTED, UPDATED, COMPLETED |
| RUN | STARTED, UPDATED, COMPLETED, FAILED, CANCELED |
| TURN | STARTED, UPDATED, COMPLETED, FAILED, CANCELED |
| MESSAGE | STARTED, DELTA, COMPLETED |
| REASONING | STARTED, DELTA, COMPLETED |
| TOOL | STARTED, DELTA, UPDATED, COMPLETED, FAILED, CANCELED |
| FILE_CHANGE | STARTED, UPDATED, COMPLETED, FAILED |
| PLAN | STARTED, UPDATED, COMPLETED |
| PROGRESS | UPDATED |
| USAGE | UPDATED, COMPLETED |
| ERROR | UPDATED, FAILED |
| STREAM_GAP | UPDATED |

Unknown future kinds and phases must not cause a client connection to fail.
Generated clients should expose the declared enum while JSON consumers are
documented to ignore unknown events they do not understand.

### 5.3 Typed payloads

`FactoryResponseEventPayload` is a discriminated `oneOf` aligned with `kind`.
The first contract should include:

- `ResponseSessionPayload`
  - provider, model, cwd when safe, declared capabilities.
- `ResponseRunPayload`
  - status, optional primary message ID, terminal outcome.
- `ResponseTurnPayload`
  - status and optional stop reason.
- `ResponseMessagePayload`
  - role, complete content blocks, optional stop reason, `partial` flag.
- `ResponseMessageDeltaPayload`
  - content-block ID/type and text or structured delta.
- `ResponseReasoningPayload`
  - `SUMMARY` or `OPAQUE` visibility and optional provider-supported text.
- `ResponseToolPayload`
  - call ID, name, category, arguments, status, result summary, error,
    truncation metadata.
- `ResponseToolDeltaPayload`
  - call ID, output stream/category, bounded partial output.
- `ResponseFileChangePayload`
  - changed paths, add/update/delete kind, status; no unbounded file bodies.
- `ResponsePlanPayload`
  - ordered steps and step status.
- `ResponseProgressPayload`
  - bounded human-readable message and stable progress code.
- `ResponseUsagePayload`
  - input, cached input, output, reasoning output tokens, duration, cost only
    when supplied and semantically comparable.
- `ResponseErrorPayload`
  - normalized code/category, retryability, retry delay/attempt, provider-safe
    message, affected item ID, and terminality.
- `ResponseStreamGapPayload`
  - dropped sequence range/count, affected item IDs when bounded, reason, and
    whether authoritative snapshots remain available.

Message content blocks initially support:

```text
TEXT
REASONING_SUMMARY
TOOL_REQUEST
IMAGE_REF
RESOURCE_REF
STRUCTURED_OUTPUT
```

Tool arguments and structured output may contain JSON values, but adapters must
apply size limits and secret/redaction rules before publication.

### 5.4 Provenance and fidelity

Every event declares what the provider actually supplied:

```yaml
FactoryResponseEventProvenance:
  properties:
    provider: { type: string }
    nativeEventType: { type: string }
    nativeEventSubtype: { type: string }
    delivery:
      enum: [NATIVE_STREAM, NATIVE_FINAL, SYNTHESIZED, REPLAY]
    representation:
      enum: [DELTA, SNAPSHOT, NOTIFICATION]
    fidelity:
      enum: [LOSSLESS, NORMALIZED, LOSSY, FINAL_ONLY, LIFECYCLE_ONLY]
```

Rules:

- Native deltas use `NATIVE_STREAM` + `DELTA`.
- Complete native messages use `NATIVE_STREAM` or `NATIVE_FINAL` + `SNAPSHOT`.
- Process start/finish events for a final-only provider use `SYNTHESIZED` +
  `LIFECYCLE_ONLY`.
- A message built from final stdout uses `SYNTHESIZED` + `FINAL_ONLY`.
- Provider-native type strings are diagnostic context, not classification
  inputs after the provider adapter has decoded the event.

### 5.5 Capabilities

The session/run start payload exposes observed adapter capabilities:

```text
nativeStreaming
messageDeltas
messageSnapshots
reasoningSummaries
toolLifecycle
toolOutputDeltas
fileChanges
plans
usage
stableItemIds
providerReconnect
```

Capabilities are determined by the selected provider, installed CLI version,
invocation flags, and observed protocol. They are not global provider promises.
An adapter that falls back from structured mode must publish a capability update
and a safe progress/error record explaining the degraded mode.

## 6. Transport behavior

### 6.1 Internal subscriptions

Replace the dispatch-fragment mental model with one session-owned response-event
stream. Dispatch IDs remain event correlation fields, and dispatch-scoped
subscriptions may remain as compatibility adapters during migration.

Internal requirements:

- One monotonic session response sequence across dispatches.
- Immutable events after publication.
- Multiple non-blocking subscribers.
- Bounded retention independent of subscriber speed.
- Catch-up by `afterSequence`.
- Explicit `STREAM_GAP` records when a cursor falls behind.
- Completed snapshots retained preferentially over transient deltas.
- Session shutdown closes all subscribers deterministically.
- Provider publication cannot block on CLI/API consumers.
- Cancellation and dispatch completion flush decoder buffers before terminal
  events are published.

### 6.2 Retention and coalescing

Do not mutate or concatenate previously published lifecycle events.

Retention tiers:

1. retain the latest authoritative snapshot for active items;
2. retain terminal failures and the final run/message outcome;
3. retain a bounded tail of deltas and progress notifications;
4. evict oldest low-value deltas first;
5. emit or update one bounded gap summary for removed ranges.

Coalescing is allowed only before publication inside a provider decoder or
delta batcher, where no consumer has observed the component sequences. Runtime
retention must not rewrite an event at an old sequence. Byte retention must be
based on serialized retained size or explicitly documented payload size and
must have focused accounting tests.

### 6.3 REST/OpenAPI SSE

Add a distinct endpoint rather than overloading canonical Factory events:

```text
GET /factory-sessions/{session_id}/response-events
Accept: text/event-stream
```

Query parameters:

```text
after_sequence   optional last acknowledged response sequence
dispatch_id      optional dispatch filter
kind             optional repeatable kind filter, if contract complexity allows
```

SSE behavior:

- `Content-Type: text/event-stream`.
- Default SSE message events, one `FactoryResponseEvent` JSON body per `data:`
  record, matching existing Factory event stream conventions.
- SSE `id` is the decimal response sequence or stable event ID; the selected
  convention must be documented and used for reconnect.
- Historical retained events are sent first, followed by live events.
- Unknown session returns `404`, never the default session.
- A stale cursor begins with a `STREAM_GAP` event and the retained window rather
  than silently skipping records.
- Completed sessions may be drained during the documented retention window and
  then return an explicit expired/not-found outcome.
- Disconnect cancels only that HTTP subscription, not provider execution.

OpenAPI work:

- Author schemas under `api/components/schemas/response-events/`.
- Add reusable `AfterResponseSequence` and response-event filter parameters.
- Add `x-event-schema` for the SSE media type, matching the established Factory
  event contract-test convention.
- Add typed error responses for unknown session, expired stream, and invalid
  cursor/filter.
- Run `make generate-api`; never hand-edit generated Go or TypeScript.
- Replace the contract test that forbids all response-stream terms with a test
  that forbids provider-native/internal implementation types while requiring
  the public `FactoryResponseEvent` contract.

### 6.4 CLI human response stream

`you run --output response-stream` continues to provide operator-oriented
progress and ends with the authoritative invocation outcome.

Rendering rules:

- Reasoning summaries, tool lifecycle, retry/throttle status, and progress may
  render as bounded prefixed lines.
- Raw reasoning, tool arguments containing secrets, full tool output, and raw
  provider events are not printed by default.
- Message deltas are not printed as primary stdout unless a future explicit
  mode requests live assistant text; this avoids duplicating the final primary
  result.
- Terminal `InvocationResponse` selection remains unchanged.
- A provider stream failure that does not fail the invocation renders a degraded
  observability notice, then still returns the invocation outcome.
- Slow stdout drops transient progress before final or error records and reports
  the number dropped.

### 6.5 CLI JSON response stream

`you run --output response-stream --json` emits NDJSON. Each line is one of:

```json
{"recordType":"response_event","event":{}}
{"recordType":"invocation_result","invocation":{}}
```

The `event` value is exactly the OpenAPI `FactoryResponseEvent` JSON shape. The
terminal `invocation_result` is exactly the shared `InvocationResponse` shape.

Contract rules:

- The first available record should be a session/run start event; consumers
  must tolerate late attachment beginning at any retained event.
- `response_event` records are zero or more.
- Exactly one terminal `invocation_result` record is emitted when the CLI can
  construct an invocation response, including non-success invocation outcomes.
- Process/bootstrap failures that occur before an invocation response exists
  emit a terminal CLI error record using the existing stable CLI error envelope;
  document this exception explicitly.
- JSON records never share lines and progress never goes to stdout outside the
  NDJSON envelope.
- Stderr remains for operator diagnostics that are not contract output.
- The CLI does not synthesize provider detail beyond the events already
  published by the canonical response stream.

This replaces the current `progress`, `compaction`, and `primary_result` private
record vocabulary after a documented compatibility window. If compatibility is
required, accept the old mode through a separately named legacy option rather
than emitting two schemas from the same flag.

## 7. Provider adapter architecture

### 7.1 Interfaces

Split command construction, event decoding, final-result extraction, and error
classification instead of growing `providerBehavior` indefinitely:

```go
type Adapter interface {
    BuildCommand(context.Context, interfaces.ProviderInferenceRequest, BuildContext) (CommandRequest, error)
    NewDecoder(DecodeContext) StreamDecoder
    ParseFinal(CommandResult) (interfaces.InferenceResponse, error)
    ClassifyFailure(FailureContext) ProviderFailure
    Capabilities(AdapterContext) ResponseStreamCapabilities
}

type StreamDecoder interface {
    ObserveStdout([]byte) []FactoryResponseEventDraft
    ObserveStderr([]byte) []FactoryResponseEventDraft
    Flush(FlushReason) []FactoryResponseEventDraft
}
```

Decoders return event drafts without runtime sequence/time. The session-owned
publisher validates the kind/phase combination, assigns identity/order metadata,
applies safety bounds, and stores the immutable event.

### 7.2 Package/file structure

The `pkg/workers/provider` directory is already at the backend package file-count
limit, so the implementation must create subpackages with durable ownership
rather than merely splitting files in place.

Proposed structure:

```text
pkg/workers/provider/
  adapter.go                 provider-neutral interfaces/registry
  execution.go               thin subprocess orchestration
  failure.go                 provider-neutral failure shape/policy
  testkit/                   reusable adapter conformance fixtures
  adapters/
    claude/
      adapter.go
      decoder.go
      failure.go
    codex/
      adapter.go
      decoder.go
      failure.go
    cursor/
      adapter.go
      decoder.go
      failure.go
    pi/
      adapter.go
      decoder.go
      failure.go
    opencode/
      adapter.go
      decoder.go
      failure.go
    agy/
      adapter.go
      bridge.go
      clean.go
      failure.go
```

The exact root may be adjusted to preserve import direction, but provider-owned
decoders must not remain in one shared `inference_progress.go`. Existing shared
helpers should move only when they are genuinely provider-neutral.

### 7.3 Adapter conformance

Each adapter must pass a shared conformance suite:

- emits a synthesized run start before provider content;
- preserves provider-session identity when provided;
- produces stable item IDs across start/update/completion;
- never emits invalid kind/phase combinations;
- flushes a final unterminated JSON/NDJSON record;
- bounds malformed/raw diagnostic data;
- does not expose submitted prompt text in malformed-event errors;
- classifies terminal provider events consistently with process exit status;
- produces a completed message snapshot or explicit failure;
- declares capabilities matching observed events;
- ignores unknown additive fields;
- converts unknown event types to bounded diagnostics without failing the run.

## 8. Provider-specific behavior

### 8.1 Codex

- Invoke `codex exec --json` for response-stream-capable execution.
- Parse the public thread event union by exact top-level event and nested item
  type; remove substring classification of names such as `item.completed`.
- Map thread ID to provider session identity.
- Map turn lifecycle, agent message, reasoning summary, command execution, file
  change, MCP tool call, collaboration call, web search, plan/todo, usage, and
  error items separately.
- Treat completed item snapshots as authoritative for their item IDs.
- Preserve `InvocationResponse.primaryResult` construction independently of
  streamed agent-message events.
- Classify `turn.failed`, stream `error`, process exit, and malformed JSON with
  explicit precedence.

### 8.2 Cursor

- Continue using print mode with `--output-format stream-json` and partial output
  only when the installed version supports the required behavior.
- Map `system/init`, user messages, assistant text, `tool_call` start/completion,
  and terminal `result` explicitly.
- Use `call_id` for tool item identity.
- Do not infer reasoning when print mode suppresses it.
- Reconcile assistant deltas with the terminal result snapshot without duplicate
  output.
- If reconnect/retry records leave an in-flight tool without completion, close
  it as canceled/unknown only when provider evidence supports that conclusion;
  otherwise publish a stream gap affecting that item.
- Keep tool-specific native shapes inside the adapter and project a stable tool
  name/category/arguments/result summary.

### 8.3 Claude CLI / Agent SDK-compatible stream

- Use `--output-format stream-json --verbose --include-partial-messages` where
  supported and intended.
- Map system initialization, raw message/content-block lifecycle, text deltas,
  tool-use input deltas, complete assistant messages, tool results, compact
  boundaries, retry notifications, and final result.
- Complete `AssistantMessage` snapshots supersede accumulated partial message
  projections.
- `system/api_retry` maps to a non-terminal normalized retry/throttle error or
  progress event with attempt/delay fields.
- Publish only provider-supported reasoning/thinking content allowed by policy.
- Preserve subagent parent IDs from complete messages where available; do not
  claim token-level subagent attribution when the provider does not supply it.

### 8.4 Pi

- Add a Pi provider/adapter only if Pi is in the supported provider registry for
  the target release; otherwise land decoder fixtures and contract readiness
  separately.
- Map session header, agent start/end, turn start/end, message
  start/update/end, and tool execution start/update/end nearly one-to-one.
- Map text, thinking, and tool-call deltas from `assistantMessageEvent` by exact
  subtype.
- Retain complete message/tool-result snapshots from terminal events.

### 8.5 OpenCode

- Detect whether the installed version supports `run --format json` raw events.
- Prefer structured raw-event mode when supported and covered by fixtures.
- Treat a successful structured invocation with no incremental records as a
  valid snapshot-only stream, not a parser failure.
- Fall back to final-output execution when structured mode is unsupported.
- Publish a capability update on fallback.
- Keep raw-event schema mapping owned by the OpenCode adapter; do not route it
  through Codex/Claude heuristics based on similar field names.

### 8.6 Agy headless bridge

Integrate the behavior of `rhishi99/agy-headless-bridge` as an owned provider
boundary. Before code import:

1. confirm upstream license compatibility and retain required notices;
2. identify the minimum behavior needed: binary discovery, Windows ConPTY,
   POSIX PTY, idle/hard timeout, ANSI/TUI cleanup, and final text extraction;
3. perform security review for command construction, environment inheritance,
   workspace attachment, terminal escape handling, and process-tree cleanup;
4. decide between invoking the maintained bridge executable/library and porting
   the minimal algorithm to Go. Prefer a subprocess integration first if it
   avoids embedding platform-specific PTY complexity and packaging remains
   reliable;
5. add Windows and POSIX test seams without requiring the real Agy binary in
   ordinary unit tests.

Agy behavior:

- Start the adapter with `nativeStreaming=false`, `messageSnapshots=true`, and
  final-only fidelity.
- Allocate a PTY because upstream Agy gates output on TTY detection.
- Collect bounded raw PTY output internally; do not publish spinner/repaint
  chunks as response events.
- Clean ANSI CSI/OSC, carriage-return repaint lines, box drawing, and spinner
  glyphs through a pure tested cleaner.
- On success, emit one completed assistant message snapshot and complete the
  run.
- On timeout, emit any safe cleaned captured text as `partial=true`, then a
  normalized timeout error and failed run.
- On missing binary, PTY setup failure, authentication failure, or nonzero exit,
  emit provider-specific classified failure without exposing raw terminal
  control data.
- Support model, timeout, workspace mode, and additional directories using
  typed command construction; never concatenate a shell command string.

## 9. Error, throttle, retry, and degradation model

### 9.1 Normalized categories

Extend the existing provider failure model with stable categories:

```text
AUTHENTICATION
AUTHORIZATION
INVALID_REQUEST
UNSUPPORTED_CAPABILITY
THROTTLED
CAPACITY
USAGE_LIMIT
TIMEOUT
CONNECTION
STREAM_DISCONNECTED
PROVIDER_SERVER
TOOL_FAILED
CANCELED
MALFORMED_RESPONSE
PROCESS_FAILED
INTERNAL
UNKNOWN
```

Do not collapse capacity, usage limit, and generic 429 into one category when a
provider supplies enough evidence to distinguish them. They may still map to an
existing broader `WorkFailureType` at the work boundary.

### 9.2 Failure shape

```go
type ProviderFailure struct {
    Category       ProviderFailureCategory
    Code           string
    SafeMessage    string
    Retryable      bool
    Terminal       bool
    Attempt        *int
    MaxAttempts    *int
    RetryAfter     *time.Duration
    HTTPStatus     *int
    AffectedItemID string
    Cause          error // internal only
}
```

Public response-error payloads exclude `Cause` and raw provider bodies.

### 9.3 Classification precedence

Adapters classify using this order:

1. explicit structured provider terminal error;
2. explicit structured retry/throttle notification;
3. context cancellation or configured timeout;
4. bridge/decoder protocol failure;
5. process exit plus bounded stderr/stdout evidence;
6. provider-specific safe needle matching as compatibility fallback;
7. unknown.

Structured success plus non-fatal stream degradation must not be overwritten by
a stale diagnostic line. Conversely, process exit zero must not override an
explicit terminal failed event.

### 9.4 Stream errors versus invocation errors

Distinguish:

- **observability degradation:** malformed optional event, dropped deltas, slow
  subscriber, or provider reconnect; publish non-terminal error/gap and continue;
- **provider turn failure:** publish terminal error and failed turn/run; map to
  work failure;
- **tool failure:** fail the tool item but allow the provider run to continue if
  the provider continues;
- **invocation selection failure:** response stream may have succeeded, but the
  shared invocation return policy cannot select a primary result; terminal
  `InvocationResponse` remains non-success;
- **transport failure:** API/CLI client disconnect does not fail the provider
  run unless runtime ownership explicitly requires cancellation.

### 9.5 Retry representation

Retries are events, not sleeps hidden from observers:

```json
{
  "kind": "ERROR",
  "phase": "UPDATED",
  "payload": {
    "code": "PROVIDER_RETRY_SCHEDULED",
    "category": "THROTTLED",
    "retryable": true,
    "terminal": false,
    "attempt": 2,
    "maxAttempts": 5,
    "retryAfterMs": 30000
  }
}
```

Provider adapters parse provider-owned retry events. Runtime-owned retry policy
publishes the same normalized shape with synthesized provenance.

## 10. Migration strategy

### Phase A — contract and pure mapping kernel

- Add provider-neutral Go domain types and validation.
- Author OpenAPI response-event schemas and endpoint contract.
- Add provider adapter interfaces and conformance testkit.
- Keep current runtime publishers behind a compatibility mapper.
- Do not change CLI output yet.

### Phase B — session stream store

- Add immutable session-ordered response-event storage and subscriptions.
- Publish compatibility progress fragments into the new store through a lossy
  mapper while provider adapters migrate.
- Add snapshot-aware retention and gap behavior.
- Preserve current dispatch subscription methods as compatibility wrappers.

### Phase C — API and CLI consumers

- Wire public session SSE.
- Change JSON response-stream CLI output to the public event envelope.
- Update human renderer to consume typed semantic events.
- Add explicit compatibility/release notes for old private NDJSON records.

### Phase D — structured provider adapters

- Migrate Codex, Cursor, and Claude first because they already expose structured
  streams.
- Add OpenCode capability negotiation/fallback.
- Add Pi when provider registry/runtime support is scheduled.
- Delete substring-based cross-provider classification after fixtures prove all
  supported adapters.

### Phase E — Agy

- Land bridge/license/security decision record.
- Add provider registry/config support.
- Add PTY bridge integration and cleaner.
- Add final-only response mapping and error classification.
- Add packaging/release smoke coverage on supported platforms.

### Phase F — cleanup and hardening

- Remove legacy fragment model and private CLI records after the compatibility
  gate.
- Remove forbidden-public-response-stream assertions that no longer express the
  intended boundary.
- Run race/stress tests for slow consumers, provider floods, disconnects, and
  completed-session retention.
- Update architecture, process maps, reference docs, and examples.

## 11. Work-story breakdown

Stories are ordered by dependency. Each story should be independently
reviewable and should deliver one observable behavior or a tightly bounded
enabling contract.

### Story 01 — Publish the canonical response-event vocabulary

**Outcome:** Maintainers can represent messages, tools, reasoning, errors, and
final-only results without provider-specific fields.

**Acceptance criteria:**

- Go types define the envelope, kinds, phases, payloads, provenance, and
  capabilities described in this plan.
- Invalid kind/phase/payload combinations return actionable validation errors.
- JSON fixtures cover a text delta, message snapshot, tool lifecycle, retry,
  final-only message, usage, and stream gap.
- Pure round-trip tests preserve every declared field.
- No type depends on CLI, HTTP, subprocess, or a provider package.

### Story 02 — Author and generate the OpenAPI response-event contract

**Outcome:** Generated Go and TypeScript clients share the public event schema.

**Acceptance criteria:**

- Authored component fragments define every public response-event type.
- `make generate-api` updates the bundle and generated clients without manual
  edits.
- Contract tests require the schema discriminator/union and representative
  examples.
- Public schemas exclude raw provider payloads and internal compaction types.
- Existing Factory event schemas remain unchanged.

### Story 03 — Expose a session-scoped response-event SSE contract

**Outcome:** An API client can discover the response-event route and reconnect
using a response sequence.

**Acceptance criteria:**

- OpenAPI defines `GET /factory-sessions/{session_id}/response-events` with SSE,
  cursor, filters, and typed errors.
- `x-event-schema` points to `FactoryResponseEvent`.
- Unknown sessions cannot fall back to the current/default session.
- Contract tests distinguish this ephemeral response stream from canonical
  `/factory-sessions/{session_id}/events`.

### Story 04 — Introduce immutable session response-event storage

**Outcome:** Multiple internal consumers receive ordered immutable events
without blocking providers.

**Acceptance criteria:**

- Events receive monotonically increasing session sequences and stable event
  IDs.
- Subscribers can catch up from `afterSequence` and continue live.
- Slow subscribers do not block publishers.
- Complete/close behavior has no goroutine leak and passes race-focused tests.
- A dispatch filter does not change global session sequence identity.

### Story 05 — Retain snapshots and report lost deltas

**Outcome:** A late consumer can reconstruct final semantic state even when
transient deltas were evicted.

**Acceptance criteria:**

- Retention preferentially preserves final message, tool failure, and run
  outcome snapshots.
- Old deltas are evicted without mutating earlier retained events.
- Stale reads receive a `STREAM_GAP` containing the correct dropped range.
- Byte and event accounting remain correct under repeated eviction.
- No runtime coalescing rewrites a published sequence.

### Story 06 — Provide a legacy fragment compatibility mapper

**Outcome:** Existing publishers continue to produce useful response events
during provider migration.

**Acceptance criteria:**

- Progress fragments map to lossy `PROGRESS/UPDATED` events.
- Response fragments map to message deltas with stable synthesized item IDs.
- Terminal and compaction markers map to run/error/gap events.
- Provenance marks mapped events as normalized or lossy.
- Existing invocation primary-result behavior is unchanged.

### Story 07 — Stream public response events over HTTP

**Outcome:** API consumers receive retained then live events through SSE.

**Acceptance criteria:**

- Handler streams one schema-valid event per SSE message and flushes promptly.
- Reconnect after a known sequence returns only newer retained/live records.
- Stale reconnect begins with a gap event.
- Client disconnect releases the subscription without canceling the run.
- Completed-session drain and expiration behavior match the documented window.
- API server and contract tests cover happy, stale, missing, completed, and
  disconnect cases.

### Story 08 — Emit canonical NDJSON from `you run`

**Outcome:** Automation consumes the same response-event JSON as the REST API.

**Acceptance criteria:**

- `--output response-stream --json` emits only complete NDJSON records.
- Response records use `recordType=response_event` and the OpenAPI event shape.
- Exactly one final `invocation_result` is emitted when an invocation response
  exists.
- Non-success invocation responses retain stable codes/statuses.
- Slow output cannot interleave or corrupt the terminal record.
- CLI/API fixture parity compares the same event object serialization.

### Story 09 — Render typed human progress

**Outcome:** Operators can distinguish reasoning summaries, tools, retries, and
gaps without reading provider-native text.

**Acceptance criteria:**

- Human output has bounded, stable treatments for tool start/end/failure,
  reasoning summary, retry/throttle, progress, and gap.
- Message deltas do not duplicate final primary output.
- Sensitive tool arguments/results and raw provider payloads are not rendered.
- Terminal invocation outcome remains last and non-interleaved.

### Story 10 — Establish provider adapter interfaces and conformance fixtures

**Outcome:** Each provider owns parsing and failure semantics behind one tested
contract.

**Acceptance criteria:**

- Adapter, decoder, final parser, capability, and failure interfaces are
  provider-neutral.
- Shared fixtures/testkit exercise the conformance cases in section 7.3.
- Subprocess orchestration does not switch on provider event names.
- Package boundaries pass `make pkg-boundary` and `make pkg-file-count`.

### Story 11 — Migrate Codex to its typed JSONL protocol

**Outcome:** Codex commands, reasoning, messages, files, tools, usage, and errors
are no longer flattened by substring matching.

**Acceptance criteria:**

- Response-stream-capable Codex invocation uses `exec --json`.
- Official representative fixtures map thread, turn, every supported item type,
  usage, and error events.
- `item.completed` is classified by nested item type, never as generic final
  text.
- Thread ID becomes provider session identity.
- Unknown additive item/event types yield bounded diagnostics and do not abort a
  successful run.
- Current Codex error corpus remains covered through the new classifier.

### Story 12 — Migrate Cursor to structured tool and message events

**Outcome:** Cursor tool calls retain identity, arguments/result summaries, and
status.

**Acceptance criteria:**

- Init, assistant, tool started/completed, result, and failure fixtures map
  explicitly.
- `call_id` correlates tool lifecycle.
- Terminal result produces the authoritative message snapshot without duplicate
  assembled output.
- Missing tool completion after a provider reconnect results in an explicit gap
  or supported cancellation state.
- Thinking is not inferred when absent from the documented stream.

### Story 13 — Migrate Claude to message/content-block streaming

**Outcome:** Claude partial text/tool input and complete messages coexist without
duplication.

**Acceptance criteria:**

- Command construction enables structured partial messages in response-stream
  mode.
- Raw content-block events map to stable message/tool items.
- Complete assistant snapshots supersede accumulated deltas.
- API retry and compact-boundary events map to safe normalized records.
- Subagent attribution is retained only where the provider supplies it.

### Story 14 — Add OpenCode structured-mode negotiation

**Outcome:** New OpenCode installations stream raw events while older ones still
return a correct final response.

**Acceptance criteria:**

- Adapter detects/configures supported structured mode without parsing help text
  on every invocation.
- Structured fixtures map through the OpenCode-owned decoder.
- Unsupported mode falls back once to final-only execution with capability
  degradation recorded.
- Final-only success and failure produce the same canonical terminal semantics.

### Story 15 — Add Pi protocol readiness or provider support

**Outcome:** Pi's agent/turn/message/tool lifecycle has a lossless canonical
mapping when Pi is enabled.

**Acceptance criteria:**

- Session header and every documented lifecycle event have fixtures.
- Tool partial results and errors retain call identity.
- Message updates map text/thinking/tool-call deltas by subtype.
- If runtime provider support is deferred, decoder readiness lands without
  exposing Pi as selectable until command/auth/config work is complete.

### Story 16 — Record the Agy integration decision

**Outcome:** Maintainers have an approved, supportable way to incorporate the
headless bridge.

**Acceptance criteria:**

- Decision record covers license/notices, invoke-versus-port choice, supported
  OSes, packaging, upgrades, and security model.
- Threat review covers shell injection, workspace paths, environment, escape
  sequences, output bounds, timeouts, and process cleanup.
- The chosen boundary is mockable without an installed Agy binary.

### Story 17 — Execute Agy headlessly and return final-only events

**Outcome:** A configured Agy worker succeeds from non-TTY CLI/API execution.

**Acceptance criteria:**

- Windows and POSIX implementations allocate a working PTY through the chosen
  bridge boundary.
- Command arguments are passed as argv, not a shell string.
- Successful output is cleaned and returned as a completed message plus normal
  inference result.
- Capabilities declare final-only fidelity.
- Missing executable, PTY failure, auth failure, timeout, and nonzero exit are
  classified distinctly.

### Story 18 — Preserve partial Agy output on timeout safely

**Outcome:** Operators receive usable partial work without mistaking a timeout
for success.

**Acceptance criteria:**

- Bounded cleaned output is emitted as `partial=true` when available.
- Timeout error and failed run follow the partial message.
- ANSI/control sequences and repaint noise never reach public events.
- Empty timeout output still yields an actionable timeout failure.

### Story 19 — Split provider ownership out of monolithic files

**Outcome:** A change to one provider's stream grammar does not require editing
the shared provider implementation file.

**Acceptance criteria:**

- Each supported provider owns command construction, decoding, final parsing,
  and failure classification in its package.
- Registry/orchestration code selects an adapter but contains no provider event
  grammar.
- Existing provider behavior/error tests are migrated to provider-owned suites
  without reducing coverage.
- Old monolithic parsing/classification helpers are deleted once unused.
- Package size and boundary checks pass.

### Story 20 — Align error classification across stream and process failures

**Outcome:** The same provider failure receives the same work/invocation outcome
whether reported structurally or through process exit.

**Acceptance criteria:**

- Structured event precedence is tested against conflicting exit/stderr data.
- Auth, invalid request, throttle, capacity, usage limit, timeout, disconnect,
  server, canceled, malformed, and unknown cases have provider fixtures.
- Retryable/terminal flags and retry-after metadata are preserved.
- Public events contain safe messages; internal errors retain diagnostic cause.
- Existing stable invocation error codes remain compatible or have documented
  migration mappings.

### Story 21 — Prove provider and consumer backpressure behavior

**Outcome:** Slow API/CLI consumers cannot stall inference or corrupt final
results.

**Acceptance criteria:**

- Stress tests flood deltas while one subscriber is blocked and another drains.
- Publisher latency remains bounded and no provider goroutine blocks on output.
- Gap accounting is correct after eviction.
- Final snapshots and invocation result survive pressure.
- Race tests cover subscribe, publish, complete, detach, expire, and close.

### Story 22 — Update public and maintainer documentation

**Outcome:** Users can choose primary, human response-stream, and JSON response-
stream output with accurate provider fidelity expectations.

**Acceptance criteria:**

- `docs/reference/config.md` documents CLI NDJSON records and examples.
- Session/API docs describe the SSE endpoint, cursor, retention, and difference
  from canonical Factory events.
- Provider docs list capability variability and final-only behavior.
- Architecture docs identify adapter, event-store, API, and CLI boundaries.
- API/invocation process maps list all relevant authored/generated/test files.
- `make docs-reference-smoke` passes.

### Story 23 — Remove the legacy private response-stream contract

**Outcome:** One supported response-event vocabulary remains across API and CLI.

**Acceptance criteria:**

- Old progress/compaction/private primary-result NDJSON records are removed after
  the compatibility window.
- Legacy fragment types and substring parser are unused and deleted.
- Contract tests require the public semantic schema and continue forbidding raw
  provider-native types.
- Release notes identify the exact old/new CLI JSON mapping.

### Story 24 — Run cross-provider functional parity

**Outcome:** Every provider produces a terminal invocation result and the best
  response-event fidelity it actually supports.

**Acceptance criteria:**

- Functional fixtures cover full stream, partial stream, snapshot-only, and
  final-only adapters.
- CLI NDJSON and API SSE decode to identical `FactoryResponseEvent` values.
- Primary-result-only and response-stream modes return equivalent final
  `InvocationResponse` outcomes.
- At least one structured provider tool lifecycle and one Agy final-only run are
  covered end to end.
- `make api-smoke`, focused provider tests, response-stream race/stress tests,
  and `make verify-pr` pass when feasible.

## 12. Task dependency graph

```text
01 Canonical domain contract
 ├─> 02 OpenAPI schemas ─> 03 SSE contract ─> 07 SSE implementation
 ├─> 04 Session store ─> 05 Retention/gaps ─> 07/08/09
 ├─> 06 Legacy mapper
 └─> 10 Adapter interfaces
      ├─> 11 Codex
      ├─> 12 Cursor
      ├─> 13 Claude
      ├─> 14 OpenCode
      ├─> 15 Pi
      └─> 16 Agy decision ─> 17 Agy execution ─> 18 Agy timeout

02 + 04 ─> 08 CLI NDJSON
04 + 05 ─> 09 Human renderer
11..18 ─> 19 Provider ownership cleanup
11..18 ─> 20 Error alignment
04 + 05 + 07 + 08 ─> 21 Backpressure proof
03 + 08 + 11..20 ─> 22 Documentation
19 + 22 ─> 23 Legacy removal
07 + 08 + 11..21 ─> 24 Cross-provider parity
```

Recommended PR grouping is one story per PR, except tightly coupled contract
stories 01–03 may be delivered as a reviewed contract kernel if generated
artifacts and tests remain understandable.

## 13. Verification matrix

| Risk | Required evidence |
|---|---|
| Schema drift | OpenAPI contract tests, `make generate-api`, generated diff review |
| CLI/API JSON mismatch | shared fixture serialization and functional parity test |
| Provider grammar drift | provider-owned captured/sanitized fixture corpus |
| Secret/prompt leakage | malformed-event and bounded-diagnostic negative tests |
| Duplicate assistant output | delta + completed snapshot + invocation-result tests |
| Tool lifecycle mismatch | stable item/call ID start-update-terminal tests |
| Slow consumer | stress, timeout, and race tests |
| Retention corruption | byte/event accounting and stale-cursor gap tests |
| Incorrect failure outcome | structured-vs-exit precedence tables per provider |
| Cross-platform Agy PTY | mocked unit tests plus Windows/POSIX release smoke lanes |
| Canonical event contamination | tests proving response events never enter `FactoryEvent` replay |
| Package sprawl | `make pkg-boundary`, `make pkg-file-count`, `make lint` |

Focused commands expected across the program:

```text
go test ./pkg/factorysessions/responsestream/... -race
go test ./pkg/workers/provider/... -race
go test ./pkg/cli/run/... ./pkg/api/... -short
make generate-api
make api-smoke
make docs-reference-smoke
make pkg-boundary
make pkg-file-count
make verify-fast
make verify-pr
```

## 14. Rollout and compatibility

- Gate the public API and new CLI JSON schema behind one release feature flag if
  implementation spans multiple releases.
- Do not expose an incomplete public discriminator union.
- During migration, dual-publish internally if needed, but expose only one
  documented schema per CLI flag/HTTP route.
- Log adapter mode and capability degradation without logging prompt or response
  bodies.
- Add counters for published events by kind/provider, gap count, malformed
  records, decoder fallback, subscriber lag, provider retry, and terminal error
  category.
- Keep provider-native fixture payloads sanitized and small.
- Publish compatibility notes before removing old CLI NDJSON record types.

## 15. Open decisions before Story 01 approval

1. Should the public endpoint be named `/response-events` (recommended) or
   `/response-stream`? The resource is a collection of events; the transport is
   a stream, so `/response-events` is clearer.
2. Should SSE `id` be `sequence` or `eventId`? Sequence simplifies the existing
   `after_sequence` model; event ID is more opaque. Use one, not both, as the
   primary reconnect cursor.
3. Is response-event retention session-wide in v1, or independently configurable
   per dispatch/provider? Session-wide with global limits and snapshot priority
   is recommended.
4. Does JSON response-stream CLI schema change in place or require a temporary
   `--response-stream-version`? Prefer a clean change before the existing
   internal-only mode is treated as stable; otherwise add explicit versioning.
5. Will Pi be a selectable provider in this program or only a conformance target?
6. Will Agy be invoked through an installed `agy-headless-bridge`, embedded as a
   Python dependency, or minimally ported to Go? Story 16 must resolve this before
   implementation.
7. Which tool argument/result fields are safe by default? Start with bounded,
   redacted structured summaries and require provider-specific allowlists for
   richer payloads.
8. Should completed response streams survive service restart in a later release?
   The v1 contract should not imply durable replay.

## 16. Definition of program completion

The program is complete when:

- the public OpenAPI contract defines `FactoryResponseEvent` and a session-scoped
  SSE route;
- `you run --output response-stream --json` emits the same canonical events as
  NDJSON followed by the shared invocation response;
- internal subscriptions use immutable session-ordered semantic events with
  snapshot-aware retention and explicit gaps;
- Codex, Cursor, Claude, and supported OpenCode versions use provider-owned
  structured decoders;
- final-only providers, including Agy, emit honest synthesized lifecycle plus an
  authoritative final/partial snapshot;
- provider-specific structured and process errors share one normalized failure
  model without losing provider distinctions;
- provider code is split into durable provider-owned adapter packages;
- legacy fragment/coalescing behavior is removed or isolated behind a documented
  compatibility boundary;
- API, CLI, provider, race/stress, documentation, and generated-artifact quality
  gates pass.
