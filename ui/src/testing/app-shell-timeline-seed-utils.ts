import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
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
  return {
    ...snapshot,
    factoryReplay: emptyHostedFactoryReplayProjection(snapshot.tick_count),
    relationsByWorkID: {},
    tracesByWorkID,
    workstationRequestsByDispatchID,
    workRequestsByID: {},
  };
}

export function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): void {
  useFactoryTimelineStore.setState({
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

  useFactoryTimelineStore.setState({
    events: [],
    latestTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: latestTick,
    worldViewCache,
  });
}
