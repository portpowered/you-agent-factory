import { useEffect } from "react";

import type {
  DashboardSnapshot,
  DashboardWorkstationRequest,
} from "../../../api/dashboard/types";
import { resolveDashboardSelection } from "../state/dashboardSelection";
import type {
  DashboardSelection,
  TerminalWorkDetail,
} from "../state/selection-types";

export function useSelectionSynchronization({
  pendingFactoryDefinition,
  projectedWorkstationRequestsByDispatchID,
  reconcilePresent,
  resetSelectionHistory,
  snapshot,
  topologyFactory,
}: {
  pendingFactoryDefinition?: DashboardSnapshot["factory"];
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  reconcilePresent: (
    reconcile: (state: {
      selection: DashboardSelection | null;
      terminalWorkDetail: TerminalWorkDetail | null;
    }) => {
      selection: DashboardSelection | null;
      terminalWorkDetail: TerminalWorkDetail | null;
    },
  ) => void;
  resetSelectionHistory: () => void;
  snapshot: DashboardSnapshot | null | undefined;
  topologyFactory?: DashboardSnapshot["factory"];
}) {
  useEffect(() => {
    if (!snapshot) {
      resetSelectionHistory();
      return;
    }

    reconcilePresent((present) => ({
      selection: resolveDashboardSelection({
        pendingFactoryDefinition,
        selection: present.selection,
        snapshot,
        topologyFactory,
        workstationRequestsByDispatchID:
          projectedWorkstationRequestsByDispatchID,
      }),
      terminalWorkDetail: present.terminalWorkDetail,
    }));
  }, [
    pendingFactoryDefinition,
    projectedWorkstationRequestsByDispatchID,
    reconcilePresent,
    resetSelectionHistory,
    snapshot,
    topologyFactory,
  ]);
}
