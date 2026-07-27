# UI Test Coverage Latency Implementation Plan

## Outcome

Frontend verification uses three names and three explicit execution contracts:

| Test class | Runtime | Purpose | Contributes to coverage |
| --- | --- | --- | --- |
| Unit tests | Node | Pure operations, projections, reducers, selectors, parsers, messages, and transport normalization | Yes |
| Component tests | jsdom + Testing Library | Rendered component and hook behavior without launching a browser | Yes |
| Browser tests | Chromium + Playwright | Behavior that depends on a real browser, filesystem downloads, layout, pointer input, or preview-server/network integration | No |

Performance tests are a fourth execution lane, not a fourth coverage class. A
performance test may exercise pure or rendered code, but it does not contribute
to coverage and does not block the fast author or pull-request coverage path.

The intended result is a coverage lane that runs only unit and component tests,
does not install or launch a browser, and provides useful pull-request feedback
within a fixed latency budget. Browser and performance confidence remains
available through separately named commands and scheduled or ownership-routed
CI jobs.

## Current problem

- The covered corpus contains hundreds of files and uses jsdom as the default
  environment, including for pure TypeScript logic.
- Whole-application `App.*` component tests repeatedly mount the dashboard,
  initialize Query clients and global stores, seed replay state, install fetch
  and EventSource doubles, and wait for asynchronous effects.
- Browser tests are already a separate command, but the current naming calls
  most non-browser behavior "UI Coverage" or "unit" even when it is a mounted
  component test.
- Performance budgets currently run inside default unit/component and covered
  passes, making timing-sensitive stress fixtures part of normal correctness
  feedback.
- Coverage uses ten count-based shards. Shard execution is imbalanced and CI
  runner setup/queueing consumes much of the parallelism benefit.

## Behavioral contracts

### Unit test contract

- Runs without jsdom, Playwright, browser installation, preview servers, or
  real network listeners.
- Owns canonical state operations and UI projections that can be verified
  without rendering React.
- Is the preferred owner for graph mutation, replay, validation, formatting,
  and serialization behavior.

### Component test contract

- Runs in jsdom with Testing Library and shared test providers.
- Mounts the narrowest component or hook that exposes the behavior.
- Uses an extracted dashboard-shell seam when the observable contract crosses
  session, stream, routing, or multi-widget composition boundaries.

### Browser test contract

- Runs in Chromium through the repository browser harness.
- Owns browser-only behavior such as real downloads, file drops, viewport
  interaction, pointer behavior, preview-server integration, and durable
  multi-session browser flows.
- Never contributes to source coverage thresholds.

### Performance test contract

- Runs through `make test-ui-performance` / `bun run test:performance`.
- Uses explicit fixtures, budgets, worker policy, and timeouts.
- Is excluded from default unit, component, and coverage discovery.
- Runs in the opt-in extended verification tier until CI routing gains a
  dedicated scheduled performance job.

## Filesystem and packaging contract

Test location follows ownership, while the filename selects the execution
lane. There will not be a repository-wide `component-tests` directory. A
separate component-test tree would obscure the production owner, duplicate the
feature hierarchy, and make moves or deletions easier to miss.

### Dashboard source tests

Unit and component tests remain next to the smallest production module that
owns the contract:

```text
ui/src/features/<feature>/
  components/
    <component>.tsx
    <component>.component.test.tsx
  hooks/
    <hook>.ts
    <hook>.component.test.tsx
  lib/
    <operation>.ts
    <operation>.unit.test.ts
  messages/
    <messages>.ts
    <messages>.unit.test.ts
  state/
    <store-or-projection>.ts
    <store-or-projection>.unit.test.ts
```

The same rule applies outside feature directories:

- API adapter and normalization unit tests stay under the owning
  `ui/src/api/<domain>/` directory.
- Shared UI component tests stay beside their implementation under
  `ui/src/components/`.
- `ui/src/testing/` contains reusable providers, fixtures, shims, and test
  builders only. It is not a destination for product-behavior tests.
- Fixtures used by only one owner stay in that owner's `fixtures/` directory.
  Cross-feature fixtures may live in `ui/src/testing/fixtures/`, with an
  explicit owner documented in the fixture module.

The suffix, rather than whether a file happens to use `.ts` or `.tsx`, is
authoritative. Hooks often have `.ts` implementations but still require React
rendering, so a rendered hook test is a component test.

| Filename | Lane | Environment |
| --- | --- | --- |
| `*.unit.test.ts` | Unit | Node |
| `*.component.test.tsx` or `*.component.test.ts` | Component | jsdom |
| `*.performance.test.ts` | Performance | Node unless the budget explicitly requires rendering |
| `*.browser.test.mjs` or `*.browser.test.ts` | Browser | Chromium + Playwright |

### Browser test tree

