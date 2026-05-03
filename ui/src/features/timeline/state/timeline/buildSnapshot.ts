import type { FactoryEvent } from "../../../../api/events";
import { projectSnapshot } from "./projectSnapshot";
import type { FactoryTimelineSnapshot } from "./snapshotTypes";
import type { WorldState } from "./types";

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
  reconstructWorldState: (events: FactoryEvent[], selectedTick: number) => WorldState,
): FactoryTimelineSnapshot {
  return projectSnapshot(reconstructWorldState(events, selectedTick));
}


