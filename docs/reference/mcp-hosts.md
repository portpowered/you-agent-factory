---
author: Agent Factory Team
last-modified: 2026-06-17
doc-id: agent-factory/guides/mcp-hosts
---

# Dynamic Workflow MCP Host Setup

Use this guide when you want an MCP-capable host (Cursor, Codex, OpenCode,
Kiro, Gemini, or another stdio MCP client) to discover and call the repository's
Factory Session tools for dynamic workflows.

For canonical runtime nouns (`FactorySession`, `Dispatch`, `FactoryArtifact`),
see `you docs orchestrators`. For durable session CLI inspection, see
`you docs sessions`.

## Server Command And Transport

The current MCP server entrypoint is:

```bash
you mcp serve
```

Transport assumptions:

| Input | Requirement |
|-------|-------------|
| Transport | stdio only for this batch |
| Protocol | MCP JSON-RPC (`2024-11-05`) over newline-delimited messages |
| Child process | The host launches `you` as a subprocess |
| stdin / stdout | JSON-RPC request/response channel |
| stderr | Reserved for process diagnostics; hosts must not parse MCP traffic from stderr |
| Working directory | Project root where workflow sources and the durable session fixture catalog resolve |
| Executable | Absolute path to a built `you` binary is recommended when the host PATH is limited |
| Arguments | `mcp serve` |
| Environment | No extra env vars are required for the default mock-backed fixture service |
| Optional flag | `--fixture-catalog <path>` when the fixture catalog is outside the discovered repository root |

The default `you mcp serve` backing service is the same deterministic durable
Factory Session fixture catalog used by `you workflow run`, `you workflow
status`, and `you workflow result`. Hosts therefore exercise the current MCP
tool surface without requiring a live factory HTTP server for the first install
smoke path.

## Serve Mode Selection

`you mcp serve` supports two documented backing modes. Both expose the same
`you.factory_session.*` tool catalog and Factory Session vocabulary; only the
execution service behind the MCP client changes.

| Mode | Launch args | Backing service | Typical use |
|------|-------------|-----------------|-------------|
| Fixture-backed (default) | `["mcp", "serve"]` | Durable session fixture catalog | Deterministic install smoke, fixture-driven async polling, and offline host validation |
| Runtime-backed | `["mcp", "serve", "--runtime"]` | Shared durable JavaScript runtime service | Live INLINE_WORKFLOW execution with real async start, status polling, and terminal or not-ready result reads |

Fixture-backed mode discovers the contract fixture catalog from the MCP host
working directory or accepts `--fixture-catalog <path>` when the catalog is
outside the project root.

Runtime-backed mode resolves workflow sources from the host working directory or
from an explicit `--project-root`. Do not combine `--runtime` with
`--fixture-catalog`.

### Runtime-backed host JSON

Use this shape when the host should call live Factory Session tools through the
shared durable runtime service:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve", "--runtime"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

Keep the default fixture-backed configuration from the generic host pattern
above when deterministic fixture scenarios are sufficient.

## Canonical Factory Session MCP Tools

Primary tool names use Factory Session vocabulary:

| Tool | Purpose |
|------|---------|
| `you.factory_session.list` | List durable Factory Sessions |
| `you.factory_session.validate_source` | Validate JavaScript orchestrator source through the Factory preview contract |
| `you.factory_session.start_sync` | Start one sync Factory Session and wait for terminal completion or timeout |
| `you.factory_session.start_async` | Start one async Factory Session for polling |
| `you.factory_session.get` | Read one durable Factory Session status and progress |
| `you.factory_session.get_result` | Read one durable Factory Session result |
| `you.factory_session.list_dispatches` | List dispatch summaries for one session |
| `you.factory_session.list_artifacts` | List FactoryArtifact summaries for one session |
| `you.factory_session.control` | Apply lifecycle controls such as pause, resume, cancel, or terminate |
| `you.factory_session.read_events` | Read ordered Factory Session event facts |

Compatibility-only workflow aliases resolve to the same handlers:

