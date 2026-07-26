import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useCurrentSelection } from "../../current-selection/hooks/core/useCurrentSelection";
import {
  factoryTimelineEntryKey,
  useFactoryTimelineStore,
} from "../../timeline/public/store";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";

export interface DashboardWorkOutcomeStream {
  identity: StreamDerivedCacheIdentity | null;
  status: "loading" | "ready";
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

export function useDashboardBentoSnapshot(
  sessionID: string | null | undefined,
  workOutcomeStream?: DashboardWorkOutcomeStream,
) {
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
  const workstationRequestsByDispatchID = useFactoryTimelineStore(
    (state) =>
      state.worldViewCache[state.selectedTick]?.workstationRequestsByDispatchID,
  );
  const selectedSnapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
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
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    workOutcomeHydrationStatus,
  };
}
