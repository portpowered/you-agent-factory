import type { FactoryEvent } from "../../../../api/events";
import {
  advanceFactoryReplay,
  projectFactoryWorldAtTick,
} from "../../../../../../packages/factory-replay/src/index.js";
import { projectSnapshot } from "./projectSnapshot";
import { applyReplayEvent } from "./replayWorldState";
import { emptyReplayWorldState } from "./replayWorldStateSupport";
import type { ReplayWorldState, WorldState } from "./types";

export interface FactoryTimelineProjection {
  replayState: ReplayWorldState;
  worldState: WorldState;
}

interface ReplayCheckpoint {
  afterEventId?: string;
  replayState: ReplayWorldState;
  selectedTick: number;
}

export const hostedFactoryReplayReducer = {
  createState: emptyReplayWorldState,
  applyEvent: (state: ReplayWorldState, event: FactoryEvent) => {
    applyReplayEvent(state, event);
    return state;
  },
  projectWorld: projectSnapshot,
};

function cloneReplayState(state: ReplayWorldState): ReplayWorldState {
  return structuredClone(state);
}

function setSelectedTick(
  state: ReplayWorldState,
  selectedTick: number,
): ReplayWorldState {
  state.tick_count = selectedTick;
  return state;
}

export function reconstructFactoryReplayState(
  events: FactoryEvent[],
  selectedTick: number,
): ReplayWorldState {
  return projectFactoryWorldAtTick({
    events: kernelEvents(events),
    reducer: hostedFactoryReplayReducer,
    tick: selectedTick,
  }).state;
}

export function advanceFactoryReplayState(
  checkpoint: ReplayWorldState,
  events: FactoryEvent[],
  selectedTick: number,
): ReplayWorldState {
  return advanceFactoryReplay({
    checkpoint: {
      acceptedEventIDs: [],
      selectedTick: checkpoint.tick_count,
      state: checkpoint,
    },
    cloneState: cloneReplayState,
    events: kernelEvents(events),
    reducer: hostedFactoryReplayReducer,
    setSelectedTick,
    tick: selectedTick,
  }).state;
}

export function buildFactoryTimelineProjection(
  events: FactoryEvent[],
  selectedTick: number,
  checkpoint?: ReplayCheckpoint,
): FactoryTimelineProjection {
  const replayState = checkpoint
    ? advanceFactoryReplay({
        checkpoint: {
          acceptedEventIDs: checkpoint.afterEventId
            ? [checkpoint.afterEventId]
            : [],
          selectedTick: checkpoint.selectedTick,
          state: checkpoint.replayState,
        },
        cloneState: cloneReplayState,
        events: kernelEvents(events),
        reducer: hostedFactoryReplayReducer,
        setSelectedTick,
        tick: selectedTick,
      }).state
    : reconstructFactoryReplayState(events, selectedTick);
  return {
    replayState,
    worldState: projectSnapshot(replayState),
  };
}

function kernelEvents(
  events: FactoryEvent[],
): Parameters<typeof projectFactoryWorldAtTick>[0]["events"] {
  return events as unknown as Parameters<
    typeof projectFactoryWorldAtTick
  >[0]["events"];
}

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
): WorldState {
  return buildFactoryTimelineProjection(events, selectedTick).worldState;
}