| Alias | Canonical tool |
|-------|------------------|
| `you.workflow.validate` | `you.factory_session.validate_source` |
| `you.workflow.run` | `you.factory_session.start_sync` |
| `you.workflow.status` | `you.factory_session.get` |
| `you.workflow.result` | `you.factory_session.get_result` |
| `you.workflow.artifacts` | `you.factory_session.list_artifacts` |

Hosts may discover either naming family. Prefer the `you.factory_session.*`
tools in new configuration.

## Complete Runtime-Backed Factory Session Transcript

The following abbreviated host conversation uses `you mcp serve --runtime` and
one illustrative dynamic workflow. The host sends each object as the
`arguments` value of an MCP `tools/call`. The responses show representative
stable fields only: timestamps, progress counts, event ids, artifact ids, and
workflow payloads vary by execution and are not fixed contract values.

First validate the source before creating runtime work:

```text
host -> you.factory_session.validate_source
{"kind":"INLINE_WORKFLOW","inlineSource":"export default async function ({ final }) { return final({ answer: 'ready' }) }"}

you ->
{"result":{"valid":true,"diagnostics":[]}}
```

Then start it asynchronously with a caller-supplied request id. Live dynamic
workflow execution requires runtime-backed serve; the default fixture-backed
mode only resolves deterministic catalog scenarios.

```text
host -> you.factory_session.start_async
{"source":{"kind":"INLINE_WORKFLOW","inlineWorkflow":{"inlineSource":{"encoding":"utf-8","inline":"export default async function ({ final }) { return final({ answer: 'ready' }) }"}}},"requestId":"req-host-demo-001"}

you ->
{"result":{"sessionId":"fs-host-demo-01","requestId":"req-host-demo-001","status":"RUNNING"}}
```

Retain `fs-host-demo-01`. Every post-start call below addresses that same
Factory Session; none creates or substitutes another session. Poll its durable
status and result independently:

```text
host -> you.factory_session.get
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","status":"RUNNING","progress":{"completedDispatches":0}}}

host -> you.factory_session.get_result
{"sessionId":"fs-host-demo-01","mode":"FINAL","includeArtifacts":true}

you ->
{"error":{"code":"factory_session.result.not_ready","retryable":true,"sessionId":"fs-host-demo-01"}}
```

Inspect the work owned by the Factory Session while it runs. A Dispatch is one
child execution; a `FactoryArtifact` is durable output metadata; and a
`FactoryEvent` is an ordered runtime fact.

```text
host -> you.factory_session.list_dispatches
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","dispatches":[{"id":"dispatch-host-demo-01","status":"RUNNING"}]}}

host -> you.factory_session.list_artifacts
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","artifacts":[{"id":"artifact-checkpoint-01","kind":"CHECKPOINT","visibility":"INTERNAL_CHECKPOINT"}]}}

host -> you.factory_session.read_events
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","events":[{"id":"event-host-demo-07","type":"FactoryEventTypeDispatchQueued","context":{"sessionSequence":7,"dispatchId":"dispatch-host-demo-01"}}]}}
```

For a workflow that is still running, this is a valid control-and-observe
sequence. Wait for each accepted operation to become visible before sending the
next operation:

```text
host -> you.factory_session.control
{"sessionId":"fs-host-demo-01","operation":"PAUSE","requestId":"req-pause-host-demo-01","reason":"host maintenance"}

you ->
{"result":{"sessionId":"fs-host-demo-01","operation":"PAUSE","outcome":"ACCEPTED","status":"PAUSED"}}

host -> you.factory_session.get
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","status":"PAUSED"}}

host -> you.factory_session.control
{"sessionId":"fs-host-demo-01","operation":"RESUME","requestId":"req-resume-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","operation":"RESUME","outcome":"ACCEPTED","status":"RUNNING"}}
```

