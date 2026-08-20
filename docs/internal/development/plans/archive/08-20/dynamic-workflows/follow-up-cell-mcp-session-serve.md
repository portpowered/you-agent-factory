# Follow-Up Cell: `dynamic-workflows-cell-mcp-session-serve`

## Status

**Completed.** Runtime-backed MCP serve is selectable through `you mcp serve
--runtime`, documented in `you docs mcp` and `you docs mcp-hosts`, and proven by
focused stdio smoke in `pkg/transports/cli/mcp/serve_runtime_smoke_test.go`. The default
fixture-backed install path remains unchanged in `pkg/transports/cli/mcp/serve_smoke_test.go`.

## Why This Cell Existed

The recovery lane `dynamic-workflows-recovery-mcp-install-plan-scope` documented
preview-only install scope and the shared stdio serve entrypoint `you mcp serve`.
Install smoke for async Factory Session execution, status polling, and result
retrieval passed through the default mock-backed fixture catalog in
`pkg/transports/cli/mcp/serve_smoke_test.go`, but live runtime-backed MCP install smoke
required a documented shared serve configuration.

## Delivered Surface

| Artifact | Purpose |
|----------|---------|
| `pkg/transports/cli/mcp/serve.go` | `--runtime` and `--project-root` flags; runtime service wiring |
| `docs/reference/mcp-hosts.md` | Fixture vs runtime serve mode selection and smoke matrix |
| `docs/reference/mcp.md` | Serve mode boundaries and automation pointers |
| `pkg/transports/cli/mcp/serve_smoke_test.go` | Fixture-backed install smoke (unchanged default path) |
| `pkg/transports/cli/mcp/serve_runtime_smoke_test.go` | Runtime-backed async start/status/result smoke |
| `pkg/transports/mcp/factorysession/execution_test.go` | Runtime-backed tool handler proof |

## Non-Goals (unchanged)

- HTTP/SSE MCP transport
- Dashboard MCP inspection UI
- Generic MCP redesign outside the shared stdio serve path
- Broader host-matrix expansion across every MCP client UI
