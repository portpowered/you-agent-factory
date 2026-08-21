import type { FactoryEvent } from "../../../../api/events";
import { FactoryEventType } from "../../../../api/generated/openapi";
import { buildFactorySessionEventReplayTimeline } from "../../lib/factory-session-event-replay-timeline";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";

const eventTime = "2026-08-21T20:00:00Z";

test("humanizes every current canonical event type but preserves a future type raw", () => {
  const currentEventTypes = Object.values(FactoryEventType);
  const futureEventType = "FUTURE_EVENT_TYPE";
  const events = [...currentEventTypes, futureEventType].map((type, index) =>
    factoryEvent(type, index + 1),
  );

  const timeline = buildFactorySessionEventReplayTimeline(
    events,
    getFactorySessionDetailMessages("en"),
    "en",
  );

  expect(
    timeline
      .slice(0, currentEventTypes.length)
      .map(({ typeLabel }) => typeLabel),
  ).toEqual(currentEventTypes.map(humanizeEventText));
  expect(timeline.at(-1)?.typeLabel).toBe(futureEventType);
});

function factoryEvent(type: string, sequence: number): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence,
      tick: sequence,
    },
    id: `event-${sequence}`,
    payload: {},
    schemaVersion: "agent-factory.event.v1",
    type,
  };
}

function humanizeEventText(value: string): string {
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
