---
author: Agent Factory Team
last-modified: 2026-06-15
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

## Related

- `you docs orchestrators`
- `you docs sessions`
- `you mcp serve --help`