Continue polling the same id. Completion is confirmed when
`you.factory_session.get` reports a terminal status and
`you.factory_session.get_result` returns a terminal result. The final
`you.factory_session.list_dispatches` view should show terminal Dispatch
statuses, `you.factory_session.list_artifacts` should expose any final
`FactoryArtifact` references, and `you.factory_session.read_events` should
contain ordered `FactoryEvent` facts for the lifecycle controls and completion.
These facts, rather than the illustrative ids or timestamps above, confirm the
outcome.

Use canonical `you.factory_session.*` names for this conversation.
`you.workflow.*` names are compatibility-only aliases for existing
integrations, not the recommended customer surface.

## Reconnect With Stable Identifiers

Before the host or its MCP child process disconnects, persist the identifiers
needed to continue inspection:

- the caller-supplied start `requestId` and returned Factory Session
  `sessionId`;
- any Dispatch `id` or FactoryArtifact `id` the host still needs to correlate
  with later status, result, or event facts; and
- the last processed FactoryEvent `id` (or its `context.sessionSequence`) as the
  event cursor.

After reconnecting to runtime-backed serve for the same project, resume reads
against the retained Factory Session id. Reconnecting does not start or replay
a Factory Session, so do not call `start_async` merely to recover visibility:

```text
host -> you.factory_session.get
{"sessionId":"fs-host-demo-01"}

you ->
{"result":{"sessionId":"fs-host-demo-01","status":"RUNNING"}}

host -> you.factory_session.read_events
{"sessionId":"fs-host-demo-01","afterEventId":"event-host-demo-07"}

you ->
{"result":{"sessionId":"fs-host-demo-01","events":[{"id":"event-host-demo-08","type":"FactoryEventTypeSessionLifecycleControl","context":{"sessionSequence":8}}]}}

host -> you.factory_session.get_result
{"sessionId":"fs-host-demo-01","mode":"FINAL","includeArtifacts":true}
```

`afterEventId` returns ordered facts after the retained event. A host that
retains `context.sessionSequence` instead may send `afterSequence`; when both
are supplied, `afterEventId` wins. Advance the persisted cursor only after the
host has processed the returned events.

## Typed Outcomes And Host Action

Treat the response envelope or lifecycle-control result as the decision input;
do not turn every failure into a new start request.

| Outcome | Host action |
|---------|-------------|
| `factory_session.result.not_ready` with `retryable: true` | Keep the same `sessionId`; poll `you.factory_session.get` and `you.factory_session.get_result` with backoff. Do not submit duplicate work. |
| `factory_session.session.not_found` with `retryable: false` | Stop polling that value. Correct a stale or malformed `sessionId`, or stop recovery if the retained session no longer exists; a reconnect is not permission to substitute a new session. |
| `factory_session.events.reconnect_cursor_not_found` with `retryable: false` | Keep the same `sessionId`, correct the stale event cursor from durable host state, and restart inspection from a known valid cursor. Do not guess that missing events were processed. |
| Control result `NO_OP`, `INVALID_STATE`, or `TERMINAL_SESSION` for `RESUME` | Do not blindly retry. Re-read the same session: `NO_OP` means no transition was needed, `INVALID_STATE` requires correcting the operation for the current state, and `TERMINAL_SESSION` means resume cannot change the terminal session. |
| `factory_session.start.request_id_conflict` with `retryable: false` | The request id was reused with a different start tuple. Preserve and retry the original `requestId` only with its original source and arguments; if the intended tuple changed, correct the request by assigning a new request id. |
| `factory_session.service.unavailable` | Stop tool-call retries until the configured fixture or runtime backing service is available. Restore or respawn the correctly configured MCP server, then retry the same safe read or the same idempotent start tuple; do not invent a replacement `sessionId` or `requestId`. |

These outcomes use the existing MCP contracts exercised by focused Factory
Session failure, runtime, and lifecycle tests. They do not introduce a separate
failure surface or change runtime recovery semantics.

## Generic MCP Host Pattern

Most stdio MCP hosts accept the same child-process shape:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

Replace:

- `command` with the built `you` binary path.
- `cwd` with the repository or customer project root where workflow sources
  resolve and the durable session fixture catalog is discoverable.

