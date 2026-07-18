# Factory Replay Relevant Files

Use this map when changing deterministic Factory-event replay or a host adapter
that consumes the replay kernel.

- `packages/factory-replay/src/index.js` owns framework-independent canonical
  ordering, event-ID acceptance, logical-tick selection, immutable checkpoint
  advancement, and reducer-driven replay orchestration. Accepted-tail callers
  must supply explicit state cloning and selected-tick adapters because replay
  state remains domain-owned.
- `packages/factory-replay/src/index.d.ts` is the public typed boundary. It
  uses `FactoryEvent` from `@you-agent-factory/client`; do not substitute a
  dashboard-local event type at this boundary.
- `packages/factory-replay/test/factory-replay.test.mjs` proves observable
  ordering, duplicate-ID acceptance, current selection, fixed historical
  projection, and checkpoint-plus-tail equivalence without mutation.
  `public-api.test.ts` validates the public TypeScript boundary.
- `ui/src/features/timeline/state/timeline/shared.ts` records the hosted
  ordering behavior that must remain aligned with the kernel during the
  migration.
- `ui/src/features/timeline/state/timeline/factory-replay-kernel.compatibility.test.ts`
  compares the package-selected historical projection with the existing hosted
  reducer and projection.
- `ui/src/features/timeline/state/timeline/replayWorldState.ts` and
  `projectSnapshot.ts` currently own dashboard-specific Factory-world reduction
  and projection. Keep browser state, checkpoints, persistence, and Zustand
  outside `packages/factory-replay`.
- `Makefile` targets `factory-replay-typecheck` and `factory-replay-test`
  include the package in local and CI verification lanes.
