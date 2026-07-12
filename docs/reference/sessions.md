---
author: Agent Factory Team
last-modified: 2026-06-20
doc-id: agent-factory/guides/sessions
---

# Sessions and Runtime

Use this guide when you need to discover live factory sessions, confirm a
service is listening, inspect the active factory on a running host, read session
status from the API, or route submit and work commands to a non-default session.
It also owns the operator path for the currently supported durable JavaScript
`FactorySession` slice: validate source, start a durable session, inspect
status, result, dispatches, artifacts, and `FactoryEvent` history, confirm the
same session in the website detail surface, and apply lifecycle controls where
the current surface supports them.

Each live session owns its own runtime state. The service coordinates and
routes requests between sessions, but runtime state such as loaded factory,
event history, current work, and relative execution-path resolution is scoped
to the addressed session id.

For the end-to-end agent playbook (read order, submission ingress, operator
loop), see `you docs agents`. For submitted-work contracts
after the factory is running, see `you docs work`. For `factory.json` topology,
see `you docs config`.

## Durable JavaScript Factory Session Path

Use this bounded operator flow for the shipped JavaScript-orchestrated session
path. `Dynamic workflow` remains shorthand only; the canonical runtime object is
always a `FactorySession`.

| Operator goal | Current surface | What to confirm |
|---------------|-----------------|-----------------|
| Validate JavaScript source before execution | CLI `you workflow validate`; API `POST /factories/preview` | Source resolution, validation, and policy checks pass before session start |
| Start and inspect one durable JavaScript session | CLI `you workflow run`, `start`, `status`, `result` | One durable `FactorySession` id, lifecycle status, progress, and final or partial result availability |
| Inspect child work performed by that session | CLI `you workflow dispatches`, `artifacts`, `events`; API durable session reads; event stream replay | Shared `Dispatch`, `FactoryArtifact`, and `FactoryEvent` records match the same session id |
| Inspect the same session in the website | Dashboard Factory Session detail surface | Session status, JavaScript phase, checkpoint refs, dispatch counts, artifacts, and lifecycle banner line up with the API/CLI reads |
| Pause, resume, cancel, or terminate where the current route supports it | Live session lifecycle routes and durable session lifecycle-control surfaces | Accepted lifecycle operations are reflected by status reads and `SESSION_LIFECYCLE_CONTROL` facts on the canonical event stream |

### Supported scope today

- Use `you workflow validate` or `POST /factories/preview` before execution when
  you need a source or policy check without creating a session.
- Use durable Factory Session reads for JavaScript execution inspection:
  `you workflow status`, `you workflow result`, `you workflow dispatches`,
  `you workflow artifacts`, and `you workflow events`.
- Use `you session show`, `GET /factory-sessions/{session_id}`, and the
  dashboard Factory Session detail surface when the session is also available
  through the running host's live session projection.
- Treat `Dispatch`, `FactoryArtifact`, and `FactoryEvent` as the shared
  inspection nouns across CLI, API, dashboard, and MCP surfaces. Do not
  introduce a separate workflow-run object model when comparing outputs.

### Bounded operator verification matrix

Use one durable JavaScript session id for the runtime checks below. The validate
step comes first and does not create a session; every later row should inspect
or control the same durable `FactorySession`.

