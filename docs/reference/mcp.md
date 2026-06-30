---
author: Agent Factory Team
last-modified: 2026-06-17
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

The default serve path also exposes async Factory Session tools such as
`you.factory_session.start_async`, `you.factory_session.get`, and
`you.factory_session.get_result` through the fixture-backed service documented
in `you docs mcp-hosts`. See that guide for the full catalog, host examples,
and automated install smoke matrix.

### Runtime-backed Factory Session tools

Use `you mcp serve --runtime` when the host should exercise live durable
JavaScript Factory Session execution through the shared runtime service instead
of the deterministic fixture catalog. Runtime mode keeps the same
`you.factory_session.*` tool catalog and Factory Session vocabulary; only the
backing execution service changes.

| Mode | Command | Backing service | When to use |
|------|---------|-----------------|-------------|
| Fixture-backed (default) | `you mcp serve` | Durable session fixture catalog | Host install smoke, offline deterministic scenarios, and fixture-driven async polling |
| Runtime-backed | `you mcp serve --runtime` | Shared durable JavaScript runtime service | Live workflow execution, real INLINE_WORKFLOW sources, and terminal result reads against the runtime path |

Runtime mode accepts optional `--project-root` when workflow sources should
resolve from a directory other than the MCP host working directory. Do not
combine `--runtime` with `--fixture-catalog`.

## Automation-Backed In Repo

The repository already proves the following without manual host UI smoke:

| Check | Where it lives |
|-------|----------------|
| CLI registration for `you mcp serve` | `pkg/cli/root.go` and `pkg/cli/mcp/serve.go` |
| Factory Session MCP catalog | `pkg/mcp/factorysession/registry.go` (`DiscoverTools`) |
| Shared fixture-backed stdio install smoke for validate plus async polling | `pkg/cli/mcp/serve_smoke_test.go` |
| Runtime-backed stdio install smoke for async start, status, and result | `pkg/cli/mcp/serve_runtime_smoke_test.go` |
| Runtime-backed stdio resume smoke for interrupted-to-resumed continuity | `pkg/cli/mcp/serve_runtime_resume_smoke_test.go` |
| Additive non-resume MCP serve regression after resume smoke | `pkg/cli/mcp/serve_runtime_resume_non_regression_test.go` |
| Packaged recovery scope and serve-mode boundaries | `tests/functional/smoke/cli_docs_smoke_test.go` |

Run focused verification locally:

```bash
go test ./pkg/cli/mcp/... ./pkg/mcp/...
go test ./tests/functional/smoke -run 'TestDocsCommandSmoke|TestRunServe_InstallSmoke|TestRunServe_RuntimeSmoke'
```

## Serve Mode Scope Boundaries

This lane proves live runtime-backed stdio MCP serve only. It does not widen
into website inspection, HTTP/SSE transport, or a broader host-matrix expansion.

| Behavior | Status |
|----------|--------|
| Default fixture-backed `you mcp serve` install smoke | **Automated in-repo** via `serve_smoke_test.go` |
| Runtime-backed `you mcp serve --runtime` async start/status/result smoke | **Automated in-repo** via `serve_runtime_smoke_test.go` |
| Runtime-backed interrupted-to-resumed MCP resume smoke | **Automated in-repo** via `serve_runtime_resume_smoke_test.go` |
| Additive fixture/runtime MCP serve regression after resume smoke | **Automated in-repo** via `serve_runtime_resume_non_regression_test.go` |
| Multi-host parity matrices across every MCP client UI | Out of scope for this lane |
| HTTP or SSE MCP transport | Unsupported; `you mcp serve` is stdio-only |
| Dashboard or website inspection of MCP sessions | Out of scope for this lane |
| Live factory HTTP runtime backing | Distinct from runtime-backed MCP serve; not required for stdio host setup |

The fixture-backed and runtime-backed serve paths are additive. Existing
fixture-backed smoke continues to prove the default install path without
depending on runtime mode.

## Related Topics

- `you docs orchestrators` — Factory Session, Dispatch, FactoryArtifact, and dynamic workflow aliases
- `you docs sessions` — live Factory Session discovery and CLI inspection
- `you docs config` — `factory.json` topology and portability