Browser tests are intentionally separate because they own compiled-application,
preview-server, browser-context, filesystem, and real-backend behavior rather
than one source module. The existing `ui/integration/` standard remains the
root, but the flat test inventory will be migrated into domain groups:

```text
ui/integration/browser/
  graph-editor/
    factory-graph-editor.browser.test.mjs
    node-placement.browser.test.mjs
    session-switch.browser.test.mjs
  sessions/
    dashboard-session-recovery.browser.test.mjs
    dashboard-session-tabs.browser.test.mjs
  replay/
    event-stream-replay.browser.test.mjs
  import-export/
    factory-import-second-session.browser.test.mjs
    factory-name-preservation.browser.test.mjs
  hosted/
    hosted-exact-session-replay.browser.test.mjs
  support/
    browser-test-harness.mjs
  fixtures/
```

Harness unit tests such as port-allocation or wait-pattern parsing do not belong
to the browser lane merely because they test browser support. They remain Node
unit tests beside `support/` and use the `.unit.test.mjs` suffix. Tests that
launch the real backend stay in the relevant browser domain and receive the
additional CI `real-backend` routing label; they do not become a fourth test
class.

### Performance test placement

A feature-owned budget stays in a `performance/` directory next to the code it
measures. The two current owners will therefore become:

```text
ui/src/features/factory-graph-editor/lib/layout/performance/
  factory-graph-layout.performance.test.ts
ui/src/features/timeline/state/timeline/performance/
  replay-retained-memory.performance.test.ts
```

Only a budget that spans multiple production domains may use a future
`ui/performance/` root. That root must not contain ordinary correctness tests.

### Public package tests

Each package under `ui/packages/<package>/` owns its test files and Vitest
configuration. Package tests remain colocated under that package's `src/` tree
and use the same suffix contract. They must not be pulled into the dashboard
project through root aliases merely to obtain coverage.

```text
ui/packages/components/src/<category>/
  <component>.tsx
  <component>.component.test.tsx
ui/packages/factory-replay/src/<area>/
  <projection>.ts
  <projection>.unit.test.ts
```

Every public package will expose `test:unit`, `test:component` when applicable,
and `test:coverage` scripts. The root runner composes those package commands and
merges their coverage artifacts. This keeps package environment, React alias,
and dependency-boundary rules local to the package that owns them.

## Lane discovery and enforcement

The system does not depend on contributor memory or imports inferred at runtime.
Lane ownership is implemented by these files:

```text
ui/vite.config.ts                   # shared aliases and coverage thresholds
ui/vitest.lanes.config.ts           # dashboard unit/component project discovery
ui/scripts/check-ui-test-lanes.mjs  # suffix/default and import-boundary audit
ui/package.json                     # canonical unit/component/performance commands
```

`vitest.lanes.config.ts` defines the named `dashboard-unit` and
`dashboard-component` projects. Public packages retain their package-owned
Vitest configurations. Coverage selects `dashboard-unit` explicitly. Browser,
component, and performance tests are never members of the covered project list.

`check-ui-test-lanes.mjs` will fail when:

- a TypeScript unit test imports DOM runners or requests a DOM environment;
- a unit test imports Testing Library React, React DOM, jsdom, Playwright, or a
  browser harness;
- a component test launches Playwright, a preview server, or a real backend;
- a component test launches a browser runner; or
- a test restores retired `App.tsx` test ownership;
- global Vitest setup installs optional browser or editor capabilities;
- generic test helpers import `DashboardScreen`; or
- production or test code imports the aggregate dashboard or timeline public
  barrel instead of a focused public entry point.

Tests stay beside their production owner. Unsuffixed `*.test.ts` files are the
backward-compatible Node unit default, while `*.test.tsx` files are the component
default. TypeScript tests that need DOM APIs are explicitly named
`*.component.test.ts`. There is no separate component-test directory and no
legacy allowlist to keep synchronized.

The command surface becomes:

| Command | Projects selected |
| --- | --- |
| `bun run test:unit` | Dashboard Node unit project |
| `bun run test:component` | Dashboard jsdom component project |
| `bun run test:coverage` | Dashboard Node unit coverage, then aggregate threshold |
| `bun run test:integration` | Existing `ui/integration/*.integration.test.mjs` browser lane |
| `bun run test:performance` | `**/*.performance.test.*` |
| `bun run check:test-lanes` | Static classification and boundary audit |

The CI workflow calls these commands rather than reconstructing glob rules in
YAML. This leaves one authoritative classification mechanism for local runs,
coverage shards, and CI.

## App composition-root end state

The target is zero `ui/src/App.*.test.tsx` files. `ui/src/App.tsx` should contain
only top-level provider and application-shell composition; direct coverage of
that wiring is lower value than repeatedly mounting the complete dashboard.
App-owned behavior must first be extracted into a named production seam, then
tested beside that seam.

