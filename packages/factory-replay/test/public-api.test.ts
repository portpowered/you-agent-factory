import type { FactoryEvent } from "@you-agent-factory/client";
import {
  advanceFactoryReplay,
  createFactoryReplayCheckpoint,
  initializeFactoryReplay,
  type FactoryReplayReducer,
  type FactoryTopologyNode,
  projectFactoryTopology,
  projectFactoryTopologyAtTick,
} from "../src/index.js";

interface ReplayState {
  ids: string[];
}

const reducer: FactoryReplayReducer<ReplayState, readonly string[]> = {
  createState: () => ({ ids: [] }),
  applyEvent: (state, event) => ({ ids: [...state.ids, event.id] }),
  projectWorld: (state) => state.ids,
};

const event = {
  context: {
    eventTime: "2026-07-18T05:00:00Z",
    sequence: 1,
    tick: 1,
  },
  id: "event-1",
  payload: {},
  type: "SESSION_STARTED",
} as FactoryEvent;

const result = initializeFactoryReplay({
  events: [event],
  reducer,
  selection: { mode: "current" },
});

void result.world;

const checkpoint = createFactoryReplayCheckpoint(result, (state) => ({
  ids: [...state.ids],
}));
const advanced = advanceFactoryReplay({
  checkpoint,
  cloneState: (state) => ({ ids: [...state.ids] }),
  events: [],
  reducer,
  setSelectedTick: (state) => state,
  tick: 1,
});

void advanced.checkpoint;

const topology = projectFactoryTopology({
  factory: {
    name: "typed-factory",
    resources: [{ capacity: 1, name: "slot" }],
  },
  selectedTick: 1,
});
const typedNode: FactoryTopologyNode | undefined = topology.nodes[0];
void typedNode;

void projectFactoryTopologyAtTick({ events: [event], tick: 1 }).issues;
