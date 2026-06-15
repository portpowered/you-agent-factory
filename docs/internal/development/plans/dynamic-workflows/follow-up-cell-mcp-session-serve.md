# Follow-Up Cell: `dynamic-workflows-cell-mcp-session-serve`

## Why This Cell Exists

The recovery lane `dynamic-workflows-recovery-mcp-install-plan-scope` proved one
repo-owned MCP install path for preview-only Factory Session tools through
`you mcp serve`. Install smoke for async Factory Session execution, status
polling, and result retrieval still depends on a missing shared MCP serve
surface.

This note is the single bounded follow-up cell for that gap. It does not reopen
backend runtime recovery, API parity, dashboard inspection, or generic MCP
redesign.

## Missing Shared MCP Surface

`you mcp serve` currently registers only `pkg/mcp/factorysession.PreviewToolDefinitions()`:

| Served today | Not served on `you mcp serve` |
|--------------|-------------------------------|
| `you.factory_session.validate_source` | `you.factory_session.start_async` |
| `you.factory_session.start_preview` | `you.factory_session.start_sync` |
| | `you.factory_session.get` |
| | `you.factory_session.get_result` |

The broader Factory Session MCP contract already exists in
`pkg/mcp/factorysession/` (`DiscoverTools`, `NewClient`, `NewClientWithService`)
and is proven through mock-client tests, but those handlers are not registered
on the canonical stdio serve path in `pkg/mcp/server/server.go`.

## Blocked Install Behavior

Hosts configured from `you docs mcp` cannot yet prove these install behaviors
through the documented serve command:

1. **Async Factory Session start** through
   `you.factory_session.start_async` over the owned stdio transport.
2. **Factory Session status polling** through `you.factory_session.get` after
   an async start.
3. **Factory Session result retrieval** through `you.factory_session.get_result`
   while running or after terminal completion.

Preview-only install smoke in `tests/functional/smoke/cli_mcp_serve_smoke_test.go`
covers discovery plus validate/start-preview only. Async run or inspect install
smoke must wait for this cell.

## Smallest Executor Lane

One executor lane should:

1. Extend `you mcp serve` to register the Factory Session execution and
   inspection tools needed for async install smoke (`start_async`, `get`,
   `get_result` at minimum) while preserving Factory Session vocabulary.
2. Wire serve handlers through the existing `pkg/mcp/factorysession` client
   surface with an injectable `factorysessionexecution.Service` backing
   (fixture catalog or runtime service), matching the preview-only serve
   injection pattern.
3. Add focused install-path smoke that spawns `you mcp serve`, starts one async
   Factory Session, polls status, and reads a not-ready or terminal result
   through the same documented host configuration.
4. Update `you docs mcp` automation and follow-up tables so reviewers can
   tell preview-only install proof from async install proof.

## Explicit Non-Goals

- Multi-host parity matrices across every MCP client UI.
- New durable API handlers, dashboard inspection panels, or backend runtime
  recovery unrelated to MCP serve registration.
- A standalone workflow-run or dynamic-run public resource.
- Dispatch, artifact, lifecycle-control, or event-read install smoke unless
  required to unblock the three behaviors above.

## Evidence That The Blocker Remains

| Check | Where it lives | Current outcome |
|-------|----------------|-----------------|
| Serve catalog is preview-only | `pkg/mcp/server/server_test.go` | `tools/list` exposes only validate_source and start_preview |
| End-to-end install smoke is preview-only | `tests/functional/smoke/cli_mcp_serve_smoke_test.go` | Proves validate/start-preview, not async run/status/result |
| Session MCP handlers exist off-serve | `pkg/mcp/factorysession/*_test.go` | Mock-client coverage only; not reachable through `you mcp serve` |

## Related

- `you docs mcp` — current copyable preview-only install path
- `docs/internal/processes/api-relevant-files.md` — MCP serve and factorysession wiring map