The concrete destinations are:

| Current App concern | Production owner and component-test destination |
| --- | --- |
| Session resolution, stream replacement, recovery | `ui/src/features/dashboard/session/` |
| Multi-session tab and timeline isolation | `ui/src/features/header/` plus `ui/src/features/dashboard/session/` |
| Timeline controls and selected-tick behavior | `ui/src/features/timeline/` |
| Current-selection details and workstation requests | `ui/src/features/current-selection/` |
| Submit and follow-up request behavior | `ui/src/features/submit-work/` |
| Graph projection and selection synchronization | `ui/src/features/factory-graph-editor/` and `ui/src/features/workflow-activity/` |
| Import/export activation and refresh routing | `ui/src/features/import/` and `ui/src/features/export/` |

If one observable contract genuinely crosses several of those owners, extract a
small `DashboardRuntimeShell` (or equivalently named existing seam) under
`ui/src/features/dashboard/components/` and place its component test beside it.
That shell test may compose multiple providers, but it must stub graph layout,
bento, trace, or work-outcome regions that are not part of the assertion. It is
not a renamed copy of the present full-App harness.

The removal rule for an App case is evidence-based:

1. Identify the asserted contract and its production owner.
2. Add or confirm the focused unit/component evidence at that owner.
3. Retain a single shell-level wiring assertion only if focused evidence cannot
   observe the cross-owner connection.
4. Delete the duplicate App case and record the before/after CI file cost.
5. Move real viewport, pointer, file, or browser-context behavior to the browser
   tree instead of preserving it in jsdom.

## Coverage design

Coverage will eventually be split into two non-browser projects:

1. Unit coverage in Node.
2. Component coverage in jsdom.

The merge step owns the aggregate threshold. Browser sources, browser harnesses,
performance tests, stories, generated API code, and test-support-only modules
remain outside the threshold denominator. Initial thresholds will be set from a
green measurement of the curated unit/component corpus. The existing 92.97%
line/statement floor is not a rollout constraint; any reduction must be visible
in the change and accompanied by a retained critical-contract manifest.

Critical contracts that must remain represented include:

- API normalization and session-scoped request construction.
- Factory replay, event ordering, checkpoint durability, and payload lineage.
- Graph mutation, validation, canonical projection, and save preparation.
- Component loading, failure, empty, edit, discard, and save behavior.
- A small app-owned set for session resolution, stream replacement, recovery,
  and cross-widget selection behavior that cannot be observed narrowly.

## Latency targets

| Lane | Target |
| --- | ---: |
| Unit tests | 90 seconds p95 |
| Component tests | 150 seconds p95 |
| Merged unit + component coverage | 180 seconds p95 including setup |
| Browser critical subset | 180 seconds p95 when ownership-routed |
| Default UI pull-request critical path | 5 minutes p95 |

Measure targets across at least five green `ubuntu-latest` runs. Local timing is
diagnostic only and must use the repository-pinned Node and Bun versions.

## Implementation stories

### UI-COV-001 — Publish trustworthy test-cost evidence

As a maintainer, I can see real unit, component, and browser file costs so that
lane changes are based on executed tests rather than reporter fixtures.

Acceptance criteria:

- Timing data comes from structured Vitest output rather than parsing arbitrary
  stdout lines.
- Shard timing artifacts are uploaded, downloaded, and merged.
- Synthetic reporter fixture strings cannot appear as executed test files.
- CI summaries group costs as `unit`, `component`, `browser`, or `performance`.

### UI-COV-002 — Separate performance verification

As a contributor, timing-sensitive large-fixture tests do not delay normal
unit/component coverage, while a named command still verifies their budgets.

Acceptance criteria:

- `make test-ui-performance` runs the graph-layout and retained-memory budgets.
- Default unit/component and coverage commands exclude performance test paths.
- `make verify-extended` includes the named performance lane.
- Focused runner-contract and Make smoke tests pass.

### UI-COV-003 — Establish explicit unit and component projects

As a contributor, pure tests run in Node and rendered tests run in jsdom so that
test names describe their actual behavior and avoid unnecessary DOM setup.

Acceptance criteria:

- Repository-owned manifests classify every covered test as unit or component.
- Pure tests do not install DOM globals.
- Component tests retain the shared jsdom setup and console guard.
- Unclassified covered tests fail a lane-audit check.

### UI-COV-004 — Create light unit/component coverage

As a contributor, required coverage completes quickly without launching a
browser and still proves the critical contracts listed above.

Acceptance criteria:

- The coverage command launches no Playwright process, preview server, or
  browser installation.
- Only the unit and component manifests execute under coverage.
- Thresholds are recorded from a green baseline and enforced at merge.
- The lane completes within the 180-second p95 target or reports the measured
  gap with the slowest owning manifest.

