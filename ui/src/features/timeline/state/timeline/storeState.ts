import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import {
  compareEventToTimelinePosition,
  createMaterializedWorkOutcomeState,
  type MaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
} from "../../../work-outcome/public/materializer";
import type { StreamDerivedCacheIdentity } from "../../lib/stream-derived-cache-identity";
import type { FactoryTimelineProjection } from "./buildSnapshot";
import { projectSnapshot } from "./projectSnapshot";
import {
  emptyWorldRuntime,
  type ReplayWorldState,
  type WorldState,
} from "./types";

export type FactoryTimelineMode = "current" | "fixed";

export interface FactoryTimelineEntryState {
  currentReplayCheckpoint?: FactoryTimelineCheckpoint;
  events: FactoryEvent[];
  identity: StreamDerivedCacheIdentity;
  latestTick: number;
  materializedWorkOutcomeState: MaterializedWorkOutcomeState;
  mode: FactoryTimelineMode;
  receivedEventIDs: string[];
  selectedTick: number;
  worldViewCache: Record<number, WorldState>;
}

export interface FactoryTimelineState {
  activeEntryKey: string | null;
  currentReplayCheckpoint?: FactoryTimelineCheckpoint;
  entriesByKey: Record<string, FactoryTimelineEntryState>;
  events: FactoryEvent[];
  latestTick: number;
  materializedWorkOutcomeState: MaterializedWorkOutcomeState;
  mode: FactoryTimelineMode;
  receivedEventIDs: string[];
  selectedTick: number;
  worldViewCache: Record<number, WorldState>;
  activateEntry: (identity: StreamDerivedCacheIdentity) => void;
  appendEvent: (event: FactoryEvent) => void;
  appendEventForEntry: (
    identity: StreamDerivedCacheIdentity,
    event: FactoryEvent,
  ) => void;
  appendEvents: (events: FactoryEvent[]) => void;
  appendEventsForEntry: (
    identity: StreamDerivedCacheIdentity,
    events: FactoryEvent[],
  ) => void;
  entryForIdentity: (
    identity: StreamDerivedCacheIdentity,
  ) => FactoryTimelineEntryState | undefined;
  replaceEvents: (events: FactoryEvent[]) => void;
  replaceEventsForEntry: (
    identity: StreamDerivedCacheIdentity,
    events: FactoryEvent[],
  ) => void;
  reset: () => void;
  resetEntry: (identity: StreamDerivedCacheIdentity) => void;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  restoreCheckpointForEntry: (
    identity: StreamDerivedCacheIdentity,
    checkpoint: FactoryTimelineCheckpoint,
  ) => void;
  selectTick: (tick: number) => void;
  selectTickForEntry: (
    identity: StreamDerivedCacheIdentity,
    tick: number,
  ) => void;
  setCurrentMode: () => void;
  setCurrentModeForEntry: (identity: StreamDerivedCacheIdentity) => void;
}

export interface FactoryTimelineSyncIdentity {
  backendScopeId: string;
  factorySessionId: string;
  logicalSessionKeyId: string;
  streamGenerationId: string;
}

