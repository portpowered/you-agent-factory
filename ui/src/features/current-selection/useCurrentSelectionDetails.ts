import { useMemo } from "react";

import type {
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardTrace,
} from "../../api/dashboard/types";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import { selectWorkItemExecutionDetails } from "./state/executionDetails";
import type { CurrentSelectionState } from "./useCurrentSelection";

export interface UseCurrentSelectionDetailsParams {
  currentSelection: CurrentSelectionState;
  selectedTrace: DashboardTrace | undefined;
  snapshot: DashboardSnapshot | null | undefined;
  workstationRequestsByDispatchID?: Record<
    string,
    DashboardRuntimeWorkstationRequest
  >;
}

export interface UseCurrentSelectionDetailsResult {
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
}

export function useCurrentSelectionDetails({
  currentSelection,
  selectedTrace,
  snapshot,
  workstationRequestsByDispatchID,
}: UseCurrentSelectionDetailsParams): UseCurrentSelectionDetailsResult {
  const { selectedNode, selectedWorkDispatchAttempts, selection } =
    currentSelection;

  const selectedWorkExecutionDetails = useMemo(
    () =>
      selection?.kind === "work-item" && selectedNode
        ? selectWorkItemExecutionDetails({
            activeExecution: selection.execution,
            dispatchID: selection.dispatchId,
            inferenceAttemptsByDispatchID:
              snapshot?.runtime.inference_attempts_by_dispatch_id,
            providerSessions: selectedWorkDispatchAttempts,
            selectedNode,
            trace: selectedTrace,
            workItem: selection.workItem,
            workstationRequestsByDispatchID:
              snapshot?.runtime.workstation_requests_by_dispatch_id ??
              workstationRequestsByDispatchID,
          })
        : null,
    [
      selectedNode,
      selectedTrace,
      selectedWorkDispatchAttempts,
      selection,
      snapshot,
      workstationRequestsByDispatchID,
    ],
  );

  return {
    selectedWorkExecutionDetails,
  };
}
