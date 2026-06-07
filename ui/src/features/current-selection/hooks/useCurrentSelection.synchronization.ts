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
  replacePresent,
  resetSelectionHistory,
  selection,
  snapshot,
  terminalWorkDetail,
  topologyFactory,
}: {
  pendingFactoryDefinition?: DashboardSnapshot["factory"];
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  replacePresent: (state: {
    selection: DashboardSelection | null;
    terminalWorkDetail: TerminalWorkDetail | null;
  }) => void;
  resetSelectionHistory: () => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
  topologyFactory?: DashboardSnapshot["factory"];
}) {
  useEffect(() => {
    if (!snapshot) {
      resetSelectionHistory();
      return;
    }

    replacePresent({
      selection: resolveDashboardSelection({
        pendingFactoryDefinition,
        selection,
        snapshot,
        topologyFactory,
        workstationRequestsByDispatchID:
          projectedWorkstationRequestsByDispatchID,
      }),
      terminalWorkDetail,
    });
  }, [
    pendingFactoryDefinition,
    projectedWorkstationRequestsByDispatchID,
    replacePresent,
    resetSelectionHistory,
    selection,
    snapshot,
    terminalWorkDetail,
    topologyFactory,
  ]);
}
