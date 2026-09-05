---
author: Agent Factory Team
last-modified: 2026-08-21
doc-id: agent-factory/guides/sessions
---

# Sessions and Runtime

Use this guide when you need to discover live factory sessions, confirm a
service is listening, inspect the active factory on a running host, read session
status from the API, or route submit and work commands to a non-default session.
For the end-to-end JavaScript workflow task—select or author source, validate,
start, inspect, and recover—use `you docs javascript-workflows`. This page owns
general Factory Session discovery, lifecycle controls, routing, and runtime
inspection shared by every orchestrator kind.

For the canonical real-pipeline lifetime and process-restart recovery runbook,
use `you docs operations`.

Use `you server` for continuous, non-resumable hosting of the exact Current
Factory. For restart-surviving hosting with an explicit recording, use
`you run --with-server --continuously --record <path>`. Continue that recording
after a stop with `you run --resume <recording>`.

Each live session owns its own runtime state. The service coordinates and
routes requests between sessions, but runtime state such as loaded factory,
event history, current work, and relative execution-path resolution is scoped
to the addressed session id.

When a session has a configured retained current-board Recording, a clean
daemon restart restores that session's Work board before the session is ready.
Process-bound dispatches that were active at stop are recorded as interrupted,
their old Worker Sessions are not exposed as live `RUNNING`, and associated
non-terminal Work is re-armed at its saved logical state for normal guards and
scheduling. Work and session inspection therefore keep the original
identities and interruption history without requiring an explicit resume.

For the end-to-end agent playbook (read order, submission ingress, operator
loop), see `you docs agents`. For submitted-work contracts
after the factory is running, see `you docs work`. For `factory.json` topology,
see `you docs config`.

## Durable Worker-ID history and finite event captures

Worker Session history is written as bounded, append-oriented Recordings data
under the configured home root at `.you-agent-factory/worker-sessions`. New
writes use the versioned v2 JSONL format; older v1 snapshot files remain
readable, but temporary files from an earlier home are not discovered. The
Worker Session ID is the durable lookup key, so a normal process restart does
not require the provider's session store for `list`, `show`, `stream`, or
`read`.

Use the canonical Worker-ID form for a provider-neutral replay:

```bash
you --server http://localhost:7437 worker-sessions stream \
  --worker-session-id <worker-session-id> --replay-only --output json
you --server http://localhost:7437 worker-sessions read \
  --worker-session-id <worker-session-id> --output json
```

The API and SSE frames may return `providerSession: null` when only durable
Worker evidence is available. Inspect `recordingHealth` on the observation,
transcript, and replay summary: `COMPLETE` means the terminal history is
present, `DEGRADED` means a readable prefix has an explicit failure, and
`INCOMPLETE` means the prefix has no confirmed terminal completion.
`recordingHealthReason` identifies the bounded tail, write, retention, or
interruption condition when one is known. A Worker-ID stream cursor is scoped
to the Worker ID and its stable stream generation; a future, stale, foreign,
or unavailable cursor is reported as a typed error rather than silently
restarting at the beginning.

## Finite Worker Session event captures

Use `worker-sessions stream --replay-only` when you need a redirect-safe
snapshot of one Worker Session without attaching a live follower. The
provider-keyed form remains available for compatibility:

```bash
you --server http://localhost:7437 worker-sessions stream \
  --provider codex --kind session_id --id <provider-session-id> \
  --replay-only --output json > worker-session.jsonl
```

Every non-empty line before the last line is the existing Worker Session stream
JSON record. The final line is one standalone replay summary, for example:

```json
{"kind":"replay-summary","complete":true,"reason":"session-completed","eventsEmitted":12}
```

`eventsEmitted` must equal the number of preceding event records. For an
`ACTIVE` Worker Session, the summary has `complete: false` and
`reason: "session-active"`; it describes the retained snapshot and does not
claim that later events were captured. For a terminal session, `complete: true`
means the retained history was drained to its terminal outcome, and `reason`
uses the stable `session-<terminal-state>` form such as
`session-completed`.

