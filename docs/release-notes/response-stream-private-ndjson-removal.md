# Response-stream private NDJSON record removal

Supported `you --json run --factory … --output response-stream` (and equivalent
`--named` invocations) no longer emit private NDJSON `recordType` values
`progress`, `compaction`, `stream_gap`, or `primary_result`. Those shapes were
compatibility-only wrappers around the public `FactoryResponseEvent` vocabulary
and a shared terminal `InvocationResponse`.

## Supported stdout vocabulary

After this removal, each non-empty stdout line from
`you --json run … --output response-stream` is exactly one of:

| `recordType` | When it appears |
| --- | --- |
| `response_event` | Every streamed observation record. The nested `event` object is the public `FactoryResponseEvent` contract shared with `GET /factory-sessions/{session_id}/response-events`. |
| `invocation_result` | Exactly once as the final stdout line when an invocation response is available (success, failure, timeout, or cancellation). The nested `invocation` object is the shared `InvocationResponse` / `primaryResult` contract also used by primary-result JSON mode. |

The supported flag path does **not** emit two schemas from the same invocation.
Migrate parsers to the public vocabulary above instead of branching on retired
private `recordType` values.

### Pre-invocation CLI error envelope (unchanged exception)

Failures that occur **before** a response-stream invocation starts (for example
unsupported flag combinations or missing factory configuration) still write a
single JSON object to **stderr**, not stdout NDJSON:

```json
{"code":"INVOCATION_OUTPUT_UNSUPPORTED","message":"…"}
```

That stderr envelope is outside the response-stream NDJSON contract and is not
wrapped in `recordType`.

## Old → new mapping

| Retired private stdout record | Public successor | How to migrate |
| --- | --- | --- |
| `recordType=progress` | `recordType=response_event` with `event.kind=PROGRESS` and `event.phase=UPDATED` | Read progress semantics from the nested `event.payload` (`label`, optional `message`) instead of top-level `kind` / `eventType` / `payload` on the private record. |
| `recordType=compaction` | `recordType=response_event` with `event.kind=STREAM_GAP` and `event.phase=UPDATED` | Read retention/compaction bounds from `event.payload` (`fromSequence`, `toSequence`, `firstAvailableSequence`, `reason`) instead of top-level compaction summary fields. |
| `recordType=stream_gap` (dropped human-progress notice) | `recordType=response_event` with `event.kind=STREAM_GAP` and `event.phase=UPDATED` | Treat CLI-side progress loss the same as API/session retention gaps: reconcile against `firstAvailableSequence` and the gap payload instead of `droppedProgressLines`. |
| `recordType=primary_result` | `recordType=invocation_result` | Read terminal invocation facts and `primaryResult` from the nested `invocation` object. The terminal stdout line is always last, including on slow stdout. |

Public field names and enum spellings match `you docs run`, `you docs sessions`,
and the generated OpenAPI `FactoryResponseEvent` / `InvocationResponse` schemas.

## Exact JSON shape pairs

Illustrative pairs below use representative fields. Production records also
carry correlation (`factorySessionId`, `runId`, `dispatchId`, `sequence`,
`recordedAt`, `provenance`, and related public metadata).

### `progress` → `response_event` (`PROGRESS` / `UPDATED`)

**Retired private record:**

```json
{
  "recordType": "progress",
  "sequence": 3,
  "dispatchId": "dispatch-42",
  "kind": "PROGRESS_FRAGMENT",
  "eventType": "PROGRESS",
  "payload": "planning next step"
}
```

**Supported public record:**

```json
{
  "recordType": "response_event",
  "event": {
    "schemaVersion": "1",
    "eventId": "evt-…",
    "sequence": 3,
    "factorySessionId": "session-abc",
    "runId": "run-xyz",
    "kind": "PROGRESS",
    "phase": "UPDATED",
    "dispatchId": "dispatch-42",
    "payload": {
      "label": "PROGRESS",
      "message": "planning next step"
    },
    "provenance": {
      "provider": "…",
      "delivery": "SYNTHESIZED",
      "representation": "NOTIFICATION",
      "fidelity": "NORMALIZED"
    }
  }
}
```

### `compaction` → `response_event` (`STREAM_GAP` / `UPDATED`)

**Retired private record:**

```json
{
  "recordType": "compaction",
  "reason": "truncated",
  "droppedSequenceCount": 1,
  "firstRetainedSequence": 4,
  "lastDroppedSequence": 3
}
```

**Supported public record:**

```json
{
  "recordType": "response_event",
  "event": {
    "schemaVersion": "1",
    "eventId": "evt-…",
    "sequence": 13,
    "kind": "STREAM_GAP",
    "phase": "UPDATED",
    "payload": {
      "fromSequence": 3,
      "toSequence": 3,
      "firstAvailableSequence": 4,
      "reason": "truncated"
    },
    "provenance": {
      "delivery": "SYNTHESIZED",
      "representation": "NOTIFICATION",
      "fidelity": "LOSSY"
    }
  }
}
```

### `stream_gap` (progress loss) → `response_event` (`STREAM_GAP` / `UPDATED`)

**Retired private record:**

```json
{
  "recordType": "stream_gap",
  "reason": "progress_backlog",
  "droppedProgressLines": 12
}
```

**Supported public record:**

```json
{
  "recordType": "response_event",
  "event": {
    "kind": "STREAM_GAP",
    "phase": "UPDATED",
    "payload": {
      "fromSequence": 0,
      "toSequence": 0,
      "firstAvailableSequence": 1,
      "reason": "progress_backlog"
    }
  }
}
```

When the runtime cannot reconstruct precise sequence bounds for a CLI-side
progress backlog, public gap payloads may report zero bounds while still
advertising the next safe `firstAvailableSequence`.

### `primary_result` → `invocation_result`

**Retired private record:**

```json
{
  "recordType": "primary_result",
  "invocation": {
    "requestId": "req-1",
    "traceId": "trace-1",
    "status": "COMPLETED",
    "primaryResult": [
      {"type": "text", "text": "Summarized changelog"}
    ]
  }
}
```

**Supported public record:**

```json
{
  "recordType": "invocation_result",
  "invocation": {
    "requestId": "req-1",
    "traceId": "trace-1",
    "status": "COMPLETED",
    "primaryResult": [
      {"type": "text", "text": "Summarized changelog"}
    ]
  }
}
```

Only the wrapper `recordType` changes. Terminal invocation semantics,
`primaryResult` selection, and shared `InvocationResponse` fields are unchanged
from primary-result JSON mode and `POST /factory-sessions/{session_id}/invocations`.

## Related public documentation

- `you docs run` — invocation output modes and NDJSON examples
- `you docs sessions` — API response-event stream, `STREAM_GAP`, and reconnect
- `you docs workers` — provider fidelity on the public response stream