| Step | Surface | Check | Expected observable outcome |
|------|---------|-------|-----------------------------|
| 1 | CLI or API preview | Run `you workflow validate` or `POST /factories/preview` against the target JavaScript workflow source. | Validation succeeds without creating a session id, confirming the source and effective policy are ready for durable execution. |
| 2 | CLI start plus durable status read | Start one durable JavaScript session with `you workflow run` or `you workflow start`, then read it with `you workflow status`. | One durable `FactorySession` id is returned and subsequent status reads show the same id, JavaScript lifecycle status, and progress for that session. |
| 3 | Durable inspection reads | Read `you workflow dispatches`, `you workflow artifacts`, and `you workflow events` for that same session id. | The shared `Dispatch`, `FactoryArtifact`, and `FactoryEvent` outputs all point back to the same `FactorySession`, and the event history shows the lifecycle and child-work facts that explain the dispatch or artifact state. |
| 4 | Website Factory Session detail | Open the dashboard Factory Session detail surface for the same session id. | The website shows the same session identity, JavaScript phase, checkpoint refs, dispatch counts, artifact visibility, and lifecycle banner state already observed through CLI or API reads. |
| 5 | Lifecycle control on the same session | Apply the supported lifecycle control route for that session, then re-read status or events. | Pause, resume, cancel, or terminate outcomes are reflected by the session status read and by canonical `SESSION_LIFECYCLE_CONTROL` facts on the same durable session event stream. |

#### What to compare across those checks

- Session identity: the durable `FactorySession` id returned at start should
  match the status, dispatch, artifact, event, and website detail reads.
- Status and phase: lifecycle state, JavaScript phase, and progress should stay
  aligned between `you workflow status`, durable API reads, and the dashboard
  detail surface.
- Child work evidence: dispatch counts, child dispatch summaries, artifact refs,
  and any final or partial result refs should all describe the same session
  history rather than different per-surface models.
- Lifecycle control evidence: after a supported control is accepted, confirm the
  new status through a durable session read and the matching
  `SESSION_LIFECYCLE_CONTROL` event in the canonical event history.

#### Scope guardrails for this matrix

- Keep the matrix narrow. It is meant to revalidate one already supported
  durable JavaScript session path, not to inventory every route or dashboard
  widget.
- Do not treat replay-resume, broader live-provider bridge parity, or broader
  MCP host parity as required outcomes for this proof. Those remain explicit
  follow-up scope outside the shipped operator slice.
- If the chosen session is already terminal, start another supported durable
  JavaScript session before attempting lifecycle-control confirmation so the
  control outcome remains observable on the same session path.

#### Reusable proof artifact for this matrix

Use the smallest existing regression surfaces that already prove the shipped
durable-session slice instead of building a one-off harness:

| Surface proved | Existing artifact | Command |
|----------------|-------------------|---------|
| Validate-first source readiness | CLI workflow validation package tests | `go test ./pkg/cli/workflow -run 'TestValidate_(ValidWorkflowNameHumanOutput|JSONOutputMatchesCanonicalValidationResult)' -count=1 -timeout 300s` |
| Durable lifecycle-control outcome and canonical lifecycle events | Service durable-session lifecycle tests | `go test ./pkg/service -run 'TestFactoryService_(CancelDurableFactorySession_RuntimeBackedSession|LiveSessionPauseResume_HTTPReturnsTypedLifecycleControl|LiveSessionPauseResume_HTTPEmitsSessionLifecycleControlEvents)' -count=1 -timeout 300s` |
| Website Factory Session detail against a real backend durable session | Browser-backed dashboard integration using the existing harness plus durable workflow fixtures | `cd ui && bun vitest run integration/durable-session-real-backend.integration.test.mjs` |

Treat those three commands as the bounded end-to-end closeout proof for this
operator slice:

- The CLI validation tests prove the validate-first path without creating a
  session.
- The service lifecycle tests prove accepted durable lifecycle control and the
  canonical `SESSION_LIFECYCLE_CONTROL` event history.
- The browser-backed dashboard integration proves the same durable-session path
  through the Factory Session detail surface, including one running summary
  path and one completed dispatch or artifact drilldown path backed by the real
  API server harness.

Record the exact UTC run time and command results in the lane progress log when
you use this proof for closeout review.

### Explicitly out of scope for this slice

- Replay-resume or persistence-semantics expansion beyond the already shipped
  durable session reads
- Broader live-provider bridge parity than the current bounded dispatch,
  artifact, and result inspection path
- Broader MCP host parity follow-up beyond the currently documented
  fixture-backed and runtime-backed host setup and smoke coverage

## When To Use This Guide

