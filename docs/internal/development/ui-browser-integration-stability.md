# UI Browser Integration Stability

This guide documents how the dashboard browser integration lane stays
deterministic when run locally, concurrently with covered UI verification, or
in the GitHub Actions `UI Browser Integration` job. Use it when adding suites,
tuning concurrency, or debugging port, preview, and download conflicts.

Canonical rerun command: `make ui-integration-test` (`cd ui && bun run test:integration`).

## Runner contract

`ui/scripts/ui-integration-runner.mjs` explicitly selects the mocked/static
browser files and invokes Vitest with bounded file parallelism:

```text
vitest run <mocked-browser-files...> --fileParallelism --maxWorkers 3
```

`UI_BROWSER_INTEGRATION_MAX_WORKERS` overrides the default for measured
comparison runs. The production dashboard is built once before Vitest starts.
Each worker owns an immutable Vite preview and Chromium process; each scenario
owns a dynamically allocated mock API port, HTTP/SSE backend, browser context,
storage, and artifact identity. A test-only runtime API origin routes each
browser context to its scenario server without rebuilding or restarting the
preview.

The two real Go backend files are not members of this runner. They remain owned
by `make ui-durable-session-real-backend-integration-test`. Node-only port and
wait-helper checks are owned by `test:unit` and are not repeated here.

## Test-scoped scenario contract

Every mocked multi-scenario file uses `isolatedMockBrowserTest` and
`describe.concurrent`. The fixture passes the Vitest test-scoped `expect`, a
scenario-bound `openBrowserPage`, and the shared immutable preview descriptor.
Concurrent cases must use the supplied `expect` for polling matchers and the
supplied page opener so API traffic receives the scenario's runtime origin.

Single-scenario mocked files remain written as sequential describes where that
makes the scenario easier to read; they are still independently scheduled by
file parallelism and do not serialize any sibling case.

The real Go-backed durable-session cases use `isolatedBrowserTest`. Each owns a
dedicated preview/API pair because the real backend intentionally uses the
same-origin Vite proxy rather than the mock server's permissive CORS contract.
Those cases execute concurrently in the dedicated UI Backend Integration lane.

## Isolation for concurrent lanes

`make run-concurrent-ui-verification-lanes` runs browser integration in a
**separate process** from sharded covered UI. Isolation boundaries:

| Resource | Isolation mechanism |
| --- | --- |
| Preview/API ports | One immutable preview per mocked worker; `findAvailablePort()` gives every scenario its own API port |
| Production build | Lane-scoped `VITE_AGENT_FACTORY_API_ORIGIN` during `bun run build` |
| Playwright traces/screenshots | `AGENT_FACTORY_BROWSER_ARTIFACT_DIR` (CI sets `.artifacts/ui-browser-integration/browser`) |
| Parallel worker artifacts | `worker-<VITEST_POOL_ID>/` below the configured browser artifact root |
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
cd ui && vitest run integration/factory-graph-editor.integration.test.mjs --no-file-parallelism --maxWorkers 1
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
