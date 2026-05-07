---
author: Codex
last modified: 2026, april, 25
doc-id: AGF-DEV-007
---

# Dashboard UI Replay Testing

This document is the canonical contributor guide for Agent Factory UI replay-driven regression coverage. Use it when a dashboard behavior is primarily projected from `/events` and you need to decide whether to add or extend replay coverage.

## Summary

- Prefer replay coverage for stream-projected dashboard behavior such as dashboard shell rendering, current selection, trace drill-downs, runtime request details, failure rendering, and timeline history.
- Keep recorded fixtures as canonical `FactoryEvent` JSONL files under `ui/integration/fixtures/`.
- Load fixtures through `ui/src/testing/replay-fixtures.ts` and use `ui/src/testing/replay-harness.ts` for App-level or browser-backed `/events` replay.
- Keep scenario and surface metadata in `ui/src/testing/replay-fixture-catalog.ts`.
- Print or verify replay coverage visibility with `bun run replay:coverage` and `bun run replay:coverage:check`.

## When To Prefer Replay Coverage

Use replay coverage when the regression depends on the ordered event stream rather than on a single isolated component prop or selector stub.

Replay coverage is the preferred seam when you need to verify:

1. App-shell behavior that depends on the live `/events` subscription.
2. Derived state that only appears after multiple event reductions, such as selection history or trace drill-down state.
3. Failure or fallback rendering driven by recorded runtime events.
4. A customer-visible event contract regression that should stay aligned with canonical `FactoryEvent` payloads.

Prefer more local tests only when the behavior is not stream-driven or when the assertion is easier to prove through a focused pure helper without replaying the event history.

## Fixture Contract

- Store replay fixtures in `libraries/agent-factory/ui/integration/fixtures/*.jsonl`.
- Each non-empty line must be one canonical UI `FactoryEvent` object.
- Keep fixtures on the current generated event contract. Do not preserve stale dashboard-local aliases in new fixtures.
- Keep scenario metadata out of the JSONL payload. Register scenario identity, covered surfaces, and verification layers in `ui/src/testing/replay-fixture-catalog.ts`.

The local fixture index at `ui/integration/fixtures/README.md` is intentionally short and should point back to this guide instead of becoming a second workflow document.

## Entry Points

### Projection Helpers

Use `ui/src/testing/replay-fixtures.ts` when a test only needs typed events or timeline projection helpers.

- `loadReplayFixtureEvents(...)` loads a cataloged fixture into typed `FactoryEvent[]`.
- `buildReplayFixtureTimelineSnapshot(...)` builds the canonical selected-tick timeline snapshot for reducer or hook-level assertions.

Use this path when the test does not need the real `EventSource` seam.

### App And Browser Replay Harness

Use `ui/src/testing/replay-harness.ts` when a scenario should mount the real Agent Factory UI event stream path.

Typical flow:

1. Create the harness and call `install()` before rendering.
2. Render the App or browser-visible surface that opens `/events`.
3. Call `replayHarness.replayFixture(...)` or `replayHarness.replayEvents(...)`.
4. Wait for replay completion or the target tick before asserting scenario-specific UI behavior.
5. Call `reset()` during cleanup.

This keeps replay deterministic without per-test `EventSource` doubles or fixed sleeps.

## Coverage Workflow

The scenario registry and tracked replay surfaces live in `ui/src/testing/replay-fixture-catalog.ts`. That file is the source of truth for:

- scenario IDs and fixture filenames
- covered stream-projected surfaces
- verification layers such as `app-smoke`, `browser-integration`, or `projection-helper`
- any browser-integration scenario metadata needed by `integration/event-stream-replay.integration.test.mjs`, so the smoke and the report stay on one registry
- any smoke-only build hygiene needed by that integration test, such as clearing inherited `VITEST*` env before shelling out to `bun run build` so the tracked `dist/` bundle stays production-shaped

After changing replay scenarios or tracked surfaces:

```bash
cd libraries/agent-factory/ui
bun run replay:coverage
bun run replay:coverage:check
```

- `replay:coverage` prints the current scenario and surface matrix from `replayFixtureCatalog`.
- `replay:coverage:check` fails when the catalog metadata becomes internally inconsistent.

Review the command output to see the current baseline and explicit remaining gaps.

## Current Phase Boundary

This project establishes a reusable replay fixture path, a shared harness, starter scenarios, and a coverage report. It does not claim complete replay coverage for every Agent Factory UI surface.

The current baseline is intentionally bounded:

- use replay coverage for the highest-value event-projected regressions first
- keep remaining gaps mechanically visible through `bun run replay:coverage`
- add new scenarios through the shared fixture and harness path instead of inventing new replay rigs

Future stories can raise coverage breadth or thresholds after this shared seam proves stable.

## Verification

For replay-guidance changes and replay-scenario changes, use the smallest relevant checks from `libraries/agent-factory/ui`:

```bash
bun run tsc
bun run test
bun run replay:coverage:check
```

`bun run test` already runs the browser-backed replay smoke in a separate `test:integration` phase after the faster unit/jsdom suite so the preview-backed replay harness does not share one Vitest process with unrelated UI tests. Use `bun run test:integration` directly only when you need to iterate on that browser path in isolation.

## References

- [Agent Factory development guide](./development.md)
- [Agent Factory replay fixtures](../../ui/integration/fixtures/README.md)
- [Agent Factory development process](../../../docs/processes/agent-factory-development.md)
