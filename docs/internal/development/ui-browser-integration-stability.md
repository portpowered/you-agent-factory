# UI Browser Integration Stability

This guide documents how the dashboard browser integration lane stays
deterministic when run locally, concurrently with covered UI verification, or
in the GitHub Actions `UI Browser Integration` job. Use it when adding suites,
tuning concurrency, or debugging port, preview, and download conflicts.

Canonical rerun command: `make ui-integration-test` (`cd ui && bun run test:integration`).

## Runner contract

`ui/scripts/ui-integration-runner.mjs` invokes Vitest with:

```text
vitest run integration --no-file-parallelism --maxWorkers 1
```

Integration files therefore execute **serially** within the lane. Do not enable
file parallelism or multiple Vitest workers for browser integration without
revisiting every shared-resource note below.

## Suites that must stay sequential

Every browser integration file uses `describe.sequential` and shares a
**lane-scoped preview harness** started in `beforeAll`:

| Suite file | Shared resource | Why sequential |
| --- | --- | --- |
| `event-stream-replay.integration.test.mjs` | One `vite preview` + global production build cache | Replay scenarios reuse the same preview URL and API origin wiring |
| `dashboard-session-tabs.integration.test.mjs` | Shared preview + per-file API servers on `preview.apiPort` | Multi-tab chrome depends on one preview build per describe block |
| `factory-name-preservation.integration.test.mjs` | Shared preview + captured download hook per test | Export/import roundtrips share preview startup cost |
| `factory-import-second-session.integration.test.mjs` | Shared preview + temp download directories | Second-session import uses real filesystem drops |
| `factory-graph-editor.integration.test.mjs` | Shared preview + download capture | Graph edit + PNG roundtrip |
| `factory-graph-editor-session-switch.integration.test.mjs` | Shared preview | Session switch retains graph selection |
| `factory-graph-editor-node-placement.integration.test.mjs` | Shared preview | Real viewport measurement |
| `factory-graph-editor-selection-no-panel-delete.integration.test.mjs` | Shared preview | Chromium hit targets |
| `maintainer-phantom-worker-graph.integration.test.mjs` | Shared preview | Graph editor tools in real layout |
| `browser-test-harness.artifacts.integration.test.mjs` | Env-only (no preview) | Artifact path resolution |
| `durable-session-real-backend.integration.test.mjs` | Shared preview + Wire-backed `browser_api_harness` per scenario | One bounded durable JavaScript session-detail proof against a real backend |
| `browser-test-harness.real-backend-setup.integration.test.mjs` | API port only (no preview) | Setup regression for `dur-sess-*` seeding without Playwright preview |

Within each file, `it` blocks run in order. Individual tests start their own
`startFactoryApiServer` on `preview.apiPort` and their own Playwright browser
context, but they **reuse** the shared preview process and the module-level
production build cache (`globalThis.__agentFactoryBrowserIntegrationBuildComplete`).

Do not parallelize cases inside these files until each case owns an isolated
preview build, API port pair, and download directory.

## Isolation for concurrent lanes

`make run-concurrent-ui-verification-lanes` runs browser integration in a
**separate process** from sharded covered UI. Isolation boundaries:

| Resource | Isolation mechanism |
| --- | --- |
| Preview/API ports | `findAvailablePort()` in `browser-test-harness.mjs`; optional overrides `AGENT_FACTORY_BROWSER_API_PORT` and `AGENT_FACTORY_BROWSER_PREVIEW_PORT` |
| Production build | Lane-scoped `VITE_AGENT_FACTORY_API_ORIGIN` during `bun run build` |
| Playwright traces/screenshots | `AGENT_FACTORY_BROWSER_ARTIFACT_DIR` (CI sets `.artifacts/ui-browser-integration/browser`) |
| Captured downloads | Per-test `mkdtemp` directories; `installCapturedDownloadHook` stores blobs in page memory, not a shared folder |
| Covered UI shards | Separate `coverage/shard-<index>` dirs (no overlap with browser preview ports) |

When debugging a concurrent local failure, check
`.artifacts/concurrent-ui-verification-lanes/ui-browser-integration.log` for the
`[UI Browser Integration]` prefixed output.

## Recommended wait patterns

Shared helpers in `ui/integration/browser-test-harness.mjs` encode durable
checkpoints. Prefer them over transient copy, heading visibility during dialog
close, or animation timing.

| Checkpoint | Helper | Use when |
| --- | --- | --- |
| Event stream caught up | `waitForDashboardReady(page, previewURL, server)` (in name-preservation helpers) + `server.replayCompleted` | Dashboard mounted and replay fixture finished streaming |
| Form ready for submit | `waitForDurableControlEnabled(locator)` | Export/import/graph save buttons become enabled after required fields |
| Import/export outcome | `waitForCapturedDownloadOrDialogError(page, dialog)` | Real download captured or dialog shows `role="alert"` error |
| Dialog dismissed | `waitForDialogHidden(dialogLocator)` | Dialog `role` hidden — not heading text that may unmount separately |
| Arbitrary durable signal | `waitForDurableCheckpoint(label, conditionFn)` | API request arrays, custom hooks, enabled graph controls |

Avoid browser-only waits for:

- Status/helper copy such as `Selected image: dashboard.png` when enabled
  controls already prove validity.
- Duplicate `textContent` checks when an accessible preview image or captured
  `PUT` body proves the same contract.
- `heading` visibility for dialog teardown when the dialog node exposes `hidden`
  state.

## Failure attribution

Lane failures should rerun with `make ui-integration-test`. For a single file:

```bash
cd ui && vitest run integration/factory-name-preservation.integration.test.mjs --no-file-parallelism --maxWorkers 1
```

For the bounded durable JavaScript Factory Session real-backend proof only:

```bash
make test-ui-durable-session-real-backend
# or
cd ui && bun run test:integration:durable-session-real-backend
```

That focused lane runs `durable-session-real-backend.integration.test.mjs` and
`browser-test-harness.real-backend-setup.integration.test.mjs` without pulling
in the full browser integration directory. Fast fixture-backed unit, hook,
adapter, and Storybook coverage stays on `make ui-test` / `cd ui && bun run test:unit`.

CI retains Playwright trace, screenshot, HTML, and diagnostics JSON under the
lane artifact directory when `AGENT_FACTORY_BROWSER_ARTIFACT_DIR` is set.

## Related references

- [development.md](development.md) — verification tiers and CI lane table
- [ui-test-lane-boundaries.md](ui-test-lane-boundaries.md) — what belongs in browser vs jsdom lanes
- `ui/integration/browser-test-harness.mjs` — preview, API server, and wait helpers
