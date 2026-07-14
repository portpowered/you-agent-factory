import type { FactoryEvent } from "../../../../api/events";

export interface FactoryEventTimelinePosition {
  eventID: string;
  eventTime: string;
  sequence: number;
  tick: number;
}

export function compareFactoryEvents(
  left: FactoryEvent,
  right: FactoryEvent,
): number {
  return compareTimelinePositions(
    left.context,
    left.id,
    right.context,
    right.id,
  );
}

export function compareEventToTimelinePosition(
  event: FactoryEvent,
  position: FactoryEventTimelinePosition,
): number {
  return compareTimelinePositions(
    event.context,
    event.id,
    position,
    position.eventID,
  );
}

export function orderedFactoryEvents(events: FactoryEvent[]): FactoryEvent[] {
  return [...events].sort(compareFactoryEvents);
}

function compareTimelinePositions(
  left: Pick<FactoryEvent["context"], "eventTime" | "sequence" | "tick">,
  leftID: string,
  right: Pick<FactoryEvent["context"], "eventTime" | "sequence" | "tick">,
  rightID: string,
): number {
  if (left.tick !== right.tick) {
    return left.tick - right.tick;
  }
  if (left.sequence !== right.sequence) {
    return left.sequence - right.sequence;
  }
  if (left.eventTime !== right.eventTime) {
    return left.eventTime.localeCompare(right.eventTime);
  }
  return leftID.localeCompare(rightID);
}