After editing host configuration, restart or reload the host so it respawns the
MCP child process.

## Cursor

Config locations:

| Scope | Path |
|-------|------|
| Project | `.cursor/mcp.json` |
| Global | `~/.cursor/mcp.json` |

Example project config:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

Required inputs:

- `command`: absolute `you` binary path when Cursor cannot see your shell PATH
- `args`: `["mcp", "serve"]`
- `cwd`: project root for workflow source resolution

Restart Cursor or run **Developer: Reload Window** after saving the config.

## Codex

Codex MCP hosts typically use the same stdio child-process model. Configure the
server in the Codex MCP settings surface for your installation and point it at
the same command line:

```text
/absolute/path/to/you mcp serve
```

Set the working directory to the target project root. If your Codex build stores
servers in a TOML or JSON settings file, keep the same `command`, `args`, and
`cwd` semantics as the generic pattern above.

## OpenCode

OpenCode MCP integrations also launch stdio servers as child processes. Add a
server entry with:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

Use the OpenCode settings file or project MCP configuration location supported
by your OpenCode version. The command and tool names must match `you mcp serve`
and the `you.factory_session.*` catalog above.

## Kiro

Kiro MCP setup follows the same stdio subprocess contract. Configure Kiro to
launch:

```text
/absolute/path/to/you mcp serve
```

with the project root as the working directory. Keep Factory Session tool names
in prompts and automation; do not introduce a separate workflow-run product
noun.

## Gemini

Gemini CLI and Gemini-aware editors that support MCP stdio servers should use
the same launch command and working-directory requirements:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

Use the MCP settings location provided by your Gemini host. Tool discovery
should surface the `you.factory_session.*` catalog after the host respawns the
child process.

## First Host Smoke Sequence

After configuration, verify the install path in this order:

1. Start or respawn the MCP server through the host configuration.
2. Discover tools and confirm the `you.factory_session.*` catalog is present.
3. Call `you.factory_session.validate_source` against a known fixture workflow
   source.
4. Call `you.factory_session.start_async` for one simple fixture session.
5. Poll with `you.factory_session.get` and `you.factory_session.get_result`.

This sequence uses Factory Session vocabulary end to end. It does not require a
separate workflow-run surface.

## Smoke Proof Boundaries

Use this matrix to tell what the repository proves in CI versus what still needs
a human-host check. Coverage here is limited to MCP install and smoke behavior.
It does not claim API parity, dashboard parity, or real-runtime execution parity.

### Coverage Matrix

| Host / path | Coverage | What this batch proves |
|-------------|----------|------------------------|
| Shared `you mcp serve` stdio server (fixture-backed default; all host examples depend on this) | **Automated in-repo** | Stdio JSON-RPC install path: `initialize`, `tools/list`, `you.factory_session.validate_source`, `you.factory_session.start_async`, `you.factory_session.get`, and not-ready `you.factory_session.get_result` through `pkg/cli/mcp/serve_smoke_test.go` |
| Runtime-backed `you mcp serve --runtime` stdio server | **Automated in-repo** | Same tool catalog with live durable JavaScript execution: async start, status polling, and not-ready or terminal `you.factory_session.get_result` through `pkg/cli/mcp/serve_runtime_smoke_test.go` |
| Runtime-backed interrupted-to-resumed MCP stdio server | **Automated in-repo** | Same runtime-backed stdio path with `you.factory_session.control` resume, stable `FactorySession` id, dispatch continuity, and typed invalid resume outcomes through `pkg/cli/mcp/serve_runtime_resume_smoke_test.go`; non-resume fixture/runtime serve regression through `pkg/cli/mcp/serve_runtime_resume_non_regression_test.go` |
| Generic stdio MCP client config pattern | **Documented manual** | Host respawn, config reload, and tool discovery through a real MCP client UI; shared server/tool behavior is automated above |
| Cursor (`.cursor/mcp.json` or `~/.cursor/mcp.json`) | **Documented manual** | Same command/args/cwd as the generic pattern plus Cursor-specific config path and reload behavior |
| Codex | **Documented manual** | Same stdio child-process launch through Codex MCP settings; no Codex-specific in-repo automation in this batch |
| OpenCode | **Documented manual** | Same stdio child-process launch through OpenCode MCP settings; no OpenCode-specific in-repo automation in this batch |
| Kiro | **Documented manual** | Same stdio child-process launch through Kiro MCP settings; no Kiro-specific in-repo automation in this batch |
| Gemini | **Documented manual** | Same stdio child-process launch through Gemini MCP settings; no Gemini-specific in-repo automation in this batch |
| HTTP or SSE MCP transport | **Unsupported this batch** | `you mcp serve` is stdio-only for this install lane |
| Live factory HTTP runtime backing | **Out of scope for MCP install smoke** | Distinct from runtime-backed MCP serve; stdio hosts do not need a factory HTTP server for either documented serve mode |
| Dashboard or website MCP session inspection | **Out of scope for this lane** | Proves stdio host setup only; no website assertions in install smoke |

