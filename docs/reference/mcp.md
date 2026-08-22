---
author: Agent Factory Team
last-modified: 2026-07-13
doc-id: agent-factory/guides/mcp
---

# MCP Host Setup

Use this guide to install `you` in an MCP host, choose a backing mode, and
verify the Factory Session tools. `you docs mcp` is the only packaged MCP setup
topic.

## Start The Stdio Server

An MCP host must launch `you` as a child process:

```bash
you server mcp
```

The server speaks MCP JSON-RPC over stdin and stdout. Keep stdout reserved for
protocol messages; process diagnostics use stderr. HTTP and SSE MCP transports
are not supported, and neither documented mode connects to a live Factory HTTP
server.

Configure these three host fields explicitly:

| Field | Value |
|-------|-------|
| Executable | `you` on the host `PATH`, or an absolute path to the installed binary |
| Arguments | `server`, `mcp` |
| Working directory | Absolute project root used to find workflow sources in runtime mode |

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
`~/.cursor/mcp.json`, then reload the window. Codex, OpenCode, Kiro, Gemini, and
other stdio MCP hosts use the same executable, arguments, and working-directory
contract in their MCP settings. Restart or reload the host after changing it so
the host respawns the child process.

## Choose A Backing Mode

Both modes expose the same canonical `you.factory_session.*` tools. They differ
only in how Factory Sessions execute.

| Mode | Host arguments | Use it for |
|------|----------------|------------|
| Fixture-backed (default) | `["server", "mcp"]` | Deterministic install smoke and offline fixture scenarios |
| Runtime-backed | `["server", "mcp", "--runtime"]` | Live durable JavaScript workflow execution |

The equivalent runtime-backed child-process command is:

```bash
you server mcp --runtime
```

Fixture-backed mode uses the canonical durable Factory Session catalog embedded
in the `you` binary. It does not need a repository checkout, a project root,
or a working-directory lookup. To use a custom deterministic catalog, pass its
path explicitly:

```json
"args": ["server", "mcp", "--fixture-catalog", "/absolute/path/to/durable-session-contract-fixtures.json"]
```

Runtime-backed mode resolves workflow sources from `cwd`. To use a different
source root, add `--project-root`:

```json
{
  "command": "/absolute/path/to/you",
  "args": ["server", "mcp", "--runtime", "--project-root", "/absolute/path/to/project"],
  "cwd": "/absolute/path/to/project"
}
```

Do not combine `--runtime` with `--fixture-catalog`. Use runtime mode for real
`INLINE_WORKFLOW` or named-source execution; the default mode resolves only the
deterministic catalog scenarios.

## Use Canonical Factory Session Tools

Tool discovery exposes this primary catalog:

| Tool | Task |
|------|------|
| `you.factory_session.list` | List durable Factory Sessions |
| `you.factory_session.validate_source` | Validate JavaScript orchestrator source without execution |
| `you.factory_session.start_sync` | Start a Factory Session and wait for a terminal or timeout result |
| `you.factory_session.start_async` | Start a Factory Session for later polling |
| `you.factory_session.get` | Read status and progress for one Factory Session |
| `you.factory_session.get_result` | Read a partial, terminal, or not-ready result |
| `you.factory_session.list_dispatches` | Inspect child dispatches |
| `you.factory_session.list_artifacts` | Inspect durable artifact metadata |
| `you.factory_session.read_events` | Read ordered Factory Session events |
| `you.factory_session.control` | Pause, resume, cancel, or terminate a Factory Session |

Source validation uses either the host working directory or an explicit
`projectRoot`. After starting, preserve the caller-supplied `requestId`, the
returned `sessionId`, and the last processed event id or session sequence.
Status, dispatch, artifact, event, control, and result calls must keep using
that same Factory Session id; reconnecting is not a reason to submit duplicate
Work.

## Run The First-Host Smoke

After saving the host configuration:

1. Reload the host and confirm it starts `you server mcp` as a child process.
2. Discover tools and confirm the canonical `you.factory_session.*` catalog.
3. Call `you.factory_session.validate_source` for a known source under the
   configured project root.
4. Call `you.factory_session.start_async` with a unique `requestId` and a
   source supported by the selected mode.
5. Keep the returned `sessionId`; poll `you.factory_session.get`, then
   `you.factory_session.get_result` until it is terminal.
6. When the workflow creates child Work, inspect dispatches, artifacts, and
   ordered events using the same `sessionId`.

For the repository fixture catalog, workflow `release-train` with request id
`req-js-run-n-001` is the published asynchronous smoke scenario. A not-ready
result while its status is running is expected and proves that polling stays on
the original Factory Session.

## Know What Is Proven

The repository automates the shared server behavior that every host depends on:

| Check | Automated proof |
|-------|-----------------|
| Fixture-backed initialize, discovery, validate, async start, status, and not-ready result | `pkg/transports/cli/mcp/serve_smoke_test.go` |
| Runtime-backed async start, status, and result | `pkg/transports/cli/mcp/serve_runtime_smoke_test.go` |
| Runtime-backed resume and dispatch continuity | `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` |
| Additive fixture/runtime regression after resume | `pkg/transports/cli/mcp/serve_runtime_resume_non_regression_test.go` |

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
| Host cannot start `you` | Use an absolute executable path, confirm it is executable, and keep `args` as separate `server` and `mcp` values. |
| No tools appear | Reload the host, inspect child-process stderr, and confirm stdout is not receiving logs or shell banners. |
| `fixture catalog` override fails | Confirm the explicit `--fixture-catalog` path is readable and contains a valid deterministic catalog. The default embedded catalog does not require this flag. |
| Named workflow or source is not found | Set `cwd` to the project root or use runtime mode with an explicit `--project-root`; confirm the source exists under a supported source location. |
| `cannot combine --runtime with --fixture-catalog` | Choose exactly one backing mode and remove the other mode's flag. |
| `factory_session.result.not_ready` with `retryable: true` | Keep the same `sessionId` and poll status/result with backoff; do not start duplicate Work. |
| `factory_session.session.not_found` | Stop polling the bad id and restore the exact `sessionId` returned by start; reconnecting does not create a replacement session. |
| Event reconnect cursor is not found | Keep the same Factory Session, restore a known event id or sequence, and do not assume missed events were processed. |
| `factory_session.start.request_id_conflict` | Reuse a request id only with its original source and arguments; use a new id for a genuinely different request. |
| `factory_session.service.unavailable` | Restore or respawn the selected fixture/runtime service, then retry the same safe read or idempotent start tuple. |
| Host expects an HTTP URL | Configure a stdio child process instead; HTTP and SSE are unsupported. |

## Related Topics

- `you docs javascript-workflows` — author, validate, execute, and inspect
  JavaScript workflows
- `you docs orchestrators` — Factory Session, dispatch, artifact, and event
  vocabulary
- `you docs sessions` — inspect live Factory Sessions from the CLI
