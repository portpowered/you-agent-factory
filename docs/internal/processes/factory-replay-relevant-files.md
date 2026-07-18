# Factory Replay Relevant Files

Use this map when changing deterministic Factory-event replay or a host adapter
that consumes the replay kernel.

- `packages/factory-replay/src/index.js` owns framework-independent canonical
  ordering, event-ID acceptance, logical-tick selection, immutable checkpoint
  advancement, reducer-driven replay orchestration, and selected-tick topology
  event selection. Accepted-tail callers must supply explicit state cloning
  and selected-tick adapters because replay state remains domain-owned.
- `packages/factory-replay/src/topology.js` owns the pure public Factory
  topology projection. Keep canonical node/connection IDs and renderer handle
  IDs aligned with `pkg/factory/contracts/factory_graph_ids.go` and the graph
  editor connection-anchor vocabulary. Unknown references must become sorted
  projection issues instead of synthesized nodes or dangling connections.
- `packages/factory-replay/src/index.d.ts` is the public typed boundary. It
  uses `FactoryEvent` from `@you-agent-factory/client`; do not substitute a
  dashboard-local event type at this boundary.
- `packages/factory-replay/test/factory-replay.test.mjs` proves observable
  ordering, duplicate-ID acceptance, current selection, fixed historical
  projection, checkpoint-plus-tail equivalence, selected-tick topology,
  stable graph identity, handle parity, and partial-topology behavior without
  mutation. `public-api.test.ts` validates the public TypeScript boundary.
- `ui/src/features/timeline/state/timeline/buildSnapshot.ts` supplies the
  hosted reducer/projection adapter to the public kernel. It owns no replay
  ordering or accepted-tail logic; keep its cloning and tick-setting adapters
  explicit because dashboard replay state remains domain-owned.
- `ui/src/features/timeline/state/factoryTimelineStore.ts` retains Zustand,
  session routing, checkpoint persistence, and diagnostics ownership while it
  calls the kernel for canonical event acceptance and replay projection.
- `ui/src/features/timeline/state/timeline/factory-replay-kernel.compatibility.test.ts`
  compares the package-selected historical projection with the existing hosted
  reducer and projection.
- `ui/src/features/timeline/state/timeline/replayWorldState.ts` owns only the
  dashboard-specific Factory-world event reducer; `projectSnapshot.ts` owns
  its projection. Keep browser state, checkpoints, persistence, and Zustand
  outside `packages/factory-replay`.
- `Makefile` targets `factory-replay-typecheck` and `factory-replay-test`
  include the package in local and CI verification lanes.
