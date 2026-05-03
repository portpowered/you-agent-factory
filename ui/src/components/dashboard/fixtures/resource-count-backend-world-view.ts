import { resourceCountAvailablePlaceID } from "./resource-count-events";

export const resourceCountBackendWorldViewCountsByTick: Record<number, Record<string, number>> = {
  1: { [resourceCountAvailablePlaceID]: 2 },
  3: { [resourceCountAvailablePlaceID]: 1 },
  4: { [resourceCountAvailablePlaceID]: 2 },
};

