---
author: Agent Factory Team
last-modified: 2026-06-15
doc-id: agent-factory/guides/mcp
---

# MCP Install Path For Factory Preview Tools

Use this guide when you need the recovery-lane preview install scope boundary
for `you mcp serve`. For the current full host setup guide, practical host
examples, and automated install smoke for validate plus async Factory Session
tools, start with `you docs mcp-hosts`.

The serve command exposes Factory preview validation for JavaScript orchestrator
sources before Factory Session execution. The default stdio server also registers
the broader Factory Session MCP catalog through the mock-backed fixture service
documented in `you docs mcp-hosts`.

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
you want `you.factory_session.validate_source` to resolve.

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

Preview validation uses the canonical `FactoryPreviewRequest` /
`FactoryPreviewResult` contract (`POST /factories/preview` vocabulary) to
validate JavaScript orchestrator source and policy before Factory Session
execution.

### Preview tools exposed by `you mcp serve`

| MCP tool | Purpose |
|----------|---------|
| `you.factory_session.validate_source` | Validate JavaScript orchestrator factory source through the canonical Factory preview contract |

### Fixture-backed Factory Session tools

The default serve path also exposes async Factory Session tools through the
fixture-backed service documented in `you docs mcp-hosts`. See that guide for
the full catalog, host examples, and automated install smoke matrix.

### Out of scope for live-runtime install smoke
Live factory HTTP runtime backing for `you mcp serve` remains out of scope for
install smoke. Hosts can prove async Factory Session tools today through the
default mock-backed fixture catalog, but not yet through a live runtime-backed
serve configuration.

## Automation-Backed In Repo

The repository already proves the following without manual host UI smoke:

| Check | Where it lives |
|-------|----------------|
| CLI registration for `you mcp serve` | `pkg/cli/root.go` and `pkg/cli/mcp/serve.go` |
| Factory Session MCP catalog | `pkg/mcp/factorysession/registry.go` (`DiscoverTools`) |
| Shared stdio install smoke for validate plus async polling | `pkg/cli/mcp/serve_smoke_test.go` |
| Packaged recovery scope and follow-up markers | `tests/functional/smoke/cli_docs_smoke_test.go` |

Run focused verification locally:

```bash
go test ./pkg/cli/mcp/... ./pkg/mcp/...
go test ./tests/functional/smoke -run 'TestDocsCommandSmoke|TestRunServe_InstallSmoke'
```

## Follow-Up Cell For Async Install Smoke

Recovery lane `dynamic-workflows-recovery-mcp-install-plan-scope` completes
with preview-only scope documentation plus the shared fixture-backed install
smoke in `pkg/cli/mcp/serve_smoke_test.go`. Live runtime-backed MCP install
smoke remains blocked by one explicit follow-up cell:

**Cell:** `dynamic-workflows-cell-mcp-session-serve`

**Missing shared MCP surface:** `you mcp serve` still defaults to the durable
session fixture catalog (`factorysessionexecution.NewFakeServiceFromContractFixtures`).
Factory Session execution tools are available through that mock-backed serve
path, but a live runtime-backed serve configuration
(`factorysessionexecution.RuntimeService`) is not yet selectable from the
documented install path.

**Blocked install behavior until that cell lands:**

| Blocked behavior | Tool | Why hosts cannot prove it today |
|------------------|------|----------------------------------|
| Live-runtime async Factory Session start | `you.factory_session.start_async` | Serve path has no runtime-backed service mode |
| Live-runtime Factory Session status polling | `you.factory_session.get` | Serve path has no runtime-backed service mode |
| Live-runtime Factory Session result retrieval | `you.factory_session.get_result` | Serve path has no runtime-backed service mode |

The full follow-up scope, non-goals, and evidence table live in
`docs/internal/development/plans/dynamic-workflows/follow-up-cell-mcp-session-serve.md`.

No additional follow-up blocker remains for preview-only discovery and
validate install smoke covered by this doc.

## Out Of Scope For Any Follow-Up Cell Named Here

| Behavior | Status |
|----------|--------|
| Multi-host parity matrices across every MCP client UI | Out of scope for the recovery lane and the session-serve follow-up cell |

## Related Topics

- `you docs orchestrators` — Factory Session, Dispatch, FactoryArtifact, and dynamic workflow aliases
- `you docs sessions` — live Factory Session discovery and CLI inspection
- `you docs config` — `factory.json` topology and portability
