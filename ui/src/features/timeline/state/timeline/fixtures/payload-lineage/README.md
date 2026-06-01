# Payload lineage golden fixtures

Synthetic SSE event sequences for `reconstructWorldState` → `projectRuntime` payload projection regression tests.

| Fixture | Session / capture | Scenario |
| --- | --- | --- |
| `work-request-with-content.json` | `synthetic-golden-payload-lineage-work-request-with-content`, captured 2026-06-01T12:00:00Z | WORK_REQUEST carries text content; Current selection place occupancy resolves `payload_status: RESOLVED`. |
| `dispatch-without-input-content.json` | `synthetic-golden-payload-lineage-dispatch-without-input-content`, captured 2026-06-01T12:00:00Z | DISPATCH_REQUEST input omits content; consumed-input pin resolves submit-time payload from prior WORK_REQUEST. |

Each fixture JSON includes a `provenance` block with session id and capture timestamp. Replay expectations live in the fixture `expected` section and are asserted by `replayWorldState.payload-lineage.test.ts`.
