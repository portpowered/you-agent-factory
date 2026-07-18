import type { FactoryEvent } from "./contracts.js";

/** The reconnect position acknowledged by a downstream replay consumer. */
export interface FactoryEventCursor {
  afterEventId: string;
  afterSequence: number;
  tick: number;
}

/** Return the session-scoped sequence when available, otherwise the event-log sequence. */
export function getFactoryEventEffectiveSequence(event: FactoryEvent): number {
  return event.context.sessionSequence ?? event.context.sequence;
}

/** Compare canonical Factory events by replay position. */
export function compareFactoryEvents(
  left: FactoryEvent,
  right: FactoryEvent,
): number {
  const tickDifference = left.context.tick - right.context.tick;
  if (tickDifference !== 0) return tickDifference;

  const sequenceDifference =
    getFactoryEventEffectiveSequence(left) -
    getFactoryEventEffectiveSequence(right);
  if (sequenceDifference !== 0) return sequenceDifference;

  const eventLogSequenceDifference =
    left.context.sequence - right.context.sequence;
  if (eventLogSequenceDifference !== 0) return eventLogSequenceDifference;

  const timeDifference = left.context.eventTime.localeCompare(
    right.context.eventTime,
  );
  return timeDifference !== 0
    ? timeDifference
    : left.id.localeCompare(right.id);
}

/** Return a canonically ordered copy without mutating caller-owned events or arrays. */
export function orderFactoryEvents(
  events: readonly FactoryEvent[],
): FactoryEvent[] {
  return [...events].sort(compareFactoryEvents);
}

/** Build the reconnect cursor acknowledged at an accepted Factory event. */
export function createFactoryEventCursor(
  event: FactoryEvent,
): FactoryEventCursor {
  return {
    afterEventId: event.id,
    afterSequence: getFactoryEventEffectiveSequence(event),
    tick: event.context.tick,
  };
}
