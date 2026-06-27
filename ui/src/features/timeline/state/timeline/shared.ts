import type { FactoryEvent } from "../../../../api/events";

export function uniqueSorted(
  values: Array<string | null | undefined>,
): string[] {
  return [
    ...new Set(
      values.filter(
        (value): value is string =>
          typeof value === "string" && value.length > 0,
      ),
    ),
  ].sort();
}

function compareFactoryEvents(left: FactoryEvent, right: FactoryEvent): number {
  if (left.context.tick !== right.context.tick) {
    return left.context.tick - right.context.tick;
  }
  if (left.context.sequence !== right.context.sequence) {
    return left.context.sequence - right.context.sequence;
  }
  if (left.context.eventTime !== right.context.eventTime) {
    return left.context.eventTime.localeCompare(right.context.eventTime);
  }
  return left.id.localeCompare(right.id);
}

function eventsAreOrdered(events: FactoryEvent[]): boolean {
  for (let index = 1; index < events.length; index += 1) {
    if (compareFactoryEvents(events[index - 1], events[index]) > 0) {
      return false;
    }
  }
  return true;
}

export function orderedEvents(events: FactoryEvent[]): FactoryEvent[] {
  return eventsAreOrdered(events)
    ? events
    : [...events].sort(compareFactoryEvents);
}
