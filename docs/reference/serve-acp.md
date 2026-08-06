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

## Choose Which Factory Answers A Prompt

`session/new` returns one select configuration option, `target`, in the `model`
category. ACP clients render it wherever they render a model picker, so
selecting a Factory works the same way selecting a model does. Switching it
mid-session uses `session/set_config_option` with `configId: "target"`, or the
`/factory <target>` prompt command.

By default every installed Factory is selectable, and `factory:@you/factory-builder`
starts as the current target. On a home where `you serve acp` has run at least
once, that means all packaged `@you/*` Factories appear in the picker.

To restrict the list, author `workers.acp.agentProfile` in the operator config:

```json
{
  "workers": {
    "acp": {
      "agentProfile": {
        "defaultTarget": "factory:@you/goal",
        "allowedTargets": ["factory:@you/goal", "factory:@you/classify"]
      }
    }
  }
}
```

| `allowedTargets` | Meaning |
|------------------|---------|
| Omitted | Unrestricted. Every installed Factory is selectable. |
| A non-empty list | Only those targets are selectable, in the authored order. `defaultTarget` must be one of them. |
| `[]` | Rejected. Omit the property instead; an empty list is treated as a malformed restriction rather than as "no restriction". |

Only Factories that are actually installed appear. A packaged Factory that has
been listed but never materialized to disk is not selectable. Targets are
unversioned `factory:<ref>` references, and unrestricted mode offers scoped
names such as `@you/goal`; a locally authored Factory whose name cannot be
expressed as a `factory:` reference is skipped rather than offered and then
rejected.

## Read The ACP Wire Log

Every `you serve acp` connection records its JSON-RPC traffic, in both
directions, to a file. Recording is on by default so that a customer who hits a
problem already has the evidence, rather than needing to know to enable
something before reproducing it.

```
~/.you-agent-factory/acp-wire/YYYY/MM/DD/<time>-acp-wire-<connection>.log
```

One file per connection, mode `0600`, JSONL — one record per line:

| Field | Meaning |
|-------|---------|
| `v` | Record schema version. |
| `seq` | Gap-free order across both directions. |
| `t` | RFC3339 UTC timestamp. |
| `conn` | Connection identity. |
| `peer` | `client` or `agent` — who authored the frame. |
| `dir` | `in` or `out`, relative to `you`. |
| `stream` | `stdin`, `stdout`, or `stderr`. |
| `bytes` | Length of the original line. |
| `frame` | The JSON-RPC frame, recorded verbatim. |
| `text` | The raw line when it was not valid JSON. |
| `err` | Why a line was recorded as `text`, or that it was truncated. |

Frames are recorded exactly as they crossed the wire, never re-encoded, so key
order and unknown fields survive. A line the server rejects is recorded too,
with `err` explaining why — that frame is usually the one worth reading.

```bash
# every method seen, in order
jq -r 'select(.frame) | "\(.dir) \(.frame.method // "response")"' \
  ~/.you-agent-factory/acp-wire/*/*/*/*.log
```

> **The log contains full prompt and response content.** It is a transcript of
> the session, not a sanitized diagnostic. Treat it accordingly before sharing
> it, and be aware anything you send to the agent appears in it verbatim.

| Environment variable | Effect |
|----------------------|--------|
| `YOU_ACP_WIRE_LOG=off` | Disables recording entirely. |
| `YOU_ACP_WIRE_LOG_DIR=<path>` | Writes transcripts under `<path>` instead of the home directory. |

Files roll at 32 MB, keeping four compressed backups for seven days. A single
frame larger than 256 KB is recorded truncated and marked, so one oversized
frame cannot consume the whole budget. Recording never writes to stdout, and a
recorder that cannot open its file is reported on stderr and then ignored
rather than failing the connection.

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
INFINITE_YOU_RUN_ACPX_REAL_CLIENT=1 INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT=1 INFINITE_YOU_ACPX_EVIDENCE_OUTPUT=<sanitized-evidence.json> go test -v ./tests/functional/transport/acp/realclient/... -run '^TestPinnedAcpxCompletesDefaultFactoryBuilderPrompt$' -count=1
```

It requires `npm`/`npx` and Node.js 22.13.0 or later, the runtime declared by
the pinned acpx package. The default functional suite deliberately leaves this
networked, process-boundary proof disabled. CI runs it separately from the
short functional coverage command, without `-short`, and treats a missing Node
or acpx prerequisite as a failure. The required CI step retains one sanitized
JSON artifact containing only the revision, acpx version, initialization,
session, target, result, provider-count, cleanup, and terminal-outcome facts.

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
version and `target` selection of `factory:@you/factory-builder`.

The same scenario invokes acpx's `prompt` command exactly once with ephemeral
text. Its disposable provider command follows the supported Codex subprocess
protocol, records only the provider identity, and is selected through the
scenario's scoped operator default. The child process receives a narrow
allowlist of runtime variables plus scenario-owned home, cache, temporary, and
provider values; it does not inherit credentials, proxy settings, or other
developer environment. The proof asserts a non-empty assistant result fact,
one `end_turn` terminal result, and exactly one `codex` provider invocation; it
does not assert, save, or print prompt text, assistant text, JSON-RPC frames,
provider arguments, environment values, or host paths. Cleanup owns the
complete process tree through a dedicated process group on Unix and a retained
kill-on-close Job Object on Windows (`taskkill /T` is only a fallback when job
ownership cannot be established). Timeout and non-zero-parent failure-path
tests verify the recorded scenario descendant exits. The test then uses `sessions close`, observes that
the disposable acpx queue owner has stopped, and removes every scenario-owned
client record, cache, and process. Failures report a bounded phase only.

## Exchange One ACP Prompt

After the client starts the child process, the check observes the implemented
V1 contract through acpx's JSON output: `initialize`, `session/new` with the
operator's configured Factory target, one ordinary text prompt, a non-empty
assistant result `session/update`, and one terminal `session/prompt` response
whose `stopReason` is the shipped `end_turn` reason. It retains no transcript
or payloads. Events
streaming, attachment cursors, `session/load`, `session/resume`, `session/close`,
control fan-out, L4 Worker Events, persistence, and remote ACP remain outside
this V1 slice. The check proves the prompt/result transport path, not the
semantic correctness of a Factory Builder authoring outcome.

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
| Pinned independent-client startup and one sanitized default-Factory prompt with one deterministic provider invocation | `tests/functional/transport/acp/realclient/pinned_acpx_test.go` |

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
