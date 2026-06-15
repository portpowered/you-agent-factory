---
author: Agent Factory Team
last-modified: 2026-06-15
doc-id: agent-factory/guides/mcp
---

# MCP Install Path For Factory Preview Tools

Use this guide when you need one copyable MCP host configuration that matches
the repo-owned `you mcp serve` stdio server. The serve path exposes Factory
preview validation for JavaScript orchestrator sources before Factory Session
execution.

For canonical `Factory`, `FactoryOrchestrator`, `FactorySession`, `Dispatch`,
`FactoryArtifact`, and `FactoryEvent` vocabulary, see `you docs orchestrators`.
Dynamic workflow wording in this guide is JavaScript orchestrator shorthand
only; it does not introduce a standalone workflow-run resource.

## Canonical Serve Command

The repository exposes one canonical MCP serve entrypoint:

```bash
you mcp serve
```

The command runs over stdio using newline-delimited JSON. MCP hosts should
spawn it as a child process and keep stdin/stdout dedicated to the MCP
transport. Do not run it as an interactive terminal command.

## Copyable Host Configuration

Point your MCP host at the installed `you` binary, pass `mcp serve` as the only
arguments, and set `cwd` to the JavaScript workflow project root whose sources
you want `you.factory_session.validate_source` and
`you.factory_session.start_preview` to resolve.

No additional environment variables are required for the current preview-only
serve path. The `you` executable must be on the host `PATH`, or you must use an
absolute executable path.

### Generic MCP host JSON

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/your-workflow-project"
    }
  }
}
```

### Absolute executable path

Use this shape when `you` is not on the host `PATH`:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["mcp", "serve"],
      "cwd": "/absolute/path/to/your-workflow-project"
    }
  }
}
```

| Field | Required value | Why |
|-------|----------------|-----|
| `command` | Installed `you` binary (`you` on `PATH` or an absolute path) | Canonical CLI entrypoint for the repo-owned MCP server |
| `args` | `["mcp", "serve"]` | Starts the preview-only stdio MCP server |
| `cwd` | Absolute path to the workflow project root | Resolves `WORKFLOW_NAME` and related JavaScript orchestrator sources the same way CLI preview does |
| `env` | Omit for the current serve path | Preview validation does not require factory-service or dashboard environment overrides |

Replace `/absolute/path/to/your-workflow-project` with the directory that
contains your workflow sources (for example a repository root with
`.claude/workflows/` or `factory/workflows/`). Tool calls still accept an
explicit `projectRoot` in the Factory preview request when the host needs a
different resolution root than `cwd`.

## What This Batch Covers

This recovery lane proves one installable MCP path for:

1. **Tool discovery** over the owned stdio transport.
2. **Factory preview validation** through `you.factory_session.validate_source`.
3. **Factory preview start checks** through `you.factory_session.start_preview`.

Both preview tools use the canonical `FactoryPreviewRequest` /
`FactoryPreviewResult` contract (`POST /factories/preview` vocabulary). They
validate JavaScript orchestrator source and policy before Factory Session
execution.

### Tools exposed by `you mcp serve`

| MCP tool | Purpose |
|----------|---------|
| `you.factory_session.validate_source` | Validate JavaScript orchestrator factory source through the canonical Factory preview contract |
| `you.factory_session.start_preview` | Run the start-preview step through the same Factory preview contract |

### Out of scope on this serve path

The current `you mcp serve` catalog is preview-only. It does **not** expose
async Factory Session execution, status polling, result retrieval, dispatch or
artifact listing, lifecycle controls, or durable session event reads.

Do not configure hosts expecting these tools until a later lane extends the
serve catalog:

| Not served today | Meaning |
|------------------|---------|
| `you.factory_session.start_async` | Async Factory Session start |
| `you.factory_session.start_sync` | Sync Factory Session start |
| `you.factory_session.get` | Durable Factory Session inspection |
| `you.factory_session.get_result` | Durable Factory Session result retrieval |
| `you.factory_session.list_dispatches` | Dispatch summaries for one Factory Session |
| `you.factory_session.list_artifacts` | FactoryArtifact summaries for one Factory Session |
| `you.factory_session.control` | Durable lifecycle controls |
| `you.factory_session.read_events` | Ordered Factory Session event reads |
| `you.factory_session.list` | Scoped Factory Session listing |

Those names exist in the broader MCP contract package for future work, but they
are not registered by the canonical serve path documented here.

## Automation-Backed In Repo

The repository already proves the following without manual host UI smoke:

| Check | Where it lives |
|-------|----------------|
| CLI registration for `you mcp serve` | `pkg/cli/root_mcp_test.go` |
| Preview tool catalog wiring | `pkg/mcp/workflow/registry_test.go` |
| Stdio discovery and `you.factory_session.validate_source` invocation | `pkg/mcp/server/server_test.go` |
| End-to-end smoke that spawns `you mcp serve` through the documented install path | `tests/functional/smoke/cli_mcp_serve_smoke_test.go` |

Run focused verification locally:

```bash
go test ./pkg/cli/... ./pkg/mcp/...
go test ./tests/functional/smoke -run TestMCPServe_RealCLI
```

## Follow-Up Work Outside This Doc

| Behavior | Status |
|----------|--------|
| Async run, status, or result install smoke through MCP | Deferred until the serve catalog exposes the needed Factory Session session tools |
| Multi-host parity matrices across every MCP client UI | Out of scope for this recovery lane |

## Related Topics

- `you docs orchestrators` — Factory Session, Dispatch, FactoryArtifact, and dynamic workflow aliases
- `you docs sessions` — live Factory Session discovery and CLI inspection
- `you docs config` — `factory.json` topology and portability
