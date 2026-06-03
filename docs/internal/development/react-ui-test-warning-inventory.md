# React UI unit test warning inventory

**Captured (UTC):** 2026-06-02  
**Maintained lane:** `make ui-test` / `cd ui && bun run test:unit` (Vitest, excludes `integration/*.integration.test.mjs`)  
**Capture tooling:** `ui/vite.config.warning-inventory.ts` + `ui/src/testing/warning-inventory-capture.setup.ts` (hooks `console.warn` / `console.error` per test case; summarize with `node ui/scripts/summarize-warning-inventory.mjs`)

## Executive summary

Focused runs across App shell (`src/App.*.test.tsx`), all `src/features/current-selection/**` unit tests, graph/layout targets (`src/App.layout-graph.test.tsx`, `src/testing/graph-editor-harness.test.ts`), and representative workflow-activity graph tests produced **828** hooked console calls during **passing** tests. All captured lines were `console.error` (React logs `act(...)` and duplicate-key issues as errors in React 19). No `console.warn` lines were recorded in this capture pass.

| Rank | Category | Approx. count | Primary sources |
| --- | --- | ---: | --- |
| 1 | React `act(...)` — Recharts internals | ~616 | `LineImpl`, `YAxisImpl`, `XAxisImpl`, `CartesianGrid` during export/dashboard chart renders |
| 2 | React `act(...)` — React Flow internals | ~246 | `NodeWrapper`, `EdgeWrapper`, `MarkerDefinitions`, `EventSynchronizer` |
| 3 | React `act(...)` — App / selection shell | ~40 | `DashboardBento`, `DashboardScreenContent`, `SelectionHarness`, `CurrentSelectionWidgetSaveNotifications`, etc. |
| 4 | Missing / duplicate React keys | 4 | Provider-session selection, execution details lists |

## Methodology

1. Installed UI deps (`cd ui && bun install`).
2. Ran Vitest with `vite.config.warning-inventory.ts` (120s test timeout, standard `vitest.setup.ts` plus capture setup).
3. Suites executed (append mode for `console-entries.jsonl`):
   - All `src/App.*.test.tsx` (14 files; 4 tests timed out at 30s default before inventory config — re-run used 120s timeout).
   - `src/features/current-selection/**` by subdomain (`workstation-selection`, `worker-selection`, `work-type-selection`, `work-state-selection`, `work-selection`, `resource-selection`, `components`, `hooks`, `base`, `dispatch-selection`, `public`; empty filter dirs skipped: `state`, `editing`, `lib`, `messages`).
   - `src/App.layout-graph.test.tsx`, `src/testing/graph-editor-harness.test.ts`.
   - Sample: `src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-layout.test.tsx`, `src/features/workflow-activity/lib/react-flow-current-activity-card-graph.test.ts` (no additional console lines).
4. Aggregated with `node ui/scripts/summarize-warning-inventory.mjs` and ad-hoc grouping by component suffix in `act` messages.

**Note:** Hooked capture only records calls that reach `console.error` / `console.warn` after setup. Vitest’s default console intercept remains enabled; messages still reached our hooks in this pass.

## Ranked categories

### 1. React `act(...)` — Recharts / chart subtree (highest volume)

**Example message:**

```text
An update to %s inside a test was not wrapped in act(...).
...
https://react.dev/link/wrap-tests-with-act LineImpl
```

**Components observed (descending count):** `LineImpl` (264), `YAxisImpl` (96), `XAxisImpl` (88), `CartesianGrid` (88).

**Affected test files (by volume):**

| Count | Test file |
| ---: | --- |
| 788 | `ui/src/App.export-submit.test.tsx` |
| (remaining act lines in other files are predominantly React Flow — see below) |

**Likely cause:** Chart mount/responsive updates from Recharts during App export and dashboard renders without awaiting async chart layout in tests.

**Cleanup direction (stories 002–004):** Wrap chart render assertions in `waitFor` / `act`, or narrow test harness mocks for export PNG flows; avoid global console suppression in production chart components.

---

### 2. React `act(...)` — React Flow / graph projection (second)

**Example message:** Same `act(...)` template with trailing component names such as `NodeWrapper`, `EdgeWrapper`, `MarkerDefinitions`, `EventSynchronizer`.

**Components observed:** `NodeWrapper` (120), `EdgeWrapper` (88), `MarkerDefinitions` (20), `EventSynchronizer` (18), `ReactFlowCurrentActivityCardView` (2).

**Affected test files (sample):**

