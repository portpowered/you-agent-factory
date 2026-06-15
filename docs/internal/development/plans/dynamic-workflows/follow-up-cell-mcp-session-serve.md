# Follow-Up Cell: `dynamic-workflows-cell-mcp-session-serve`

## Why This Cell Exists

The recovery lane `dynamic-workflows-recovery-mcp-install-plan-scope` documented
preview-only install scope and the shared stdio serve entrypoint `you mcp serve`.
Install smoke for async Factory Session execution, status polling, and result
retrieval now passes through the default mock-backed fixture catalog in
`pkg/cli/mcp/serve_smoke_test.go`, but live runtime-backed MCP install smoke
still depends on a missing shared serve configuration.

This note is the single bounded follow-up cell for that gap. It does not reopen
backend runtime recovery, API parity, dashboard inspection, or generic MCP
redesign.

## Missing Shared MCP Surface

`you mcp serve` currently resolves its backing service to the durable session
fixture catalog (`factorysessionexecution.NewFakeServiceFromContractFixtures`).
The stdio server already registers the full Factory Session MCP catalog from
`pkg/mcp/factorysession.DiscoverTools()` through that client, but there is no
documented install path that selects `factorysessionexecution.RuntimeService`
for live JavaScript orchestrator execution.

| Served today through fixture-backed `you mcp serve` | Not yet selectable for install smoke |
|-----------------------------------------------------|--------------------------------------|
| `you.factory_session.validate_source` | Runtime-backed `you.factory_session.start_async` |
| `you.factory_session.start_preview` | Runtime-backed `you.factory_session.get` |
| `you.factory_session.start_async` | Runtime-backed `you.factory_session.get_result` |
| `you.factory_session.get` | |
| `you.factory_session.get_result` | |

## Blocked Install Behavior

Hosts configured from `you docs mcp-hosts` can prove async Factory Session tools
through the default fixture catalog, but they cannot yet prove these install
behaviors through a live runtime-backed serve configuration:

1. **Live-runtime async Factory Session start** through
   `you.factory_session.start_async` backed by `RuntimeService`.
2. **Live-runtime Factory Session status polling** through
   `you.factory_session.get` after a runtime-backed async start.
3. **Live-runtime Factory Session result retrieval** through
   `you.factory_session.get_result` while running or after terminal completion.

Fixture-backed install smoke in `pkg/cli/mcp/serve_smoke_test.go` covers
discovery, validate, async start, and polling. Live runtime install smoke must
wait for this cell.

## Smallest Executor Lane

One executor lane should:

1. Extend `you mcp serve` with one documented runtime-backed service mode that
   preserves Factory Session vocabulary and reuses the existing
   `pkg/mcp/factorysession` client surface.
2. Wire that mode through `factorysessionexecution.RuntimeService` without
   widening into API parity or dashboard inspection work.
3. Add focused install-path smoke that spawns `you mcp serve` in runtime mode,
   starts one async Factory Session, polls status, and reads a not-ready or
   terminal result through the same documented host configuration.

## Non-Goals

- HTTP/SSE MCP transport
- Dashboard MCP inspection UI
- Generic MCP redesign outside the shared stdio serve path
- Backend runtime recovery beyond selecting an existing runtime service for serve

## Evidence

| Artifact | Purpose |
|----------|---------|
| `docs/reference/mcp-hosts.md` | Current full host setup and fixture-backed smoke matrix |
| `docs/reference/mcp.md` | Recovery-lane preview scope boundary and follow-up pointer |
| `pkg/cli/mcp/serve_smoke_test.go` | Automated fixture-backed install smoke |
| `pkg/mcp/factorysession/registry.go` | `PreviewToolDefinitions()` preview-only catalog helper |
