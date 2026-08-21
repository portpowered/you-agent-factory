# Response-stream private NDJSON record removal

Supported `you --json run --factory … --output response-stream` (and equivalent
`--named` invocations) no longer emit private NDJSON `recordType` values
`progress`, `compaction`, `stream_gap`, or `primary_result`. Those values were
compatibility-only wrappers around the canonical Factory event stream and the
terminal invocation response.

## Supported stdout vocabulary

After this removal, each non-empty stdout line from
`you --json run … --output response-stream` is exactly one of:

| `recordType` | When it appears |
| --- | --- |
| `factory_event` | Every streamed observation. The nested `event` object is the canonical `FactoryEvent`, including its `context`, `id`, `payload`, `schemaVersion`, and `type` fields. |
| `invocation_result` | Exactly once as the final stdout line when an invocation response is available. The nested `response` object is the `InvocationResponse`, including `status` and, on success, `primaryResult`. |

The terminal line is always last, including when stdout is slow. The CLI does
not emit a raw Factory event or a second terminal schema from the same
invocation. Migrate parsers to the canonical `FactoryEvent` and
`InvocationResponse` objects instead of branching on retired private
`recordType` values.

### Pre-invocation CLI error envelope (unchanged exception)

Failures that occur before a response-stream invocation starts (for example
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
| `recordType=progress` | `recordType=factory_event` | Consume the canonical `FactoryEvent` envelope and interpret its `type`, `context`, and `payload`; there is no private progress wrapper or top-level `eventType`/`kind`/`payload` field. |
| `recordType=compaction` | `recordType=factory_event` when a canonical Factory event is emitted | Do not parse a private compaction summary. Use the canonical event stream and its event cursor/context for ordering and reconciliation. |
| `recordType=stream_gap` | `recordType=factory_event` when a canonical Factory event is emitted | Do not parse `droppedProgressLines`; reconnect or reconcile using the public Factory event stream and its `FactoryEvent` cursor/context. |
| `recordType=primary_result` | `recordType=invocation_result` | Read terminal invocation facts and `primaryResult` from the nested `response` object. |

The public field names and enum spellings match `you docs run` and the
generated OpenAPI `FactoryEvent` and `InvocationResponse` schemas. The
response-event presentation documented for Factory Sessions is not the
`--json run` wire format.

## Exact JSON shape pairs

Illustrative pairs below use representative fields. Production records also
carry the complete canonical correlation and ordering context.

### `progress` → `factory_event`

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
  "recordType": "factory_event",
  "event": {
    "context": {
      "eventTime": "2026-08-20T20:00:03Z",
      "sequence": 3,
      "sessionSequence": 3
    },
    "id": "factory-event-3",
    "payload": {},
    "schemaVersion": "agent-factory.event.v1",
    "type": "RUN_RESPONSE"
  }
}
```

### `compaction` and `stream_gap` → canonical event stream

**Retired private records:**

```json
{"recordType": "compaction", "reason": "truncated", "droppedSequenceCount": 1}
{"recordType": "stream_gap", "reason": "progress_backlog", "droppedProgressLines": 12}
```

Those private summaries are no longer stdout records. A supported stream line,
when a canonical event is available, has the same public envelope as the
example above and carries ordering in `event.context` rather than private
compaction or progress-loss fields.

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
  "response": {
    "requestId": "req-1",
    "traceId": "trace-1",
    "status": "COMPLETED",
    "primaryResult": [
      {"type": "text", "text": "Summarized changelog"}
    ]
  }
}
```

Only the outer discriminator and nesting are changed for the terminal record;
the shared `InvocationResponse` fields and `primaryResult` selection remain
the same as the API invocation response.

## Related public documentation

- `you docs run` — invocation output modes and NDJSON examples
- `you docs sessions` — public Factory Session event streams and reconnect
- `you docs workers` — provider fidelity on the public response stream
