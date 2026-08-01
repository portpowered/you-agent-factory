# UI Test Lane Boundaries

This document defines what each dashboard UI verification lane must prove, which
observable contracts belong at each layer, and the minimum browser-integration
coverage for import/export and graph-editing flows. Use it when adding regressions
or deciding whether an assertion belongs in unit/jsdom coverage or Chromium.

Canonical lane names match [development.md](development.md): **Unit** (Node),
**Component** (browserless DOM emulation), and **Browser** (`ui/integration/`). The required UI
Coverage lane runs the Node unit project only; component, browser, and
performance tests are separate confidence lanes and do not affect its threshold.

The component lane runs browserless tests in two compatibility groups: Bun is
the default for tests without Vitest-only capabilities, and Vitest temporarily
owns files that still use `vi` APIs or explicit Vitest environment directives.
`bun run test:component` measures both groups together and fails when their total
wall time exceeds 150 seconds (override with `UI_COMPONENT_MAX_DURATION_MS`).
CI runs this as the dedicated **Frontend Component** lane with a 300-second
hosted-Linux ceiling to account for its slower DOM emulation; local development
retains the tighter 150-second default. Performance tests do not share either
budget or lane.

Component setup is capability-based. `ui/src/testing/vitest.setup.ts` contains
only the guarded console policy, Testing Library configuration, and React act
flag. Tests that render dependencies requiring `ResizeObserver`, download-anchor
behavior, or browser storage import
`ui/src/testing/vitest-dom-capabilities.setup.ts` explicitly. Monaco is replaced
through test-only resolver aliases before component modules load. Generic
helpers under `ui/src/testing/` cannot import `DashboardScreen`; dashboard
composition rendering belongs to
`ui/src/features/dashboard/components/testing/dashboard-screen-test-render.tsx`.

## Dashboard-unit throughput baselines

Performance work on the Node unit lane must keep timing evidence separate from
observer-heavy resource probes. Use the canonical `dashboard-unit` project and
its package-script worker/retry policy, then record at least three comparable
runs with the same classified file/test workload. Freeze that workload with
`vitest list --project=dashboard-unit --json`, recording both the unique file
count and a hash of the normalized sorted file paths. Keep process-tree or
memory sampling in a separate probe when the sampler materially changes wall
time; do not include that probe in the timing median.

Vitest 4's summary labels aggregate `collectDuration` as `import`. Treat that
field as collection/import preparation in reports and do not mistake its
worker-aggregate value for sequential wall time. The `tests` field is the
aggregate test-body duration; `setup`, `transform`, and `environment` are
reported with the same aggregate semantics. The reproducible baseline and the
current workload fingerprint live in
[`dashboard-unit-baseline.md`](plans/ui-test-latency/dashboard-unit-baseline.md).

When a Vitest project extends the root config, an empty project array does not
necessarily clear a root array such as `setupFiles`. If a Node-only project
must avoid shared browser setup, gate that root value on the explicit project
selection and keep the component/combined path unchanged.

## Observable contracts by layer

Place regressions at the **shallowest layer that still observes the
customer-visible contract**. Do not repeat the same assertion in a more expensive
lane unless the cheaper lane cannot observe it reliably.

| Layer | Environment | Prove | Do not prove here |
| --- | --- | --- | --- |
| **Unit** | Node, without DOM setup | Pure helpers, fixture builders, replay catalog metadata, PNG payload helpers, graph projection math, API adapter normalization | React rendering, DOM globals, layout pixels, real downloads, preview servers |
| **Component** | jsdom + Testing Library | One widget or hook: accessible names, controlled form state, local error/empty/loading UI, mocked fetch bodies for that card | Cross-card navigation, session tabs, real Chromium gestures, OS download plumbing |
| **Dashboard composition component** (`*.component.test.tsx`) | jsdom + the dashboard test shell | Cross-owner session, replay, checkpoint, and trace wiring that cannot be observed through one widget or hook; the shell mounts `DashboardScreen`, never `App.tsx` | Route-specific packaged-factory/emulator trees, real PNG bytes on disk, preview-server startup, Playwright timing |
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
  (use `styles.page-shell-background.component.test.ts` or `theme-role-regression.component.test.ts`).
- Transient UI such as in-flight button labels (`Activating factory...`),
  animation timing, or spinner frames unless the durable outcome is otherwise
  unreachable.

## Minimum browser contracts

### Import and export (PNG)

| Contract | Cheaper lane owner | Minimum browser proof |
| --- | --- | --- |
| `writeFactoryExportPng` / `readFactoryImportPng` payload shape | Unit tests under `ui/src/features/export` and `ui/src/features/import` | Not required when unit coverage exists |
| Import preview dialog shows embedded factory name | `dashboard-import-preview-dialog.test.tsx` with a mocked PNG read | One real roundtrip: export PNG from dashboard, drop file, preview image visible |
| `REPLACE_CURRENT` preserves session factory name | jsdom activation-body assertions | `factory-name-preservation.integration.test.mjs` captures PUT body from real export/import |
| `UPSERT_NAMED_AND_ACTIVATE` target naming | jsdom mode-selection test | `factory-name-preservation.integration.test.mjs` create-new-named scenario |
| Non-default session tab scoping | Not app-shell default | `factory-import-second-session.integration.test.mjs` |
| Full export dialog field validation and status copy | `export-factory-dialog.test.tsx` | **Not** repeated in browser — browser stops at download + activation payload |

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
| `page-shell-background.integration.test.mjs` | `styles.page-shell-background.component.test.ts` | Foundation background is a compiled CSS token contract; full preview mount added no durable behavior beyond jsdom. |
| `header-palette-menu-selected-state.integration.test.mjs` | `theme-role-regression.component.test.ts`, `dashboard-header.test.tsx` | Selected palette menu foreground is token/contrast coverage; browser static HTML injection duplicated theme tests. |
| Export-dialog status copy in `App.import.test.tsx` roundtrip | Browser import/export suites | jsdom roundtrip now drops a harness-built PNG and asserts activation `PUT` only; export UI copy stays in component/jsdom tests. |

## Choosing a lane (quick reference)

```
New regression?
├─ Pure function / fixture? → colocated *.test.ts or *.unit.test.ts in the Node project
├─ Single card/widget? → colocated *.component.test.tsx with harness doubles
├─ Needs cross-owner dashboard stream/replay wiring? → feature-owned dashboard composition component test
├─ Needs download, preview server, multi-tab, or real drop path? → current ui/integration/*.integration.test.mjs browser lane
└─ Visual-only story state? → Storybook story + optional Storybook Vitest lane
```

Prefer the narrowest import that expresses ownership. A focused public entry
point such as `timeline/public/store` is useful when multiple consumers need a
stable contract; a direct focused cross-feature import is also permitted when it
prevents module fan-out. Aggregate feature `public/index` barrels and the general
shared-component barrel are prohibited because small tests otherwise compile
unrelated stores, persistence code, and widgets. The feature-boundary scanner is
advisory by default during this latency migration; set
`AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT=1` for a legacy-policy audit.

## Related references

- [development.md](development.md) — verification tiers, CI lane table, harness modules
- [ui-browser-integration-stability.md](ui-browser-integration-stability.md) — sequential browser suites, concurrent-lane isolation, durable wait helpers
- [ui-coverage-speed-closeout.md](ui-coverage-speed-closeout.md) — timing history for slow files
- `ui/src/testing/replay-fixture-catalog.ts` — replay surface ownership across layers
