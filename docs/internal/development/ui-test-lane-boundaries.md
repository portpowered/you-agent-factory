# UI Test Lane Boundaries

This document defines what each dashboard UI verification lane must prove, which
observable contracts belong at each layer, and the minimum browser-integration
coverage for import/export and graph-editing flows. Use it when adding regressions
or deciding whether an assertion belongs in unit/jsdom coverage or Chromium.

Canonical lane names match [development.md](development.md) and the required CI
jobs: **UI Coverage** (jsdom-oriented Vitest + Bun unit), **UI Browser
Integration** (`ui/integration/*.integration.test.mjs`), and optional Storybook
lanes for visual states.

## Observable contracts by layer

Place regressions at the **shallowest layer that still observes the
customer-visible contract**. Do not repeat the same assertion in a more expensive
lane unless the cheaper lane cannot observe it reliably.

| Layer | Environment | Prove | Do not prove here |
| --- | --- | --- | --- |
| **Unit** | Node or minimal DOM | Pure helpers, fixture builders, replay catalog metadata, PNG encode/decode helpers, graph projection math, API adapter normalization | Full React trees, layout pixels, real downloads, preview servers |
| **Component** | jsdom + Testing Library | One widget or hook: accessible names, controlled form state, local error/empty/loading UI, mocked fetch bodies for that card | Cross-card navigation, session tabs, real Chromium gestures, OS download plumbing |
| **Covered feature / app shell** (`App.*.test`, card integration under `ui/src`) | jsdom + `renderApp` or feature harness | Activation request bodies (`PUT /factory-sessions/...`), import-mode selection wiring, event-stream refresh after activation, graph-editor draft mutations with harness doubles, theme token resolution on `document.documentElement` | Real PNG bytes on disk, preview-server startup, multi-tab session chrome, Playwright timing |
| **Browser integration** | Chromium + lane-owned preview/API harness | Durable end-to-end behavior: real downloads, drag/drop with filesystem paths, multi-session tabs, replay fixtures through live `GET /factory-sessions/{session_id}/events` (compatibility `GET /events` remains for legacy harness coverage only), final visible graph/workstation state after save | Re-proving CSS token math already covered by compiled-theme jsdom tests; repeating every jsdom import-dialog copy string |
| **Storybook** | Built `storybook-static` + play functions | Visual states, isolated card stories, responsive layout snapshots where jsdom is insufficient | Full factory-session API contracts (prefer jsdom or browser lanes above) |

### Browser integration prioritizes durable behavior

Browser integration (`make ui-integration-test`) should assert outcomes that
depend on a real browser runtime:

- **Saved payloads** — request bodies captured by the lane API server after
  export, import activation, or graph save.
- **Network effects** — session-scoped `PUT`/`POST` routing, per-tab event-stream
  subscription, activation modes (`REPLACE_CURRENT`, `UPSERT_NAMED_AND_ACTIVATE`).
- **Downloads and imports** — captured download blobs, dropped PNG paths, import
  preview image rendering from real file bytes.
- **Final visible state** — selected workstation buttons, graph nodes after
  edit/save, replay-driven headings after stream catch-up.

Avoid browser-only coverage for:

- Static copy that jsdom already asserts on the same component.
- Computed-style checks on compiled theme tokens without dashboard wiring
  (use `styles.page-shell-background.test.ts` or `theme-role-regression.test.ts`).
- Transient UI such as in-flight button labels (`Activating factory...`),
  animation timing, or spinner frames unless the durable outcome is otherwise
  unreachable.

## Minimum browser contracts

### Import and export (PNG)

| Contract | Cheaper lane owner | Minimum browser proof |
| --- | --- | --- |
| `writeFactoryExportPng` / `readFactoryImportPng` payload shape | Unit tests under `ui/src/features/export` and `ui/src/features/import` | Not required when unit coverage exists |
| Import preview dialog shows embedded factory name | jsdom `App.import.test.tsx` (mocked PNG read) | One real roundtrip: export PNG from dashboard, drop file, preview image visible |
| `REPLACE_CURRENT` preserves session factory name | `App.import.test.tsx` plus import-activation API tests | Not repeated when the activation body is directly asserted |
| `UPSERT_NAMED_AND_ACTIVATE` target naming | `App.import.test.tsx`, import-activation API, and import activation hook tests | Not repeated when mode and target naming are directly asserted |
| Non-default session tab scoping | Not app-shell default | `factory-import-second-session.integration.test.mjs` |
| Full export dialog field validation and status copy | jsdom `App.import.test.tsx` or component tests | **Not** repeated in browser — browser stops at download + activation payload |