### Automated In-Repo Proof

`pkg/cli/mcp/serve_smoke_test.go` exercises the documented fixture-backed install
path through `mcpcli.RunServe` with injected `ServeConfig.Service` and piped
stdio (an equivalent MCP-client harness, not a host-specific config file):

| Step | MCP method / tool | Behavior proved |
|------|-------------------|-----------------|
| Handshake | `initialize` | Protocol version `2024-11-05` |
| Discovery | `tools/list` | Canonical tools present: `you.factory_session.validate_source`, `you.factory_session.start_async`, `you.factory_session.get`, `you.factory_session.get_result` |
| Validate fixture | `tools/call` → `you.factory_session.validate_source` | Valid preview outcome for a temp workflow fixture |
| Async start | `tools/call` → `you.factory_session.start_async` | Fixture `req-js-run-n-001` returns `RUNNING` |
| Status poll | `tools/call` → `you.factory_session.get` | Running session status for the started session id |
| Result poll | `tools/call` → `you.factory_session.get_result` | Typed `factory_session.result.not_ready` envelope while the session is still running |

The same file also proves missing `--fixture-catalog` startup failure when no
service is injected.

`pkg/cli/mcp/serve_runtime_smoke_test.go` exercises runtime-backed serve through
`mcpcli.RunServe` with `ServeConfig.RuntimeBacked: true` and no injected service
so the smoke path uses the same wiring as `you mcp serve --runtime`:

| Step | MCP method / tool | Behavior proved |
|------|-------------------|-----------------|
| Handshake | `initialize` | Protocol version `2024-11-05` |
| Discovery | `tools/list` | Canonical Factory Session tools present |
| Async start | `tools/call` → `you.factory_session.start_async` | INLINE_WORKFLOW async session start through the shared runtime service |
| Status poll | `tools/call` → `you.factory_session.get` | Non-terminal `RUNNING` or terminal `SUCCEEDED` status for the started session id |
| Result poll | `tools/call` → `you.factory_session.get_result` | Typed `factory_session.result.not_ready` while running or terminal result after completion |

This automation proves the shared MCP server and tool invocation surface that
every host example above depends on. Fixture-backed smoke does **not** depend on
runtime mode. Runtime-backed smoke does **not** prove host UI discovery, host
config file parsing, dashboard inspection, or live factory HTTP runtime
execution.

`pkg/cli/mcp/serve_runtime_resume_smoke_test.go` exercises runtime-backed
interrupted-to-resumed continuity on the same stdio server path:

