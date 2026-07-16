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

REST bundle members are direct response bodies. CLI JSON bundle members are the
direct outputs of the corresponding `--json` commands. Each MCP bundle member
is the response from the corresponding `you.factory_session.*` tool, with its
typed value directly under `result`. The bundle itself is only a caller-supplied
capture container; it is not a product response or an alternate session model.

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

Protocol envelopes, HTTP status and headers, CLI rendering or diagnostics, and
MCP JSON-RPC request-correlation metadata do not belong in this contract
because they are not Factory Session facts.

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
capture bundles using the real REST body, CLI `--json`, and MCP tool-result
shapes. The success fixture has two completed dispatches, a result artifact, a
final result, and ordered terminal-success cursors. The failure fixture has a
completed dispatch, a failed dispatch, a diagnostic artifact, session and
dispatch failure facts, and ordered terminal-failure cursors. Neither fixture
starts a process or constructs runtime composition; later functional scenarios
may instead pass their own capture bundles to the normalizers.
