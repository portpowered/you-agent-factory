import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import {
  appendTimelineEvents,
  cacheWithSnapshot,
  emptyTimelineState,
  replaceTimelineEvents,
  selectTimelineTick,
  setTimelineCurrentMode,
  type TimelineStoreStateDeps,
} from "./storeState";
import type { WorldState } from "./types";

const eventTime = "2026-05-31T12:00:00.000Z";

function timelineEvent(id: string, tick: number): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload: {},
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  };
}

function snapshotForTick(tick: number): WorldState {
  return {
    factory_state: "RUNNING",
    relationsByWorkID: {},
    runtime: {
      failed_work_items_by_id: {},
      place_occupancy: {},
      terminal_work_by_id: {},
      work_items_by_id: {},
    },
    tick_count: tick,
    topology: {
      edges: [],
      submit_work_types: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    tracesByWorkID: {},
    uptime_seconds: 0,
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

const deps: TimelineStoreStateDeps = {
  buildFactoryTimelineSnapshot: (_events, tick) => snapshotForTick(tick),
  orderedEvents: (events) =>
    [...events].sort((left, right) => left.context.tick - right.context.tick),
};

describe("timeline storeState helpers", () => {
  it("returns current state when no incoming events are provided", () => {
    const event = timelineEvent("noop-1", 1);
    const current = {
      ...emptyTimelineState(),
      events: [event],
      latestTick: 1,
      receivedEventIDs: [event.id],
      selectedTick: 1,
      worldViewCache: { 1: snapshotForTick(1) },
    };

    const next = appendTimelineEvents(current, [], deps);

    expect(next).toEqual({
      events: current.events,
      latestTick: current.latestTick,
      mode: current.mode,
      receivedEventIDs: current.receivedEventIDs,
      selectedTick: current.selectedTick,
      worldViewCache: current.worldViewCache,
    });
  });

  it("returns current state when all appended events are duplicates", () => {
    const event = timelineEvent("dup-1", 1);
    const current = {
      ...emptyTimelineState(),
      events: [event],
      latestTick: 1,
      receivedEventIDs: [event.id],
      selectedTick: 1,
      worldViewCache: { 1: snapshotForTick(1) },
    };

    const next = appendTimelineEvents(current, [event], deps);

    expect(next).toEqual({
      events: current.events,
      latestTick: current.latestTick,
      mode: current.mode,
      receivedEventIDs: current.receivedEventIDs,
      selectedTick: current.selectedTick,
      worldViewCache: current.worldViewCache,
    });
  });

  it("keeps selected tick in fixed mode when appending events", () => {
    const first = timelineEvent("fixed-1", 1);
    const second = timelineEvent("fixed-2", 2);
    const current = {
      ...emptyTimelineState(),
      events: [first],
      latestTick: 1,
      mode: "fixed" as const,
      receivedEventIDs: [first.id],
      selectedTick: 1,
      worldViewCache: { 1: snapshotForTick(1) },
    };

    const next = appendTimelineEvents(current, [second], deps);

    expect(next.mode).toBe("fixed");
    expect(next.selectedTick).toBe(1);
    expect(next.latestTick).toBe(2);
    expect(next.events.map((event) => event.id)).toEqual([
      "fixed-1",
      "fixed-2",
    ]);
  });

  it("reuses cached snapshots when selecting a tick", () => {
    const cached = snapshotForTick(2);
    const buildSpy = vi.fn(deps.buildFactoryTimelineSnapshot);
    const current = {
      events: [timelineEvent("select-1", 1), timelineEvent("select-2", 2)],
      latestTick: 2,
      worldViewCache: { 2: cached },
    };

    const next = selectTimelineTick(current, 2, {
      ...deps,
      buildFactoryTimelineSnapshot: buildSpy,
    });

    expect(next.mode).toBe("fixed");
    expect(next.selectedTick).toBe(2);
    expect(next.worldViewCache[2]).toBe(cached);
    expect(buildSpy).not.toHaveBeenCalled();
  });

  it("restores current mode at the latest tick", () => {
    const latest = snapshotForTick(3);
    const current = {
      events: [timelineEvent("current-1", 1), timelineEvent("current-3", 3)],
      latestTick: 3,
      worldViewCache: { 3: latest },
    };

    const next = setTimelineCurrentMode(current, deps);

    expect(next.mode).toBe("current");
    expect(next.selectedTick).toBe(3);
    expect(next.worldViewCache[3]).toBe(latest);
  });

  it("replaceTimelineEvents rebuilds current-mode state from scratch", () => {
    const first = timelineEvent("replace-1", 1);
    const second = timelineEvent("replace-2", 2);

    const next = replaceTimelineEvents([second, first], deps);

    expect(next.mode).toBe("current");
    expect(next.events.map((event) => event.id)).toEqual([
      "replace-1",
      "replace-2",
    ]);
    expect(next.latestTick).toBe(2);
    expect(next.selectedTick).toBe(2);
    expect(next.receivedEventIDs).toEqual(["replace-1", "replace-2"]);
    expect(next.worldViewCache[2]).toEqual(snapshotForTick(2));
  });

  it("cacheWithSnapshot leaves cache unchanged when tick is already cached", () => {
    const cached = snapshotForTick(4);
    const cache = { 4: cached };
    const events = [timelineEvent("cache-4", 4)];

    const next = cacheWithSnapshot(events, cache, 4, deps);

    expect(next).toBe(cache);
  });
});