- `ui/src/App.current-selection.test.tsx`
- `ui/src/App.replay-workstation-requests.test.tsx`
- `ui/src/App.session-stream.test.tsx`
- `ui/src/features/current-selection/hooks/useCurrentSelection.test.tsx`
- `ui/src/features/current-selection/components/current-selection-widget.graph-draft-conflict-notifications.test.tsx`

**Likely cause:** React Flow internal state updates after graph mount, fitView, or edge/node hydration in App shell and selection harness tests.

**Cleanup direction:** Stabilize graph harness helpers under `ui/src/testing/`, flush microtasks after graph mount, use existing `act` patterns from workflow-activity hook tests where appropriate.

---

### 3. React `act(...)` — Dashboard / current-selection shell (third)

**Example trailing components:** `SelectionHarness` (22), `DashboardScreenContent` (4), `DashboardExportDialog` (3), `DashboardBento` (2), `AgentBentoLayout` (2), `CurrentSelectionWidgetSaveNotifications` (6), `SelectionDetailLayout` (1).

**Affected test files:**

- `ui/src/App.toolbar-locale.test.tsx`
- `ui/src/App.session-stream.test.tsx`
- `ui/src/App.replay-workstation-requests.test.tsx`
- `ui/src/App.current-selection.test.tsx`
- `ui/src/features/current-selection/base/components/current-selection-detail-layout.test.tsx`

**Likely cause:** Bento layout, stream status, and selection panel async updates (React Query, layout effects) not fully settled before assertions.

---

### 4. Missing / duplicate React keys (low count, high signal)

**Example message:**

```text
Encountered two children with the same key, `%s`. Keys should be unique so that components maintain their identity across updates.
```

**Affected test files:**

- `ui/src/features/current-selection/components/current-selection-widget.provider-session-selection.test.tsx`
- `ui/src/features/current-selection/work-selection/components/execution-details.test.tsx`

**Likely cause:** List rendering uses unstable or colliding keys when switching provider-session or execution-detail rows in tests.

**Cleanup direction (story 003):** Fix key props in the component projection or test fixtures; do not mask with global suppression.

---

## Suites with no hooked console noise in this pass

The following were executed and contributed **zero** additional lines to `console-entries.jsonl` beyond categories above:

- Most `src/features/current-selection/**` subdomain suites (hundreds of tests) — noise is concentrated in App shell + a handful of selection tests.
- `ui/src/App.layout-graph.test.tsx`, `ui/src/testing/graph-editor-harness.test.ts` (when run after App batch).
- Sample workflow-activity graph unit tests listed in methodology.

## Categories not observed in this capture

These were explicitly checked for but **did not appear** in hooked output (may still exist in other suites or as `console.warn` in future runs):

- Invalid DOM prop warnings (`React does not recognize the ... prop`)
- Controlled / uncontrolled input warnings
- Radix / accessibility dialog description warnings
- Deprecated React API warnings

Re-run the full maintained lane with capture setup after major UI dependency upgrades to refresh this list.

## Commands for maintainers

```bash
cd ui
rm -rf .warning-inventory
bunx vitest run --config vite.config.warning-inventory.ts --exclude 'integration/*.integration.test.mjs' src/App.export-submit.test.tsx
node scripts/summarize-warning-inventory.mjs
# Inspect .warning-inventory/ranked-warnings.json and console-entries.jsonl
```

Append additional suites:

```bash
VITEST_WARNING_INVENTORY_APPEND=1 bunx vitest run --config vite.config.warning-inventory.ts --exclude 'integration/*.integration.test.mjs' src/features/current-selection/components
node scripts/summarize-warning-inventory.mjs
```

## App shell cleanup (story 002)

**Status (UTC 2026-06-03):** All `src/App.*.test.tsx` suites pass the warning-inventory capture with **zero** hooked `console.error` / `console.warn` lines.

## Current-selection cleanup (story 003)

**Status (UTC 2026-06-03):** All `src/features/current-selection/**` unit tests pass the warning-inventory capture with **zero** hooked `console.error` / `console.warn` lines.

**Harness approach:**

| Concern | Mitigation |
| --- | --- |
| Duplicate inference-attempt React keys in tests | Derive default `inference_request_id` from `attempt` in `buildDashboardInferenceAttemptFixture` |
| `useCurrentSelection` hook act noise | Prefer `renderHook` + pre-synced `resolveDashboardSelection` seeds; stub `useTerminalWorkDetailCleanup` in hook tests |
| Save-notification act noise | `settleCurrentSelectionEffects()` after render/save; use `userEvent` for async save clicks; flush toast helpers with settle |
| Selection detail undo/redo act noise | `userEvent` clicks + post-render settle in layout shell tests |

