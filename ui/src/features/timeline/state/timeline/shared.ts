import type { FactoryEvent } from "../../../../api/events";

export function uniqueSorted(values: Array<string | null | undefined>): string[] {
  return [
    ...new Set(
      values.filter(
        (value): value is string => typeof value === "string" && value.length > 0,
      ),
    ),
  ].sort();
}

export function orderedEvents(events: FactoryEvent[]): FactoryEvent[] {
  return [...events].sort((left, right) => {
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
  });
}


