import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { useFactoryTimelineStore } from "./factoryTimelineStore";

const eventTime = "2026-05-31T12:00:00.000Z";

function timelineEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructure = timelineEvent(
  "timeline-ops-initial",
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [],
      workTypes: [
        {
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
          ],
        },
      ],
      workstations: [],
    },
  },
);

const workRequest = timelineEvent(
  "timeline-ops-work",
  2,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Ops Story",
        trace_id: "trace-ops",
        work_id: "work-ops",
        work_type_id: "task",
      },
    ],
  },
);

describe("factory timeline store operations", () => {
  afterEach(() => {
    useFactoryTimelineStore.getState().reset();
  });

  it("replaceEvents rebuilds timeline state in current mode", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);
    store.selectTick(1);

    store.replaceEvents([initialStructure]);

    const state = useFactoryTimelineStore.getState();
    expect(state.mode).toBe("current");
    expect(state.events).toHaveLength(1);
    expect(state.events[0]?.id).toBe("timeline-ops-initial");
    expect(state.latestTick).toBe(1);
    expect(state.selectedTick).toBe(1);
    expect(state.receivedEventIDs).toEqual(["timeline-ops-initial"]);
    expect(state.worldViewCache[1]).toBeDefined();
    expect(state.worldViewCache[2]).toBeUndefined();
  });

  it("ignores duplicate event ids on appendEvents", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);

    const before = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);

    const after = useFactoryTimelineStore.getState();
    expect(after.events).toEqual(before.events);
    expect(after.latestTick).toBe(before.latestTick);
    expect(after.selectedTick).toBe(before.selectedTick);
    expect(after.receivedEventIDs).toEqual(before.receivedEventIDs);
    expect(after.worldViewCache).toEqual(before.worldViewCache);
  });

  it("ignores duplicate event ids on appendEvent", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvent(initialStructure);

    const before = useFactoryTimelineStore.getState();
    store.appendEvent(initialStructure);

    const after = useFactoryTimelineStore.getState();
    expect(after.events).toEqual(before.events);
    expect(after.latestTick).toBe(before.latestTick);
    expect(after.receivedEventIDs).toEqual(before.receivedEventIDs);
  });
});
