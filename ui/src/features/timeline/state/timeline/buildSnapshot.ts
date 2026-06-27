import type { FactoryEvent } from "../../../../api/events";
import { projectSnapshot } from "./projectSnapshot";
import type { ReplayWorldState, WorldState } from "./types";

export interface FactoryTimelineProjection {
  replayState: ReplayWorldState;
  worldState: WorldState;
}

export function buildFactoryTimelineProjection(
  events: FactoryEvent[],
  selectedTick: number,
  reconstructWorldState: (
    events: FactoryEvent[],
    selectedTick: number,
  ) => ReplayWorldState,
): FactoryTimelineProjection {
  const replayState = reconstructWorldState(events, selectedTick);
  return {
    replayState,
    worldState: projectSnapshot(replayState),
  };
}

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
  reconstructWorldState: (
    events: FactoryEvent[],
    selectedTick: number,
  ) => ReplayWorldState,
): WorldState {
  return buildFactoryTimelineProjection(
    events,
    selectedTick,
    reconstructWorldState,
  ).worldState;
}
