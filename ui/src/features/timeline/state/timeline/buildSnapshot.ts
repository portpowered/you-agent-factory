import type { FactoryEvent } from "../../../../api/events";
import { projectSnapshot } from "./projectSnapshot";
import type { ReplayWorldState, WorldState } from "./types";

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
  reconstructWorldState: (events: FactoryEvent[], selectedTick: number) => ReplayWorldState,
): WorldState {
  return projectSnapshot(reconstructWorldState(events, selectedTick));
}