### UI-COV-005 — Reconcile whole-App component tests

As a contributor, full dashboard mounts remain only for app-owned integration
contracts, while feature-local behavior is proven by faster unit or component
tests.

Acceptance criteria:

- No `ui/src/App.*.test.tsx` files remain at the end of migration.
- Every retained shell-level case has a documented cross-owner contract.
- Widget-local, formatting, projection, and validation behavior moves to unit
  or focused component owners before duplicate App assertions are removed.
- Retained shell cases use one minimal seeded seam per compatible scenario and
  avoid real graph layout unless layout is the behavior under test.
- Before/after CI file timings are recorded for each reconciled suite.

### UI-COV-006 — Route browser tests by ownership

As a contributor, browser tests run when browser-owned surfaces change, without
blocking unrelated unit/component changes.

Acceptance criteria:

- Graph, import/export, session/recovery, and replay browser groups have explicit
  path ownership.
- An always-present aggregate check reports passed or not applicable.
- The full browser corpus remains available on main/nightly and through a named
  local command.

### UI-COV-007 — Right-size coverage parallelism

As a maintainer, coverage uses the smallest runner count that satisfies the
latency target.

Acceptance criteria:

- Shards are balanced by measured duration rather than file count.
- One-, two-, four-, and current ten-shard costs include setup and queue time.
- The selected configuration meets the p95 target without reducing reliability.

## Task breakdown

1. Complete UI-COV-002 first to remove known performance work from coverage.
2. Fix structured timing and shard artifact transport in UI-COV-001.
3. Add the unit/component classification manifest and audit in UI-COV-003.
4. Measure and adopt the curated non-browser coverage contract in UI-COV-004.
5. Reconcile `App.*` suites incrementally under UI-COV-005, starting with
   current selection, layout graph, replay dispatch, follow-up submission, and
   workstation-request replay.
6. Add browser ownership routing in UI-COV-006 after the non-browser required
   lane is stable.
7. Re-measure and simplify sharding in UI-COV-007.

## Implementation progress — 2026-07-26

The whole-App decomposition is implemented locally:

- `ui/src/App.*.test.tsx` decreased from 19 files to zero.
- The test dashboard shell no longer imports `App.tsx`, so dashboard session and
  replay tests do not compile the packaged-factory or emulator route trees.
- Route selection moved to
  `ui/src/features/app-routing/lib/resolve-app-surface.unit.test.ts` and runs in
  the Node unit project.
- The retained cross-owner cases are component tests under dashboard and trace
  ownership:
  - `dashboard-screen.selected-tick.component.test.tsx`
  - `dashboard-replay-wiring.component.test.tsx`
  - `useDashboardSnapshot.isolated.bun.component.test.tsx`
  - `usePersistedTimelineCheckpoint.bun.component.test.tsx`
  - `dashboard-trace-wiring.component.test.tsx`
- The former dashboard session/timeline isolation shell was removed after its
  three contracts were reconciled with the focused timeline-entry,
  dashboard-session-store/provider, header-tab, and dashboard-snapshot tests.
- The original 64 full-App helper mounts were reconciled to 12 dashboard-only
  composition mounts. Widget, hook, projection, routing, formatting, import,
  export, submit, graph layout, and request-detail duplicates were deleted only
  after their focused owners were identified and run.
- `ui/vitest.lanes.config.ts` now provides named `dashboard-unit` and
  `dashboard-component` projects. All colocated `*.test.ts` tests default to the
  Node unit project; `*.test.tsx` and `*.component.test.ts` tests belong to the
  jsdom component project. Performance directories remain excluded from both.
- The legacy TypeScript corpus was executed exhaustively under Node. Of 403
  candidate files, 374 passed immediately. The 29 failures genuinely exercised
  DOM or browser APIs and were renamed `*.component.test.ts`; two additional
  files with DOM environment directives were renamed the same way. No temporary
  legacy classification manifest was needed.
- `ui/scripts/check-ui-test-lanes.mjs` enforces the suffix defaults, rejects DOM
  runner imports and DOM environment directives in unit tests, and continues to
  prohibit `App.tsx` ownership and browser runners in component tests.
- `bun run test:unit` and `make ui-test` now select only `dashboard-unit` with a
  Node environment. `bun run test:component` selects the jsdom project. Coverage
  selects `dashboard-unit` explicitly; component, browser, and performance tests
  are independent lanes.

Local pinned-Node evidence after decomposition:

