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
| Default and named selectors route through their resolved concrete Factory Session UUIDs, never an alias-only or process-global stream. | `App.session-stream.test.tsx` — **retargets the live stream URL on session tab switches and closes the prior connection** asserts the resolved default UUID route, then a concrete named UUID route, with no `/events` fallback. The focused event-stream hook tests remain the lower-level transport proof. | Covered at the required App boundary. | `cd ui && bunx vitest run src/App.session-stream.test.tsx --no-file-parallelism --maxWorkers 1` |
| Logical remap or stale-cursor recovery invalidates and retries only the affected exact stream while another session remains unchanged. | `App.session-recovery.test.tsx` — **remaps only the stale exact session while preserving another session** drives logical remap through production App preflight, verifies the replacement concrete UUID stream has no stale cursor, and proves the second session retains its checkpoint, paused state, and tab identity. The hook suites remain the lower-level recovery proof. | Covered at the required App boundary. | `cd ui && bunx vitest run src/App.session-recovery.test.tsx --no-file-parallelism --maxWorkers 1` |
| Bounded materialized work-outcome history survives persistence and restore with its exact continuation accumulator. | `App.session-recovery.test.tsx` — **renders six restored outcomes before one uniquely applied live tail** restores and renders ticks 0–6 from IndexedDB before the stream has delivered a message. The materialized round-trip and chart hook suites remain the lower-level persistence and projection proof. | App composition covered. **Browser composition gap:** the canonical browser scenario still needs a baseline of at least six samples before live delivery. | `cd ui && bunx vitest run src/App.session-recovery.test.tsx src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.materialized-round-trip.test.ts src/features/work-outcome/hooks/useWorkOutcomeChart.test.ts --no-file-parallelism --maxWorkers 1` |
| One uniquely identified live tail extends restored history exactly once; duplicate delivery neither replaces history nor changes cumulative outcomes. | `App.session-recovery.test.tsx` — **renders six restored outcomes before one uniquely applied live tail** renders ticks 0–6, appends tick 7 once, redelivers the same event, and asserts unchanged samples plus exactly one stored tail event. The chart hook and timeline-entry tests remain the lower-level equivalence proof. | App composition covered. **Browser composition gap:** baseline-plus-one equivalence is not yet rendered in the canonical browser scenario. | `cd ui && bunx vitest run src/App.session-recovery.test.tsx src/features/work-outcome/hooks/useWorkOutcomeChart.test.ts src/features/timeline/state/entries/factoryTimelineEntry.append.test.ts --no-file-parallelism --maxWorkers 1` |
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
