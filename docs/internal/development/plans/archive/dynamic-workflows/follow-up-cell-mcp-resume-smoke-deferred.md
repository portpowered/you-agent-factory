# Follow-Up Cells: MCP Resume Smoke Deferred Surfaces

## Why This Note Exists

The lane `dynamic-workflows-cell-mcp-resume-smoke` added one focused runtime-backed
MCP smoke path that interrupts a durable JavaScript `FactorySession`, resumes it
through existing `you.factory_session.*` MCP tools, proves lifecycle continuity
and no child-dispatch replay, and preserves typed invalid resume outcomes.

That slice proves:

- runtime-backed MCP resume via `you.factory_session.get`, `you.factory_session.control`,
  `you.factory_session.list_dispatches`, and `you.factory_session.get_result` on the
  shared stdio `mcpcli.RunServe` path
- same `dur-sess-*` session ID across pre-resume reads, resume control, and
  post-resume terminal inspection
- pre-resume and post-resume dispatch parity without replaying completed child work
- typed `TERMINAL_SESSION` and `NO_OP` resume outcomes in MCP result envelopes
  instead of opaque transport failures
- additive non-resume fixture-backed and runtime-backed MCP serve regression without
  widening into host packaging, HTTP/SSE transport, website inspection, or API-only
  resume smoke

This note records what the lane intentionally defers. It does not reopen the
completed MCP resume smoke path or widen this batch into dashboard inspection,
HTTP transport redesign, multi-host parity matrices, or generic CLI resume smoke.

## In-Scope Work (Completed)

| Surface | MCP wiring | Proof |
|---------|------------|-------|
| Interrupted-to-resumed success | `pkg/transports/mcp/factorysession/control.go`, `pkg/transports/cli/mcp/serve.go` | `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` (`TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl`) |
| Resume continuity without replay | `pkg/transports/mcp/factorysession/list_dispatches.go` | `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` (`TestRunServe_RuntimeResumeSmoke_DispatchContinuityPreservesCompletedChildDispatchesWithoutReplay`) |
| Typed invalid resume outcomes | `pkg/transports/mcp/factorysession/control.go` | `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` (`TestRunServe_RuntimeResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead`, `TestRunServe_RuntimeResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead`) |
| Additive non-resume MCP serve regression | existing fixture/runtime stdio serve paths | `pkg/transports/cli/mcp/serve_runtime_resume_non_regression_test.go` |
| Shared durable backend ownership | `pkg/factorysessionexecution` resume helpers consumed by MCP tool handlers | runtime-backed MCP harness in `serve_runtime_resume_smoke_test.go` |

## Deferred Follow-Up Cells

This batch records **one** primary deferred surface per follow-up cell rather than
implementing website, API, and CLI resume smokes here.

| Follow-up cell | Deferred surface | Current posture |
|----------------|------------------|-----------------|
| `dynamic-workflows-cell-website-session-inspection` | Dashboard inspection of interrupted/resumed durable sessions | Website detail surface is read-oriented today; resumed lifecycle drilldown remains deferred |
| `dynamic-workflows-cell-real-backend-api-session-parity` | API-only resume continuity proof separate from MCP loopback | Backend lifecycle and restart-resume parity lanes already prove shared durable semantics; API-focused resume smoke remains optional follow-up |
| `dynamic-workflows-cell-cli-resume-smoke` | CLI resume/control parity for interrupted durable sessions | Completed in a separate lane; MCP resume smoke does not replace CLI loopback proof |

## Non-Goals For The Completed MCP Resume Smoke Lane

- MCP host install, packaging, or per-host UI parity matrices.
- Dashboard or website inspection work.
- Public REST contract changes beyond consuming existing durable session routes through MCP tool handlers.
- Backend persistence redesign or checkpoint-store changes.
- HTTP or SSE MCP transport.
- Workflow-specific MCP resume resource models outside the shared `you.factory_session.*` catalog.
- Replacing fixture-backed or runtime-backed non-resume MCP serve regression paths.

## Transport-Neutral Boundary

MCP resume and dispatch inspection route through the shared `pkg/transports/mcp/factorysession`
tool handlers backed by `factorysessionexecution.Service`. Tool responses use
generated API shapes (`FactorySessionDurableReadModel`, `ListFactorySessionDispatchesResponse`,
`FactorySessionLifecycleControlResponse`) rather than workflow-only MCP fields.
Compatibility `you.workflow.*` aliases remain optional; resume smoke proves the
canonical `you.factory_session.*` surface only.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `pkg/transports/cli/mcp/serve_runtime_resume_smoke_test.go` | Successful resume, continuity/no-replay, and invalid resume MCP proof |
| `pkg/transports/cli/mcp/serve_runtime_resume_non_regression_test.go` | Non-resume MCP serve regression and shared-surface scope guard |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map and focused verification commands |
| `follow-up-cell-cli-resume-smoke-deferred.md` | Explicit CLI resume-smoke boundary (completed separately) |
| `follow-up-cell-real-backend-api-session-parity-deferred.md` | Explicit API parity deferral |
| `follow-up-cell-website-session-inspection-deferred.md` | Explicit website inspection deferral |
