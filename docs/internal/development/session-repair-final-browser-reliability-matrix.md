# Session Repair Final Browser Reliability Evidence Matrix

This matrix maps each browser persistence guarantee to observable automated
behavior. It is reviewer guidance, not a test inventory or a release gate of
its own. The listed tests exercise runtime, rendered, or durable browser
outcomes; source scans, route inventories, documentation link checks, asset
internals, and test registrations are not accepted as behavioral proof.

Run commands from the repository root. The focused unit command uses one worker
so App and IndexedDB harnesses do not contend for shared process resources.

## Commands

`Focused UI persistence suites`:

```bash
cd ui && bun x vitest run \
  src/App.session-stream.test.tsx \
  src/App.session-recovery.test.tsx \
  src/App.multi-session-timeline-isolation.test.tsx \
  src/App.multi-session-checkpoint-debounce.test.tsx \
  src/features/dashboard/lib/preflight/dashboard-session-sync-preflight.test.ts \
  src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.test.tsx \
  src/features/dashboard/hooks/event-stream/useFactoryEventStream.reconnect.test.tsx \
  src/features/dashboard/hooks/event-stream/useFactoryEventStream.stale-cursor.test.tsx \
  src/features/dashboard/lib/session-persistence/diagnostics.test.ts \
  src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.materialized-round-trip.test.ts \
  src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.replacement-failure.test.ts \
  src/features/timeline/state/checkpoint-persistence/durability/timelineCheckpointPersistence.durable-transaction.test.ts \
  src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.ordered-writes.test.ts \
  src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.shared-contexts.test.ts \
  --no-file-parallelism --maxWorkers 1
```

`UI Browser Integration` is the mandatory real-browser lane and has the same
local owner as `make ui-integration-test`:

```bash
make ui-integration-test
```

## Automated evidence

| Guarantee | Observable invariant and concrete behavioral proof | Executing command or mandatory lane |
| --- | --- | --- |
| Quiet reload | `ui/src/App.session-stream.test.tsx`, **renders a restored tick-zero checkpoint after preflight and a quiet stream open**: hydration and preflight complete, the restored dashboard becomes current, and no event message is required. | `Focused UI persistence suites`; also `make ui-test`. |
| Resolved identity and remap | `ui/src/App.session-stream.test.tsx`, **retargets the live stream URL on session tab switches and closes the prior connection**, proves default and named selectors open streams on concrete resolved Factory Session UUIDs. `ui/src/App.session-recovery.test.tsx`, **remaps only the stale exact session while preserving another session**, proves the replacement stream opens without the old cursor. | `Focused UI persistence suites`; also `make ui-test`. |
| A-to-B isolation | `ui/src/App.multi-session-timeline-isolation.test.tsx`, **pauses only A while B keeps its persisted timeline, cursor, and live stream**, and `ui/src/App.multi-session-checkpoint-debounce.test.tsx`, **flushes each stream's latest checkpoint during rapid A to B to A switching**, prove tab-local timeline and persistence ownership. | `Focused UI persistence suites`; also `make ui-test`. |
| Materialized resume | `ui/src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.materialized-round-trip.test.ts` preserves the bounded retained baseline and exact continuation accumulator. `ui/src/App.session-recovery.test.tsx`, **renders six restored outcomes before one uniquely applied live tail**, proves the restored projection is visible before a deduplicated continuation. | `Focused UI persistence suites`; also `make ui-test`. |
| Durable ordering | `ui/src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.ordered-writes.test.ts` proves strictly newer admission and rejection of older or equal saves. `ui/src/features/timeline/state/checkpoint-persistence/durability/timelineCheckpointPersistence.durable-transaction.test.ts` proves a write is not reported complete before its transaction completes. | `Focused UI persistence suites`; also `make ui-test`. |
| Lifecycle flush | `ui/src/App.multi-session-checkpoint-debounce.test.tsx` proves unmount, `pagehide`, hidden visibility, rejected-write containment, and overlapping A/B handoff behavior while preserving the newer exact-stream state. | `Focused UI persistence suites`; the production `pagehide` path is also exercised by `UI Browser Integration`. |
| Shared IndexedDB | `ui/src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.shared-contexts.test.ts` proves independent-context ordering, exact-session clearing, and stale-generation isolation. `ui/integration/dashboard-shared-indexeddb-browser-contexts.integration.test.mjs`, **restores isolated concrete sessions and resumes one tail across lifecycle**, proves two same-origin pages retain separate UUIDs, generations, cursors, and rendered samples across durable handoff. | `Focused UI persistence suites` for the unit proof; mandatory `UI Browser Integration` / `make ui-integration-test` for the two-page proof. |
| Bounded diagnostics | `ui/src/features/dashboard/lib/session-persistence/diagnostics.test.ts` proves the newest 100 redaction-safe outcome records are retained as defensive snapshots with non-reversible correlation tokens, while payload-shaped or caller-supplied actions are rejected. Diagnostics must never contain raw Factory Events, Work content, identities, checkpoint payloads, or unbounded console/network history. | `Focused UI persistence suites`; also `make ui-test`. |
| Failure recovery | `ui/src/features/dashboard/hooks/event-stream/useFactoryEventStream.reconnect.test.tsx` proves one cursor-free retry after a stale reconnect cursor. `ui/src/features/dashboard/hooks/event-stream/useFactoryEventStream.stale-cursor.test.tsx` proves only the affected checkpoint and runtime queries are cleared. `ui/src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.replacement-failure.test.ts` proves request, transaction, and cancellation failures preserve the last committed checkpoint and allow a later replacement. | `Focused UI persistence suites`; browser recovery remains mandatory in `UI Browser Integration`. |

