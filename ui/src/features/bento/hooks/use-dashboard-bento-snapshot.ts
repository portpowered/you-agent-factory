import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useCurrentSelection } from "../../current-selection/hooks/useCurrentSelection";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";

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
) {
  const timelineEvents = useFactoryTimelineStore((state) => state.events);
  const selectedTimelineTick = useFactoryTimelineStore(
    (state) => state.selectedTick,
  );
  const worldViewCache = useFactoryTimelineStore(
    (state) => state.worldViewCache,
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
    selectedSnapshot,
    selectedTimelineTick,
    snapshot,
    timelineEvents,
    worldViewCache,
  };
}