Replay-only exits successfully after the summary and does not send cancellation
or wait for live events. Leave out `--replay-only` (or use `--follow`) for the
normal retained-then-live stream. `--replay-only` and `--follow` are mutually
exclusive and fail before a stream request is made. Command diagnostics, when
enabled with `--verbose` or `--debug`, remain on stderr so they do not corrupt
the redirected NDJSON file.

## JavaScript Factory Session Model

The task procedure and complete CLI/API/MCP command sequence live in
`you docs javascript-workflows`. The material here only maps that procedure to
the shared Factory Session model used by the rest of this session reference.

A JavaScript execution is a `FactorySession` whose `orchestratorKind` is
`JAVASCRIPT`. It is not a workflow run or another resource alongside a Factory
Session. Use one Factory Session id from start through status, lifecycle control,
dispatch and artifact inspection, event replay, and result retrieval.

### Canonical surface map

Choose canonical Factory Session surfaces first. The CLI does not yet provide a
canonical `session start` spelling, so API or MCP is the canonical start path;
the `you session` commands are canonical for CLI discovery, inspection, and the
currently shipped pause, resume, cancel, and terminate controls.

| Goal | API | CLI | MCP | Dashboard |
|------|-----|-----|-----|-----------|
| Validate JavaScript source without starting a session | `POST /factories/preview` | No Factory Session-named spelling; use the API or MCP path | `you.factory_session.validate_source` | Factory preview or editor validation |
| Start synchronously | `POST /factory-sessions/sync` | No Factory Session-named spelling; use the API or MCP path | `you.factory_session.start_sync` | Start from the Factory Session entry surface when offered |
| Start asynchronously | `POST /factory-sessions/async` | No Factory Session-named spelling; use the API or MCP path | `you.factory_session.start_async` | Start from the Factory Session entry surface when offered |
| List or read sessions | `GET /factory-sessions`, `GET /factory-sessions/{session_id}` | `you session list --scope persisted`, `you session show {session_id}` | `you.factory_session.list`, `you.factory_session.get` | Factory Sessions list and Factory Session detail |
| Read the result | `GET /factory-sessions/{session_id}/results` | `you session show {session_id}` exposes result availability and refs | `you.factory_session.get_result` | Factory Session detail result state |
| Inspect dispatches | `GET /factory-sessions/{session_id}/dispatches` | Use the dispatch inspection guidance below | `you.factory_session.list_dispatches` | Factory Session detail dispatches |
| Inspect artifacts | `GET /factory-sessions/{session_id}/artifacts` | `you session show {session_id}` exposes artifact refs | `you.factory_session.list_artifacts` | Factory Session detail artifacts |
| Read ordered events | `GET /factory-sessions/{session_id}/events` | No Factory Session-named spelling; use the API or MCP path | `you.factory_session.read_events` | Factory Session detail live updates and history |
| Observe ephemeral response events | `GET /factory-sessions/{session_id}/response-events` | No Factory Session-named spelling; use the API path | No Factory Session-named spelling; use the API path | No dashboard-owned response-event stream today |
| Control lifecycle | `POST /factory-sessions/{session_id}/{pause\|resume\|cancel\|terminate}` | `you session pause\|resume\|cancel\|terminate {session_id}` | `you.factory_session.control` | Available actions on Factory Session detail |

### Migration guidance: dispatch inspection after CLI removal

The `you session dispatches` command is removed. No single CLI command replaces every dispatch inspection need.

- Use `you session show SESSION_ID` for Factory Session status and summary.
- Use `you metrics --session SESSION_ID --group-by workstation` for workstation aggregates.
- Use `you metrics --session SESSION_ID --group-by worker` for worker aggregates.
- Use `you metrics --session SESSION_ID --group-by provider` for provider aggregates.
- Use `GET /factory-sessions/SESSION_ID/dispatches` for exact durable dispatch records.
- Use `you.factory_session.list_dispatches` for exact dispatch records through MCP.
- Use `you worker-sessions list --work-id WORK_ID` for Work-specific Worker Session drill-down.