| Need | Use |
|------|-----|
| Validate JavaScript source before durable execution | [Durable JavaScript Factory Session Path](#durable-javascript-factory-session-path) and `you docs orchestrators` |
| Recover a stopped `@you/goal` run through existing session and work controls | [Stopped goal inspect and recovery](#stopped-goal-inspect-and-recovery) and `you docs packaged-goal` |
| Confirm anything is listening before `you submit` or `POST /factory-sessions/{session_id}/work` | [Session list](#session-list) |
| Read the active factory name and directory on a live host | [Factory query](#factory-query) |
| Inspect lifecycle phase, engine activity, and token buckets | [Session status API](#session-status-api) |
| Open the operator dashboard in a browser | [Dashboard](#dashboard) |
| Inspect orchestrator-aware runtime for one live session | [Session show](#session-show) and `you docs orchestrators` |
| Target a non-default session on submit or work commands | [`--server` and `--session`](#server-and-session-routing) |
| Choose a run mode that stays up for later submissions | [Run modes](#run-modes) |
| Pause or resume a live Factory Session without losing buffered work | [Session pause and resume](#session-pause-and-resume) |

## Session list

`you session list` is the primary liveness check. It calls
`GET /factory-sessions` on the running host.

### Copy-paste examples

```bash
# Human table on the default local port (7437).
you session list

# API-shaped JSON for automation.
you session list --json

# Non-default port (session commands use --port, not global --server).
you session list --port 9090
```

### Human output

When sessions exist, stdout is a tab-separated table:

```text
SESSION ID    PROJECT    FOLDER PATH    FACTORY DIR    DEFAULT    ORCHESTRATOR KIND    TARGET KIND    TARGET NAME
```

`ORCHESTRATOR KIND` is `PETRI` or `JAVASCRIPT` when the list response includes
a runtime projection. Empty cells mean the host did not include runtime metadata
for that summary row.

When no sessions are open:

```text
No live factory sessions were found.
```

An empty table means the **service responded** but no live sessions are registered
yet — start or attach a factory before submitting work.

### Unreachable host

**Connection refused** or **endpoint not reachable** means nothing is listening on
the configured host and port. Start the factory with `you`, `you run --continuously`,
or `you run --dir <factory>` before retrying.

### Discover session ids

The `SESSION ID` column contains the routable id for each list row. On
single-session local hosts that id can remain the accepted `~default` selector,
not the session's canonical identity. In JSON output, use
`runtime.streamIdentity.factorySessionId` when that runtime projection is
present. A session read, sync preflight, or stream handshake can also return the
resolved `factorySessionId` UUID. Retain the resolved UUID for subsequent reads,
dashboard persistence, and event connections.

## Session show

`you session show` reads `GET /factory-sessions/{session_id}` for one live
`FactorySession` projection, including orchestrator kind and kind-specific runtime
fields.

### Copy-paste examples

```bash
# Resolve the default session through the compatibility selector.
you session show

# Named live session.
you session show session-beta

# API-shaped JSON.
you --json session show session-beta
```

Human output uses `FactorySession` as the canonical runtime noun. Petri sessions
show marking token counts and enabled transitions. JavaScript sessions show
phase, checkpoint refs, child dispatch counts, and dynamic workflow shorthand
only as JavaScript terminology. See `you docs orchestrators` for the accepted
alias rules.

## Stopped goal inspect and recovery

Use this path when a shared invocation or one-shot `you run --named @you/goal`
returns stopped-state recovery context instead of a primary result. Keep the
operator flow on the existing `FactorySession`, `Work`, and `Dispatch`
surfaces.

1. Inspect the `FactorySession` with `you session show <session-id>` or
   `GET /factory-sessions/{session_id}`.
2. Inspect the affected `Work` with `you work show <work-id> --session <session-id>`
   when the response or session stop summary identifies one work item.
3. Read the stop summary fields to distinguish a paused session lifecycle from
   blocked work, needs-human work, or an interrupted dispatch.
4. Apply the existing lifecycle or work control that matches that stop reason,
   then re-read the same session and work surfaces to confirm progress.

| Stop reason | What to confirm during inspect | Existing next step |
|-------------|--------------------------------|--------------------|
| Paused `FactorySession` | Session lifecycle is paused; buffered work remains attached to the same session. | `you session resume <session-id>` |
| Blocked `Work` | Work id, work state, latest dispatch or result summary, and suggested recovery surface. | Existing work repair, work move, or follow-up submission controls |
| Needs-human `Work` | The human input, approval, or artifact review required for that work item. | Existing human-input, approval, or repair step in the current workflow |
| Interrupted `Dispatch` or session | Interruption status and latest dispatch or result summary. | Existing dispatch retry, work repair, or session workflow controls |

For the `@you/goal` packaged-factory examples and invocation-specific failure
codes, use `you docs packaged-goal`.

## Session pause and resume

`you session pause` and `you session resume` control one live `FactorySession`
through the existing lifecycle routes:

```http
POST /factory-sessions/{session_id}/pause
POST /factory-sessions/{session_id}/resume
```

Pausing stops automatic progression while the service keeps accepting inbound
work and worker results. Resume records the lifecycle transition, wakes the
runtime internally, and drains ready buffered submissions and completed worker
results through the normal engine path. You do **not** need another submission,
worker result, or dispatch signal after resume to restart processing.

### Copy-paste examples

```bash
# Pause the default compatibility session (~default).
you session pause

# Pause or resume a named live session.
you session pause session-beta
you session resume session-beta

# API-shaped JSON for automation (place global flags before the subcommand).
you --json session pause
you --server http://localhost:9090 --json session resume session-beta
```

When you omit the session id, pause and resume follow the same default
compatibility session routing as `you session show` (`~default`).

### Human output

| Outcome | Pause stdout | Resume stdout |
|---------|--------------|---------------|
| Accepted | `Paused factory session <session-id>` | `Resumed factory session <session-id>` |
| No-op | `Factory session <session-id> is already paused` | `Factory session <session-id> is already running` |

Rejected controls return a non-zero exit code with an error message such as
`pause rejected for <session-id>: <detail>` or
`resume rejected for <session-id>: <detail>`. A missing session returns
`factory session "<session-id>" not found`. Connection failures return
`factory sessions endpoint not reachable at <url>`.

With `--json`, stdout is the API-shaped `FactorySessionLifecycleControlResponse`
(`sessionId`, `operation`, `outcome`, `status`, and optional `detail`).

### Buffered work while paused

While a live Factory Session is paused:

- `POST /factory-sessions/{session_id}/work` and `you submit` can still accept
  new work; accepted submissions stay buffered and are not applied until resume.
- Completed worker results stay buffered and are not routed through result
  handling until resume.
- `GET /factory-sessions/{session_id}/status` and `you session show` report
  `factoryState: PAUSED` (or the equivalent lifecycle status on session reads)
  without treating buffered work as already processed.
- A wake signal observed while paused does not drop buffered submissions or
  results; resume re-signals the runtime so that work can drain.

After a successful resume, inspect progress with `you session show`,
`GET /factory-sessions/{session_id}`, or
`GET /factory-sessions/{session_id}/events`. Canonical
`SESSION_LIFECYCLE_CONTROL` events record pause and resume for replay and
historical status reads.

### Canonical resume and result facts

Resume never creates a replacement Factory Session. Keep using the
`sessionId` returned by the original start request for lifecycle, result,
dispatch, checkpoint, and artifact reads. A successful resume returns that same
identifier, and terminal or already-running requests return a typed no-op or
rejection without changing results or dispatches.

Use typed fields instead of parsing status messages:

| Fact | REST | CLI | MCP | Dashboard |
|------|------|-----|-----|-----------|
| Session identity and lifecycle | `GET /factory-sessions/{session_id}` | `you --json session show <session-id>` | `you.factory_session.get` | Factory Session detail header and lifecycle status |
| Result availability | `GET /factory-sessions/{session_id}/result` | durable workflow `status` and `result` JSON | `you.factory_session.get_result` | Explicit not-ready, partial, failed-with-partial, or final result state |
| Latest approved checkpoint | session runtime checkpoint reference and canonical checkpoint events | session JSON and event inspection | Factory Session get and event tools | replay timeline checkpoint reference |
| Artifact lineage | `artifactRefs` and `/factory-sessions/{session_id}/artifacts` | result JSON artifact refs | result and artifact inspection tools | session-owned artifact drilldown links |

Result availability has stable typed meanings across those surfaces:

- `NOT_READY` means no customer result is available yet.
- `PARTIAL` means useful output and any referenced artifacts are available while
  the session can still continue.
- `FINAL` means the terminal result is available.
- `FAILED_WITH_PARTIAL` means execution terminated unsuccessfully but preserved
  useful output and artifact references.

Paused, interrupted, and terminal reads retain their checkpoint and artifact
references across reconnect and replay until a newer canonical event changes
the projection. Artifact references are session-scoped API paths; clients
should follow those references rather than constructing host filesystem paths.

Resume failures are also typed. `INVALID_RESUME_STATE` identifies a missing,
malformed, or non-approved checkpoint; `TERMINAL_SESSION` preserves an already
terminal session; `NO_OP` preserves an already-running session. Missing
sessions and unreachable services remain distinct not-found and transport
failures. None of these outcomes authorizes clients to switch to a new session
identifier.

### Maintainer verification

After editing this reference topic, run `make docs-reference-smoke` from the
repository root.

## Factory query

`you factory query` reads the **current factory definition** for a live session
from `GET /factory-sessions/{session_id}/factory`. When you omit `--session` on
downstream commands, the API uses the default compatibility session (`~default`).

It confirms which factory name and topology the **runtime** loaded — not merely
which `factory.json` exists in a checkout.

### Copy-paste examples

```bash
# Human table from the default API base URI.
you factory query

# API-shaped JSON (place global flags before the subcommand).
you --json factory query

# Non-default host or port via global --server.
you --server http://localhost:9090 factory query
you --server http://localhost:9090 --json factory query
```

### Human output

```text
NAME    KIND    ID    FACTORY DIRECTORY
```

`KIND` is `default-root` for the default current factory name or `named` for other
active factories.

### Errors

| Symptom | Meaning |
|---------|---------|
| `factory not reachable at <url>` | Transport failure — nothing listening at `--server`. |
| `running service has no active current factory` | Host is up but no current factory is activated; start a factory or activate a named factory. |

Run `you session list` first when you are unsure which sessions exist.

## Session status API

For deeper runtime health than list or factory query, call:

```http
GET /factory-sessions/{session_id}/status
```

Replace `{session_id}` with a live session id from `you session list` (often
`~default` on single-session hosts).

### Example request

```bash
curl -s "http://localhost:7437/factory-sessions/~default/status"
```

Use the same host and port as your running service (`--server` on HTTP client
commands encodes host and port; session list uses `--port` instead).

### Response fields

| Field | Meaning |
|-------|---------|
| `factoryState` | Factory lifecycle phase — for example `IDLE`, `RUNNING`, `PAUSED`, `COMPLETED`, `FAILED`. |
| `runtimeStatus` | Whether the engine is actively processing — `IDLE`, `ACTIVE`, or `FINISHED`. |
| `categories` | Token counts by lifecycle bucket: `initial`, `processing`, `terminal`, and `failed`. |
| `totalTokens` | Total tokens across categories. |
| `resources` | Optional resource usage entries when the factory declares resources. |

`factoryState` can be `RUNNING` while `runtimeStatus` is `IDLE` when the factory
is up but no work is in flight. Read `factoryState`, `runtimeStatus`, and
`categories` together when deciding whether to submit more work or wait for
completion.

## Session invocation API

Use the invocation API when you want one request to submit text input, wait for
the factory's invocation policy to resolve, and return one canonical
`primaryResult` without reconstructing work history yourself.

Synchronous invocation and future `POST /factory-sessions/sync` style callers
should treat `SESSION_COMPLETED` on the canonical `FactoryEvent` stream as the
authoritative terminal marker for one durable `FactorySession` execution.
`SESSION_RESULT_UPDATED` carries partial, final, or failed-with-partial customer
result availability before that terminal marker. Legacy `RUN_REQUEST` and
`RUN_RESPONSE` events remain compatibility surfaces during migration.

```http
POST /factory-sessions/{session_id}/invocations
```

The current API contract preserves text-first compatibility and also accepts
structured `args` for factories that declare `invocationSignature`. Legacy
requests still send top-level `sourceKind: "text"` and canonical `content` as
ordered `WorkContent` parts. Structured `args` values must decode as strings or
arrays of strings keyed by parameter name, external name, or alias. Reserved
source kinds such as `fileRef` and `audioStream` are named in the contract for
future compatibility, but current runtimes do not accept them yet.

### Example request

```bash
curl -s \
  -X POST \
  -H "Content-Type: application/json" \
  "http://localhost:7437/factory-sessions/~default/invocations" \
  -d '{
    "sourceKind": "text",
    "content": [
      { "type": "text", "text": "Summarize the failing test output." }
    ]
  }'
```

### Structured args compatibility

When `args` is present, omit compatibility `content` unless you are
intentionally exercising a source-conflict validation path. Factories without an
active `invocationSignature` reject `args` before dispatch.

`args` is the structured counterpart to CLI factory arguments:

- Keys may use the parameter `name`, `externalName`, or any declared alias
- Values must be a string or an array of strings
- Defaults, required checks, repeated handling, alias resolution, and stdin
  routing normalize through the same backend path used by `you run`
- Pre-dispatch argument failures return non-2xx `ErrorResponse` payloads instead
  of a terminal `InvocationResponse`

Use `you run --named <factory> --help` or `you run --factory <factory.json> --help`
when you want the selected factory's authored argument descriptions, defaults,
accepted values, output hints, and examples before constructing an API request.

### Example success response

```json
{
  "requestId": "req-123",
  "traceId": "trace-123",
  "status": "COMPLETED",
  "primaryResult": [
    { "type": "text", "text": "The root failure is a missing fixture path." }
  ]
}
```

`primaryResult` is present only when the invocation resolves successfully.
Selection follows the session's active `Factory.invocationReturn` policy. When
that field is omitted, the runtime uses the documented
`SUBMITTED_WORK_TERMINAL` fallback from `you docs config`.

### Non-success outcomes

| Case | Result |
|------|--------|
| Conflicting or ambiguous input sources | HTTP `400` with stable code `INVOCATION_INPUT_SOURCE_CONFLICT` |
| Empty selected text input | HTTP `400` with stable code `INVOCATION_INPUT_EMPTY` |
| Goal work routed to a blocked state | `status: FAILED`, code `INVOCATION_BLOCKED`, with `sessionId` / `workId` / `workState` recovery context when available |
| Goal work routed to a human-input-required state | `status: FAILED`, code `INVOCATION_NEEDS_HUMAN`, with the same shared recovery context |
| Waiting stopped because the session was paused | `status: FAILED`, code `INVOCATION_PAUSED`, no success payload |
| Interruption metadata explains the stop | `status: FAILED`, code `INVOCATION_INTERRUPTED`, no success payload |
| Invocation scope failed before a primary result existed | `status: FAILED`, code `INVOCATION_RUNTIME_FAILURE`, no success payload |
| No primary result can be resolved after work settles | `status: FAILED`, code `INVOCATION_PRIMARY_RESULT_UNRESOLVED`, no `primaryResult` |
| Invocation times out or is canceled | `status: TIMED_OUT` with `INVOCATION_TIMED_OUT`, or `status: CANCELED` with `INVOCATION_CANCELED` |

The CLI `you run --factory` mode uses the same invocation contract for input
resolution and primary-result selection; it just writes the successful
`primaryResult` to stdout instead of returning an HTTP response.

## Agent-run dispatch inspection

Use factory-session dispatch inspection when you need to distinguish final
agent output from bounded tool diagnostics or transcript metadata after an
`AGENT_RUN` dispatch.

| Surface | What it shows for agent runs |
|---------|------------------------------|
| Dashboard dispatch detail | Final output in the ordinary dispatch result area; separate **Agent run** section with `tool_policy`, `tool_call_count`, bounded `tool_diagnostics`, and transcript summaries when present |
| Session API / replay projections | `agentRunInspection` on workstation request responses; replay-safe `AGENT_RUN_RESPONSE` events carry the same bounded diagnostic payload |
| Modelhost (`you models inspect`, `/models`) | Readiness, lifecycle, and lease state only — not agent transcript ownership |

Agent-run inspection uses typed fields rather than mixed log blobs:

- **Final output** — the accepted or failed work result used for routing and
  downstream payload propagation.
- **Tool diagnostics** — safe summaries for allowed tool start, success, denial,
  or failure. Raw process output and secrets are not primary customer results.
- **Transcript metadata** — bounded per-message role and summary entries when
  the runtime records them.

When an agent dispatch fails, inspect `failure_class` and optional
`recovery_action` in the agent-run inspection view. See `you docs workers`
for the supported agent-run failure classes and tool-policy behavior.

`INFERENCE_RUN` dispatches continue to expose inference-attempt inspection
separately. Agent-backed dispatches do not substitute inference-attempt views
for agent-run diagnostics.

## Dashboard

When the service was started via `you` or `you run` without `--quiet`, open:

**`http://localhost:7437/dashboard/ui`**

Use the same host and port as the API unless you passed `--server` or `--port` on
the process that bound the listener. The dashboard shows live session selection,
work position, factory activity, and Factory Session lifecycle control status
alongside CLI inspection. Paused and running lifecycle copy in the session
lifecycle banner is derived from canonical API status and replayed
`SESSION_LIFECYCLE_CONTROL` events rather than dashboard-owned state. For the
shipped JavaScript durable-session slice, use the **Factory session** detail
surface to compare the same `FactorySession` status, JavaScript phase,
checkpoint refs, dispatch counts, artifacts, and lifecycle banner state that
you can read through `you workflow status`, `you workflow result`,
`you workflow dispatches`, `you workflow artifacts`, and `you workflow events`.

## `--server` and `--session` routing

HTTP client commands that talk to a **running** service share global `--server`
(default `http://localhost:7437`). Place global flags **before** the subcommand:

```bash
you --server http://localhost:9090 submit --name task-1 --work-type-name task --payload request.md
you --server http://localhost:9090 --json work list
```

| Command family | Host selection | Session selection |
|----------------|----------------|-----------------|
| `you session list` / `create` / `delete` | `--port` (default `7437`) | Session id is a subcommand argument on `create` / `delete` |
| `you session show`, `you session pause`, `you session resume` | Global `--server` | Session UUID is an optional subcommand argument; omission accepts the `~default` compatibility selector |
| `you factory query`, `you submit`, `you work …` | Global `--server` | `--session` on submit, batch submit, and work commands |
| `you run` | Binds locally to host/port from `--server` | N/A — starts or attaches runtime |

When `--session` is omitted on submit and work commands, the CLI accepts the
`~default` compatibility selector. A list row's `SESSION ID` may itself remain
`~default`; use `runtime.streamIdentity.factorySessionId` from JSON list output
when present, or resolve the UUID through a session read, sync preflight, or
stream handshake. Pass that resolved UUID on later submit and verify commands:

```bash
you submit --session session-beta \
  --name driver-incident-review \
  --work-type-name task \
  --payload request.md

you work show <work-id> --session session-beta
```

See `you docs work` for submit success output and verification with
`you work show` / `you work list`.

## Run modes

Choose how you start the factory based on whether later submissions need a
still-running service:

| How you start | Stays running for later `you submit` / `POST /factory-sessions/{session_id}/work`? |
|---------------|------------------------------------------------------|
| `you` (no args) | Yes — continuous mode; watches default inputs and keeps the service up. |
| `you run --continuously` | Yes — processes work until you stop the process. |
| `you run` (batch, no `--continuously`) | No — exits when the factory goes idle; restart before later CLI or API submissions. |

For steady operator loops (check running → submit → verify), prefer `you` or
`you run --continuously`. See `you docs agents` for the full operator loop and
pre-submit checklist.

## Event stream lifecycle and reconnect

API, CLI, dashboard, and future MCP tools observe the same canonical
`FactoryEvent` stream for one selected Factory Session. Open the session-scoped
route with the resolved Factory Session UUID so reconnect cursors and stream
recovery always carry canonical live identity:

`GET /factory-sessions/{session_id}/events`

Historical events are sent first in ascending tick order, followed by live
events on the same connection. Reconnect clients pass `after_event_id` or
`after_sequence` on that session-scoped URL to receive only events newer than
the acknowledged point. For live dashboard traffic, probe reconnect recovery
with `Accept: application/json` on the same route when the UI needs structured
`cursor_stale` or unknown-session outcomes before reopening Server-Sent Events.

| Surface | How lifecycle is observed |
|---------|---------------------------|
| Validate-first setup | `you workflow validate` and `POST /factories/preview` confirm source and policy readiness before a durable session exists. |
| API | `GET /factory-sessions/{session_id}/events` is the normal event stream for dashboard, Factory Session, durable replay, and reconnect traffic; pass `after_event_id` or `after_sequence` on that route. `GET /events` remains a **compatibility-only** process-global stream for legacy tooling and operator diagnostics—new session-aware consumers should migrate to the session-scoped route. |
| CLI | `you session show` prints live-session lifecycle timestamps, dispatch status, artifact refs, and best-effort partial/final result refs from the session API; `you workflow status`, `result`, `dispatches`, `artifacts`, and `events` do the same for durable JavaScript session inspection. |
| Dashboard | Opens the selected session's `GET /factory-sessions/{session_id}/events` stream, replays lifecycle events into the timeline projection, and shows reconnecting/stale, partial, paused, running, and terminal states in the session lifecycle banner. |
| MCP (planned) | Status/result/event tools should map `NOT_READY`, `PARTIAL`, `FINAL`, `FAILED_WITH_PARTIAL`, `INTERRUPTED`, and `RECONCILED` to the same `FactorySessionResultStatus` and dispatch status vocabulary as the session API and event stream. |

Lifecycle brackets use `SESSION_STARTED`, `SESSION_RESULT_UPDATED`, and
`SESSION_COMPLETED`. `SESSION_LIFECYCLE_CONTROL` records pause, resume, and other
accepted Factory Session lifecycle controls so replay and status reads can derive
`PAUSED` and `RUNNING` state from the same canonical stream. Orchestrator
progress, dispatch queue/interruption/reconciliation, and `ARTIFACT_CREATED`
events remain on the same stream so reconnect replay can rebuild phase, dispatch
counts, artifact refs, and terminal outcomes without waiting for only
`SESSION_COMPLETED`.

## Related Topics

- `you docs orchestrators` — Factory, FactoryOrchestrator, FactorySession, Dispatch, FactoryArtifact, FactoryEvent, and dynamic workflow aliases
- `you docs agents` — agent orientation, operator loop, and topic router
- `you docs work` — `you submit`, `POST /factory-sessions/{session_id}/work`, and verification commands
- `you docs config` — `factory.json` topology and portability
- `you docs batch-inputs` — batch ingress when the factory is already running
- `docs/architecture/session-runtime-ownership.md` — maintainer reference for service/session ownership and state boundaries