| Verification | Result |
| --- | ---: |
| Route unit project | 5 tests, 183ms total |
| Retained dashboard/trace component projects | 21 tests, 14.58s total |
| Layout/graph focused replacement set | 61 tests, 7.02s total |
| Submit/bento/outcome focused replacement set | 52 tests, 8.80s total |
| Export/import/header focused replacement set | 91 tests, 11.19s total |
| Session/stream/checkpoint focused replacement set | 50 tests, 10.19s total |
| Full non-browser legacy corpus after decomposition | 821 files / 5,758 tests, 573.19s wall time |
| Exhaustive migrated Node unit project | 372 files / 3,111 tests, 44.80s wall time |
| Node-only coverage baseline | 54.58% statements / 46.22% branches / 52.42% functions / 54.87% lines |

These local values are implementation evidence, not the final CI latency
baseline. UI-COV-001 and UI-COV-007 still own structured CI timing and shard
right-sizing. The full-corpus run reported 706.02 aggregate worker-seconds of
imports and 195.98 aggregate worker-seconds of environment setup, making
legacy unit/component classification the next dominant latency opportunity.
The migration reduces the canonical short UI lane to the 44.80-second Node
project and removes jsdom initialization from that path (0.02 aggregate
worker-seconds of environment work in the final proof run). Coverage floors are set
slightly below the measured baseline at 54% statements, 46% branches, 52%
functions, and 54% lines. This is an intentional coverage reduction in exchange
for removing the 425-second component lane from the required coverage path.

### Component-latency follow-up

The first component-latency pass preserves the existing worker configuration
and does not introduce another required/full tier. It changes ownership and
import behavior instead:

- Global setup is minimal. ResizeObserver, browser storage, anchor, and command
  capabilities are explicit in the 61 component files that proved they need
  them during the exhaustive run.
- Monaco modules are resolved to lightweight test modules before production
  components import them. The test setup file no longer imports Monaco.
- Timeline and dashboard aggregate barrels were replaced with focused public
  entry points for stores, stream identity, checkpoint persistence, session
  context, topology replay, and screen composition.
- Generic app-shell helpers accept children and no longer import
  `DashboardScreen` or its widget graph. The feature-owned dashboard renderer
  owns that import.
- Pure hook decisions now have Node seams for current-selection derivation,
  detail state, reconnect cursors, topology stability, graph-controller
  composition, graph-session toggles, selection gestures, and stable guard-row
  keys.

Measured locally with the same pinned runtime and existing command settings:

| Measurement | Before | After |
| --- | ---: | ---: |
| Component project | 375 files / 2,180 tests / 424.38s | 363 files / 2,122 tests / 418.03s |
| Node unit project | 372 files / 3,111 tests / 44.80s | 387 files / 3,188 tests / 44.40s |
| Combined unit + component wall time | 469.18s | 462.43s |

The component lane improved by 6.35 seconds against the immediately preceding
green run and by 7.32 seconds against the original 425.35-second baseline. The
Node lane did not inherit the removed jsdom latency. Assertion work is only
111.89 aggregate seconds in the final component report, so file startup and
remaining production import graphs are still the dominant reconciliation
target. Future reductions should continue extracting whole jsdom files or
mocking heavy rendering boundaries before imports; shortening already-small
hook assertions will not materially change wall time.

## App component-test investigation

### Measured shape

The current `ui/src/App*.test.tsx` corpus contains:

- 19 files and approximately 6,674 lines of test code.
- 73 directly declared `it` / `test` cases, plus table-driven cases.
- 64 calls that mount the complete app through `renderApp` or
  `renderAppWithDashboardShell`.
- 101 explicit `waitFor` calls and 122 `findBy*` queries.
- Hundreds of render, query, interaction, and async-settlement operations.

Recent covered CI file timings put the dominant App component tests at:

| App component test | Covered file time |
| --- | ---: |
| `App.current-selection.test.tsx` | 24.73s |
| `App.layout-graph.test.tsx` | 14.55s |
| `App.replay-dispatch-selection.test.tsx` | 13.20s |
| `App.follow-up-submit.test.tsx` | 11.15s |
| `App.replay-workstation-requests.test.tsx` | 10.56s |
| `App.timeline.test.tsx` | 9.88s |
| `App.localization-times.test.tsx` | 8.08s |
| `App.replay-stream.test.tsx` | 7.93s |

### Why the App component tests are slow

The cost is architectural rather than one slow assertion:

1. **Every case mounts the dashboard composition root.** `renderApp` creates a
   Query client, seeds the current Factory document under multiple query keys,
   installs fetch and EventSource doubles, reloads persisted dashboard layout,
   wraps session providers, seeds timeline state, and renders the app shell.
2. **Most cases render far more UI than they assert.** Unless a suite imports
   the workflow-activity and work-outcome stubs, a request-detail, locale, or
   submission assertion also pays for graph, bento, timeline, trace, and widget
   composition.
3. **React Flow and replay initialization are asynchronous.** App cases wait for
   graph nodes, dashboard headings, stream-derived state, seeded tick changes,
   or effect settlement before making assertions. Coverage instrumentation
   magnifies the transform and import cost of these dependency trees.
