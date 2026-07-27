# UI Browser Test Isolation And Latency Plan

## Problem and target outcome

The dashboard browser lane currently serializes every integration file even
though most files use an in-process mocked HTTP/SSE backend and can own isolated
ports, preview processes, browser contexts, and fixture state. The canonical
outcome is that every browser scenario can execute independently, while CI uses
bounded concurrency chosen from measured runner capacity rather than relying on
serialization for correctness.

The first implementation stage targets mocked-backend browser files. Real Go
backend proofs, Node-only harness checks, and Storybook checks must have explicit
owners instead of being discovered incidentally by a directory-wide Vitest run.

## Isolation protocol

- Build the production dashboard once before Vitest workers start. Workers must
  treat `ui/dist` as immutable and must not race to rebuild it.
- Each Vitest worker owns a dynamically allocated preview/API port pair. The
  Vite preview proxies relative dashboard API requests to that worker's mock
  HTTP/SSE server.
- A scenario that opts into within-file concurrency owns its own dynamically
  allocated preview/API port pair. Its callback receives the test-scoped
  `expect` instance because Vitest concurrent tests cannot safely use global
  polling matchers.
- Each test continues to own a fresh browser context and mock server state.
  Browser processes and preview processes may be reused only inside their
  worker ownership boundary.
- CI artifacts are written below worker-specific directories so identical test
  labels and process-local artifact sequences cannot overwrite one another.
- Mocked browser files run with file parallelism and a bounded worker count.
  `UI_BROWSER_INTEGRATION_MAX_WORKERS` is the comparison control; the initial
  default is two.
- Tests that intentionally share IndexedDB, tabs, or lifecycle state keep that
  sharing inside one scenario. No state is shared across test scenarios.

## Delivery stages and acceptance criteria

### 1. Mocked browser files execute concurrently

- The mocked-browser runner names its files explicitly and excludes real Go
  backend and Node-only helper files.
- A production build completes before Vitest starts.
- At least two mocked browser files execute concurrently without port, preview,
  API-state, build-output, or artifact collisions.
- The full mocked-browser command passes at the default worker count and can be
  rerun with `UI_BROWSER_INTEGRATION_MAX_WORKERS=1` for comparison.
- Slow-file and total phase timings remain available for before/after evidence.

### 2. Every mocked browser scenario becomes independently schedulable

- Remove file-level shared mutable preview variables and `describe.sequential`
  where test cases still share one API port or preview lifecycle.
- Introduce a test-scoped scenario fixture that owns preview, mock API, browser
  context, fixture state, downloads, and cleanup.
- Tests that need multiple tabs receive them from the same scenario context;
  unrelated tests never share browser storage or mock-server state.
- The graph, import/export, recovery, replay, and session suites pass when
  individual cases are scheduled concurrently with another case from the same
  file.

### 3. Real backend integration has one explicit owner

- `durable-session-real-backend.integration.test.mjs` and
  `browser-test-harness.real-backend-setup.integration.test.mjs` run only in the
  dedicated UI Backend Integration lane, not the mocked browser lane.
- Build the Go browser API harness once and execute the binary rather than
  compiling it through `go run` for every scenario.
- Each real-backend scenario owns a unique port and session identity.
- The broad browser job no longer requires Go setup after the real-backend
  files are removed from its contract.

### 4. Node-only checks move to the unit/component owner

- Port availability and browser wait-helper checks run through `test:unit` and
  are not repeated in the browser lane.
- Direct mock HTTP contract checks either remain Node integration tests under
  the unit command or move beside the harness module with focused ownership.
- Browser artifact behavior remains browser-backed because it exercises real
  Playwright context, page, trace, and diagnostic behavior.

### 5. Storybook has an independent parallel lane

- `ui-integration-test` no longer builds Storybook or runs Storybook browser
  checks.
- Storybook build and browser checks have a classifier-controlled job that can
  run in parallel with mocked browser integration.
- Storybook browser checks reuse a bounded number of Chromium processes or
  contexts and use measured concurrency rather than five unconditional serial
  browser launches.
- Storybook interaction, responsive, graph-touch, viewport, and emulator
  contracts remain directly verified.

