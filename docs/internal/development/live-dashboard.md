---
author: ralph agent
last modified: 2026, april, 12
doc-id: AGF-DOC-001
---

# Agent Factory Live Dashboard

This document is the contributor guide for the Agent Factory browser shell. Use it when you need to run the dashboard locally, understand how the event stream is assembled, or extend the embedded UI and its backend contract.

## What It Covers

The live dashboard is a standalone browser surface owned by `libraries/agent-factory`. It is served by the factory process, uses a reconstruction-first backend read model, and renders one running workflow instance for one factory process at a time.

## Dashboard Surfaces

The dashboard consists of four connected pieces:

1. `pkg/cli/dashboard` reconstructs CLI dashboard read models from raw engine snapshots, dispatch history, marking state, and trace helpers.
2. `pkg/api` exposes the browser-facing transport:
   - `GET /events` for canonical factory event SSE replay and live updates
   - `/dashboard/ui` for the embedded SPA shell
3. `ui/` contains the standalone React application. New timeline work should consume `/events` and project selected ticks from canonical factory history.
4. `pkg/api/dashboard_ui.go` serves the generated `ui/dist/` bundle through the `ui` package's embed registration when `make ui-build` has refreshed the local dashboard assets.

## Current Constraints

- State is in-memory only. Restarting the factory resets the dashboard state.
- Scope is one factory process and one loaded workflow instance.
- The graph is an operator-focused simplified workflow view, not a full raw Petri net renderer.
- `WorkID` is the primary drill-down identity. The browser resolves retained traces from the event timeline store instead of calling removed trace endpoints.
- Live updates are delivered with SSE through `/events`. Polling is not the primary implementation path.

## Run The Embedded Dashboard

Use this path to validate the production-style embedded shell.

1. Start the factory with the dashboard-serving HTTP port enabled.

```bash
cd libraries/agent-factory
make build
./bin/agent-factory run --dir ./examples/basic/factory --port 7437
```

1. Open the embedded dashboard shell:

```text
http://127.0.0.1:7437/dashboard/ui
```

1. Confirm the shell loads and that browser requests reach these endpoints:
   - `GET /events`

## Run The Frontend Dev Server

Use this path when iterating on the React UI without rebuilding embedded assets on each change.

1. Keep a local factory instance running:

```bash
cd libraries/agent-factory
./bin/agent-factory run --dir ./examples/basic/factory --port 7437
```

1. Start the Vite dev server in a second terminal:

```bash
cd libraries/agent-factory/ui
bun install --frozen-lockfile
bun run dev
```

1. Open the Vite URL shown in the terminal. The dev server proxies `/events` to `http://127.0.0.1:7437` for local iteration only.

2. If the factory runs on another origin, set `AGENT_FACTORY_API_ORIGIN` before `bun run dev`.

## Backend Contract

### Event API

`GET /events` returns canonical factory event history followed by live runtime events. The dashboard reconstructs topology, selected-tick runtime state, trace drill-downs, and session summaries from that event stream.

The API handler should only stream canonical events. It should not rebuild dashboard-specific snapshots or re-classify raw runtime records locally.

### Active Throttle Pauses

Dashboard snapshots expose active provider/model throttle pause windows at `runtime.active_throttle_pauses`. The field is omitted when no provider/model lane is currently paused.

Each entry is derived from the aggregate engine snapshot, not by HTTP handlers reading dispatcher internals. Operators and tests can rely on these fields:

| Field | Meaning |
|-------|---------|
| `lane_id` | Stable provider/model lane identity used to distinguish affected work lanes. |
| `provider` | Provider name for the paused lane. |
| `model` | Model name for the paused lane. |
| `paused_at` | Time the pause was recorded when available. |
| `paused_until` | Time the dispatcher should stop filtering the lane after reconciliation. |
| `recover_at` | Operator-facing recovery time. This currently matches `paused_until`. |
| `affected_transition_ids` | Transition identifiers when the aggregate snapshot can derive them reliably. |
| `affected_workstation_names` | Workstation names when the aggregate snapshot can derive them reliably. |
| `affected_worker_types` | Worker type identifiers when the aggregate snapshot can derive them reliably. |
| `affected_work_type_ids` | Work type identifiers when the aggregate snapshot can derive them reliably. |