4. **Isolation cleanup is broad.** Each case clears Query clients, Testing
   Library DOM, bento/export/session/stream/timeline/selection stores, browser
   shims, module mocks, and stubbed globals. This is correct isolation, but it is
   expensive when feature-local behavior caused the full mount.
5. **Some assertions have narrower owners already.** Header localization,
   timeline controls, workstation request details, submit-work forms, graph
   projection semantics, and current-selection cards have focused component or
   unit suites. App coverage sometimes repeats those contracts while proving
   only a small amount of additional wiring.

Simply splitting App files or sharing one mounted app between unrelated cases
is not the reconciliation path. Splitting adds more setup, while shared mounts
make stateful failures order-dependent. The number and breadth of complete app
mounts must fall.

### Reconciliation path

#### Retain as extracted dashboard-shell component coverage

Keep a small set of cases whose observable result truly crosses ownership
boundaries, but move them beside an extracted dashboard-shell seam:

- Concrete Factory Session resolution and EventSource replacement.
- Quiet-stream recovery and exact-session remapping.
- Multi-session checkpoint and timeline isolation.
- One graph-to-current-selection-to-trace wiring path.
- One live replay-to-visible-dashboard projection path.
- Import/export activation routing when it crosses toolbar, session, query, and
  stream refresh ownership.

These retained cases should use a minimal shell with workflow graph, work
outcome, trace, or bento regions stubbed unless that region is part of the
contract. They must not import or render `App.tsx`.

#### Move to focused component tests

- Move request timestamp localization and invalid timestamp fallback from
  `App.localization-times` to the request-detail component owner; retain one
  locale-provider wiring smoke if needed.
- Move timeline enable/disable and fixed/current tick controls from
  `App.timeline` to `TickSliderControl` and timeline projection owners; retain
  one selected-tick dashboard composition case.
- Move submit form validation, multimodal request shaping, and failure-state
  preservation from `App.follow-up-submit` to submit-work component/hook owners;
  retain one app-level successful request routing case.
- Move workstation-request rendering and mixed script/inference row details
  from `App.replay-workstation-requests` to current-selection request-detail
  components; retain one replay-to-request-selection wiring case.
- Move graph node semantics and omitted-edge handling from `App.layout-graph`
  to graph projection and focused graph component owners; retain tick-zero shell
  readiness and one graph selection case.
- Continue the existing current-selection split: keep cross-widget selection,
  trace, and bento persistence; move responsive layout, card copy, trace-row
  details, and selection-kind rendering to their focused component owners.

#### Move to browser tests only when the browser is the contract

- Real React Flow zoom/pointer behavior.
- Responsive viewport geometry and bento drag/resize behavior.
- Real file download/drop and preview-server integration.

These browser cases remain outside coverage. Pure graph operations and jsdom
component wiring continue to provide non-browser coverage for the underlying
logic.

#### Coverage membership during reconciliation

Do not wait for every App suite to be rewritten before making coverage light.
The first curated coverage manifest should include only the named shell cases
already extracted from `App.*`. The remaining App corpus stays runnable outside
coverage until each case is moved or deleted with equivalent focused evidence.

### App reconciliation order

1. `App.localization-times` and `App.timeline`: narrow component owners already
   exist and the cross-app residue is small.
2. `App.replay-workstation-requests` and `App.follow-up-submit`: move detailed
   rendering/form behavior, retain one routing case each.
3. `App.layout-graph`: move graph semantics and browser-owned zoom behavior.
4. `App.current-selection`: continue the prior successful split around the
   remaining cross-widget cases.
5. Session recovery, session stream, and multi-session suites last; these have
   the strongest justification for cross-owner composition and should move to
   the extracted minimal shell rather than remain complete app mounts.

## Component runtime experiment — 2026-07-26

The runtime experiment used fresh processes and representative Monaco and React
Flow component files. `bun vitest` was not treated as a Bun migration: Vitest
continued to launch Node workers. Native-Bun results below came from `bun test`
with no Vitest process.

| Case | Current Vitest/jsdom | Vitest/Happy DOM | Native `bun test` |
| --- | ---: | ---: | ---: |
| Monaco prompt editor, before focused package import | 2.67s, 2.17s import | not retained | not retained |
| Monaco prompt editor, after focused package import | 0.59–0.60s, 84–85ms import | 0.39s | 0.169–0.172s |
| React Flow visual-group layer, 16 tests | 2.96–2.97s, 2.34s import | not required | 0.520–0.527s |
| Broad React Flow edit integration, 17 tests | 9.08–10.59s, 4.71–4.73s import | 13.31s | not compatible without decomposition |

