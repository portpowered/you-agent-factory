---
author: Agent Factory Team
last-modified: 2026-08-03
doc-id: agent-factory/guides/serve-acp
---

# Host You As An ACP Agent

Use this guide to launch `you serve acp`, the exact child-process command an
Agent Client Protocol (ACP) client uses to host You as an ACP agent. `you docs
serve-acp` is the packaged topic for this surface.

## Start The Stdio Server

An ACP client must launch `you` as a child process:

```bash
you serve acp
```

The command hosts the same production ACP server the process composes through
`root.BuildProcess` -- it constructs no second service graph, Chat Sessions
service, Factory Sessions service, or transport graph, and it does not start
the HTTP or dashboard server.

Stdin and stdout carry newline-delimited ACP JSON-RPC only. Concise, sanitized
diagnostics and startup or serve failures use stderr; stdout never receives a
banner, progress text, help output, or error text. No prompt, credential,
provider command, unsafe path, or private topology detail is written to
either stream.

## Shutdown Behavior

| Trigger | Outcome |
|---------|---------|
| Clean stdin EOF | The command exits successfully after in-flight protocol work completes. |
| Process-context cancellation | New protocol work stops being admitted, transport- and runtime-owned work is joined or unwound, and the command returns the documented cancellation outcome. |
| Startup or serve failure | A sanitized diagnostic is written to stderr and the command returns the documented non-success outcome; no Factory or provider runtime work is left active. |

## Configure A Minimal ACP Client

Configure these two host fields explicitly:

| Field | Value |
|-------|-------|
| Executable | `you` on the host `PATH`, or an absolute path to the installed binary |
| Arguments | `serve`, `acp` |

A generic client configuration:

```json
{
  "command": "/absolute/path/to/you",
  "args": ["serve", "acp"]
}
```

## Prove A Pinned Headless Client Launch

The repository also keeps a non-editor interoperability check for the pinned
headless [`acpx@0.13.0`](https://www.npmjs.com/package/acpx/v/0.13.0) client.
It builds the current checkout before the client starts it; it never selects a
globally installed `you` or `acpx` executable.

```bash
INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1 go test ./tests/functional/transport/acp/realclient/... -run TestPinnedAcpxCreatesDefaultFactoryBuilderSession
```

It requires `npm`/`npx` and Node.js 22.13.0 or later, the runtime declared by
the pinned acpx package. The default functional suite deliberately leaves this
networked, process-boundary proof disabled; the repository CI enables it in its
functional lane.

The check runs the effective client command in this shape, with a fresh
temporary home, project directory, npm cache, and server binary on every run:

```bash
(cd <disposable-project> && npx --yes --package acpx@0.13.0 acpx --format json you-real-client sessions new)
```

Its disposable `<disposable-project>/.acpxrc.json` uses the portable custom
agent form required by acpx on Windows and supported on every host:

```json
{
  "agents": {
    "you-real-client": {
      "argv": ["<disposable-you-binary>", "serve", "acp"]
    }
  }
}
```

The machine-readable `session_ensured` fact must include client and ACP session
identities. The disposable acpx session record must show a negotiated protocol
version and `target` selection of `factory:@you/factory-builder`. The test then
uses `sessions close` and removes every scenario-owned client record, cache,
and process. Failures report a bounded launch phase only; they do not print ACP
frames, configuration contents, environment values, or host paths.

## Exchange One ACP Prompt

After the client starts the child process, it exchanges ACP JSON-RPC over
stdin and stdout. A representative exchange:

```jsonc
// -> stdin: negotiate the protocol
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":true}}}

// -> stdin: open a session against the operator's configured Factory target
{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/absolute/path/to/project","mcpServers":[]}}

// -> stdin: send one ordinary text prompt using the returned sessionId
{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"<sessionId from session/new>","prompt":[{"type":"text","text":"Summarize the changelog."}]}}

// <- stdout: a session/update notification carries the agent's final text before the session/prompt response
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"<sessionId>","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"..."}}}}

// <- stdout: the terminal session/prompt response
{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}
```

The client observes this implemented V1 contract: `initialize`, `session/new`
with the operator's configured Factory target, and exactly one ordinary text
prompt ending in one terminal `session/prompt` response. Events streaming,
attachment cursors, `session/load`, `session/resume`, `session/close`, control
fan-out, L4 Worker Events, persistence, remote ACP, and Factory Builder work
are not implemented in this V1 slice and are not advertised.

## Distinguish This From Related ACP And MCP Surfaces

`you serve acp` is the only command that hosts You as an ACP agent for an
external client. It is a distinct direction from these related commands:

- `you workers acp add` and `you workers acp delete` manage You's own use of
  external ACP-speaking provider agents -- the opposite integration
  direction. See [Providers and ACP agents](providers.md).
- `you mcp serve` hosts the unrelated Model Context Protocol tool server. See
  [MCP host setup](mcp.md).

Neither command is an alias for `you serve acp`, and `you acp serve` is not a recognized command.

## Know What Is Proven

The repository automates the shared server behavior every ACP client depends
on:

| Check | Automated proof |
|-------|-----------------|
| Discovery, help, and command-tree contracts for `you serve` and `you serve acp` | `pkg/transports/cli/root_serve_test.go` |
| Exact stdin/stdout/context forwarding, clean EOF, cancellation, and sanitized stderr-only failure diagnostics | `pkg/transports/cli/root_serve_test.go` |
| One real Factory prompt through `root.BuildProcess`: `initialize` -> `session/new` -> `session/prompt`, with protocol-only stdout | `tests/functional/cli/serve_acp/serve_acp_prompt_test.go` |

These tests prove the stdio protocol contract and command wiring, not a
specific ACP client's UI or configuration parser.

Run the shared automated checks from the repository root:

```bash
go test ./pkg/transports/cli/... -run TestServeACP
go test ./tests/functional/cli/serve_acp/...
```

## Related Topics

- `you docs providers` -- ACP presets, custom integrations, and provider
  lifecycle for You's own outbound ACP usage
- `you docs mcp` -- the unrelated `you mcp serve` MCP host setup
- `you docs sessions` -- inspect live Factory Sessions from the CLI
