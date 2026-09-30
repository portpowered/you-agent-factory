---
author: Agent Factory Team
last-modified: 2026-07-13
doc-id: agent-factory/guides/mcp
---

# MCP Host Setup

Use this guide to install `you` in an MCP host, run a subagent, inspect
Factory Sessions, and discover operator configuration and providers.
`you docs mcp` is the packaged MCP setup topic.

## Start The Stdio Server

An MCP host must launch `you` as a child process:

```bash
you server mcp
```

The server speaks MCP JSON-RPC over stdin and stdout. Keep stdout reserved for
protocol messages; process diagnostics use stderr. HTTP and SSE MCP transports
are not supported, and the stdio server does not connect to a live Factory HTTP
server.

Configure these three host fields explicitly:

| Field | Value |
|-------|-------|
| Executable | `you` on the host `PATH`, or an absolute path to the installed binary |
| Arguments | `server`, `mcp` |
| Working directory | Absolute project root used to find workflow sources |

No extra environment variables are required. A generic host configuration is:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["server", "mcp"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

For Cursor, save that object in project `.cursor/mcp.json` or global
`~/.cursor/mcp.json`, then reload the window. Other stdio MCP hosts use the
same executable, arguments, and working-directory contract. Restart or reload
the host after changing it so the host respawns the child process.

For OpenCode, add the local server with its installed CLI:

```text
opencode mcp add you-agent-factory --global -- /absolute/path/to/you server mcp
opencode mcp list
```

The list must show `you-agent-factory` connected. Set the OpenCode workspace to
the project whose files the subagent should inspect. OpenCode starts the MCP
child from that workspace.

## Run A Subagent

Call `you.subagent` with a short `prompt`:

```json
{"prompt":"Summarize the purpose of this repository in one sentence."}
```

The tool runs one bounded `@you/subagent` Factory invocation and returns its
answer as `result.text`. Optional `provider`, `model`, and `reasoningEffort`
fields select the worker route for that call. Omit them to use the operator's
configured provider and model and the provider's default reasoning effort.
The subagent can inspect and edit its workspace with agent tools. Workspace
editing is enabled by default and requires no edit flag. Set
`workingRoot` to an absolute project directory when the MCP server's working
directory is not the project being edited; otherwise the server's working
directory is used. Each call opens and closes its own Factory Session.

If no provider default is configured, run `you init --provider opencode`,
`you init --provider codex`, or `you init --provider pi`; you can also supply
`provider` in the tool call. Use these public provider IDs directly. Factory
Session tools use the same process-owned Sessions service.

### Troubleshoot a subagent timeout

`timeoutMillis` sets the maximum wait for a `you.subagent` result in
milliseconds. A `factory_session.subagent.timed_out` error includes the Factory
Session ID. It includes `requestId`, `traceId`, and `workId` when available,
plus any explicitly selected `provider` and `model` and a `suggestedAction`.
The call requests cancellation and closes its live Factory Session on timeout.

Typed tool and domain errors return a readable message in the first MCP content
text with `isError=true`; `structuredContent` retains the machine-readable
`code`, `retryable`, `sessionId`, and `details` fields.

If `timeoutMillis` is omitted, `you.subagent` waits up to 20 minutes (1200000
milliseconds). A shorter caller deadline or MCP host tool wait can end the
operation sooner. For substantial coding edits, set the MCP host tool wait
longer than `timeoutMillis` plus cleanup time. Short probes can use a shorter
`timeoutMillis`.

Workspace edits may have occurred. Inspect the workspace before retrying.
After inspection, choose another configured model or a longer timeout.

## Discover Configuration And Providers

The server publishes the `subagent-configuration` skill through the MCP Skills
extension. A supporting host can call `skills/list` and `skills/get`, then
read `skill://subagent-configuration/SKILL.md` with `resources/read`. The skill
explains provider names, operator defaults, and custom ACP setup.

Use `resources/list` to discover these resources:

| Resource URI | Content |
|--------------|---------|
| `skill://subagent-configuration/SKILL.md` | Subagent configuration instructions |
| `you://operator/config/schema` | JSON Schema derived from the operator OpenAPI contract |
| `you://operator/config/current` | Current operator configuration file, or `{}` when absent |
| `you://providers/catalog` | Current provider names, models, reasoning efforts, and readiness |

The current configuration is read on each request. Its file is
`~/.you-agent-factory/config.json` on macOS and Linux, or
`%USERPROFILE%\.you-agent-factory\config.json` on Windows. Keep sensitive
values from that file out of shared transcripts. The skill describes how to
modify the file or use the `you` CLI to add a custom provider.

The published MCP manifest in `@you-agent-factory/api/mcp` defines the tools,
resources, and skill available from the server.

## Choose A Project Root

Workflow sources resolve from `cwd`. To use a different source root, add
`--project-root`:

```json
{
  "command": "/absolute/path/to/you",
  "args": ["server", "mcp", "--project-root", "/absolute/path/to/project"],
  "cwd": "/absolute/path/to/project"
}
```

## Use Canonical Factory Session Tools

Tool discovery exposes `you.subagent` and this Factory Session catalog:

| Tool | Task |
|------|------|
| `you.factory_session.list` | List live Factory Sessions by default; use `scope: "persisted"` for durable sessions or `scope: "all"` for both |
| `you.factory_session.validate_source` | Validate JavaScript orchestrator source without execution |
| `you.factory_session.start_sync` | Start a Factory Session and wait for a terminal or timeout result |
| `you.factory_session.start_async` | Start a Factory Session for later polling |
| `you.factory_session.get` | Read status and progress for one durable Factory Session |
| `you.factory_session.get_result` | Read a partial, terminal, or not-ready result |
| `you.factory_session.list_dispatches` | Inspect child dispatches |
| `you.factory_session.list_artifacts` | Inspect durable artifact metadata |
| `you.factory_session.read_events` | Read ordered Factory Session events |
| `you.factory_session.control` | Pause, resume, cancel, or terminate a Factory Session |

`you.subagent` closes its live Factory Session before returning. Its response
includes the session ID and terminal result. A live ID seen in
`you.factory_session.list` during execution is not readable through the
durable-only `you.factory_session.get` tool.

Source validation uses either the host working directory or an explicit
`projectRoot`. After starting, preserve the caller-supplied `requestId`, the
returned `sessionId`, and the last processed event id or session sequence.
Status, dispatch, artifact, event, control, and result calls must keep using
that same Factory Session id; reconnecting is not a reason to submit duplicate
Work.

## Run The First-Host Smoke

After saving the host configuration:

1. Reload the host and confirm it starts `you server mcp` as a child process.
2. Discover tools and confirm the canonical `you.factory_session.*` catalog,
   the four resources above, and the `subagent-configuration` skill.
3. Call `you.factory_session.validate_source` for a known source under the
   configured project root.
4. Call `you.factory_session.start_async` with a unique `requestId` and a
   source supported by the selected mode.
5. Keep the returned `sessionId`; poll `you.factory_session.get`, then
   `you.factory_session.get_result` until it is terminal.
6. When the workflow creates child Work, inspect dispatches, artifacts, and
   ordered events using the same `sessionId`.

For an asynchronous workflow, a not-ready result while its status is running
is expected. Poll the original Factory Session id.

## Know What Is Proven

The repository automates the shared server behavior that every host depends on:

| Check | Automated proof |
|-------|-----------------|
| Initialize, discovery, validate, async start, status, and not-ready result | `pkg/transports/cli/mcp/serve_smoke_test.go` |
| Async start, status, and result | `pkg/transports/cli/mcp/serve_runtime_smoke_test.go` |
| Resume and dispatch continuity | `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` |

These tests prove the stdio protocol and Factory Session tool behavior, not a
specific host UI or configuration parser. Manually confirm that the selected
host reloads its configuration, spawns the child, discovers the tools, and can
complete the first-host smoke. They also do not prove HTTP/SSE transport,
dashboard inspection, or live Factory HTTP backing.

Run the shared automated checks from the repository root:

```bash
go test ./pkg/transports/cli/mcp/... ./pkg/transports/mcp/...
go test ./tests/functional/smoke -run TestDocsCommandSmoke
```

## Troubleshoot Setup And Calls

| Symptom or outcome | Action |
|--------------------|--------|
| Host cannot start `you` | Use an absolute executable path, confirm it is executable, and keep `server` and `mcp` as separate arguments. |
| No tools or resources appear | Reload the host, inspect child-process stderr, and confirm stdout is not receiving logs or shell banners. |
| Provider is unavailable | Read `you://providers/catalog` for identity and readiness, then check its installation and authentication. |
| Named workflow or source is not found | Set `cwd` to the project root or pass an explicit `--project-root`; confirm the source exists under a supported source location. |
| `factory_session.result.not_ready` with `retryable: true` | Keep the same `sessionId` and poll status/result with backoff; do not start duplicate Work. |
| `factory_session.session.not_found` | Stop polling the bad id and restore the exact `sessionId` returned by start; reconnecting does not create a replacement session. |
| Event reconnect cursor is not found | Keep the same Factory Session, restore a known event id or sequence, and do not assume missed events were processed. |
| `factory_session.start.request_id_conflict` | Reuse a request id only with its original source and arguments; use a new id for a genuinely different request. |
| `factory_session.service.unavailable` | Restart the MCP child process, then retry the same safe read or idempotent start tuple. |
| Host expects an HTTP URL | Configure a stdio child process instead; HTTP and SSE are unsupported. |

## Related Topics

- `you docs javascript-workflows` — author, validate, execute, and inspect
  JavaScript workflows
- `you docs orchestrators` — Factory Session, dispatch, artifact, and event
  vocabulary
- `you docs sessions` — inspect live Factory Sessions from the CLI
