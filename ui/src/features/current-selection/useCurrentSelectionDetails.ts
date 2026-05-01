import { useMemo } from "react";

import type {
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationNode,
} from "../../api/dashboard/types";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
import type { SelectedWorkItemExecutionDetails } from "../../state/executionDetails";
import type { CurrentSelectionState } from "./useCurrentSelection";

export interface UseCurrentSelectionDetailsParams {
  currentSelection: CurrentSelectionState;
  selectedTrace: DashboardTrace | undefined;
  snapshot: DashboardSnapshot | null | undefined;
  workstationRequestsByDispatchID?: Record<string, DashboardRuntimeWorkstationRequest>;
}

export interface UseCurrentSelectionDetailsResult {
  selectedWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
  terminalWorkExecutionDetails: SelectedWorkItemExecutionDetails | null;
}

export function useCurrentSelectionDetails({
  currentSelection,
  selectedTrace,
  snapshot,
  workstationRequestsByDispatchID,
}: UseCurrentSelectionDetailsParams): UseCurrentSelectionDetailsResult {
  const {
    selectedNode,
    selectedWorkDispatchAttempts,
    selection,
    terminalWorkDetail,
  } = currentSelection;

  const selectedWorkExecutionDetails = useMemo(
    () =>
      selection?.kind === "work-item" && selectedNode
        ? selectWorkItemExecutionDetails({
            activeExecution: selection.execution,
            dispatchID: selection.dispatchId,
            inferenceAttemptsByDispatchID: snapshot?.runtime.inference_attempts_by_dispatch_id,
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

  const terminalWorkExecutionDetails = useMemo(() => {
    const terminalWorkAttempt = latestTerminalAttempt(terminalWorkDetail);
    const terminalWorkItem =
      terminalWorkDetail?.workItem ??
      (terminalWorkDetail
        ? {
            display_name: terminalWorkDetail.label,
            work_id: terminalWorkDetail.traceWorkID,
          }
        : undefined);

    return terminalWorkDetail && terminalWorkItem
      ? selectWorkItemExecutionDetails({
          dispatchID: terminalWorkAttempt?.dispatch_id,
          inferenceAttemptsByDispatchID: snapshot?.runtime.inference_attempts_by_dispatch_id,
          providerSessions: terminalWorkDetail.attempts ?? [],
          selectedNode: findWorkstationNodeForAttempt(snapshot, terminalWorkAttempt),
          trace: selectedTrace,
          workItem: terminalWorkItem,
          workstationRequestsByDispatchID:
            snapshot?.runtime.workstation_requests_by_dispatch_id ??
            workstationRequestsByDispatchID,
        })
      : null;
  }, [selectedTrace, snapshot, terminalWorkDetail, workstationRequestsByDispatchID]);

  return {
    selectedWorkExecutionDetails,
    terminalWorkExecutionDetails,
  };
}

function latestTerminalAttempt(
  detail: CurrentSelectionState["terminalWorkDetail"],
): DashboardProviderSessionAttempt | undefined {
  return detail?.attempts?.[detail.attempts.length - 1];
}

function findWorkstationNodeForAttempt(
  snapshot: DashboardSnapshot | null | undefined,
  attempt: DashboardProviderSessionAttempt | undefined,
): DashboardWorkstationNode | undefined {
  if (!snapshot || !attempt) {
    return undefined;
  }

  const transitionNode = snapshot.topology.workstation_nodes_by_id[attempt.transition_id];
  if (transitionNode) {
    return transitionNode;
  }

  return Object.values(snapshot.topology.workstation_nodes_by_id).find(
    (node) => node.workstation_name === attempt.workstation_name,
  );
}