The Work-specific command requires a Work identifier. It does not replace session-level dispatch reads.

The start response supplies `{session_id}`. Keep that exact id for every later
call. `Dispatch`, `FactoryArtifact`, and `FactoryEvent` are session-owned facts:
their session identity and result or artifact references must describe the same
Factory Session, not parallel per-surface resources.

### Canonical example with one session id

For a start response containing `session_id: fs-js-42`, use
`GET /factory-sessions/fs-js-42`,
`GET /factory-sessions/fs-js-42/dispatches`,
`GET /factory-sessions/fs-js-42/artifacts`,
`GET /factory-sessions/fs-js-42/events`, and
`GET /factory-sessions/fs-js-42/results`. Apply lifecycle control to that same
id, for example `POST /factory-sessions/fs-js-42/pause`, then confirm the
outcome through the session read and its ordered events. Open `fs-js-42` in the
dashboard Factory Session detail rather than creating a separate workflow-run
identity for the UI.

### Supported scope today

- Use `POST /factories/preview` (or
  `you.factory_session.validate_source`) before execution when
  you need a source or policy check without creating a session.
- Use the exact `/factory-sessions/{session_id}` reads in the canonical surface
  map for JavaScript execution inspection.
- Use `you session show`, `GET /factory-sessions/{session_id}`, and the
  dashboard Factory Session detail surface when the session is also available
  through the running host's live session projection.
- Treat `Dispatch`, `FactoryArtifact`, and `FactoryEvent` as the shared
  inspection nouns across CLI, API, dashboard, and MCP surfaces. Do not
  introduce a separate workflow-run object model when comparing outputs.
- Ordinary live `you run` and packaged-factory invocations keep Factory Session
  durable execution in memory for now. They do not create or update
  project-local `.you-agent-factory/durable-sessions`. Explicit restart/resume
  snapshot persistence remains available when callers or tests opt in with
  `PersistencePolicyEnabled`.

### Bounded operator verification matrix

Use one durable JavaScript session id for the runtime checks below. The validate
step comes first and does not create a session; every later row should inspect
or control the same durable `FactorySession`.

| Step | Surface | Check | Expected observable outcome |
|------|---------|-------|-----------------------------|
| 1 | Factory Preview | Call `POST /factories/preview` or `you.factory_session.validate_source` against the target JavaScript source. | Validation succeeds without creating a session id, confirming the source and effective policy are ready for durable execution. |
| 2 | Factory Session start and read | Start with `POST /factory-sessions/async` or `you.factory_session.start_async`, then read `GET /factory-sessions/{session_id}` or call `you.factory_session.get`. | One durable `FactorySession` id is returned and subsequent reads show the same id, JavaScript lifecycle status, and progress for that session. |
| 3 | Session-owned inspection reads | Read the canonical dispatch, artifact, and event endpoints or their `you.factory_session.*` MCP tools for that same session id. | The shared `Dispatch`, `FactoryArtifact`, and `FactoryEvent` outputs all point back to the same `FactorySession`, and the event history shows the lifecycle and child-work facts that explain the dispatch or artifact state. |
| 4 | Website Factory Session detail | Open the dashboard Factory Session detail surface for the same session id. | The website shows the same session identity, JavaScript phase, checkpoint refs, dispatch counts, artifact visibility, and lifecycle banner state already observed through CLI or API reads. |
| 5 | Lifecycle control on the same session | Apply the supported lifecycle control route for that session, then re-read status or events. | Pause, resume, cancel, or terminate outcomes are reflected by the session status read and by canonical `SESSION_LIFECYCLE_CONTROL` facts on the same durable session event stream. |

#### What to compare across those checks

- Session identity: the durable `FactorySession` id returned at start should
  match the status, dispatch, artifact, event, and website detail reads.
