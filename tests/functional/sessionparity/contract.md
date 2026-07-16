# Factory Session parity contract

This is a test-only contract for comparing captured REST, CLI JSON, and MCP
observations of one Factory Session. It is input/output data only: it does not
create a runtime, service, persistence store, dependency graph, or alternate
canonical Factory Session model. Later customer-boundary scenarios provide the
captured observations.

## Stable facts

`Projection` retains the following required facts:

| Field | Required stable value |
| --- | --- |
| `identity.sessionId` | Non-empty Factory Session identifier. |
| `lifecycle.status` | Declared Factory Session lifecycle status. |
| `hashes.sourceHash` | Non-empty resolved source hash. |
| `progress` | Total, completed, failed, and in-flight dispatch counts. |
| `dispatches` | Ordered dispatch facts, correlated by `sessionId`; empty when none exist. |
| `artifacts` | Ordered artifact facts, correlated by `sessionId`; empty when none exist. |
| `results` | Ordered result facts, correlated by `sessionId`; empty when none exist. |
| `failures` | Ordered failure facts, correlated by `sessionId`; empty when none exist. |
| `eventCursors` | Canonical Factory Event cursors, correlated by `sessionId`; empty only when no canonical events have been observed. |

`lifecycle.phase`, `hashes.requestedPolicyHash`,
`hashes.effectivePolicyHash`, and `failures[].dispatchId` are optional stable
facts. Their absence is meaningful and must not be silently replaced with a
default value during normalization.

## Ordering and equivalence

Dispatches, artifacts, results, and failures retain the customer-visible
collection order and carry a one-based `order` value. `eventCursors` retain
canonical Factory Event order and use strictly increasing `sequence` values;
they are never sorted by transport arrival time. All retained child facts carry
the parent `sessionId`, so parity tests can reject facts correlated to another
Factory Session.

`NormalizeREST`, `NormalizeCLIJSON`, and `NormalizeMCP` accept captured JSON
observations only. They read `session`, `factorySession`, and
`result.factorySession` respectively, then decode the same stable field names
into `Projection`. Missing required facts and incompatible JSON values return a
`NormalizationError` with the customer interface and stable field path; no
value is defaulted to create false parity.

Protocol envelopes, HTTP status and headers, CLI rendering or diagnostics, and
MCP JSON-RPC request-correlation metadata do not belong in this contract
because they are not Factory Session facts.

## Deterministic fixture observations

`TerminalSuccessObservations` and `TerminalFailureObservations` provide static
captured JSON for REST, CLI JSON, and MCP. The success fixture has two completed
dispatches, a result artifact, a final result, and ordered terminal-success
cursors. The failure fixture has a completed dispatch, a failed dispatch, a
diagnostic artifact, a failure correlated to that dispatch, and ordered
terminal-failure cursors. Neither fixture starts a process or constructs runtime
composition; later functional scenarios may instead pass their own captures to
the normalizers.