### Graph editing

| Contract | Cheaper lane owner | Minimum browser proof |
| --- | --- | --- |
| Draft graph mutations, edge handles, validation targets | `react-flow-current-activity-card*.test.ts*`, `graph-editor-harness*.test.ts` | Not required for pure graph math |
| Edit mode toggling and local panel state | jsdom graph-editor card tests | Only when behavior depends on real layout measurement or Chromium hit targets |
| Save topology `PUT` with canonical session name | `useFactoryDocumentSave.test.tsx` | Browser graph-save coverage verifies the network payload shape without repeating name preservation |
| Node placement after add/delete in real viewport | Partial jsdom coverage | `factory-graph-editor-node-placement.integration.test.mjs`, `factory-graph-editor-selection-no-panel-delete.integration.test.mjs` |
| Session switch retaining graph selection | Cross-session shell behavior | `factory-graph-editor-session-switch.integration.test.mjs` |
| Export after graph edit + PNG roundtrip | jsdom activation bodies | `factory-graph-editor.integration.test.mjs` |

## Redundancy policy

When the same observable assertion appears in multiple lanes:

1. Keep it in the **cheapest** lane that still observes the contract reliably.
2. Remove or narrow the duplicate in the expensive lane.
3. Document the removal in the PR / closeout notes when the deleted coverage
   was previously called out in slow-file reports.

### Applied examples (prd-ui-ci-acceleration-and-test-rationalization-005)

| Removed / narrowed | Kept in cheaper lane | Rationale |
| --- | --- | --- |
| Dispatch history, locale, workstation editing, state-node detail, work-outcome chart refresh, responsive widget catalog, and terminal occupancy cases in `App.current-selection.test.tsx` | Feature/widget suites under `ui/src/features/current-selection/**` and `ui/src/features/work-outcome/**` | Whole-dashboard `renderApp` duplicated widget-local behavior; see `ui-current-selection-app-shell-split-closeout.md` for inventory and before/after timing. |
| Canonical multi-kind section layout walk in `App.current-selection.test.tsx` | `current-selection-widget.canonical-section-layout.test.tsx` | Exercises the same expandable section contract through `CurrentSelectionWidget` without mounting the full dashboard graph. |

### Applied examples (prd-ui-ci-acceleration-and-test-rationalization-004)

| Removed / narrowed | Kept in cheaper lane | Rationale |
| --- | --- | --- |
| `page-shell-background.integration.test.mjs` | `styles.page-shell-background.test.ts` | Foundation background is a compiled CSS token contract; full preview mount added no durable behavior beyond jsdom. |
| `header-palette-menu-selected-state.integration.test.mjs` | `theme-role-regression.test.ts`, `dashboard-header.test.tsx` | Selected palette menu foreground is token/contrast coverage; browser static HTML injection duplicated theme tests. |
| Export-dialog status copy in `App.import.test.tsx` roundtrip | Browser import/export suites | jsdom roundtrip now drops a harness-built PNG and asserts activation `PUT` only; export UI copy stays in component/jsdom tests. |

## Choosing a lane (quick reference)

```
New regression?
├─ Pure function / fixture? → unit test near the module
├─ Single card/widget? → component test with harness doubles
├─ Needs full App fetch/stream wiring but not real browser? → App.* or renderApp jsdom test
├─ Needs download, preview server, multi-tab, or real drop path? → ui/integration/*.integration.test.mjs
└─ Visual-only story state? → Storybook story + optional Storybook Vitest lane
```

## Related references

- [development.md](development.md) — verification tiers, CI lane table, harness modules
- [ui-browser-integration-stability.md](ui-browser-integration-stability.md) — sequential browser suites, concurrent-lane isolation, durable wait helpers
- [ui-coverage-speed-closeout.md](ui-coverage-speed-closeout.md) — timing history for slow files
- `ui/src/testing/replay-fixture-catalog.ts` — replay surface ownership across layers
