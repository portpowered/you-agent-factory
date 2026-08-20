# Follow-Up Cells: Real-Backend Factory Session API Parity Deferred Routes

## Why This Note Exists

The recovery lane `dynamic-workflows-cell-real-backend-api-session-parity` wired the
smallest real-backend Factory Session API slice for simple final-only JavaScript
sessions:

- `POST /factory-sessions/async` and `POST /factory-sessions/sync`
- scoped `GET /factory-sessions`
- `GET /factory-sessions/{session_id}`
- `GET /factory-sessions/{session_id}/results`
- `GET /factory-sessions/{session_id}/events` with reconnect cursor support

That slice proves stable session identity, terminal lifecycle status, final-result
semantics, and canonical Factory Event replay through `factorysessionexecution.RuntimeService`
without introducing standalone workflow-run API nouns.

This note records the deferred route families encountered while implementing the
narrow slice. It does not reopen the completed start/list/get/result/events work.

## In-Scope Routes (Completed)

| Route family | Backend wiring | Proof |
|--------------|----------------|-------|
| Async/sync start | `pkg/service/durable_session_execution.go`, `pkg/transports/http/handlers_durable_execution.go` | `server_durable_session_execution_test.go` |
| Scoped list/get | `pkg/service/durable_session_read.go`, `pkg/transports/http/handlers_durable_read.go` | `server_durable_session_read_test.go` |
| Final result read | same read service + `handlers_durable_read.go` | `server_durable_session_result_test.go` |
| Canonical event replay | same read service + `handlers_events.go` | `server_durable_session_events_test.go` |
| Contract alignment | authored OpenAPI + `assertRealBackendSessionAPISliceRoutes` | `generated_contract_durable_session_test.go` |

## Deferred Follow-Up Cells

| Follow-up cell | Deferred surface | Current transport posture |
|----------------|------------------|---------------------------|
| `dynamic-workflows-cell-real-backend-api-dispatch-artifact` | `GET /factory-sessions/{session_id}/dispatches`, `GET /factory-sessions/{session_id}/dispatches/{dispatch_id}`, `GET /factory-sessions/{session_id}/artifacts`, `GET /factory-sessions/{session_id}/artifacts/{artifact_id}` | `501 NotImplemented` stubs in `pkg/transports/http/handlers_factory.go`; OpenAPI + mapper contracts remain compiled |
| `dynamic-workflows-cell-real-backend-api-lifecycle-controls` | `POST /factory-sessions/{session_id}/approve`, `/pause`, `/resume`, `/cancel`, `/terminate`, `/retry-dispatch` | `501 NotImplemented` stubs in `pkg/transports/http/handlers_factory.go`; resume behavior waits on this cell |
| `dynamic-workflows-cell-real-backend-api-website-inspection` | Dashboard durable Factory Session inspection through `ui/src/features/factory-session-detail/` and generated durable read models | Live-session oriented today; HTTP wiring for durable `dur-sess-*` reads is deferred |
| `dynamic-workflows-cell-mcp-session-serve` | Live runtime-backed MCP host install smoke for async start/status/result | Documented in `follow-up-cell-mcp-session-serve.md`; default `you mcp serve` stays fixture-backed |
| `dynamic-workflows-cell-real-backend-api-live-provider-bridge` | Provider dispatch bridge parity for queued/running/reconciled dispatches and live external tool sessions | Dispatch bridge lane; not part of the simple final-only API slice |

## Smallest Executor Lanes

### `dynamic-workflows-cell-real-backend-api-dispatch-artifact`

1. Route durable `dur-sess-*` dispatch and artifact reads through
   `factorysessionexecution.RuntimeService` (`ListDispatches`, `GetDispatch`,
   `ListArtifacts`, `GetArtifact`) using the existing `apisurface` projection helpers.
2. Prove runtime-backed API loopback for at least one simple session with projected
   dispatch/artifact rows from `ProjectRuntimeExecutionRecords`.
3. Keep live Petri dispatch/artifact compatibility on existing session-runtime paths.

### `dynamic-workflows-cell-real-backend-api-lifecycle-controls`

1. Route durable lifecycle controls through `RuntimeService` pause/resume/cancel/
   terminate/approve/retry-dispatch using `pkg/transports/mapping/factorysession/factory_session_lifecycle.go`.
2. Prove resume and other controls preserve inspectable partial results, dispatches,
   and artifacts per OpenAPI `FactorySessionLifecycleControlLinks`.
3. Keep typed `INVALID_STATE`, `TERMINAL_SESSION`, and `CONFLICT` outcomes aligned with
   fake-service contract fixtures.

### `dynamic-workflows-cell-real-backend-api-website-inspection`

1. Extend `ui/src/api/factory-sessions/api.ts` and factory-session detail components to
   consume durable `FactorySessionDurableReadModel`, result, dispatch, artifact, and event
   surfaces already defined in OpenAPI.
2. Preserve explicit loading, empty, error, and terminal states for durable sessions.
3. Use Factory Session and Factory Event vocabulary only; do not introduce workflow-run UI nouns.

### `dynamic-workflows-cell-mcp-session-serve`

See `follow-up-cell-mcp-session-serve.md`. MCP host installation with live runtime-backed
serve remains out of scope for the API parity slice.

### `dynamic-workflows-cell-real-backend-api-live-provider-bridge`

1. Wire provider-session integration for JavaScript child dispatches after dispatch/artifact
   API reads land.
2. Prove live-provider bridge parity through shared dispatch lifecycle events and
   `providerSessionRefs` on dispatch projections.
3. Keep Petri dispatch-request compatibility on the existing factory runtime path.

## Non-Goals For The Completed Slice

- Dispatch reads, artifact reads, lifecycle controls (including resume), website
  inspection, MCP host installation beyond preview/fixture scope, and live-provider bridge
  parity.
- Standalone workflow-run API resources, route families, or generated types.
- Broad Petri-runtime rewrites unrelated to durable JavaScript session parity.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `pkg/transports/http/handlers_factory.go` | `501` stubs for deferred durable route families |
| `pkg/transports/http/server_durable_session_deferred_routes_test.go` | Runtime-backed proof that deferred routes stay stubbed |
| `assertDeferredRealBackendSessionRouteFamilies` in `generated_contract_durable_session_test.go` | OpenAPI deferred-route registry separate from the narrow in-scope slice |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map for in-scope vs deferred boundaries |
