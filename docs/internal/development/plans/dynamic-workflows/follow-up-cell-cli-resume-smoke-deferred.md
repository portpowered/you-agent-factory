# Follow-Up Cells: CLI Resume Smoke Deferred Surfaces

## Why This Note Exists

The lane `dynamic-workflows-cell-cli-resume-smoke` added one focused CLI smoke path
that interrupts a durable JavaScript `FactorySession`, resumes it through existing
shared `you session` commands, proves lifecycle continuity and no child-dispatch
replay, and preserves typed invalid resume outcomes.

That slice proves:

- runtime-backed CLI resume via `you session show`, `you session resume`, and
  `you session dispatches` against the shared HTTP durable session surface
- same `dur-sess-*` session ID across pre-resume reads, resume control, and
  post-resume terminal inspection
- pre-resume and post-resume dispatch parity without replaying completed child work
- typed `TERMINAL_SESSION` and `NO_OP` resume outcomes instead of opaque transport
  failures
- additive non-resume CLI session create, show, list, and lifecycle-control
  regression without widening into response-stream renderer work

This note records what the lane intentionally defers. It does not reopen the
completed CLI resume smoke path or widen this batch into MCP host work, API
transport redesign, website inspection, backend persistence redesign, or generic
CLI response-stream rendering.

## In-Scope Work (Completed)

| Surface | CLI wiring | Proof |
|---------|------------|-------|
| Interrupted-to-resumed success | `pkg/transports/cli/session/show.go`, `pkg/transports/cli/session/lifecycle_control.go`, `pkg/transports/cli/root_work.go` | `pkg/transports/cli/session/smoke/resume_smoke_test.go` (`TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands`) |
| Resume continuity without replay | `pkg/transports/cli/session/dispatches.go` | `pkg/transports/cli/session/smoke/resume_smoke_test.go` (`TestCLIResumeSmoke_DurableResumeContinuityPreservesCompletedChildDispatchesWithoutReplay`) |
| Typed invalid resume outcomes | `pkg/transports/cli/session/lifecycle_control.go` | `pkg/transports/cli/session/smoke/resume_smoke_test.go` (`TestCLIResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead`, `TestCLIResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead`) |
| Additive non-resume CLI regression | existing `pkg/transports/cli/session` create/show/list/lifecycle-control commands | `pkg/transports/cli/session/smoke/resume_non_regression_test.go` |
| Shared durable backend ownership | `pkg/factorysessionexecution` resume helpers consumed by HTTP handlers | runtime-backed HTTP harnesses in `pkg/transports/cli/session/smoke/resume_smoke_test.go` |

## Deferred Follow-Up Cells

This batch records **one** primary deferred surface per follow-up cell rather than
implementing MCP, API, and website resume smokes here.

| Follow-up cell | Deferred surface | Current posture |
|----------------|------------------|-----------------|
| `dynamic-workflows-cell-mcp-session-serve` | MCP resume/control parity for interrupted durable sessions | Fixture-backed and runtime-backed MCP serve exists; interrupted checkpoint resume smoke through MCP tools is deferred |
| `dynamic-workflows-cell-real-backend-api-session-parity` | API-only resume continuity proof separate from CLI loopback | Backend lifecycle and restart-resume parity lanes already prove shared durable semantics; API-focused resume smoke remains optional follow-up |
| `dynamic-workflows-cell-website-session-inspection` | Dashboard inspection of interrupted/resumed durable sessions | Website detail surface is read-oriented today; resumed lifecycle drilldown remains deferred |

## Non-Goals For The Completed CLI Resume Smoke Lane

- MCP host install, packaging, or live `you mcp serve` resume smoke.
- Dashboard or website inspection work.
- Public REST contract changes beyond consuming existing durable session routes.
- Backend persistence redesign or checkpoint-store changes.
- Generic CLI response-stream renderer work (`pkg/factory/sessions/responsestream`).
- Workflow-specific resume commands outside the shared `you session` surface.
- Replacing fixture-backed or live-session CLI regression paths.

## Transport-Neutral Boundary

CLI resume and dispatch inspection read the shared HTTP durable session routes
(`GET /factory-sessions/{session_id}`, `/dispatches`, `/resume`) and decode
generated API shapes. JSON output is the shared API contract; human labels render
existing projection fields only. `you workflow dispatches` remains on the
execution-service CLI path and is out of scope for this resume smoke lane.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `pkg/transports/cli/session/smoke/resume_smoke_test.go` | Successful resume, continuity/no-replay, and invalid resume CLI proof |
| `pkg/transports/cli/session/smoke/resume_non_regression_test.go` | Non-resume CLI session regression and shared-surface scope guard |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map and focused verification commands |
| `follow-up-cell-mcp-session-serve.md` | Explicit MCP resume-smoke deferral |
| `follow-up-cell-real-backend-api-session-parity-deferred.md` | Explicit API parity deferral |
| `follow-up-cell-website-session-inspection-deferred.md` | Explicit website inspection deferral |