| Concern | Mitigation |
| --- | --- |
| Recharts `act` noise in export/stream suites | Opt-in `ui/src/testing/app-shell-work-outcome-stub.tsx` (mocks `useWorkOutcomeChart` + `WorkOutcomeWidget`) |
| React Flow `act` noise in export/stream suites | Opt-in `ui/src/testing/app-shell-workflow-activity-stub.tsx` |
| Suites that assert on real charts or graph | Do **not** import the stubs; use `waitForAppShellWorkGraphReady()` when waiting on React Flow nodes |
| Session stream loading shell | Call `resetTimelineForInitialStreamLoad()` **before** `renderApp({ seedTimelineFromSnapshot: false })`; use `emitTimelineMessagesAct()` for EventSource emits |

Stub imports today: `App.export-submit`, `App.export-dialog`, `App.follow-up-trace`, `App.session-stream`.

## Graph and layout cleanup (story 004)

**Status (UTC 2026-06-03):** PRD graph/layout targets pass warning-inventory capture with **zero** hooked console lines:

- `ui/src/App.layout-graph.test.tsx`, `ui/src/testing/graph-editor-harness.test.ts`
- Representative workflow-activity graph suites (`react-flow-current-activity-card-graph*.test.*`, `react-flow-current-activity-card-graph-layout.test.tsx`)
- Broader React Flow component/harness spot-check (`react-flow-current-activity-card*.test.tsx`, `factory-graph-editor-flow.test.tsx`, trace graph viewport/nodes tests)

**Harness approach:**

| Concern | Mitigation |
| --- | --- |
| Full `ReactFlowCurrentActivityCard` mount act noise (empty topology) | `settleWorkflowActivityGraphEffects()` from `ui/src/testing/workflow-activity-test-utils.ts` after render |
| App shell graph assertions | `waitForAppShellWorkGraphReady()` (story 002); deterministic layout via `buildDashboardTestGraphLayout` mock in `app-shell-test-utils` |
| Graph editor hook tests | `graph-editor-harness.ts` mock graph/draft wiring (no React Flow mount) |

## PRD story mapping

| Story | Primary inventory targets |
| --- | --- |
| 002 App shell | `App.export-submit.test.tsx`, `App.session-stream.test.tsx`, `App.toolbar-locale.test.tsx`, `App.current-selection.test.tsx`, replay/stream tests |
| 003 current-selection | `current-selection-widget.*`, `useCurrentSelection.test.tsx`, `execution-details.test.tsx`, provider-session selection |
| 004 graph/layout | `App.layout-graph.test.tsx`, `graph-editor-harness.test.ts`, workflow-activity React Flow suites, trace/factory-graph-editor flow tests |
| 005–006 guard | Allowlist only named patterns above; Recharts/React Flow shims must stay narrow |

## Console guard (story 005)

**Status (UTC 2026-06-03):** Reusable opt-in guard lives at `ui/src/testing/strict-console-guard.ts`.

- `installStrictConsoleGuard()` hooks `console.warn` / `console.error` for a single test.
- `useStrictConsoleGuard({ allowlist })` wires Vitest `beforeEach` / `afterEach` for a describe block.
- `withStrictConsole(options, callback)` runs an async callback under a temporary guard.
- Allowlist entries require `name`, `level`, narrow `match` (substring or specific RegExp), and `reason`; broad wildcards are rejected at install time.
- Story 006 adopts the guard on cleaned App shell and current-selection suites.

## Console guard adoption (story 006)

**Status (UTC 2026-06-03):** Guarded suites verified with `make ui-test`.

| Suite | Guard wiring | Allowlist |
| --- | --- | --- |
| App shell (`src/App.*.test.tsx`) | `ui/src/testing/guarded-suite-console.setup.ts` imported from `vitest.setup.ts` (path-scoped) | None (zero hooked noise after stories 002–004) |
| current-selection components | Same path-scoped setup; excludes `hooks/` and manual-guard files below | None for most files |
| `current-selection-widget.graph-draft-conflict-notifications.test.tsx` | `useStrictConsoleGuard()` on describe root | `widget-save-notifications-mutation-settle` — async mocked document-save re-renders `CurrentSelectionWidgetSaveNotifications` |
| `current-selection-detail-layout.test.tsx` (actions describe) | `useStrictConsoleGuard()` on describe root | `detail-layout-history-controls` — undo/redo updates `SelectionDetailLayout` subscribers |
| `hooks/useCurrentSelection.test.tsx` | Excluded from path guard (render/probe harness); hook sync behavior required | N/A |

**Helpers:** `settleCurrentSelectionEffects()` and `waitForCurrentSelection()` in `ui/src/testing/current-selection-test-utils.ts`. Set `VITEST_DISABLE_GUARDED_CONSOLE=1` to bisect guard failures locally.
