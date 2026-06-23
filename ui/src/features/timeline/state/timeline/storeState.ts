import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import type { FactoryTimelineProjection } from "./buildSnapshot";
import { projectSnapshot } from "./projectSnapshot";
import {
  emptyWorldRuntime,
  type ReplayWorldState,
  type WorldState,
} from "./types";

export type FactoryTimelineMode = "current" | "fixed";

export interface FactoryTimelineState {
  currentReplayCheckpoint?: FactoryTimelineCheckpoint;
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
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  selectTick: (tick: number) => void;
  setCurrentMode: () => void;
}

export interface FactoryTimelineCheckpoint {
  afterEventId?: string;
  afterSequence?: number;
  replayState: ReplayWorldState;
  selectedTick: number;
}

interface TimelineCheckpointProjection {
  checkpoint: FactoryTimelineCheckpoint;
  worldState: WorldState;
}

export interface TimelineStoreStateDeps {
  buildFactoryTimelineProjection: (
    events: FactoryEvent[],
    selectedTick: number,
    checkpoint?: FactoryTimelineCheckpoint,
  ) => FactoryTimelineProjection;
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
  | "currentReplayCheckpoint"
  | "events"
  | "latestTick"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  return {
    currentReplayCheckpoint: undefined,
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

function latestAppliedEvent(
  events: FactoryEvent[],
  tick: number,
): FactoryEvent | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.context.tick <= tick) {
      return event;
    }
  }
  return undefined;
}

function checkpointFromProjection(
  events: FactoryEvent[],
  selectedTick: number,
  projection: FactoryTimelineProjection,
): TimelineCheckpointProjection {
  const latestEvent = latestAppliedEvent(events, selectedTick);
  return {
    checkpoint: {
      afterEventId: latestEvent?.id,
      afterSequence:
        latestEvent?.context.sessionSequence ?? latestEvent?.context.sequence,
      replayState: projection.replayState,
      selectedTick,
    },
    worldState: projection.worldState,
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

function projectCurrentTick(
  events: FactoryEvent[],
  selectedTick: number,
  deps: TimelineStoreStateDeps,
  checkpoint?: FactoryTimelineCheckpoint,
): TimelineCheckpointProjection {
  const usableCheckpoint =
    checkpoint && checkpoint.selectedTick <= selectedTick
      ? checkpoint
      : undefined;
  const projectionEvents = usableCheckpoint
    ? events.filter(
        (event) => event.context.tick > usableCheckpoint.selectedTick,
      )
    : events;
  return checkpointFromProjection(
    events,
    selectedTick,
    deps.buildFactoryTimelineProjection(
      projectionEvents,
      selectedTick,
      usableCheckpoint,
    ),
  );
}

export function appendTimelineEvents(
  current: Pick<
    FactoryTimelineState,
    | "events"
    | "currentReplayCheckpoint"
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
  | "events"
  | "currentReplayCheckpoint"
  | "latestTick"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  const receivedEventIDs = new Set(current.receivedEventIDs);
  const nextEvents = incomingEvents.filter(
    (event) => !receivedEventIDs.has(event.id),
  );

  if (nextEvents.length === 0) {
    return {
      events: current.events,
      currentReplayCheckpoint: current.currentReplayCheckpoint,
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
  const selectedTick =
    current.mode === "current" ? latestTick : current.selectedTick;
  const currentProjection =
    current.mode === "current"
      ? projectCurrentTick(
          events,
          selectedTick,
          deps,
          current.currentReplayCheckpoint,
        )
      : current.currentReplayCheckpoint;

  return {
    events,
    currentReplayCheckpoint:
      currentProjection && "checkpoint" in currentProjection
        ? currentProjection.checkpoint
        : currentProjection,
    latestTick,
    mode: current.mode,
    receivedEventIDs: [
      ...current.receivedEventIDs,
      ...nextEvents.map((event) => event.id),
    ],
    selectedTick,
    worldViewCache:
      current.mode === "current" &&
      currentProjection &&
      "worldState" in currentProjection
        ? { [selectedTick]: currentProjection.worldState }
        : cacheWithSnapshot(events, {}, selectedTick, deps),
  };
}

export function replaceTimelineEvents(
  events: FactoryEvent[],
  deps: TimelineStoreStateDeps,
): Pick<
  FactoryTimelineState,
  | "currentReplayCheckpoint"
  | "events"
  | "latestTick"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  const ordered = deps.orderedEvents(events);
  const latestTick = Math.max(0, ...ordered.map((event) => event.context.tick));
  const currentProjection = projectCurrentTick(ordered, latestTick, deps);

  return {
    currentReplayCheckpoint: currentProjection.checkpoint,
    events: ordered,
    latestTick,
    mode: "current",
    receivedEventIDs: ordered.map((event) => event.id),
    selectedTick: latestTick,
    worldViewCache: {
      [latestTick]: currentProjection.worldState,
    },
  };
}

export function restoreTimelineCheckpoint(
  checkpoint: FactoryTimelineCheckpoint,
): Pick<
  FactoryTimelineState,
  | "currentReplayCheckpoint"
  | "events"
  | "latestTick"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  const worldState = projectSnapshot(checkpoint.replayState);
  return {
    currentReplayCheckpoint: checkpoint,
    events: [],
    latestTick: checkpoint.selectedTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: checkpoint.selectedTick,
    worldViewCache: {
      [checkpoint.selectedTick]: worldState,
    },
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
    worldViewCache: cacheWithSnapshot(
      current.events,
      current.worldViewCache,
      tick,
      deps,
    ),
  };
}

export function setTimelineCurrentMode(
  current: Pick<
    FactoryTimelineState,
    "currentReplayCheckpoint" | "events" | "latestTick" | "worldViewCache"
  >,
  deps: TimelineStoreStateDeps,
): Pick<
  FactoryTimelineState,
  "currentReplayCheckpoint" | "mode" | "selectedTick" | "worldViewCache"
> {
  const cachedCurrentWorldView = current.worldViewCache[current.latestTick];
  const currentCheckpoint = current.currentReplayCheckpoint;
  if (
    cachedCurrentWorldView &&
    (!currentCheckpoint ||
      currentCheckpoint.selectedTick === current.latestTick)
  ) {
    return {
      currentReplayCheckpoint: currentCheckpoint,
      mode: "current",
      selectedTick: current.latestTick,
      worldViewCache: current.worldViewCache,
    };
  }

  const currentProjection = projectCurrentTick(
    current.events,
    current.latestTick,
    deps,
    currentCheckpoint,
  );
  return {
    currentReplayCheckpoint: currentProjection.checkpoint,
    mode: "current",
    selectedTick: current.latestTick,
    worldViewCache: {
      ...current.worldViewCache,
      [current.latestTick]: currentProjection.worldState,
    },
  };
}
