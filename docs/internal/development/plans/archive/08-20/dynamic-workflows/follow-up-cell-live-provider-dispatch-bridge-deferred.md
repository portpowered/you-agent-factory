# Follow-Up Cells: Live Provider Child Dispatch Bridge Deferred Surfaces

## Why This Note Exists

The lane `dynamic-workflows-cell-live-provider-dispatch-bridge` wired one bounded
real child-execution path into durable JavaScript `FactorySession` execution so
`agent.run`, `parallel`, and `pipeline` can record provider-backed child dispatch
behavior through shared session-backend inspection while preserving the existing
deterministic fake child path as a selectable coexistence mode.

That slice proves:

- live child execution mode via `Runtime.ChildExecutorMode = "live-provider"`
- stable dispatch identity, lifecycle transitions, provider-session refs, and
  artifact refs through `ProjectRuntimeExecutionRecords`
- typed failed-child detail without corrupting sibling dispatches or parent
  session inspection
- fake-child non-regression for `agent.run`, `parallel`, and `pipeline` without
  changing workflow source syntax

This note records deferred follow-up cells encountered while implementing the
bounded bridge. It does not reopen the completed runtime bridge or widen this
batch into website inspection, graph editor work, MCP install packaging, or
another broad API surface.

## In-Scope Work (Completed)

| Surface | Backend wiring | Proof |
|---------|----------------|-------|
| Live child executor bridge | `pkg/factorysessionexecution/livechild/provider.go` (`ProviderChildExecutor`, `childExecutorHooks`) | `livechild/provider_test.go`, `fixtures/runtime_live_child_test.go` |
| Fake/live coexistence seam | `workflowruntime.Hooks.NewChildExecutor` with default `FakeChildExecutor` fallback in `pkg/orchestrators/javascript/runtime/runtime.go` | `fixtures/runtime_child_executor_coexistence_test.go`, `runtime/host_children_test.go`, `runtime/host_pipeline_policy_test.go` |
| Durable dispatch inspection | `pkg/factorysessionexecution/projection_consistency.go` | `fixtures/runtime_record_projection_test.go`, `fixtures/runtime_live_child_failure_test.go` |
| Runtime mode selection | `StartRequest.Runtime.ChildExecutorMode` via `pkg/factorysessionexecution/normalize.go` and `resolveChildExecutorMode` | `fixtures/runtime_live_child_dispatch_test.go`, `fixtures/runtime_child_executor_coexistence_test.go` |

## Deferred Follow-Up Cells

| Follow-up cell | Deferred surface | Current posture |
|----------------|------------------|-----------------|
| `dynamic-workflows-cell-cli-live-dispatch-smoke` | CLI loopback for runtime-backed `you workflow run` / `you workflow dispatches` with `live-provider` child execution and provider-session correlation | **Completed** in `dynamic-workflows-cell-cli-live-dispatch-smoke`; scope and deferrals in `follow-up-cell-cli-live-dispatch-smoke-deferred.md` |
| `dynamic-workflows-cell-mcp-session-serve` | Live runtime-backed MCP serve for async start/status/result against real child dispatches | Documented in `follow-up-cell-mcp-session-serve.md`; default `you mcp serve` stays fixture-backed |
| `dynamic-workflows-cell-real-backend-api-website-inspection` | Dashboard durable Factory Session dispatch inspection for live child execution mode, provider-session refs, and artifact refs | Live-session oriented today; durable `dur-sess-*` dispatch detail UI is deferred |
| `dynamic-workflows-cell-live-provider-bridge-parity` | Broader live-provider bridge parity across queued/running/reconciled Petri dispatches and external tool sessions | Bounded JavaScript durable runtime bridge only; Petri dispatch-request compatibility stays on the existing factory runtime path |

## Smallest Executor Lanes

### `dynamic-workflows-cell-cli-live-dispatch-smoke` (completed)

Completed in lane `dynamic-workflows-cell-cli-live-dispatch-smoke`. See
`follow-up-cell-cli-live-dispatch-smoke-deferred.md` for proof inventory and
explicit MCP/website deferrals.

### `dynamic-workflows-cell-mcp-session-serve`

See `follow-up-cell-mcp-session-serve.md`. Live runtime-backed MCP serve remains
out of scope for the bounded JavaScript runtime bridge.

### `dynamic-workflows-cell-real-backend-api-website-inspection`

1. Extend `ui/src/features/factory-session-detail/` to consume durable dispatch
   detail including `executionMode`, provider-session refs, and artifact refs
   already exposed by the backend projection layer.
2. Preserve explicit loading, empty, error, and terminal states for durable
   sessions.
3. Use Factory Session and Dispatch vocabulary only.

### `dynamic-workflows-cell-live-provider-bridge-parity`

1. Prove live-provider bridge parity through shared dispatch lifecycle events and
   `providerSessionRefs` on dispatch projections beyond the bounded JavaScript
   durable runtime slice.
2. Keep Petri dispatch-request compatibility on the existing factory runtime path.

## Non-Goals For The Completed Bridge

- Website or dashboard inspection work.
- Graph editor or current-selection UI work.
- MCP host install or packaging changes.
- Broad API or event-stream parity sweeps beyond the shared session-backend
  inspection shape already owned by this lane.
- Replacing the deterministic fake child path or changing workflow source syntax
  for `agent.run`, `parallel`, or `pipeline`.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `pkg/factorysessionexecution/fixtures/runtime_child_executor_coexistence_test.go` | Fake/live coexistence on shared workflow fixtures |
| `pkg/orchestrators/javascript/runtime/host_children_test.go` | Runtime-level fake child coverage for host primitives |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map for child-executor mode selection |
