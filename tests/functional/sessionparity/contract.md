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

`NormalizeREST`, `NormalizeCLIJSON`, and `NormalizeMCP` accept test-only capture
bundles. A bundle groups the separate customer reads needed for parity:

- the durable Factory Session read model;
- the session-scoped dispatch and artifact list responses;
- the session result response; and
- the ordered canonical Factory Event response.

REST bundle members are direct response bodies, except that the raw
`text/event-stream` events body is JSON-string encoded so it can live in the
capture bundle. Its `data:` frames contain the canonical Factory Event values
emitted by the HTTP handler. CLI JSON bundle members are the direct outputs of
the corresponding `--json` commands, including the events array. Each MCP
bundle member is a complete JSON-RPC `tools/call` response: the server's
`CallToolResult` is under the outer `result`, and `content[0].text` contains the
serialized `ToolResponse` whose inner `result` is the typed customer value. The
bundle itself is only a caller-supplied capture container; it is not a product
response or an alternate session model.

The normalizers map `sessionId`, `status`, hashes, progress counts, dispatch
`id`/`status`/`dispatchKind`, artifact `id`/`kind`, result status and canonical
content, failure details, and Factory Event `id`/`type`/`context` fields into
`Projection`. The result correlation identifier is deterministically derived as
`<sessionId>:result` because the customer result response is session-scoped and
does not declare a separate result identifier. Session, dispatch, and result
failure identifiers are likewise derived from their owning stable identifiers
and surface so distinct declared failure facts remain visible. Result content is
JSON-canonicalized while retaining array order, so object key order does not
create transport-only drift. Missing required facts and incompatible JSON values
return a `NormalizationError` with the customer interface and stable field path;
in particular, each required progress count is decoded with presence awareness
and is never defaulted to zero.

The adapters exclude only transport mechanics that do not describe Factory
Session state:

- REST capture status, headers, request identifiers, methods, and paths describe
  the HTTP exchange rather than the session.
- CLI presentation choices and command diagnostics describe rendering and the
  local command invocation rather than the session.
- REST SSE comments plus `event`, `id`, and `retry` fields describe stream
  framing; each Factory Event JSON value under `data:` remains fully retained.
- MCP JSON-RPC versions, envelope fields, request IDs, and tracing or
  request-correlation metadata describe the tool exchange rather than the
  session. `CallToolResult` framing is decoded, while the typed value serialized
  through `content[0].text` remains fully retained.

Focused tests mutate representative exclusions on each real capture shape and
require an unchanged projection. In particular, REST coverage adds HTTP
exchange metadata and valid SSE comment, `id`, `event`, and `retry` fields to
the handler-shaped stream, while MCP coverage replaces every real JSON-RPC
request ID around its `CallToolResult`. Paired mutations change a retained REST
Factory Event type and the retained CLI/MCP session status inside the real
customer values, preventing framing exclusion from hiding Factory Session
drift.

## Difference reports

`Compare` compares two already-normalized projections and returns every
semantic difference as a `Difference` with a stable field path plus compact,
normalized JSON `expected` and `actual` values. It compares object fields in
lexicographic path order and collection items by their retained order. Missing
facts are reported with `null` on the missing side, unexpected facts with
`null` on the expected side, and duplicate or reordered facts through their
affected indexed paths. The report is deterministic and never incorporates
transport formatting, observation timestamps, or protocol metadata.

## Deterministic fixture observations

`TerminalSuccessObservations` and `TerminalFailureObservations` provide static
capture bundles using real REST JSON and SSE bodies, CLI `--json` outputs, and
MCP JSON-RPC `tools/call` response shapes. The success fixture has two completed
dispatches, a result artifact, a final result, and ordered terminal-success
cursors. The failure fixture has a completed dispatch, a failed dispatch, a
diagnostic artifact, session and dispatch failure facts, and ordered
terminal-failure cursors. Neither fixture starts a process or constructs runtime
composition; later functional scenarios may instead pass their own capture
bundles to the normalizers.
