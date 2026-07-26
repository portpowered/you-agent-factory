import type { DashboardSnapshot } from "../../api/dashboard/types";
import { buildFactoryTimelineSnapshot } from "../../features/timeline/state/factoryTimelineStore";
import { resourceCountTimelineEvents } from "./fixtures/resource-count-events";

export function resourceOccupancySnapshotForTick(
  tick: number,
): DashboardSnapshot {
  return buildFactoryTimelineSnapshot(resourceCountTimelineEvents, tick);
}
