# Session Repair Final Browser Reliability Evidence Matrix

This matrix records the strongest behavioral evidence present on the branch before
the remaining App and browser composition work. It is reviewer guidance, not a
test inventory or a release gate of its own. The tests below assert runtime or
rendered behavior directly; no meta-test scans source files, routes, registrations,
commands, documentation links, or asset bundles to enforce this document.

Run commands from the repository root. Focused Vitest commands use one worker so
the App and browser harnesses do not contend for shared process resources.

## Automated evidence

| Observable reliability invariant | Strongest behavioral proof and asserted outcome | Coverage status | Focused verification command |
| --- | --- | --- | --- |
| A quiet tick-zero checkpoint becomes current after hydration, successful preflight, and stream open, without waiting for a message. | `App.session-stream.test.tsx` — **renders a restored tick-zero checkpoint after preflight and a quiet stream open** independently controls hydration, preflight, stream opening, and message arrival; it renders the restored dashboard, removes the loading shell, and keeps the stream at zero delivered messages. | Covered at the required App boundary. No duplicate test needed. | `cd ui && bunx vitest run src/App.session-stream.test.tsx --no-file-parallelism --maxWorkers 1` |
| Default and named selectors route through their resolved concrete Factory Session UUIDs, never an alias-only or process-global stream. | `useFactoryEventStream.test.tsx` — **opens the resolved UUID session event stream for default-session selectors** and **never opens the process-global /events stream for dashboard traffic** prove default routing. `useFactoryEventStream.reconnect-url.test.tsx` — **opens reconnect cursors on the selected non-default session stream** proves named routing with its cursor. | Hook behavior is covered. **App composition gap:** one App test still needs to demonstrate both resolved default and named UUID requests through the production composition boundary. | `cd ui && bunx vitest run src/features/dashboard/hooks/event-stream/useFactoryEventStream.test.tsx src/features/dashboard/hooks/event-stream/useFactoryEventStream.reconnect-url.test.tsx --no-file-parallelism --maxWorkers 1` |
| Logical remap or stale-cursor recovery invalidates and retries only the affected exact stream while another session remains unchanged. | `useDashboardSnapshot.test.tsx` — **remaps a stale factory session id through logical identity without reusing the reconnect cursor** proves remap behavior. `useFactoryEventStream.stale-cursor.test.tsx` — **clears only the affected session checkpoint and runtime queries before replaying from scratch** proves cursor recovery while retaining the seeded `session-beta` cache. Superseded-session preflight cases also prove delayed remap and stale-cursor results cannot alter the newly active session. | Hook behavior is covered. **App composition gap:** targeted recovery has not yet been exercised with two sessions through App composition. | `cd ui && bunx vitest run src/features/dashboard/hooks/useDashboardSnapshot.test.tsx src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.test.tsx src/features/dashboard/hooks/event-stream/useFactoryEventStream.stale-cursor.test.tsx --no-file-parallelism --maxWorkers 1` |
| Bounded materialized work-outcome history survives persistence and restore with its exact continuation accumulator. | `timelineCheckpointPersistence.materialized-round-trip.test.ts` — **preserves the complete retained baseline without sharing mutable references** and **applies deterministic presentation limits while preserving the exact accumulator** prove durable bounded history. `useWorkOutcomeChart.test.ts` — **preserves the restored baseline and extends it with one accepted live event** restores seven samples before adding a tail. | Persistence and hook behavior are covered. **App and browser composition gap:** neither required composition proof currently renders a restored baseline of at least six samples before live delivery. | `cd ui && bunx vitest run src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.materialized-round-trip.test.ts src/features/work-outcome/hooks/useWorkOutcomeChart.test.ts --no-file-parallelism --maxWorkers 1` |
| One uniquely identified live tail extends restored history exactly once; duplicate delivery neither replaces history nor changes cumulative outcomes. | `useWorkOutcomeChart.test.ts` — **preserves the restored baseline and extends it with one accepted live event** asserts seven restored samples, an eighth tail sample, unchanged samples after duplicate delivery, and exactly one stored event with the tail ID. `factoryTimelineEntry.append.test.ts` — **restores one persisted boundary and applies only a reconnect suffix** proves ordered deduplicated replay and outcome projection. | Hook and timeline-entry behavior are covered. **App and browser composition gap:** baseline-plus-one equivalence is not yet rendered at those boundaries. | `cd ui && bunx vitest run src/features/work-outcome/hooks/useWorkOutcomeChart.test.ts src/features/timeline/state/entries/factoryTimelineEntry.append.test.ts --no-file-parallelism --maxWorkers 1` |
| A supported page lifecycle boundary durably writes the latest exact-stream checkpoint without allowing an older or other-session write to win. | `App.multi-session-checkpoint-debounce.test.tsx` — **flushes on pagehide without blocking navigation or repeating the handoff**, **flushes only when visibility changes to hidden**, and **keeps newer same-stream state authoritative while A and B lifecycle writes overlap** prove the production lifecycle handoff. The durable-transaction and shared-context persistence suites prove transaction completion and cross-session ordering. | App and persistence behavior are covered. **Real-browser gap:** the canonical browser scenario does not yet cross the lifecycle boundary and reopen the latest durable checkpoint. | `cd ui && bunx vitest run src/App.multi-session-checkpoint-debounce.test.tsx src/features/timeline/state/checkpoint-persistence/durability/timelineCheckpointPersistence.durable-transaction.test.ts src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.shared-contexts.test.ts --no-file-parallelism --maxWorkers 1` |
| Two same-origin pages keep concrete UUID, cursor, stream generation, targeted cleanup, and rendered chart state isolated. | `dashboard-shared-indexeddb-browser-contexts.integration.test.mjs` — **restores and tails two concrete sessions without cross-tab checkpoint contamination** is the canonical proof. In one browser context it opens two same-origin pages, restores separate alpha and beta cursors, rejects cross-session stream URLs, deletes only a stale alpha generation, renders independent chart ticks before and after tails, bounds error capture, and closes pages, context, server, and checkpoint data in failure-safe teardown. | Covered by the canonical merged two-page scenario. Extend this scenario only for the six-sample and lifecycle gaps; do not create a parallel cross-context scenario. | `cd ui && bunx vitest run integration/dashboard-shared-indexeddb-browser-contexts.integration.test.mjs --no-file-parallelism --maxWorkers 1` |

The remaining gaps are intentionally composition gaps. Existing hook, persistence,
and timeline tests remain the primary lower-level proofs and should not be copied
into new variants.

## Optional interactive viewport evidence

Interactive inspection is separate from automated pass/fail evidence. When a
controllable in-app browser is available, inspect the final restored chart and its
one-tail continuation at one supported small viewport and one supported large
viewport. Record only the viewport sizes, selected concrete Factory Session UUID,
lifecycle action, baseline sample count, final sample count, control usability,
readability, and unintended horizontal overflow; do not record raw Factory Event
payloads or work content.

If browser-control metadata such as `sandboxPolicy` is unavailable, record that as
an external manual-evidence limitation. It must not skip, weaken, or condition the
App, persistence, or UI Browser Integration assertions. The mandatory automated
browser command remains:

```bash
make test-ui-browser-integration
```

