# Response events

# Why? 
We need a CLI agnostic event stream of operations as the CLI responds back so that our internal systems can process them. 

## What
We have this in response events. 

Response events are the provider-neutral, transient observations produced while a
worker is running. They let the CLI and API show messages, tool activity,
reasoning, usage, and failures without exposing a provider's native event
format.

The flow is:

1. A provider adapter reads its native response stream.
2. The adapter maps each useful native record to a validated response-event
   draft: `kind`, `phase`, typed `payload`, provenance, and correlation IDs.
3. The Factory Session publisher adds session identity, a monotonic `sequence`,
   `eventId`, `recordedAt`, and the response-event schema version.
4. Consumers receive the same `FactoryResponseEvent` contract through the
   session SSE endpoint or as `response_event` records in CLI JSON streaming.

The canonical envelope is
[`FactoryResponseEvent`](../../../api/components/schemas/response-events/FactoryResponseEvent.yaml),
and the provider-facing draft vocabulary is defined in
[`response_drafts.go`](../../../pkg/services/workers/response_drafts.go).

## Most valuable events

| Kind | What it communicates |
| --- | --- |
| `MESSAGE` | Assistant content like their text responses. Comes in either incremental `DELTA`s or an authoritative `COMPLETED` snapshot. |
| `TOOL` | Tool execution context. Start, bounded argument summaries, output deltas, and terminal result summaries. |
| `ERROR` | A safe error code and message, plus retry information when known. |
| `RUN`, `TURN`, `SESSION` | Lifecycle boundaries used to organize the stream and correlate provider sessions. |
| `REASONING`, `PLAN`, `PROGRESS` | Safe summaries of intermediate activity without leaking provider-native records. |
| `FILE_CHANGE` | A normalized path, operation, and summary of a file mutation. |
| `USAGE` | Token counts and model information reported by the provider. |

## Event structure

Every published event has four logical parts:

- **Identity and ordering:** `schemaVersion`, `eventId`, `sequence`,
  `recordedAt`, and `factorySessionId`.
- **Correlation:** `runId` and optional `dispatchId`, `turnId`, `itemId`,
  `parentItemId`, and `providerSessionRef`.
- **Meaning:** `kind`, `phase`, and `payload`. The allowed payload shape and
  phase depend on the kind; for example, `MESSAGE/DELTA` carries a text delta,
  while `MESSAGE/COMPLETED` carries content blocks.
- **Provenance:** provider, native event type/subtype, delivery mode,
  representation (`DELTA`, `SNAPSHOT`, or `NOTIFICATION`), and mapping fidelity.

Adapters must emit the shared vocabulary rather than adding provider-specific
kinds or copying arbitrary native JSON into `payload`.

## Example: mapping a Codex JSON stream

For example, a Codex adapter can receive these JSONL records:

```json
{"type":"thread.started","thread_id":"thread-123"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"command-1","type":"command_execution","command":"go test ./pkg/example","aggregated_output":"","exit_code":null,"status":"in_progress"}}
{"type":"item.completed","item":{"id":"command-1","type":"command_execution","command":"go test ./pkg/example","aggregated_output":"ok pkg/example","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"Tests passed."}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":5,"reasoning_output_tokens":1}}
```

The logical mapping is:

| Codex record | Response event(s) |
| --- | --- |
| `thread.started` | `SESSION/STARTED` with `providerSessionRef=thread-123` |
| `turn.started` | `TURN/STARTED` |
| `item.started` + `command_execution` | `TOOL/STARTED` snapshot |
| `item.completed` + `command_execution` | `TOOL/COMPLETED` snapshot |
| `item.completed` + `agent_message` | `MESSAGE/COMPLETED` snapshot |
| `turn.completed` | `USAGE/UPDATED`, then `TURN/COMPLETED` |

During CLI JSON response streaming, each mapped event is one newline-delimited
record. Publication metadata is illustrative here, but the payload and
provenance show the canonical mapping:

```json
{"recordType":"response_event","event":{"schemaVersion":"agent-factory.response-event.v1","eventId":"evt-3","sequence":3,"recordedAt":"2026-07-21T18:30:00Z","factorySessionId":"session-1","runId":"run-1","itemId":"command-1","providerSessionRef":"thread-123","kind":"TOOL","phase":"STARTED","provenance":{"provider":"codex","nativeEventType":"item.started","delivery":"NATIVE_STREAM","representation":"SNAPSHOT","fidelity":"NORMALIZED"},"payload":{"toolCallId":"command-1","toolName":"command_execution","status":"in_progress","argumentsSummary":{"category":"command","command":"go test ./pkg/example"},"resultSummary":{"status":"in_progress","output":""}}}}
{"recordType":"response_event","event":{"schemaVersion":"agent-factory.response-event.v1","eventId":"evt-4","sequence":4,"recordedAt":"2026-07-21T18:30:01Z","factorySessionId":"session-1","runId":"run-1","itemId":"command-1","providerSessionRef":"thread-123","kind":"TOOL","phase":"COMPLETED","provenance":{"provider":"codex","nativeEventType":"item.completed","delivery":"NATIVE_STREAM","representation":"SNAPSHOT","fidelity":"NORMALIZED"},"payload":{"toolCallId":"command-1","toolName":"command_execution","status":"completed","argumentsSummary":{"category":"command","command":"go test ./pkg/example"},"resultSummary":{"status":"completed","output":"ok pkg/example","exitCode":0}}}}
{"recordType":"response_event","event":{"schemaVersion":"agent-factory.response-event.v1","eventId":"evt-5","sequence":5,"recordedAt":"2026-07-21T18:30:02Z","factorySessionId":"session-1","runId":"run-1","itemId":"message-1","providerSessionRef":"thread-123","kind":"MESSAGE","phase":"COMPLETED","provenance":{"provider":"codex","nativeEventType":"item.completed","delivery":"NATIVE_STREAM","representation":"SNAPSHOT","fidelity":"NORMALIZED"},"payload":{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"Tests passed."}]}}}
```

The CLI writes one final `invocation_result` record after the response-event
records. That terminal result is authoritative invocation output and is not a
response event. The SSE endpoint sends the `FactoryResponseEvent` itself as the
SSE data value rather than adding the CLI `recordType` wrapper.
