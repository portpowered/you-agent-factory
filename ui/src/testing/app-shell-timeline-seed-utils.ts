import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
import { projectFactoryTopology } from "../../packages/factory-replay/src/index.js";
import {
  emptyHostedFactoryReplayProjection,
  useFactoryTimelineStore,
  type WorldState,
} from "../features/timeline/state/factoryTimelineStore";

function timelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): WorldState {
  const factoryReplay = emptyHostedFactoryReplayProjection(snapshot.tick_count);
  if (snapshot.factory) {
    factoryReplay.topology = projectFactoryTopology({
      factory: snapshot.factory,
      selectedTick: snapshot.tick_count,
    });
  }

  return {
    ...snapshot,
    factoryReplay,
    relationsByWorkID: {},
    tracesByWorkID,
    workstationRequestsByDispatchID,
    workRequestsByID: {},
  };
}

function seedTimelineState(
  timelineState: Pick<
    ReturnType<typeof useFactoryTimelineStore.getState>,
    | "events"
    | "latestTick"
    | "mode"
    | "receivedEventIDs"
    | "selectedTick"
    | "worldViewCache"
  >,
): void {
  useFactoryTimelineStore.setState((current) => {
    const activeEntry = current.activeEntryKey
      ? current.entriesByKey[current.activeEntryKey]
      : undefined;
    if (!current.activeEntryKey || !activeEntry) {
      return timelineState;
    }
    return {
      ...timelineState,
      entriesByKey: {
        ...current.entriesByKey,
        [current.activeEntryKey]: {
          ...activeEntry,
          ...timelineState,
        },
      },
    };
  });
}

export function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): void {
  seedTimelineState({
    events: [],
    latestTick: snapshot.tick_count,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: snapshot.tick_count,
    worldViewCache: {
      [snapshot.tick_count]: timelineSnapshot(
        snapshot,
        tracesByWorkID,
        workstationRequestsByDispatchID,
      ),
    },
  });
}

export function seedTimelineSnapshots(snapshots: DashboardSnapshot[]): void {
  const worldViewCache = Object.fromEntries(
    snapshots.map(
      (snapshot) =>
        [
          snapshot.tick_count,
          timelineSnapshot(snapshot) satisfies WorldState,
        ] as const,
    ),
  );
  const latestTick = Math.max(
    ...snapshots.map((snapshot) => snapshot.tick_count),
  );

  seedTimelineState({
    events: [],
    latestTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: latestTick,
    worldViewCache,
  });
}
