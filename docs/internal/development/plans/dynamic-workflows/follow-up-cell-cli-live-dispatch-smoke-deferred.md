# Follow-Up Cells: CLI Live-Provider Dispatch Smoke Deferred Surfaces

## Why This Note Exists

The lane `dynamic-workflows-cell-cli-live-dispatch-smoke` added one focused CLI
smoke path that starts a durable JavaScript `FactorySession` through the shared
execution-service backend, re-reads stable status and terminal result, and proves
bridged live-provider child dispatch and artifact inspection through existing
`you workflow` commands.

That slice proves:

- runtime-backed CLI execution via `--execution-provider javascript-runtime`,
  `--project-root`, and `--child-executor-mode live-provider`
- same session ID and terminal outcome across `you workflow run`, `status`, and
  `result`
- bridged-child evidence through `you workflow dispatches` and
  `you workflow artifacts` (provider session refs, output artifact ids, dispatch
  linkage) using shared `ListDispatchesResponseToAPI` /
  `ListArtifactsResponseToAPI` projections
- additive fake-child and fixture-backed CLI regression without live-provider
  markers

This note records what the lane intentionally defers. It does not reopen the
completed CLI smoke path or widen this batch into MCP host setup, website
inspection, HTTP-only smoke, or a broader multi-surface parity sweep.

## In-Scope Work (Completed)

| Surface | CLI wiring | Proof |
|---------|------------|-------|
| Live-provider session start and re-read | `pkg/transports/cli/sessionexecution/run.go`, `status.go`, `result.go`, `pkg/transports/cli/root_work.go` | `run_test.go` (`TestRunSync_LiveProviderJavaScriptSession_ReReadStatusAndResult`, `TestRunSync_JavaScriptRuntimeBackend_UsesRealExecutionServiceWithoutFixtureStub`) |
| Bridged dispatch and artifact inspection | `pkg/transports/cli/sessionexecution/inspection.go` | `run_test.go` (`TestLiveProviderJavaScriptSession_DispatchAndArtifactCLIInspection`) |
| Fake/fixture CLI regression | default `--execution-provider fake` | `loopback_test.go` (`TestFixtureBackedCLIInspectionRegression_FullLoopWithoutLiveProviderFlags`), `run_test.go` (`TestRunSync_JavaScriptRuntimeFakeChildCLIInspectionRegression`, `TestRunSync_ExplicitFakeChildMode_OverridesLiveConfiguredServiceCLI`) |
| MCP-free live child provider | `factorysessionexecution.SmokeLiveChildProvider()` in `pkg/factorysessionexecution/service.go` | injected in live-provider CLI tests; no `you mcp serve` startup |

## Deferred Follow-Up Cells

This batch records **one** primary deferred surface per follow-up cell rather than
implementing both MCP and website inspection here.

| Follow-up cell | Deferred surface | Current posture |
|----------------|------------------|-----------------|
| `dynamic-workflows-cell-mcp-session-serve` | Live runtime-backed MCP serve for async start/status/result against real child dispatches | Documented in `follow-up-cell-mcp-session-serve.md`; default `you mcp serve` stays fixture-backed |
| `dynamic-workflows-cell-real-backend-api-website-inspection` | Dashboard durable Factory Session dispatch inspection for live child execution mode, provider-session refs, and artifact refs | Live-session oriented today; durable `dur-sess-*` dispatch detail UI is deferred |

## Non-Goals For The Completed CLI Smoke Lane

- MCP host install, packaging, or live `you mcp serve` runtime backing.
- Dashboard or website inspection work.
- HTTP-only CLI smoke or transport-specific dispatch semantics outside shared
  `factorysessionexecution` projections.
- Broader API, MCP, or website parity sweeps beyond direct CLI-observable reads.
- Replacing fake-child or fixture-backed CLI regression paths.

## Transport-Neutral Boundary

CLI dispatch and artifact rendering reads shared service projections and maps
through `pkg/transports/mapping/factorysession` (`ListDispatchesResponseToAPI`,
`ListArtifactsResponseToAPI`) before human formatting. JSON output is the shared
API shape; human labels render existing projection fields only.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `pkg/transports/cli/sessionexecution/run_test.go` | Live-provider run/status/result and dispatch/artifact CLI proof |
| `pkg/transports/cli/sessionexecution/loopback_test.go` | Fixture-backed CLI regression without live-provider flags |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map and focused verification commands |
| `follow-up-cell-mcp-session-serve.md` | Explicit MCP live-smoke deferral |
| `follow-up-cell-real-backend-api-session-parity-deferred.md` | Explicit website inspection deferral |
