import { useCallback, useEffect, useRef, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useCurrentSelection } from "../../current-selection/hooks/core/useCurrentSelection";
import type {
  DashboardCardStateContext,
  DashboardCardStateSnapshot,
} from "../../dashboard/lib/dashboard-card-state";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import {
  factoryTimelineEntryKey,
  useFactoryTimelineStore,
} from "../../timeline/public/store";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";

export interface DashboardWorkOutcomeStream {
  identity: StreamDerivedCacheIdentity | null;
  status: "loading" | "ready";
}

export interface DashboardBentoCardStateContext
  extends DashboardCardStateContext {
  dirtyCardInstanceIDs: ReadonlySet<string>;
  workOutcomeHydrationStatus: DashboardWorkOutcomeStream["status"];
}

export type DashboardBentoDirtyStateReporter = (
  widgetInstanceID: string,
  isDirty: boolean,
) => void;

export type DashboardBentoCardStateReporter = (
  widgetInstanceID: string,
  state: DashboardCardStateSnapshot,
) => void;

export interface DashboardBentoSnapshot {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  dashboardCardStateContext: DashboardBentoCardStateContext;
  getDashboardCardState: (
    widgetInstanceID: string,
  ) => DashboardCardStateSnapshot | undefined;
  materializedWorkOutcomeState: unknown;
  reportDashboardCardState: DashboardBentoCardStateReporter;
  reportDashboardCardDirtyState: DashboardBentoDirtyStateReporter;
  restoreDashboardCardState: (
    widgetInstanceID: string,
    state: DashboardCardStateSnapshot,
  ) => void;
  restoredDashboardCardStates: Readonly<
    Record<string, DashboardCardStateSnapshot>
  >;
  selectedSnapshot: DashboardSnapshot | undefined;
  selectedTimelineTick: number;
  snapshot: DashboardSnapshot;
  workOutcomeHydrationStatus: DashboardWorkOutcomeStream["status"];
}

interface WorkOutcomeTimelineSelectionState {
  entriesByKey?: Record<
    string,
    | {
        materializedWorkOutcomeState: unknown;
        selectedTick: number;
      }
    | undefined
  >;
  materializedWorkOutcomeState: unknown;
  selectedTick: number;
}

export function selectDashboardWorkOutcomeInput(
  state: WorkOutcomeTimelineSelectionState,
  stream?: DashboardWorkOutcomeStream,
): {
  hydrationStatus: DashboardWorkOutcomeStream["status"];
  materializedWorkOutcomeState: unknown;
  selectedTimelineTick: number;
} {
  const exactEntryKey = stream?.identity
    ? factoryTimelineEntryKey(stream.identity)
    : null;
  const exactEntry = exactEntryKey ? state.entriesByKey?.[exactEntryKey] : null;
  const exactEntryPending =
    stream?.status === "ready" && exactEntryKey !== null && !exactEntry;

  return {
    hydrationStatus:
      stream?.status === "loading" || exactEntryPending ? "loading" : "ready",
    materializedWorkOutcomeState: exactEntryKey
      ? exactEntry?.materializedWorkOutcomeState
      : state.materializedWorkOutcomeState,
    selectedTimelineTick: exactEntryKey
      ? (exactEntry?.selectedTick ?? 0)
      : state.selectedTick,
  };
}

const EMPTY_DASHBOARD_SNAPSHOT: DashboardSnapshot = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 0,
  topology: {
    edges: [],
    submit_work_types: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 0,
};

interface DashboardBentoCardStateRegistry {
  dirtyCardInstanceIDs: ReadonlySet<string>;
  getDashboardCardState: (
    widgetInstanceID: string,
  ) => DashboardCardStateSnapshot | undefined;
  reportDashboardCardState: DashboardBentoCardStateReporter;
  reportDashboardCardDirtyState: DashboardBentoDirtyStateReporter;
  restoreDashboardCardState: (
    widgetInstanceID: string,
    state: DashboardCardStateSnapshot,
  ) => void;
  restoredDashboardCardStates: Readonly<
    Record<string, DashboardCardStateSnapshot>
  >;
}

