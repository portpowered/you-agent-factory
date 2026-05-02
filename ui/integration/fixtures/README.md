# Agent Factory UI Replay Fixtures

This directory is the canonical home for recorded Agent Factory UI replay fixtures.
For the full contributor workflow, when to prefer replay coverage, and the
coverage-report commands, use the canonical maintainer guide:
[Dashboard UI Replay Testing](../../../docs/development/dashboard-ui-replay-testing.md).

## Fixture Contract

- Store captured replay fixtures as newline-delimited JSON in `integration/fixtures/*.jsonl`.
- Each non-empty line must be one canonical `FactoryEvent` object from the UI event contract.
- Keep fixture files as recorded event streams. Do not embed scenario-only assertions or derived snapshot state in the JSONL.

## Shared Loader Surface

- Use `src/testing/replay-fixtures.ts` to load replay fixtures into typed `FactoryEvent[]`.
- Use `buildReplayFixtureTimelineSnapshot(...)` from that same module when a test needs the canonical timeline projection seam instead of raw event parsing.
- Use `src/testing/replay-harness.ts` when an App-level test should mount the real `/events` stream seam and replay one of these fixtures without hand-rolled `EventSource` mocks.
- Add fixture-level metadata such as covered surfaces and verification layers to `replayFixtureCatalog` in `src/testing/replay-fixture-catalog.ts` rather than duplicating ad hoc maps in individual tests.
- Run `bun run replay:coverage` after changing replay scenarios or tracked coverage surfaces to print the current coverage matrix, and use `bun run replay:coverage:check` in review workflows to catch metadata drift.

## Current Fixtures

- `event-stream-replay.jsonl` — baseline replay smoke covering dashboard shell, selected work, and trace drill-down rendering.
- `event-stream-replay-2.jsonl` — captured runtime-config replay covering workspace setup, current selection, and trace drill-down projections.
- `failure-analysis-replay.jsonl` — failure-path replay covering queued work, failed selection rendering, and fixed-tick history navigation.
- `graph-state-smoke-replay.jsonl` — dashboard-shell replay covering graph markers, terminal selection, and tick rewinds.
- `runtime-details-replay.jsonl` — request-detail replay covering pending, successful, and failed runtime workstation projections.
- `weird-number-summary-replay.jsonl` — focused failed-summary replay covering the one-dispatch/three-failed-work-items regression.
