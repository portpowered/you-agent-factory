import {
  FACTORY_TOPOLOGY_RELATIONSHIPS,
  type FactoryActivityProjection,
  type FactoryDispatchOverlayProjection,
  type FactoryLoadProjection,
  type FactoryReplayReducer,
  type FactoryResourceOccupancyProjection,
  type FactoryTopologyConnectionResult,
  type FactoryTopologyHandle,
  type FactoryTopologyProjection,
  type FactoryTopologyProjectionIssue,
  type FactoryWorkProgressProjection,
  type FactoryWorkStateCountProjection,
  factoryTopologyNodeId,
  projectFactoryActivity,
  projectFactoryLoad,
  projectFactoryStateAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgress,
} from "@you-agent-factory/factory-replay";

type ReplayEvent = Parameters<
  typeof projectFactoryStateAtTick
>[0]["events"][number];
interface State {
  ids: string[];
  selectedTick: number;
}
const reducer: FactoryReplayReducer<State> = {
  createState: (selectedTick) => ({ ids: [], selectedTick }),
  applyEvent: (state, event) => ({ ...state, ids: [...state.ids, event.id] }),
};
declare const events: readonly ReplayEvent[];
const replay = projectFactoryStateAtTick({ events, reducer, tick: 1 });
const factory: NonNullable<
  Parameters<typeof projectFactoryLoad>[0]["factory"]
> = {
  name: "consumer",
  resources: [{ capacity: 2, id: "gpu", name: "gpu" }],
  workTypes: [
    {
      id: "story",
      name: "story",
      states: [{ id: "ready", name: "ready", type: "INITIAL" }],
    },
  ],
};
const topologyEvents: Parameters<
  typeof projectFactoryTopologyAtTick
>[0]["events"] = [
  {
    context: {
      eventTime: "2026-07-18T00:00:01Z",
      sequence: 0,
      tick: 1,
    },
    id: "factory",
    payload: { factory },
    schemaVersion: "agent-factory.event.v1",
    type: "INITIAL_STRUCTURE_REQUEST",
  },
];
const topology: FactoryTopologyProjection = projectFactoryTopologyAtTick({
  events: topologyEvents,
  tick: replay.selectedTick,
});
const load: FactoryLoadProjection = projectFactoryLoad({
  activeDispatches: [],
  factory,
  selectedTick: replay.selectedTick,
  works: [{ id: "queued", stateId: "ready", workTypeId: "story" }],
});
const activity: FactoryActivityProjection = projectFactoryActivity({
  activeDispatches: [],
  factory,
  selectedTick: replay.selectedTick,
});
const progress: FactoryWorkProgressProjection = projectFactoryWorkProgress({
  activeWorkIds: [],
  selectedTick: replay.selectedTick,
  works: [{ id: "queued", state: { category: "INITIAL" } }],
});
const semanticNodeId = factoryTopologyNodeId("resource", "gpu");
const semanticHandle: FactoryTopologyHandle = {
  id: FACTORY_TOPOLOGY_RELATIONSHIPS["work-type-state"].source.handleId,
  role: "source",
};
const structuredIssue: FactoryTopologyProjectionIssue | undefined =
  topology.issues[0];
const connectionResult: FactoryTopologyConnectionResult | undefined = undefined;
const overlay: FactoryDispatchOverlayProjection | undefined =
  activity.activeDispatchOverlays[0];
const occupancy: FactoryResourceOccupancyProjection | undefined =
  load.resourceOccupancy[0];
const count: FactoryWorkStateCountProjection | undefined =
  load.workStateCounts[0];
void {
  connectionResult,
  count,
  occupancy,
  overlay,
  progress,
  semanticHandle,
  semanticNodeId,
  structuredIssue,
  topology,
};