### 6. Success artifacts avoid unnecessary teardown cost

- Browser contexts remain test-scoped; tests do not manually scrub and reuse
  unrelated pages.
- Full screenshot, HTML, and trace artifacts are retained on failure.
- Successful scenarios write only bounded diagnostics unless a test explicitly
  opts into full evidence.
- Timing evidence distinguishes navigation/test work, artifact capture, and
  context/server cleanup.

## Verification

- `cd ui && bun x vitest run scripts/ui-integration-runner.test.mjs`
- `cd ui && UI_BROWSER_INTEGRATION_MAX_WORKERS=1 bun run test:integration`
- `cd ui && UI_BROWSER_INTEGRATION_MAX_WORKERS=2 bun run test:integration`
- `make ui-integration-test`
- `make ui-durable-session-real-backend-integration-test`
- `make ui-test`
- `make ui-lint`

Compare the stable `[ui-browser-integration]` phase and slow-file summaries at
one and two workers. Parallelism is accepted only when the repeated two-worker
run stays green and reduces wall time without losing failure artifacts.

## Initial protocol results

Measured locally on 2026-07-26 after the explicit mocked/static inventory was
introduced:

| Configuration | Browser phase | Result |
| --- | ---: | --- |
| One worker | 136.48s | 16 files, 39 tests passed |
| Two workers | 67.09s | 16 files, 39 tests passed |
| Three workers | 48.73s | 16 files, 39 tests passed |

The first two-worker trial exposed a real pagehide/checkpoint race in the stale
session-identity scenario. The test now leaves the React application before
replacing IndexedDB state, preventing the outgoing dashboard from overwriting
the seeded stale fixture. A focused concurrent run with the slow node-placement
suite and subsequent complete two- and three-worker runs passed.

The completed protocol applies test-scoped API ports, browser contexts, and
Vitest matcher context to every mocked multi-scenario file. Mocked cases share
one immutable Vite preview per worker and receive a runtime API origin before
page scripts execute. The three real-Go durable-session cases instead own
dedicated preview/API pairs and execute concurrently in the UI Backend
Integration lane.

After the complete conversion, the mocked browser phase passed in 53.0s with
two workers and 34.9s with three workers. After removing the redundant
name-preservation browser file, the default three-worker inventory passed 15
files and 36 scenarios in 32.6s. Three workers are now the default.
The focused Go-backed lane passed four checks in 7.7s. Storybook build and
browser checks passed independently in 88s and now run as a sibling CI job
rather than extending the mocked browser critical path.

The redundant factory-name-preservation browser file was removed from the
inventory. Its save-name contract is owned by
`useFactoryDocumentSave.test.tsx`; replace-current and create-named import
behavior is owned by `App.import.test.tsx`, the import activation API tests,
and `use-factory-import-activation.test.tsx`. Browser coverage remains for PNG
export/import, graph save payloads, and non-default-session activation where
Chromium or full application composition is material.

## Remaining latency after file parallelism

The post-parallel critical path is no longer the sum of all browser files. It is
the immutable production build plus the slowest scheduled worker bucket.

- Production build: approximately 20-23s locally.
- Mocked browser phase: 32.6s at the default three workers after redundant
  browser coverage was moved to its existing unit/component owners.
- Real-Go UI backend integration: approximately 7.7s with three durable-session
  cases executing concurrently.
- Storybook: approximately 88s locally, now parallel with the other UI jobs.
- Node placement is independently schedulable within its file: four scenarios
  complete in approximately 9-10s wall time.
- Largest individual scenarios are the node-placement, graph-save, PNG
  roundtrip, and name-preservation paths at roughly 5-8s each.

Next reductions, in priority order:

1. Move additional graph dirty-state, editor toggle/discard wiring, placement math, and
   payload-shaping assertions to existing operation/component owners; retain
   one browser proof for real viewport placement, Chromium hit targets, PNG
   roundtrip, and saved network payloads.
2. Capture full screenshot/HTML/trace artifacts only on failure and measure
   context-close/artifact time separately on CI, where the artifact root is
   enabled.
3. Use the first PR's `ubuntu-latest` timing to decide whether four workers
   improve wall time without increasing scenario flake or CPU contention.
