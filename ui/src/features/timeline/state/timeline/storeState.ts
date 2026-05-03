import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import { emptyWorldRuntime, type WorldState } from "./types";

export type FactoryTimelineMode = "current" | "fixed";

export interface FactoryTimelineState {
  events: FactoryEvent[];
  latestTick: number;
  mode: FactoryTimelineMode;
  receivedEventIDs: string[];
  selectedTick: number;
  worldViewCache: Record<number, WorldState>;
  appendEvent: (event: FactoryEvent) => void;
  appendEvents: (events: FactoryEvent[]) => void;
  replaceEvents: (events: FactoryEvent[]) => void;
  reset: () => void;
  selectTick: (tick: number) => void;
  setCurrentMode: () => void;
}

export interface TimelineStoreStateDeps {
  buildFactoryTimelineSnapshot: (
    events: FactoryEvent[],
    selectedTick: number,
  ) => WorldState;
  orderedEvents: (events: FactoryEvent[]) => FactoryEvent[];
}

export function emptyDashboardSnapshot(): DashboardSnapshot {
  return {
    factory_state: "UNKNOWN",
    runtime: emptyWorldRuntime(),
    tick_count: 0,
    topology: {
      edges: [],
      submit_work_types: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 0,
  };
}

function emptyTimelineSnapshot(): WorldState {
  return {
    ...emptyDashboardSnapshot(),
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

export function emptyTimelineState(): Pick<
  FactoryTimelineState,
  "events" | "latestTick" | "mode" | "receivedEventIDs" | "selectedTick" | "worldViewCache"
> {
  return {
    events: [],
    latestTick: 0,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: 0,
    worldViewCache: {
      0: emptyTimelineSnapshot(),
    },
  };
}

export function cacheWithSnapshot(
  events: FactoryEvent[],
  cache: Record<number, WorldState>,
  tick: number,
  deps: TimelineStoreStateDeps,
): Record<number, WorldState> {
  return cache[tick]
    ? cache
    : { ...cache, [tick]: deps.buildFactoryTimelineSnapshot(events, tick) };
}

export function appendTimelineEvents(
  current: Pick<
    FactoryTimelineState,
    | "events"
    | "latestTick"
    | "mode"
    | "receivedEventIDs"
    | "selectedTick"
    | "worldViewCache"
  >,
  incomingEvents: FactoryEvent[],
  deps: TimelineStoreStateDeps,
): Pick<
  FactoryTimelineState,
  "events" | "latestTick" | "mode" | "receivedEventIDs" | "selectedTick" | "worldViewCache"
> {
  const receivedEventIDs = new Set(current.receivedEventIDs);
  const nextEvents = incomingEvents.filter((event) => !receivedEventIDs.has(event.id));

  if (nextEvents.length === 0) {
    return {
      events: current.events,
      latestTick: current.latestTick,
      mode: current.mode,
      receivedEventIDs: current.receivedEventIDs,
      selectedTick: current.selectedTick,
      worldViewCache: current.worldViewCache,
    };
  }

  const events = deps.orderedEvents([...current.events, ...nextEvents]);
  const latestTick = nextEvents.reduce(
    (maxTick, event) => Math.max(maxTick, event.context.tick),
    current.latestTick,
  );
  const selectedTick = current.mode === "current" ? latestTick : current.selectedTick;

  return {
    events,
    latestTick,
    mode: current.mode,
    receivedEventIDs: [...current.receivedEventIDs, ...nextEvents.map((event) => event.id)],
    selectedTick,
    worldViewCache: cacheWithSnapshot(events, {}, selectedTick, deps),
  };
}

export function replaceTimelineEvents(
  events: FactoryEvent[],
  deps: TimelineStoreStateDeps,
): Pick<
  FactoryTimelineState,
  "events" | "latestTick" | "mode" | "receivedEventIDs" | "selectedTick" | "worldViewCache"
> {
  const ordered = deps.orderedEvents(events);
  const latestTick = Math.max(0, ...ordered.map((event) => event.context.tick));

  return {
    events: ordered,
    latestTick,
    mode: "current",
    receivedEventIDs: ordered.map((event) => event.id),
    selectedTick: latestTick,
    worldViewCache: cacheWithSnapshot(ordered, {}, latestTick, deps),
  };
}

export function selectTimelineTick(
  current: Pick<
    FactoryTimelineState,
    "events" | "latestTick" | "worldViewCache"
  >,
  tick: number,
  deps: TimelineStoreStateDeps,
): Pick<FactoryTimelineState, "mode" | "selectedTick" | "worldViewCache"> {
  return {
    mode: "fixed",
    selectedTick: tick,
    worldViewCache: cacheWithSnapshot(current.events, current.worldViewCache, tick, deps),
  };
}

export function setTimelineCurrentMode(
  current: Pick<
    FactoryTimelineState,
    "events" | "latestTick" | "worldViewCache"
  >,
  deps: TimelineStoreStateDeps,
): Pick<FactoryTimelineState, "mode" | "selectedTick" | "worldViewCache"> {
  return {
    mode: "current",
    selectedTick: current.latestTick,
    worldViewCache: cacheWithSnapshot(
      current.events,
      current.worldViewCache,
      current.latestTick,
      deps,
    ),
  };
}