The broad integration under genuine Happy DOM also leaked Factory-validation
requests to `localhost:3000` and emitted socket errors. It passed, but the
environment changed request behavior and increased assertion time to 8.42s.
Happy DOM therefore is not a safe component-lane default.

Fresh dependency-only imports show that the libraries themselves are not the
multi-second bottleneck:

- `@xyflow/react`: 28–30ms in Node and 16–20ms in Bun.
- `@monaco-editor/react`: 5–7ms in both runtimes.

The Monaco trace instead found `components/ui/index.ts` loading the complete UI
catalog, including Radix controls, calendar, `react-day-picker`, charts, and
Recharts. Replacing that barrel with
`@you-agent-factory/components/primitives` reduced the file by about 77% before
any runtime change. The broad graph trace transformed roughly 1,000 modules,
including unrelated current-selection, submit-work, terminal-work,
provider-session, replay, and visualization surfaces. Its cost is the app
import graph plus broad interaction assertions, not React Flow initialization.

### Native-Bun component migration boundary

Native Bun is worth adopting for focused component tests that use Testing
Library, ordinary mock functions, and DOM APIs supported by Happy DOM. Do not
move app-style files that depend on Vitest-hoisted `vi.mock`, jsdom-specific
request behavior, or full dashboard composition until they are decomposed.

### Current component latency inventory

The post-PR #1331 local profile groups the remaining dominant component cost by
cause rather than runner alone. The largest feature clusters in that profile
were factory-session detail (3,586ms across 19 files), current selection
(3,567ms across 32 files), workflow activity (3,511ms across 18 files),
dashboard (1,163ms across 8 files), trace drill-down (1,032ms across 5 files),
and header (899ms across 4 files).

| Category | Representative file | Profiled source time | Reconciliation |
| --- | --- | ---: | --- |
| Dashboard app-shell lifecycle | `use-dashboard-snapshot-checkpoint-lifecycle.component.test.tsx` | 1,344ms | Replaced by the feature-owned `usePersistedTimelineCheckpoint` Bun contract; focused execution is about 203ms. |
| Dashboard app-shell session wiring | `dashboard-session-timeline-isolation.component.test.tsx` | 1,089ms | Removed as duplicate coverage of timeline entries, dashboard session state, header tabs, and the snapshot hook. |
| Module-mocked async hook | `use-dashboard-checkpoint-preflight.test.tsx` | 778ms | Replaced four Vitest files (884 lines) with one feature-owned Bun core contract using explicit resolver and mutation effects; focused execution is about 635ms and no longer constructs mocked application providers. |
| Omnibus widget rendering | `current-selection-widget.provider-session-selection.test.tsx` | 720ms | Removed the redundant 500-line widget suite; inference-history selection state now runs in the feature-owned `useSelectedProviderSessionState` Bun contract (about 164ms), while selection controls remain covered by WorkItemCard and workstation-attempt contracts. |
| Graph/editor host composition | `react-flow-current-activity-card-host-contract.test.tsx` | 700ms | Retain the small host boundary; continue replacing aggregate imports with owner modules. |
| Workflow-activity bento aggregation | `workflow-activity-bento-card.test.tsx` | 385ms | Replaced the six-case, module-mocked graph omnibus with a two-case Bun contract around the extracted bento shell. Shared card chrome, editor controls, graph layout, and accessibility behavior stay with their existing feature owners. |
| Trace-grid aggregation | `trace-grid-card.test.tsx` plus three scroll suites | 401ms for the Vitest omnibus, plus duplicate Bun work | Replaced the mocked graph-wide suite with a three-case Bun owner contract for states, table/selection behavior, and localization. Consolidated seven overlapping scroll cases into one long-table ownership invariant under `components/trace-grid-card/`. |
| Cross-owner dashboard wiring | `dashboard-replay-wiring.component.test.tsx` | 697ms | Removed the app-shell duplicate: stream append and fixed-tick behavior are owned by the Bun snapshot hook, timeline store, slider, and bento snapshot contracts. |
| Graph validation and charts | workflow editor validation and work chart tests | 675–691ms | WorkChart's 18 rendered contracts run under Bun. The duplicate workflow-activity editor validation suite is removed, and the seven feature-owned `useFactoryValidation` contracts now run under Bun in about 654ms with an injected validator rather than a process-global module mock. |
| Chart wrapper duplication | `d3-information-card.test.tsx` | 540ms | Replaced the 600-line, ten-case Vitest suite with a three-case Bun owner contract for frame composition, localization/series wiring, and state forwarding. Legend, geometry, zoom, and state behavior remain with the dedicated WorkChart contracts. |
| Aggregate selection switching | `current-selection-widget.selection-switch.test.tsx` | 483ms | Removed the four-case Vitest suite. Workstation, worker, resource, work-item, and work-state dispatch plus cross-selection resets already run in the isolated Bun widget and save contracts and in each detail-card owner. |
| Public barrel self-tests | five feature `public/index.ts` files and three smoke tests | 93ms in the profiled Vitest smoke plus Bun import work | Removed the unused aggregate barrels and the tests whose only consumer was the barrel itself. Focused public subpaths remain available where a stable single-capability contract is useful. |
| Stream recovery hook | `useFactoryEventStream.stale-cursor.test.tsx` | 405ms | The six stale-cursor recovery contracts now run under Bun using the shared global-stub and spy compatibility seam; the hook remains beside its event-stream owner. |
| Cross-feature import workflow | `react-flow-current-activity-card-import-flows.test.tsx` | 457ms | Removed the eight-case current-activity omnibus after reconciling PNG parsing/drop errors, preview dialogs, activation errors, and messages with import owners. A three-case Bun controller contract now owns preview readiness, successful close/refresh, and failed activation retention. |
| Current-selection editing | prompt-edit and header save/discard integration tests | 443–670ms | Removed both duplicate widget suites. Invalid-to-valid prompt recovery and save submission run in the Bun save contract; exact header-only workstation and worker actions run in their feature-owned detail-card contracts. |
| Cross-owner trace wiring | `dashboard-trace-wiring.component.test.tsx` | 616ms | Removed the app-shell duplicate after reconciling event lineage, trace merging, trace rendering, and selection callbacks with their timeline, trace, and current-selection owners. |

