import type {
  DashboardActiveExecution,
  DashboardPlaceRef,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../terminal-work/terminal-work-card";
import type { DashboardSelection, TerminalWorkDetail } from "./types";
import {
  findTerminalWorkItem,
  inferStateWorkTerminalStatus,
  placeNodeID,
  resolveTrackedWorkSelection,
} from "./useCurrentSelection.helpers";

export function useCurrentSelectionActions({
  commitSelectionState,
  completedWorkItems,
  failedWorkItems,
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
  terminalWorkDetail,
}: {
  commitSelectionState: (state: { selection: DashboardSelection | null; terminalWorkDetail: TerminalWorkDetail | null }) => void;
  completedWorkItems: TerminalWorkItem[];
  failedWorkItems: TerminalWorkItem[];
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  const selectWorkstation = (nodeId: string) => {
    commitSelectionState({
      selection: { kind: "node", nodeId },
      terminalWorkDetail: null,
    });
  };

  const selectWorkItem = (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => {
    commitSelectionState({
      selection: {
        dispatchId,
        execution,
        kind: "work-item",
        nodeId,
        workItem,
      },
      terminalWorkDetail: null,
    });
  };

  const selectWorkByID = (workID: string) => {
    const resolvedSelection = resolveTrackedWorkSelection({
      snapshot,
      terminalWorkDetail,
      workID,
      workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
    });
    if (!resolvedSelection) {
      return;
    }

    commitSelectionState({
      selection: resolvedSelection,
      terminalWorkDetail:
        terminalWorkDetail?.traceWorkID === workID ? terminalWorkDetail : null,
    });
  };

  const selectStateNode = (placeId: string) => {
    commitSelectionState({
      selection: { kind: "state-node", placeId },
      terminalWorkDetail: null,
    });
  };

  const openTerminalWorkDetail = (status: TerminalWorkStatus, item: TerminalWorkItem) => {
    const detail = {
      attempts: item.attempts,
      failureMessage: item.failureMessage,
      failureReason: item.failureReason,
      label: item.label,
      status,
      traceWorkID: item.traceWorkID,
      workItem: item.workItem,
    };
    const resolvedSelection = resolveTrackedWorkSelection({
      snapshot,
      terminalWorkDetail: detail,
      workID: item.traceWorkID,
      workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
    });

    commitSelectionState({
      selection: resolvedSelection ?? selection,
      terminalWorkDetail: detail,
    });
  };

  const selectStateWorkItem = (place: DashboardPlaceRef, workItem: DashboardWorkItemRef) => {
    const resolvedSelection = resolveTrackedWorkSelection({
      nodeID: placeNodeID(snapshot, place),
      snapshot,
      workID: workItem.work_id,
      workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
    });
    if (resolvedSelection) {
      commitSelectionState({
        selection: resolvedSelection,
        terminalWorkDetail: null,
      });
      return;
    }

    const terminalStatus = inferStateWorkTerminalStatus(snapshot, place, workItem);
    if (!terminalStatus) {
      return;
    }

    const terminalItem = findTerminalWorkItem(
      terminalStatus === "failed" ? failedWorkItems : completedWorkItems,
      workItem,
    );
    if (terminalItem) {
      openTerminalWorkDetail(terminalStatus, terminalItem);
      return;
    }

    commitSelectionState({
      selection,
      terminalWorkDetail: {
        failureMessage:
          snapshot?.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]?.failure_message,
        failureReason:
          snapshot?.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]?.failure_reason,
        label: workItem.display_name?.trim() || workItem.work_id,
        status: terminalStatus,
        traceWorkID: workItem.work_id,
        workItem,
      },
    });
  };

  const selectWorkstationRequest = (request: DashboardWorkstationRequest) => {
    commitSelectionState({
      selection: {
        dispatchId: request.dispatch_id,
        kind: "workstation-request",
        nodeId: request.workstation_node_id,
        request,
      },
      terminalWorkDetail: null,
    });
  };

  return {
    openTerminalWorkDetail,
    selectStateNode,
    selectStateWorkItem,
    selectWorkByID,
    selectWorkItem,
    selectWorkstation,
    selectWorkstationRequest,
  };
}
