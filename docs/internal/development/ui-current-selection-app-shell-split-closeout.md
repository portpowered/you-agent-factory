# App.current-selection App-Shell Split Closeout

Story: `prd-ui-ci-acceleration-and-test-rationalization-005`

## Selected high-cost suite

`ui/src/App.current-selection.test.tsx` — categorized as `app-shell-integration` in
`ui/scripts/ui-test-cost-report.mjs` and repeatedly among the slowest covered UI files
because it mounted the full dashboard for widget-local current-selection behavior.

## Before timing (2026-06-11 UTC, local runner)

```
bunx vitest run src/App.current-selection.test.tsx
Duration 90.31s (tests 66.48s)
26 tests
```

Slowest individual cases before split:

| Test | Approx. duration |
| --- | --- |
| `renders one selected-work dispatch history list with mixed inference and script-backed rows` | multi-second (large DOM expansion) |
| `supports rearranging shared-grid widgets without replacing graph selection` | 3.6s |
| `smoke tests the composed bento dashboard at a narrow viewport` | 3.4s |
| `shows selected state node details from the graph` | 3.5s |
| `keeps every current-selection kind on the canonical title and expandable section layout` | multi-kind graph walk |

## Unique observable behaviors (pre-split inventory)

| Behavior | Former owner | Post-split owner |
| --- | --- | --- |
| Mixed inference/script dispatch history rows | App shell | `work-item-card.test.tsx`, `workstation-detail-card.history.test.tsx` |
| Canonical title + expandable sections per selection kind | App shell | `current-selection-widget.canonical-section-layout.test.tsx` |
| Empty selected-work dispatch history | App shell | `selected-work-dispatch-history.test.tsx` |
| Trace unavailable when work has no trace ID | App shell | `trace-grid-card.test.tsx` |
| zh-CN labels for selection + trace enums | App shell | `current-selection-widget.localization.test.tsx`, `trace-grid-card.test.tsx` |
| Workstation active-execution filtering | App shell | `useCurrentSelection.selection-helpers.test.ts`, workstation widget tests |
| Editable workstation configuration across switches | App shell | `workstation-detail-card.editable-configuration.test.tsx` |
| Localized workstation editing options | App shell | `current-selection-widget.localization.test.tsx` |
| State-node detail from graph selection | App shell | `state-node-detail.test.tsx` |
| Work outcome chart series + timeline refresh | App shell | `work-chart.test.tsx`, `d3-information-card.test.tsx`, `useWorkOutcomeChart.component.test.ts` |
| Responsive bento widget catalog at 1366/1024/640 | App shell | `agent-bento.test.tsx`, `dashboard-screen.single-scroll.test.tsx` |
| Terminal place occupancy details | App shell | `state-node-detail.test.tsx`, `terminal-work-summary-detail.test.tsx` |
| Retained trace history unavailable copy | App shell | `trace-grid-card.test.tsx` |
| Cross-widget trace drill-down after work selection | App shell | **retained** in `App.current-selection.test.tsx` |
| Work vs workstation vs request selection contract through live graph | App shell | **retained** in `App.current-selection.test.tsx` |
| React Flow zoom does not break graph selection | App shell | **retained** in `App.current-selection.test.tsx` |
| Workstation vs work graph selection attributes | App shell | **retained** in `App.current-selection.test.tsx` |
| Bento layout persistence + stream refresh without losing selection | App shell | **retained** in `App.current-selection.test.tsx` |
| Trend widgets hidden when trace data is present | App shell | **retained** in `App.current-selection.test.tsx` |
| Narrow-viewport composed dashboard smoke | App shell | **retained** in `App.current-selection.test.tsx` |
| Terminal work selection updates trace card | App shell | **retained** in `App.current-selection.test.tsx` |

## After timing (2026-06-11 UTC, same local runner)

```
bunx vitest run src/App.current-selection.test.tsx
Duration 31.99s (tests 23.05s, 10 cases down from 26)
~65% faster wall clock than the pre-split app-shell suite on this runner
```

Feature-owned replacement:

```
bunx vitest run src/features/current-selection/components/widget-tests/selection/current-selection-widget.canonical-section-layout.test.tsx
Duration ~0.7s (canonical workstation/work-item/request/state layout)
```

Worker, resource, and work-type canonical sections remain in existing widget suites
(`current-selection-widget.test.tsx`, `current-selection-widget.work-type.test.tsx`).

## Rationale

Whole-dashboard `renderApp` remains only where multiple dashboard surfaces must be
wired together (graph viewport, bento grid, trace card, terminal work, event stream).
Widget-local layout, dispatch history, chart, and locale behavior now lives in
feature-owned suites that mount `CurrentSelectionWidget` or narrower cards directly.
