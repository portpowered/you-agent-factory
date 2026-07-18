import {
  FACTORY_TOPOLOGY_RELATIONSHIPS,
  factoryTopologyNodeId,
  projectFactoryActivity,
  projectFactoryLoad,
  projectFactoryStateAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgress,
} from "@you-agent-factory/factory-replay";

const events = [
  {
    schemaVersion: "agent-factory.event.v1",
    id: "later",
    type: "WORK_REQUEST",
    context: { eventTime: "2026-07-18T00:00:02Z", sequence: 0, tick: 2 },
    payload: { type: "FACTORY_REQUEST_BATCH", works: [] },
  },
  {
    schemaVersion: "agent-factory.event.v1",
    id: "selected",
    type: "WORK_REQUEST",
    context: { eventTime: "2026-07-18T00:00:01Z", sequence: 0, tick: 1 },
    payload: { type: "FACTORY_REQUEST_BATCH", works: [] },
  },
];
const replay = projectFactoryStateAtTick({
  events,
  tick: 1,
  reducer: {
    createState: (selectedTick) => ({ ids: [], selectedTick }),
    applyEvent: (state, event) => ({ ...state, ids: [...state.ids, event.id] }),
  },
});
const progress = projectFactoryWorkProgress({
  activeWorkIds: [],
  selectedTick: replay.selectedTick,
  works: [
    { id: "failed", state: { category: "FAILED" } },
    { id: "queued", state: { category: "INITIAL" } },
  ],
});
const factory = {
  name: "installed-consumer",
  resources: [{ capacity: 2, id: "gpu", name: "gpu" }],
  workers: [
    {
      id: "writer",
      name: "writer",
      resources: [{ capacity: 1, name: "gpu" }],
    },
  ],
  workTypes: [
    {
      id: "story",
      name: "story",
      states: [
        { id: "ready", name: "ready", type: "INITIAL" },
        { id: "done", name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      id: "write",
      inputs: [{ state: "ready", workType: "story" }],
      name: "write",
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 1, name: "gpu" }],
      worker: "writer",
    },
  ],
};
const topologyEvents = [
  {
    schemaVersion: "agent-factory.event.v1",
    id: "factory",
    type: "INITIAL_STRUCTURE_REQUEST",
    context: { eventTime: "2026-07-18T00:00:00Z", sequence: 0, tick: 1 },
    payload: { factory },
  },
];
const presentation = {
  positions: new Map(),
  selectedTick: 1,
};
const originalEvents = structuredClone(topologyEvents);
const firstTopology = projectFactoryTopologyAtTick({
  events: topologyEvents,
  tick: presentation.selectedTick,
});
presentation.positions.set(factoryTopologyNodeId("resource", "gpu"), {
  x: 100,
  y: 50,
});
projectFactoryTopologyAtTick({
  events: [
    {
      ...topologyEvents[0],
      id: "other-factory",
      payload: { factory: { name: "other" } },
    },
  ],
  tick: presentation.selectedTick,
});
const repeatedTopology = projectFactoryTopologyAtTick({
  events: topologyEvents,
  tick: presentation.selectedTick,
});
const load = projectFactoryLoad({
  activeDispatches: [
    {
      id: "dispatch-1",
      resourceClaims: [{ quantity: 1, resourceName: "gpu" }],
    },
  ],
  factory,
  selectedTick: 1,
  works: [{ id: "work-1", stateId: "ready", workTypeId: "story" }],
});
const activity = projectFactoryActivity({
  activeDispatches: [
    {
      id: "dispatch-1",
      inputRoutes: [{ stateId: "ready", workTypeId: "story" }],
      resourceNames: ["gpu"],
      startedTick: 1,
      transitionId: "write",
      workIds: ["work-1"],
    },
  ],
  factory,
  selectedTick: 1,
});
if (replay.appliedEvents.map(({ id }) => id).join(",") !== "selected") {
  throw new Error("packed replay did not reconstruct the selected tick");
}
if (
  progress.total !== 2 ||
  progress.counts.failed !== 1 ||
  progress.counts.queued !== 1
) {
  throw new Error("packed replay did not classify Work exclusively");
}
if (
  !firstTopology.ok ||
  JSON.stringify(firstTopology) !== JSON.stringify(repeatedTopology) ||
  JSON.stringify(topologyEvents) !== JSON.stringify(originalEvents)
) {
  throw new Error("packed topology projection was not disposable and pure");
}
const nodesById = new Map(firstTopology.nodes.map((node) => [node.id, node]));
for (const connection of firstTopology.connections) {
  const relationship = FACTORY_TOPOLOGY_RELATIONSHIPS[connection.kind];
  const source = nodesById.get(connection.source.nodeId);
  const target = nodesById.get(connection.target.nodeId);
  if (
    !source?.handles.some(
      (handle) =>
        handle.id === connection.source.handleId &&
        handle.id === relationship.source.handleId,
    ) ||
    !target?.handles.some(
      (handle) =>
        handle.id === connection.target.handleId &&
        handle.id === relationship.target.handleId,
    )
  ) {
    throw new Error("packed topology connection did not bind semantic handles");
  }
}
if (
  load.workStateCounts.find(({ workStateId }) => workStateId === "story:ready")
    ?.count !== 1 ||
  load.resourceOccupancy[0]?.occupiedQuantity !== 1 ||
  activity.activeDispatchOverlays[0]?.dispatchId !== "dispatch-1"
) {
  throw new Error(
    `packed selected-tick projections were incomplete: ${JSON.stringify({ activity, load })}`,
  );
}
process.stdout.write(
  "reconstructed tick 1 and projected topology, load, activity, and progress\n",
);