function useDashboardBentoCardStateRegistry(
  sessionID: string | null | undefined,
): DashboardBentoCardStateRegistry {
  const [dirtyCardInstanceIDs, setDirtyCardInstanceIDs] = useState<
    ReadonlySet<string>
  >(() => new Set());
  const previousSessionIDRef = useRef(sessionID);
  const reportDashboardCardDirtyState = useCallback(
    (widgetInstanceID: string, isDirty: boolean) => {
      setDirtyCardInstanceIDs((currentIDs) => {
        const nextIDs = new Set(currentIDs);
        if (isDirty) {
          nextIDs.add(widgetInstanceID);
        } else {
          nextIDs.delete(widgetInstanceID);
        }

        if (nextIDs.size === currentIDs.size) {
          const hasSameIDs = [...nextIDs].every((id) => currentIDs.has(id));
          if (hasSameIDs) {
            return currentIDs;
          }
        }

        return nextIDs;
      });
    },
    [],
  );
  const dashboardCardStateByInstanceIDRef = useRef<
    Map<string, DashboardCardStateSnapshot>
  >(new Map());
  const [restoredDashboardCardStates, setRestoredDashboardCardStates] =
    useState<Record<string, DashboardCardStateSnapshot>>({});
  const reportDashboardCardState = useCallback<DashboardBentoCardStateReporter>(
    (widgetInstanceID, state) => {
      dashboardCardStateByInstanceIDRef.current.set(widgetInstanceID, state);
    },
    [],
  );
  const getDashboardCardState = useCallback(
    (widgetInstanceID: string) =>
      dashboardCardStateByInstanceIDRef.current.get(widgetInstanceID),
    [],
  );
  const restoreDashboardCardState = useCallback(
    (widgetInstanceID: string, state: DashboardCardStateSnapshot) => {
      setRestoredDashboardCardStates((currentStates) => ({
        ...currentStates,
        [widgetInstanceID]: state,
      }));
    },
    [],
  );

  useEffect(() => {
    if (previousSessionIDRef.current === sessionID) {
      return;
    }

    previousSessionIDRef.current = sessionID;
    setDirtyCardInstanceIDs(new Set());
    dashboardCardStateByInstanceIDRef.current.clear();
    setRestoredDashboardCardStates({});
  }, [sessionID]);

  return {
    dirtyCardInstanceIDs,
    getDashboardCardState,
    reportDashboardCardState,
    reportDashboardCardDirtyState,
    restoreDashboardCardState,
    restoredDashboardCardStates,
  };
}

export function useDashboardBentoSnapshot(
  sessionID: string | null | undefined,
  workOutcomeStream?: DashboardWorkOutcomeStream,
): DashboardBentoSnapshot {
  const {
    dirtyCardInstanceIDs,
    getDashboardCardState,
    reportDashboardCardState,
    reportDashboardCardDirtyState,
    restoreDashboardCardState,
    restoredDashboardCardStates,
  } = useDashboardBentoCardStateRegistry(sessionID);

  const materializedWorkOutcomeState = useFactoryTimelineStore(
    (state) =>
      selectDashboardWorkOutcomeInput(state, workOutcomeStream)
        .materializedWorkOutcomeState,
  );
  const selectedTimelineTick = useFactoryTimelineStore(
    (state) =>
      selectDashboardWorkOutcomeInput(state, workOutcomeStream)
        .selectedTimelineTick,
  );
  const workOutcomeHydrationStatus = useFactoryTimelineStore(
    (state) =>
      selectDashboardWorkOutcomeInput(state, workOutcomeStream).hydrationStatus,
  );
  const hasRestoredCheckpoint = useFactoryTimelineStore(
    (state) => state.currentReplayCheckpoint != null,
  );
  const timelineMode = useFactoryTimelineStore(
    (state) => state.mode ?? "current",
  );
  const streamStatus = useDashboardStreamStore((state) => state.streamState);
  const workstationRequestsByDispatchID = useFactoryTimelineStore(
    (state) =>
      state.worldViewCache[state.selectedTick]?.workstationRequestsByDispatchID,
  );
  const selectedSnapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const hasAuthoritativeSnapshot = useFactoryTimelineStore(
    (state) =>
      state.worldViewCache[state.selectedTick] != null &&
      (state.events.length > 0 || state.currentReplayCheckpoint != null),
  );
  const snapshot = selectedSnapshot ?? EMPTY_DASHBOARD_SNAPSHOT;
  const currentSelection = useCurrentSelection({
    sessionID: sessionID ?? DEFAULT_FACTORY_SESSION_ID,
    snapshot,
    workstationRequestsByDispatchID,
  });
  return {
    currentSelection,
    materializedWorkOutcomeState,
    getDashboardCardState,
    reportDashboardCardState,
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    dashboardCardStateContext: {
      dirtyCardInstanceIDs,
      hasAuthoritativeSnapshot,
      recoveryPending:
        hasRestoredCheckpoint || workOutcomeHydrationStatus === "loading",
      streamStatus: streamStatus.status,
      timelineMode,
      workOutcomeHydrationStatus,
    } satisfies DashboardBentoCardStateContext,
    workOutcomeHydrationStatus,
    reportDashboardCardDirtyState,
    restoreDashboardCardState,
    restoredDashboardCardStates,
  };
}