The dominant categories remain production import fan-out, complete dashboard or
widget mounts, and process-global module mocks. Assertion execution is not the
primary cost. Migration therefore starts by narrowing ownership and dependency
seams; changing only the runner for an app-sized test preserves most latency.

After the trace-grid reconciliation, the complete component command owns 233
Bun files and 113 remaining Vitest files and completes locally in 75.18s:
37.64s in Bun and 37.54s in Vitest. The last measured unit lane is 18.05s, so
the combined non-browser correctness feedback is approximately 93.23s.

The same reconciliation removed the canonical-section omnibus widget loop.
Every detail owner structurally composes `SelectionDetailLayout`; the base
layout and individual detail-card contracts own that behavior without mounting
every selection kind through the aggregate widget.

WorkChart now owns a dedicated `components/work-chart/` package directory.
Production modules and Storybook coverage stay at that owner root, while the
rendered legend, series/state, and interaction contracts live under its
`contracts/` directory with one shared fixture module. This reduced the flat
work-outcome component folder below its legacy allowance, which was removed.

Keep the taxonomy as unit, component, and browser tests. Native Bun is an
execution sublane of component tests, not a fourth test type:

- Keep migrated files beside their component owners, named
  `*.bun.component.test.tsx`.
- Put shared Bun DOM registration and Testing Library cleanup in
  `ui/src/testing/bun/component.setup.ts`.
- Put Monaco-only module doubles in
  `ui/src/testing/bun/monaco.setup.ts`; graph tests must not pay that setup cost.
- Configure preloads in `ui/bunfig.toml` and add a targeted
  `test:component:bun` package script.
- Exclude `*.bun.component.test.tsx` from the Vitest component project and teach
  `check-ui-test-lanes.mjs` that these files still belong to the component lane.
- Keep the Node-only unit coverage manifest unchanged. Native-Bun component
  tests do not become coverage members merely because their runner is faster.

### Migration tasks

1. Add the shared Bun/Happy DOM component setup using Bun's supported preload
   mechanism and Testing Library cleanup.
2. Establish lazy source-package aliases so a Monaco test does not import graph
   or chart setup. Avoid the experimental `--tsconfig-override` path on Bun
   1.3.13; it emitted an internal directory-mismatch error in this repository.
3. Migrate `monaco-prompt-editor.test.tsx` first and preserve its two behavioral
   assertions under native Bun.
4. Migrate `factory-graph-visual-group-layer.test.tsx` next; its unchanged 16
   cases already passed under native Bun in the experiment.
5. Run both old and new commands during one transition change, then remove each
   migrated file from Vitest discovery to prevent duplicate execution.
6. Continue import-graph cleanup for the broad workflow graph integration.
   Split feature-owned projection, save, dialog, and interaction contracts
   before considering it for Bun.
7. Record full component-lane latency after the first two migrations. Expand
   the Bun sublane only when a representative file is at least 30% faster and
   shows no environment-specific request or DOM behavior.

## Quality gates

- `cd ui && bun run test:performance`
- Focused `ui/scripts/ui-coverage-runner.test.mjs`
- Focused Make verification smoke tests
- `make ui-test` after unit/component manifests change
- `make test-ui-coverage` after coverage membership or thresholds change
- `make ui-integration-test` only when browser ownership or harness behavior
  changes
- `make ui-lint` for frontend scripts/configuration changes