The browser lane additionally runs
`ui/integration/dashboard-session-recovery.integration.test.mjs` for matching
checkpoint reuse, stale-generation rejection, and accessible retry behavior.
Lower-level hook, persistence, and timeline tests remain the primary focused
proofs and should not be copied into new variants.

## Final-revision gate record

Results in this section must be refreshed after the last documentation or
contract change; results from an earlier revision are not reusable.

The following commands ran on 2026-07-15 at 10:44 UTC against the story 002
working tree based on `66d01d7b0d` and containing this matrix revision.

| Gate | Final-revision result |
| --- | --- |
| Focused UI persistence suites | Passed: 14 files; 81 tests passed and 2 expected failures remained expected. |
| `make ui-test` | Passed: 789 files; 5,495 tests passed and 2 expected failures remained expected. |
| `make ui-integration-test` / `UI Browser Integration` | Passed: 18 files and 50 mandatory browser tests, including quiet recovery and the two-page shared-IndexedDB scenario. |
| `make ui-lint` | Passed, including localization, semantic-token, feature-boundary, shared-boundary, form-control, and disclosure guards. |
| UI typecheck | Passed through the `make verify-fast` `make typecheck` phase. |
| `make verify-fast` | Did not pass. Typecheck and all 789 UI files passed; the short Go phase failed only in unchanged `internal/contractstaging/manifest_test.go`: `TestMergeCommitTipResolvesSourceCommitWithoutFalseShallowFailure` and `TestMergeCommitInRevListWithoutPathChangesResolvesSourceCommit` attempted `git checkout master` after system Git configuration initialized their temporary repositories on `main`. A focused rerun reproduced both failures, and `git diff --exit-code origin/main -- internal/contractstaging/manifest_test.go` confirmed this story did not change the failing file. No aggregate pass is claimed. |
| `make api-smoke` | Not required: this closeout made no public REST, authored OpenAPI, handler, or generated-client change. |

## Optional interactive viewport evidence

Interactive inspection is separate from automated pass/fail evidence. When a
controllable in-app browser is available, a maintainer may inspect the restored
chart and one-tail continuation at one supported small and one supported large
viewport. Record only viewport sizes, the selected concrete Factory Session
UUID, lifecycle action, baseline and final sample counts, control usability,
readability, and unintended horizontal overflow. Do not record raw Factory
Event payloads or Work content.

Planner-side browser-control metadata such as `sandboxPolicy` is not a product
input and is not consulted by App, persistence, or browser-integration tests.
Its absence prevents only this optional manual observation. The old Batch 003
planner queue artifact is therefore neither a production failure nor a reason
to skip or weaken an assertion. Completion depends on the mandatory automated
commands above, never on this manual step.
