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
- `ui/packages/factory-visualizers/src/factory-topology-replay.tsx` owns the
  public read-only topology renderer. Optional graph chrome must be resolved
  through `factory-topology-chrome.ts` before rendering; it may conditionally
  mount presentation regions but must not change the caller-provided topology,
  activity, or load projection. Keep disabled regions unmounted so they do not
  remain in the accessibility tree. The chrome-region React Flow wrapper must
  retain the node-click pointer-event preservation hook so nested read-only
  `GraphNodeButton` controls remain browser-clickable above the pan pane.
- `ui/packages/factory-visualizers/src/factory-topology-active-work.tsx` derives
  read-only workstation Work rows solely from active Dispatch work IDs and
  logical ticks. Keep the first-three-plus-overflow presentation independent
  of optional chrome, and do not add selection or mutation affordances there.
  Its row, duration, and overflow labels are required caller-localized
  messages; do not introduce English fallback copy in the public package.
- `ui/packages/factory-visualizers/scripts/verify-topology-chrome-modes.mjs`
  verifies dense runtime evidence and viewport-control behavior for every
  chrome preset at each responsive browser width. Extend this helper when a
  chrome region changes instead of testing only the default/full Storybook
  story.
- `ui/packages/factory-visualizers/src/factory-topology-flow-support.ts` holds
  pure endpoint and activity-count helpers used by the disposable React Flow
  projection; keep it free of renderer state and caller mutation.
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
- `ui/packages/factory-visualizers/src/factory-topology-flow-projection.ts`
  derives caller-validated `FactoryVisualizationLayoutV1` annotations as
  separate, inert React Flow nodes; `factory-topology-replay-nodes.tsx` owns
  their read-only node and image rendering. Keep them out of canonical topology
  layout and edge routing; visibility may remove only those projected nodes so React Flow
  fit-to-view remains derived from what is currently visible. Embedded raster
  annotations must decode into Blob URLs, revoke those URLs on replacement,
  removal, image failure, and unmount, and contain preparation failures in the
  affected annotation rather than failing the topology region. Node empty-state
  visibility is likewise derived from selected-tick Work-State counts, active
  Dispatches, and active routes; retain identity and telemetry outside its
  activity-detail region.
- `ui/packages/factory-visualizers/src/factory-topology-replay.tsx` validates
  unknown caller layout input with the client parser against the prepared
  canonical node-ID context before deriving React Flow data. Report invalid
  layout as safe field-level diagnostics and contain it to the visualizer;
  never rely on a TypeScript-only layout assertion at this boundary.
- `ui/packages/factory-visualizers/src/factory-recording-topology-replay.tsx`
  validates and owns static recording replay, then forwards the caller-owned
  layout sidecar unchanged to the controlled topology renderer. Other hosts
  should likewise supply the sidecar as an explicit presentation input rather
  than attaching it to replay state or recordings.
- `ui/src/features/dashboard/components/topology-replay/hosted-topology-replay.tsx`
  is the hosted replay boundary. It forwards its optional caller-owned layout
  sidecar to the controlled renderer and must not place it in timeline or
  stream-store state.
- `Makefile` targets `factory-replay-typecheck` and `factory-replay-test`
  include the package in local and CI verification lanes.