export interface FactoryTimelineCheckpoint {
  afterEventId?: string;
  afterSequence?: number;
  materializedWorkOutcomeState: MaterializedWorkOutcomeState;
  replayState: ReplayWorldState;
  selectedTick: number;
  syncIdentity?: FactoryTimelineSyncIdentity;
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
  canonicalizeEvents: (events: FactoryEvent[]) => FactoryEvent[];
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
  | "materializedWorkOutcomeState"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  return {
    currentReplayCheckpoint: undefined,
    events: [],
    latestTick: 0,
    materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
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
  materializedWorkOutcomeState: MaterializedWorkOutcomeState,
  projection: FactoryTimelineProjection,
): TimelineCheckpointProjection {
  const latestEvent = latestAppliedEvent(events, selectedTick);
  return {
    checkpoint: {
      afterEventId: latestEvent?.id,
      afterSequence:
        latestEvent?.context.sessionSequence ?? latestEvent?.context.sequence,
      materializedWorkOutcomeState,
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
  materializedWorkOutcomeState: MaterializedWorkOutcomeState,
  deps: TimelineStoreStateDeps,
  checkpoint?: FactoryTimelineCheckpoint,
  acceptedTail?: FactoryEvent[],
): TimelineCheckpointProjection {
  const usableCheckpoint =
    checkpoint && checkpoint.selectedTick <= selectedTick
      ? checkpoint
      : undefined;
  const projectionEvents = usableCheckpoint
    ? (acceptedTail ??
      events.filter(
        (event) => event.context.tick > usableCheckpoint.selectedTick,
      ))
    : events;
  return checkpointFromProjection(
    events,
    selectedTick,
    materializedWorkOutcomeState,
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
    | "materializedWorkOutcomeState"
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
  | "materializedWorkOutcomeState"
  | "mode"
  | "receivedEventIDs"
  | "selectedTick"
  | "worldViewCache"
> {
  const receivedEventIDs = new Set(current.receivedEventIDs);
  const unorderedAcceptedEvents: FactoryEvent[] = [];
  for (const event of deps.canonicalizeEvents(incomingEvents)) {
    if (
      receivedEventIDs.has(event.id) ||
      (current.materializedWorkOutcomeState.cursor &&
        compareEventToTimelinePosition(
          event,
          current.materializedWorkOutcomeState.cursor,
        ) <= 0)
    ) {
      continue;
    }
    receivedEventIDs.add(event.id);
    unorderedAcceptedEvents.push(event);
  }

  if (unorderedAcceptedEvents.length === 0) {
    return current;
  }

  const acceptedEventIDs = new Set(
    unorderedAcceptedEvents.map((event) => event.id),
  );
  const events = deps.canonicalizeEvents([
    ...current.events,
    ...unorderedAcceptedEvents,
  ]);
  const acceptedTail = events.filter((event) => acceptedEventIDs.has(event.id));
  const latestTick = acceptedTail.reduce(
    (maxTick, event) => Math.max(maxTick, event.context.tick),
    current.latestTick,
  );
  const selectedTick =
    current.mode === "current" ? latestTick : current.selectedTick;
  const materializedWorkOutcomeState = reduceMaterializedWorkOutcomeEvents(
    current.materializedWorkOutcomeState,
    acceptedTail,
  );
  const currentProjection = projectCurrentTick(
    events,
    latestTick,
    materializedWorkOutcomeState,
    deps,
    current.currentReplayCheckpoint,
    acceptedTail,
  );
  const selectedWorldViewCache = cacheWithSnapshot(
    events,
    current.worldViewCache,
    selectedTick,
    deps,
  );

  return {
    events,
    currentReplayCheckpoint: currentProjection.checkpoint,
    latestTick,
    materializedWorkOutcomeState,
    mode: current.mode,
    receivedEventIDs: [
      ...current.receivedEventIDs,
      ...acceptedTail.map((event) => event.id),
    ],
    selectedTick,
    worldViewCache:
      current.mode === "current"
        ? { [selectedTick]: currentProjection.worldState }
        : {
            ...selectedWorldViewCache,
            [latestTick]: currentProjection.worldState,
          },
  };
}

export function replaceTimelineEvents(
  events: FactoryEvent[],
  deps: TimelineStoreStateDeps,
): ReturnType<typeof emptyTimelineState> {
  return appendTimelineEvents(emptyTimelineState(), events, deps);
}

export function restoreTimelineCheckpoint(
  checkpoint: FactoryTimelineCheckpoint,
): Pick<
  FactoryTimelineState,
  | "currentReplayCheckpoint"
  | "events"
  | "latestTick"
  | "materializedWorkOutcomeState"
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
    materializedWorkOutcomeState: checkpoint.materializedWorkOutcomeState,
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
    current.currentReplayCheckpoint?.materializedWorkOutcomeState ??
      createMaterializedWorkOutcomeState(),
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
