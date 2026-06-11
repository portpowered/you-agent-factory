---
author: Agent Factory Team
last-modified: 2026-06-08
doc-id: agent-factory/guides/sessions
---

# Sessions and Runtime

Use this guide when you need to discover live factory sessions, confirm a
service is listening, inspect the active factory on a running host, read session
status from the API, or route submit and work commands to a non-default session.

Each live session owns its own runtime state. The service coordinates and
routes requests between sessions, but runtime state such as loaded factory,
event history, current work, and relative execution-path resolution is scoped
to the addressed session id.

For the end-to-end agent playbook (read order, submission ingress, operator
loop), see `you docs agents`. For submitted-work contracts
after the factory is running, see `you docs work`. For `factory.json` topology,
see `you docs config`.

## When To Use This Guide

| Need | Use |
|------|-----|
| Confirm anything is listening before `you submit` or `POST /factory-sessions/{session_id}/work` | [Session list](#session-list) |
| Read the active factory name and directory on a live host | [Factory query](#factory-query) |
| Inspect lifecycle phase, engine activity, and token buckets | [Session status API](#session-status-api) |
| Open the operator dashboard in a browser | [Dashboard](#dashboard) |
| Inspect orchestrator-aware runtime for one live session | [Session show](#session-show) and `you docs orchestrators` |
| Target a non-default session on submit or work commands | [`--server` and `--session`](#server-and-session-routing) |
| Choose a run mode that stays up for later submissions | [Run modes](#run-modes) |

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

Use the `SESSION ID` column when routing other commands with `--session` (for
example `you submit --session session-beta` or `you work list --session session-beta`).
On single-session local hosts the default compatibility session is often `~default`.

## Session show

`you session show` reads `GET /factory-sessions/{session_id}` for one live
`FactorySession` projection, including orchestrator kind and kind-specific runtime
fields.

### Copy-paste examples

```bash
# Default compatibility session (~default).
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

The current API contract is text-first. Send top-level `sourceKind: "text"` and
canonical `content` as ordered `WorkContent` parts. Reserved source kinds such
as `fileRef` and `audioStream` are named in the contract for future
compatibility, but current runtimes do not accept them yet.

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
| No primary result can be resolved | `status: FAILED`, code `INVOCATION_PRIMARY_RESULT_UNRESOLVED`, no `primaryResult` |
| Invocation times out or is canceled | `status: TIMED_OUT` or `status: CANCELED`, no success payload |

The CLI `you run --factory` mode uses the same invocation contract for input
resolution and primary-result selection; it just writes the successful
`primaryResult` to stdout instead of returning an HTTP response.

Invocation dispatches still honor the active factory's execution-family
`modelProvider` precedence: workstation override, factory default, worker
fallback, then operator default. See `you docs config` for `DEFAULT` deferral
and retired `runner` migration guidance.

## Dashboard

When the service was started via `you` or `you run` without `--quiet`, open:

**`http://localhost:7437/dashboard/ui`**

Use the same host and port as the API unless you passed `--server` or `--port` on
the process that bound the listener. The dashboard shows live session selection,
work position, and factory activity alongside CLI inspection.

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
| `you factory query`, `you submit`, `you work …` | Global `--server` | `--session` on submit, batch submit, and work commands |
| `you run` | Binds locally to host/port from `--server` | N/A — starts or attaches runtime |

When `--session` is omitted on submit and work commands, the CLI targets the
default compatibility session (`~default`). After `you session list`, pass the
same session id on submit and verify commands:

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
`FactoryEvent` stream for one live session.

| Surface | How lifecycle is observed |
|---------|---------------------------|
| API | `GET /events` and `GET /factory-sessions/{session_id}/events` stream canonical lifecycle variants; reconnect with `after_event_id` or `after_sequence`. |
| CLI | `you session show` prints lifecycle timestamps, dispatch status, artifact refs, and best-effort partial/final result refs from the session API. |
| Dashboard | Replays lifecycle events into the timeline projection and shows reconnecting/stale, partial, and terminal states in the session lifecycle banner. |
| MCP (planned) | Status/result/event tools should map `NOT_READY`, `PARTIAL`, `FINAL`, `FAILED_WITH_PARTIAL`, `INTERRUPTED`, and `RECONCILED` to the same `FactorySessionResultStatus` and dispatch status vocabulary as the session API and event stream. |

Lifecycle brackets use `SESSION_STARTED`, `SESSION_RESULT_UPDATED`, and
`SESSION_COMPLETED`. Orchestrator progress, dispatch queue/interruption/reconciliation,
and `ARTIFACT_CREATED` events remain on the same stream so reconnect replay can rebuild
phase, dispatch counts, artifact refs, and terminal outcomes without waiting for only
`SESSION_COMPLETED`.

## Related Topics

- `you docs orchestrators` — Factory, FactoryOrchestrator, FactorySession, Dispatch, FactoryArtifact, FactoryEvent, and dynamic workflow aliases
- `you docs agents` — agent orientation, operator loop, and topic router
- `you docs work` — `you submit`, `POST /factory-sessions/{session_id}/work`, and verification commands
- `you docs config` — `factory.json` topology and portability
- `you docs batch-inputs` — batch ingress when the factory is already running
- `docs/architecture/session-runtime-ownership.md` — maintainer reference for service/session ownership and state boundaries
