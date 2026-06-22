# Follow-Up Cells: Durable JavaScript Website Session Inspection Deferred Surfaces

## Why This Note Exists

The lane `dynamic-workflows-cell-website-session-inspection` extended the existing
Factory Session detail surface so durable JavaScript sessions render shared status,
orchestrator kind, script phase or runtime status, child dispatch summaries,
warnings, and available result or artifact references from shared typed session
data, with explicit loading, missing, and error states.

That slice proves:

- durable `FactorySessionDurableReadModel` normalization into shared `FactorySession`
  runtime shape for the detail hook and panel
- durable dispatch list and result/artifact enrichment through shared API adapters
- customer-facing runtime labels and bounded inspection UI inside the existing
  `factory-session` bento widget
- focused UI tests plus Storybook browser verification for success and non-success
  durable JavaScript inspection states

This note records deferred follow-up cells encountered while implementing the
bounded website inspection lane. It does not reopen the completed summary,
dispatch/artifact, or loading/error work.

## In-Scope Work (Completed)

| Surface | Website wiring | Proof |
|---------|----------------|-------|
| Durable session summary | `ui/src/api/factory-sessions/normalize-session-get.ts`, `ui/src/features/factory-session-detail/` | `factory-session-detail-panel.test.tsx` (`dur-sess-js-run-n-001`) |
| Dispatch, warning, artifact, result inspection | `api-durable-inspection.ts`, `normalize-durable-inspection.ts`, `use-factory-session-detail.ts` | `factory-session-detail-panel.test.tsx` (`dur-sess-js-success-002`) |
| Loading, missing, error states | `factory-session-detail-panel.tsx` (`StatusNotice`) | `factory-session-detail-panel.test.tsx`, Storybook loading/not-found/error stories |
| Browser verification | `factory-session-detail-panel.stories.tsx` with `tags: ["test"]` | `vitest.storybook.config.ts` storybook project |

## Deferred Follow-Up Cells

| Follow-up cell | Deferred surface | Current posture |
|----------------|------------------|-----------------|
| `dynamic-workflows-cell-website-dispatch-drilldown` | Per-dispatch detail drilldown (`GET /factory-sessions/{session_id}/dispatches/{dispatch_id}`) with status transitions, execution mode, provider-session refs, and typed failure detail | Detail panel lists dispatch summaries only; no dispatch detail route or disclosure UI |
| `dynamic-workflows-cell-website-artifact-drilldown` | Artifact content retrieval and preview (`GET /factory-sessions/{session_id}/artifacts/{artifact_id}`) from inspection refs | Detail panel shows artifact kind/id labels only; no artifact body or download affordance |
| `dynamic-workflows-cell-website-lifecycle-controls` | Pause, resume, cancel, terminate, approve, and retry-dispatch controls for durable sessions | Backend lifecycle routes exist; website detail surface is read-only |
| `dynamic-workflows-cell-website-event-replay` | Durable session event stream replay, reconnect cursors, and timeline integration from the detail surface | Event replay lives in timeline/dashboard hooks; not wired into factory-session detail |
| `dynamic-workflows-cell-website-live-provider-inspection` | Live-provider child execution mode, provider-session correlation, and bridged-child failure detail in website dispatch rows | Backend projection exposes execution mode and failure detail; website dispatch list omits those fields |
| `dynamic-workflows-cell-website-real-backend-e2e` | End-to-end website inspection against a running backend with real `dur-sess-*` sessions | Verification uses deterministic fetch mocks and Storybook fixtures |

## Smallest Executor Lanes

### `dynamic-workflows-cell-website-dispatch-drilldown`

1. Extend `use-factory-session-detail.ts` or a sibling hook to fetch durable dispatch
   detail when a dispatch row is selected.
2. Render status transitions, execution mode, provider-session refs, and failure
   detail using shared `FactoryDispatch` vocabulary.
3. Preserve explicit loading, empty, and error states inside the existing detail
   surface.

### `dynamic-workflows-cell-website-artifact-drilldown`

1. Fetch durable artifact detail from inspection refs shown in the detail panel.
2. Add a bounded preview or download affordance without introducing workflow-run
   nouns.
3. Keep artifact retrieval behind shared factory-session API adapters.

### `dynamic-workflows-cell-website-lifecycle-controls`

1. Wire lifecycle control POST routes through shared factory-session API helpers.
2. Surface control outcomes and post-control inspection links in the detail panel.
3. Reuse existing dashboard action and error treatments.

### `dynamic-workflows-cell-website-event-replay`

1. Connect durable session event replay to the timeline or a bounded detail
   disclosure without widening into a full dashboard redesign.
2. Preserve reconnect cursor semantics from `useFactoryEventStream`.

### `dynamic-workflows-cell-website-live-provider-inspection`

1. Project live-provider execution mode and provider-session refs into dispatch
   rows after dispatch drilldown lands.
2. Show typed bridged-child failure detail for failed dispatches.

### `dynamic-workflows-cell-website-real-backend-e2e`

1. Add one Playwright or integration path that loads a real backend durable
   JavaScript session in the dashboard.
2. Keep fixture-backed unit and Storybook coverage as the default fast path.

## Non-Goals For The Completed Lane

- Factory graph editor or current-selection cleanup unrelated to durable JavaScript
  session inspection.
- Replay, resume, or persistence-specific website surfaces beyond the bounded
  summary and list rendering already shipped.
- Transport redesign, new top-level workflow pages, or another broad dashboard
  parity sweep.
- Website-only dynamic workflow run models or feature-local synthetic runtime
  semantics.

## Evidence

| Artifact | Purpose |
|----------|---------|
| `ui/src/features/factory-session-detail/components/factory-session-detail-panel.test.tsx` | Focused durable JavaScript success and non-success UI tests |
| `ui/src/features/factory-session-detail/components/factory-session-detail-panel.stories.tsx` | Deterministic Storybook browser verification |
| `docs/internal/processes/api-relevant-files.md` | Maintainer map for durable website inspection adapters |