| Step | MCP method / tool | Behavior proved |
|------|-------------------|-----------------|
| Handshake | `initialize` | Protocol version `2024-11-05` |
| Discovery | `tools/list` | Canonical resume/control tools present: `you.factory_session.control`, `you.factory_session.list_dispatches` |
| Async start | `tools/call` → `you.factory_session.start_async` | WORKFLOW_NAME resumable JavaScript session start |
| Interrupt | `tools/call` → `you.factory_session.control` | Accepted interrupt-dispatch on in-flight child work |
| Resume | `tools/call` → `you.factory_session.control` | Accepted resume on the same durable session id |
| Status/result | `you.factory_session.get`, `you.factory_session.get_result` | Resumed lifecycle timestamps and terminal result continuity |
| Dispatch continuity | `you.factory_session.list_dispatches` | Completed child dispatches preserved without replay |
| Invalid resume | `you.factory_session.control` | Typed `TERMINAL_SESSION` and `NO_OP` outcomes in result envelopes |

`pkg/cli/mcp/serve_runtime_resume_non_regression_test.go` keeps the resume lane
additive: fixture-backed install smoke, runtime-backed async serve, canonical
`you.factory_session.*` resume inspection, and shared vocabulary guardrails
remain stable after the resume smoke additions.

### Manual Host Smoke Sequence

For each **documented manual** host row above, run this exact sequence after
saving the host configuration from the matching section earlier in this guide:

1. Start or respawn the MCP server through the host configuration (host must
   launch `you mcp serve` with the documented `command`, `args`, and `cwd`).
2. Discover tools in the host UI or tool catalog and confirm the
   `you.factory_session.*` catalog is present (compatibility `you.workflow.*`
   aliases may also appear).
3. Call `you.factory_session.validate_source` against a known fixture workflow
   source in the configured project root.
4. Call `you.factory_session.start_async` for one simple fixture session (for
   example factory id `customer-support-triage` with request id
   `req-js-run-n-001` when using the default fixture catalog).
5. Poll with `you.factory_session.get` until the session is observable, then
   call `you.factory_session.get_result` and confirm either a not-ready envelope
   while running or a terminal result after completion.

Record manual pass/fail per host in release notes or QA checklists outside this
repository when host-specific behavior is under review.

## Shared MCP Install Blockers

Install smoke for this batch found **no bounded follow-up blocker** on the shared
MCP server startup or tool invocation path.

Evidence:

| Shared MCP step | Harness | Outcome |
|-----------------|---------|---------|
| Stdio server startup (`you mcp serve`) | `pkg/cli/mcp/serve_smoke_test.go` via `RunServe` with injected fixture service | Succeeds; missing `--fixture-catalog` failure is expected and documented when no service is injected |
| JSON-RPC handshake | Same harness | `initialize` returns protocol version `2024-11-05` |
| Tool discovery | Same harness | `tools/list` exposes canonical `you.factory_session.*` tools |
| Fixture validation | Same harness | `you.factory_session.validate_source` returns a valid preview outcome |
| Async start | Same harness | `you.factory_session.start_async` returns `RUNNING` for fixture `req-js-run-n-001` |
| Status / result polling | Same harness | `you.factory_session.get` returns running session status; `you.factory_session.get_result` returns typed `factory_session.result.not_ready` while running |

These steps exercise the shared stdio MCP path that every host example in this
guide depends on. They do not require a host-specific wrapper, a new
workflow-run surface, or live factory HTTP runtime backing.

The following gaps are **not** shared MCP blockers for this batch:

| Gap | Why it is out of scope here |
|-----|----------------------------|
| Host UI config reload and tool discovery (Cursor, Codex, OpenCode, Kiro, Gemini) | Host-only wrapper behavior; shared server/tool invocation is automated above |
| HTTP or SSE MCP transport | Unsupported transport choice for `you mcp serve` in this batch, not a startup failure on the documented stdio path |
| Live factory HTTP runtime backing for MCP hosts | Distinct from runtime-backed MCP serve; neither documented serve mode requires a factory HTTP server |
| Dashboard or website MCP session inspection | Website and dashboard surfaces are outside this stdio install lane |

No additional shared MCP startup, registration, or invocation follow-up batch is
required before customers can configure hosts against the current `you mcp serve`
command and Factory Session tool catalog documented above.

## Related

- `you docs orchestrators`
- `you docs sessions`
- `you mcp serve --help`