Derived affected-lane fields are optional. If topology or worker metadata cannot prove a field, the read model leaves that field empty instead of guessing from rendered names, logs, or handler-local string matching.

Pause windows are process-local runtime state. They are not persisted and disappear when the factory process restarts. Expired pauses are absent from the dashboard snapshot after the dispatcher reconciles pause state on a runtime tick.

### SSE API

`GET /events` uses server-sent events and follows this flow:

1. Send recorded factory event history immediately after connect.
2. Subscribe to live canonical factory events.
3. Emit each new event as a default SSE message.
5. Stop cleanly when the request context or service context is canceled.

This keeps the browser on one event-first model for both bootstrap and live updates.

## Frontend Contract

The browser UI should keep one source of truth for dashboard state:

1. Bootstrap from historical `GET /events` records.
2. Merge later `/events` updates into the same typed client cache.
3. Keep graph layout deterministic from backend topology data alone.
4. Resolve trace drill-down from `WorkID`, not token ID.

If you change the shape of dashboard data, update the TypeScript types in `ui/src/dashboard/`, the React tests, and the backend handler tests together.
Trace projections should expose `provider_session` directly on each dispatch attempt from the shared event-derived trace read model instead of asking browser code to scrape logs.

## Build And Verification

Run these checks from `libraries/agent-factory/` when changing the dashboard:

```bash
make dashboard-verify # Rebuild UI assets, then run Go vet and short Go tests
make ui-deps          # Install dashboard UI dependencies from ui/bun.lock
make ui-test          # Run Vitest through Bun
make ui-build         # Build TypeScript and Vite production assets through Bun
make ui-storybook     # Build Storybook static assets through Bun
make ui-test-storybook # Serve Storybook static assets and run interaction checks
```

`make dashboard-verify` is the preferred review-readiness gate after dashboard source changes that affect embedded assets. It serializes the Vite build before Go embed scanning so `go vet` and `go test` do not race against hashed asset rotation in `ui/dist/`.

Use `make ui-storybook` followed by `make ui-test-storybook` for dashboard Storybook interaction verification. The runner serves `ui/storybook-static`, waits for the dashboard Storybook index, and executes browser-backed story play functions using runtime setup and API mocks owned by `ui/.storybook`; it must stay separate from `website/.storybook` and website runner scripts.

When you are working directly inside `libraries/agent-factory/ui`, the supported package-manager commands are:

```bash
bun install --frozen-lockfile
bun run tsc
bun run test
bun run build
bun run build-storybook
bun run test-storybook
```

`bun run build` or `make ui-build` is required whenever shipped UI assets change because the Go dashboard route consumes the generated `ui/dist/` output through a local embed-registration file.
For acceptance verification, prefer the embedded `/dashboard/ui` route from a local factory server after rebuilding `ui/dist/`. This serves the fresh dashboard deployment from the same origin as the API and avoids the Vite proxy entirely. If a separate static deployment is necessary, build with `VITE_AGENT_FACTORY_API_ORIGIN=http://127.0.0.1:<factory-port>` so the browser client calls the local factory server directly.

Avoid `ui/package.json`'s `vite preview` path for review sign-off in worktrees because it is a long-running server and can leave a child process behind after the agent work is otherwise done. The preview configuration intentionally disables proxy fallback and uses `--strictPort` so accidental direct preview runs fail loudly instead of silently attaching to a different port or backend.

## Extension Points

When extending the dashboard, keep responsibilities in these locations:

- CLI dashboard read-model assembly: `pkg/cli/dashboard`
- HTTP transport and embedded asset serving: `pkg/api`
- Runtime event subscription: `pkg/factory/runtime` and shared event subsystems
- Browser rendering, selection state, and trace queries: `ui/src/dashboard/`

Prefer extending the shared read model and typed browser contract rather than adding dashboard-only formatting logic in handlers or ad hoc client transforms.
