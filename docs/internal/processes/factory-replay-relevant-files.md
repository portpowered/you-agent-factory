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
  Backend resource availability arcs are internal workstation IO evidence;
  project their authored resource relationship once, not as Work-State routes.
- `pkg/factory/events/snapshot/initial_structure.go` owns the canonical public
  Factory definition serialized into initial and replacement topology events.
  Preserve durable public entity IDs and worker/workstation resource
  requirements here; selected-tick consumers cannot recover evidence omitted
  by this producer.
- `packages/factory-replay/src/activity.js` owns pure selected-tick active
  Dispatch, affected-workstation, Work-reference, and resource-occupancy
  projection. Missing optional relationships retain identifiable Dispatches
  and use structured issues or unavailable occupancy evidence.
- `packages/factory-replay/src/progress.js` owns the pure, mutually exclusive
  customer Work progress partition. Keep failed, completed, active, queued,
  and unclassified precedence centralized there; canonical event-to-evidence
  reconstruction remains in `index.js`.
- `packages/factory-replay/src/index.d.ts` is the public typed boundary. It
  uses `FactoryEvent` from `@you-agent-factory/client`; do not substitute a
  dashboard-local event type at this boundary.
- `packages/factory-replay/test/factory-replay.test.mjs` proves observable
  ordering, duplicate-ID acceptance, current selection, fixed historical
  projection, checkpoint-plus-tail equivalence, selected-tick topology,
  stable graph identity, handle parity, and partial-topology behavior without
  mutation. Its backend-boundary regression consumes events generated through
  the Go snapshot producer rather than hand-authoring richer event factories.
  `public-api.test.ts` validates the public TypeScript boundary.
- `packages/factory-replay/test/activity.test.mjs` proves Dispatch
  start/completion, concurrency, historical and same-tick selection, stable
  topology references, occupancy certainty, partial input, and immutability.
- `packages/factory-replay/test/progress.test.mjs` proves every Work category,
  precedence overlap, lifecycle and recovery transitions, same-tick ordering,
  identity deduplication, system-Work filtering, incomplete-data fallback, and
  category-total equality.
- `ui/src/features/timeline/state/timeline/buildSnapshot.ts` supplies the
  hosted reducer/projection adapter to the public kernel. It owns no replay
  ordering or accepted-tail logic; keep its cloning and tick-setting adapters
  explicit because dashboard replay state remains domain-owned.
- `ui/src/features/timeline/state/timeline/projections/projectFactoryReplay.ts`
  maps the hosted selected replay state into the package's direct-state topology,
  activity, occupancy, and Work-progress operations, then derives the
  legacy-shaped dashboard topology/runtime compatibility view from those shared
  decisions. Keep this adapter free of renderer types and derive it from
  checkpoint-safe replay state rather than a hidden copy of event history.
- `ui/src/features/timeline/state/factoryTimelineStore.ts` retains Zustand,
  session routing, checkpoint persistence, and diagnostics ownership while it
  calls the kernel for canonical event acceptance and replay projection.
- `ui/src/features/timeline/state/timeline/factory-replay-kernel.compatibility.test.ts`
  compares the package-selected historical projection with the existing hosted
  reducer and projection.
- `ui/src/features/timeline/state/timeline/projections/factory-replay-projections.compatibility.test.ts`
  proves the hosted adapter preserves shared topology IDs and handle parity,
  Dispatch/resource occupancy, exclusive Work progress, and same-tick
  recomputation through observable selected-tick output.
- `ui/src/features/timeline/state/timeline/replayWorldState.ts` owns only the
  dashboard-specific Factory-world event reducer; `projectSnapshot.ts` owns
  its projection. Keep browser state, checkpoints, persistence, and Zustand
  outside `packages/factory-replay`.
- `ui/packages/factory-visualizers/src/factory-topology-replay.tsx` converts
  caller-validated `FactoryVisualizationLayoutV1` annotations into separate,
  inert React Flow nodes. Keep them out of canonical topology layout and edge
  routing; visibility may remove only those projected nodes so React Flow
  fit-to-view remains derived from what is currently visible. Embedded raster
  annotations must decode into Blob URLs, revoke those URLs on replacement,
  removal, image failure, and unmount, and contain preparation failures in the
  affected annotation rather than failing the topology region. Node empty-state
  visibility is likewise derived from selected-tick Work-State counts, active
  Dispatches, and active routes; retain identity and telemetry outside its
  activity-detail region.
- `Makefile` targets `factory-replay-typecheck` and `factory-replay-test`
  include the package in local and CI verification lanes.