- Status and phase: lifecycle state, JavaScript phase, and progress should stay
  aligned between canonical Factory Session reads and the dashboard
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
- Do not treat portable JavaScript recording resume, broader live-provider
  bridge parity, or broader MCP host parity as required outcomes for this
  proof. See `you docs record-replay` for the supported Factory Event resume
  path and portable recording limits. This matrix covers durable JavaScript
  session reads and controls.
- If the chosen session is already terminal, start another supported durable
  JavaScript session before attempting lifecycle-control confirmation so the
  control outcome remains observable on the same session path.

#### Reusable proof artifact for this matrix

Use the smallest existing regression surfaces that already prove the shipped
durable-session slice instead of building a one-off harness:

| Surface proved | Existing artifact | Command |
|----------------|-------------------|---------|
| Validate-first source readiness | CLI workflow validation package tests | `go test ./pkg/services/factory_runtime/transports/cli/workflow -run 'TestValidate_(ValidWorkflowNameHumanOutput|JSONOutputMatchesCanonicalValidationResult)' -count=1 -timeout 300s` |
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

- Portable JavaScript recording resume and persistence-semantics expansion
  beyond the already shipped durable session reads
- Broader live-provider bridge parity than the current bounded dispatch,
  artifact, and result inspection path
- Broader MCP host parity follow-up beyond the currently documented
  fixture-backed and runtime-backed host setup and smoke coverage

## When To Use This Guide

