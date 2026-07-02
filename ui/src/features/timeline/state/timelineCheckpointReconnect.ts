import type { FactoryEventReconnectCursor } from "../../../api/events";

import type { FactoryTimelineCheckpoint } from "./factoryTimelineStore";

export function reconnectCursorFromCheckpoint(
  checkpoint: FactoryTimelineCheckpoint | null,
): FactoryEventReconnectCursor | undefined {
  if (!checkpoint) {
    return undefined;
  }
  if (!checkpoint.afterEventId && checkpoint.afterSequence == null) {
    return undefined;
  }
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
  };
}