| Need | Use |
|------|-----|
| Validate JavaScript source before durable execution | [JavaScript Factory Session model](#javascript-factory-session-model) and `you docs javascript-workflows` |
| Recover a stopped `@you/goal` run through existing session and work controls | [Stopped goal inspect and recovery](#stopped-goal-inspect-and-recovery) and `you docs run` |
| Confirm anything is listening before `you submit` or `POST /factory-sessions/{session_id}/work` | [Session list](#session-list) |
| Read the active factory name and directory on a live host | [Factory show](#factory-show) |
| Inspect lifecycle phase, engine activity, and token buckets | [Session status API](#session-status-api) |
| Open the operator dashboard in a browser | [Dashboard](#dashboard) |
| Inspect orchestrator-aware runtime for one live session | [Session show](#session-show) and `you docs orchestrators` |
| Target a non-default session on submit or work commands | [`--server` and `--session`](#server-and-session-routing) |
| Choose a run mode that stays up for later submissions | [Run modes](#run-modes) |
| Pause or resume a live Factory Session without losing buffered work | [Session pause and resume](#session-pause-and-resume) |
| Reconnect to ephemeral invocation response events over SSE | [Response-event stream lifecycle and reconnect](#response-event-stream-lifecycle-and-reconnect) |

## Session list

`you session list` is the primary liveness check. It calls
`GET /factory-sessions` on the running host.

### Copy-paste examples

```bash
# Human table on the default local port (7437).
you session list

# API-shaped JSON for automation.
you session list --json

# Non-default port for list and create.
you session list --port 9090
you session create --dir /workspace/fleet --port 9090

# Delete through the selected server. The legacy --port flag is rejected.
you --server http://localhost:9090 session delete session-beta
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
the configured host and port. Start the Factory with explicit Work, such as
`you run --continuously --work ./docs/examples/startup-work.json`, before
retrying.

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

For named-Factory inputs and output modes, use `you docs run`. The inspection
and recovery controls remain on this page.

## Session lifecycle controls

`you session pause`, `resume`, `cancel`, and `terminate` control one
`FactorySession` through the lifecycle routes:

```http
POST /factory-sessions/{session_id}/pause
POST /factory-sessions/{session_id}/resume
POST /factory-sessions/{session_id}/cancel
POST /factory-sessions/{session_id}/terminate
```

Local placement is the default for these four controls. Use persistent
`--remote --server <uri>` to select exactly one already-running You server;
the same placement grammar is accepted before or after the command path. The
placement choice selects the local or HTTP adapter. Run waits for its selected
invocation result by default; use the run-level `--output primary` or
`--output response-stream` selector for stdout behavior. Local hosting remains
separate (`--with-server`, `--with-site`, `--listen`).

Pausing stops automatic progression while the service keeps accepting inbound
work and worker results. Resume records the lifecycle transition, wakes the
runtime internally, and drains ready buffered submissions and completed worker
results through the normal engine path. Cancel and terminate preserve the same
session identity and return the typed lifecycle outcome.

### Copy-paste examples

```bash
# Pause the default compatibility session (~default).
you session pause

# Pause or resume a named live session.
you session pause session-beta
you session resume session-beta

# Cancel or terminate the same session.
you session cancel session-beta
you session terminate session-beta

# Select an exact already-running server; both flag positions are valid.
you --remote --server http://factory.example:7437 session pause session-beta
you session resume session-beta --remote --server http://factory.example:7437

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
(`sessionId`, `operation`, `outcome`, `status`, and optional `detail`) for all
four controls.

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

## Factory show

`you factory show` reads the **current factory definition** for a live session
from `GET /factory-sessions/{session_id}/factory`. When you omit `--session` on
downstream commands, the API uses the default compatibility session (`~default`).

It confirms which factory name and topology the **runtime** loaded — not merely
which `factory.json` exists in a checkout.

### Copy-paste examples

```bash
# Human table from the default API base URI.
you factory show

# API-shaped JSON (place global flags before the subcommand).
you --json factory show

# Non-default host or port via global --server.
you --server http://localhost:9090 factory show
you --server http://localhost:9090 --json factory show
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

For deeper runtime health than list or factory show, call:

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

When the service was started with `you server` or `you run --with-site`, open
the dashboard URL reported after readiness. The preferred default is:

**`http://localhost:7437/dashboard/ui`**

Use the actual host and port reported by the process. `--listen <host:port>`
selects an exact loopback endpoint for `you server` and server-enabled runs.
When omitted, the commands use the default local bind; an explicit local
global `--server` remains a warned compatibility selector and can advance to
the next available port on collision. Ordinary `you run` and `you run --with-server` do not open a browser, and
bare `you` prints help without starting a service. The dashboard shows live session selection,
work position, factory activity, and Factory Session lifecycle control status
alongside CLI inspection. Paused and running lifecycle copy in the session
lifecycle banner is derived from canonical API status and replayed
`SESSION_LIFECYCLE_CONTROL` events rather than dashboard-owned state. For the
shipped JavaScript durable-session slice, use the **Factory session** detail
surface to compare the same `FactorySession` status, JavaScript phase,
checkpoint refs, dispatch counts, artifacts, and lifecycle banner state exposed
by the canonical Factory Session API routes.

## `--server` and `--session` routing

HTTP client commands that talk to a **running** service share global `--server`
(default `http://localhost:7437`). Place global flags **before** the subcommand:

```bash
you --server http://localhost:9090 submit --name task-1 --work-type-name task --payload request.md
you --server http://localhost:9090 --json work list
```

| Command family | Host selection | Session selection |
|----------------|----------------|-----------------|
| `you session list` / `create` | `--port` (default `7437`) | Session id is a subcommand argument on `create` |
| `you session delete` | Global `--server` | Session id is a required subcommand argument; legacy `--port` is rejected |
| `you session show`, `pause`, `resume`, `cancel`, `terminate` | Global `--server` | Session UUID is an optional subcommand argument; omission accepts the `~default` compatibility selector |
| `you factory show`, `you submit`, `you work …` | Global `--server` | `--session` on submit, batch submit, and work commands |
| `you server --listen <host:port>` | Binds the Current Factory continuously to the exact loopback host/port | N/A — starts a runtime |
| `you run --with-server --listen <host:port>` / `--with-site` | Binds only for the run lifetime to the exact loopback host/port | N/A — starts a runtime |
| Ordinary `you run` | Does not bind an HTTP listener | N/A — starts a runtime |

For a server that is already running, use remote placement explicitly:

```bash
you --remote --server <uri> run "Review the release notes"
```

For session pause, resume, cancel, and terminate, an explicit global
`--server <uri>` also selects the addressed running server; `--remote` remains
accepted when you want to make remote placement explicit.

`--remote` selects the running API endpoint and never starts local hosting.
It conflicts with `--with-server` and `--with-site`; use `--listen
<host:port>` on those local server-enabled forms instead.

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
| `you` (no args) | No — prints root help and performs no runtime or listener activation. |
| `you server` | Yes — serves the exact Current Factory until cancellation; it is continuous, non-resumable hosting. |
| `you run --continuously --with-server --record <path>` | Yes — processes Work, serves until cancellation, and writes the explicit recording used for restart recovery. |
| `you run --continuously --with-server` or `--with-site` | Yes — processes Work and serves until cancellation without an explicit recording path. |
| `you run --continuously` without a server flag | No HTTP service — the local runtime remains alive until cancellation. |
| One-shot `you run --with-server` or `--with-site` | Only for the run lifetime — the listener is joined before the command returns. |
| Ordinary one-shot `you run` | No — starts no listener and exits when the factory becomes idle or invocation completes. |

Finite server-enabled runs succeed when no Work is admitted or every admitted
Work item is terminal. If the queue drains around non-terminal Work, the
Factory Session is incomplete: the command returns failure with
`Error: factory session drained with N non-terminal work items; run is incomplete`
on stderr and joins the listener/runtime before returning. Continuous runs
remain live while idle and end only when cancelled.

For steady operator loops (check running → submit → verify), prefer
`you server` or `you run --continuously --with-server --record <path>`. See
`you docs agents` for the full operator loop and pre-submit checklist.

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
| Validate-first setup | `POST /factories/preview` or MCP `you.factory_session.validate_source` confirms source and policy readiness before a durable session exists. |
| API | `GET /factory-sessions/{session_id}/events` is the normal event stream for dashboard, Factory Session, durable replay, and reconnect traffic; pass `after_event_id` or `after_sequence` on that route. `GET /events` remains a **compatibility-only** process-global stream for legacy tooling and operator diagnostics—new session-aware consumers should migrate to the session-scoped route. |
| CLI | `you session show` prints Factory Session lifecycle timestamps, dispatch status, artifact refs, and best-effort partial/final result refs. Use `you metrics --session SESSION_ID --group-by worker` for aggregates. |
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

## Response-event stream lifecycle and reconnect

Use this route when you need **ephemeral invocation observation** for one live
Factory Session — incremental message, tool, progress, and related public
`FactoryResponseEvent` records while work is running. These records are
**outside canonical `FactoryEvent` replay** and never derive canonical Factory
state. They are not a substitute for durable session history.

Open the session-scoped route with the resolved Factory Session UUID:

`GET /factory-sessions/{session_id}/response-events`

For CLI one-shot invocation output modes (primary result, human
response-stream, and NDJSON automation), see `you docs run`. The API route
below is the session-owned SSE counterpart for consumers that attach to a
running Factory Session instead of parsing `you run` stdout.

### How this differs from canonical Factory events

| Stream | Route | What it carries | Replay role |
|--------|-------|-----------------|-------------|
| Canonical Factory events | `GET /factory-sessions/{session_id}/events` | Ordered `FactoryEvent` history for lifecycle, dispatches, artifacts, and replay projections | Durable session history for dashboard timeline, status derivation, and reconnect replay within the session's retained event history |
| Ephemeral response events | `GET /factory-sessions/{session_id}/response-events` | Ordered `FactoryResponseEvent` observation records for invocation progress | Session-scoped retained catch-up, then live continuation only while the session retains response-event history |

Do not treat response events as canonical Factory events. Dashboard timeline
replay, `SESSION_LIFECYCLE_CONTROL` derivation, and durable JavaScript session
inspection belong on the Factory event stream above.

### Copy-paste example

Replace `{session_id}` with the live Factory Session UUID from
`you session list` or a session start response. Omit `after_sequence` to begin
at the start of the session's **currently retained** response-event history.

```bash
curl -s -N \
  "http://localhost:7437/factory-sessions/{session_id}/response-events?after_sequence=12"
```

Each Server-Sent Events `id` is the decimal `FactoryResponseEvent.sequence`.
After you process a record, pass that sequence as `after_sequence` on the next
connection so the server resumes after the last acknowledged point.

### Retained catch-up, live continuation, and gaps

The connection first sends **retained matching records in ascending response
sequence**, then continues with **live matching records** on the same
connection.

| Cursor | Behavior |
|--------|----------|
| `after_sequence` omitted | Start at the beginning of retained response-event history for the selected session |
| `after_sequence` set to the last acknowledged `FactoryResponseEvent.sequence` | Send only retained records with a greater sequence, then continue live |
| `after_sequence` predates retained history | The first emitted record is `STREAM_GAP` with `fromSequence`, `toSequence`, and `firstAvailableSequence` describing the lost range instead of silently skipping unavailable sequences |

When you see `STREAM_GAP`, treat the described sequence range as unavailable.
Reconnect from `firstAvailableSequence` (or omit the cursor to replay from the
start of retained history) and reconcile your consumer state against the gap
payload rather than assuming contiguous observation.

Optional query filters such as `dispatch_id` and `kind` narrow which retained
and live records are delivered. Invalid cursor or filter values return typed
`400` responses before the stream opens.

### Typed HTTP outcomes before the stream opens

Response-event streaming **never falls back** to the current or default session.
Use the explicit `session_id` you intend to observe.

| Case | HTTP | Stable code |
|------|------|-------------|
| Unknown `session_id` | `404` | `RESPONSE_EVENT_SESSION_NOT_FOUND` |
| Retained response-event history has expired for the session | `410` | `RESPONSE_EVENT_STREAM_EXPIRED` |
| Invalid `after_sequence` or filter | `400` | `INVALID_RESPONSE_EVENT_CURSOR` or related bad-request codes |

These outcomes are distinct from Factory event reconnect probes on
`GET /factory-sessions/{session_id}/events`, which use `after_event_id` or
`after_sequence` against canonical `FactoryEvent` cursors and different typed
not-found or stale-cursor handling.

### Retention limits and non-promises

Response-event history is **session-scoped and ephemeral**. The service retains
only a bounded window while the Factory Session is live and for a limited time
after completion. Consumers must handle `STREAM_GAP`, stream expiration, and
sparse observation without assuming durable restart replay of response events
beyond that retention window.

The service does **not** promise:

- durable process-restart replay of response events after retained history
  expires,
- byte-identical provider transcripts on the public response stream, or
- that response-event observation replaces canonical `FactoryEvent` history for
  lifecycle, dispatch, or artifact facts.

Authoritative invocation success still comes from `primaryResult` on invocation
responses and from terminal work or session facts on canonical Factory events.
See `you docs run` for CLI output-mode contracts and `you docs workers` for
provider fidelity variability.

### Maintainer verification

After editing this reference topic, run `make docs-reference-smoke` from the
repository root and review the generated OpenAPI operation description for
`GET /factory-sessions/{session_id}/response-events` so packaged wording stays
aligned with the authored contract.

## Related Topics

- `you docs orchestrators` — Factory, FactoryOrchestrator, FactorySession, Dispatch, FactoryArtifact, FactoryEvent, and dynamic workflow aliases
- `you docs run` — CLI primary-result, human response-stream, and NDJSON invocation output modes
- `you docs agents` — agent orientation, operator loop, and topic router
- `you docs work` — `you submit`, `POST /factory-sessions/{session_id}/work`, and verification commands
- `you docs config` — `factory.json` topology and portability
- `you docs batch-inputs` — batch ingress when the factory is already running
- `docs/architecture/session-runtime-ownership.md` — maintainer reference for service/session ownership and state boundaries
