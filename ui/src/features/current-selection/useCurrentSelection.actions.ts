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

type CurrentSelectionActionArgs = {
  commitSelectionState: (state: { selection: DashboardSelection | null; terminalWorkDetail: TerminalWorkDetail | null }) => void;
  completedWorkItems: TerminalWorkItem[];
  failedWorkItems: TerminalWorkItem[];
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
};

export function useCurrentSelectionActions({
  commitSelectionState,
  completedWorkItems,
  failedWorkItems,
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
  terminalWorkDetail,
}: CurrentSelectionActionArgs) {
  const commitResolvedWorkSelection = (
    workID: string,
    detail: TerminalWorkDetail | null,
    dispatchID?: string,
  ) => {
    const resolvedSelection = resolveTrackedWorkSelection({
      dispatchID,
      snapshot,
      terminalWorkDetail: detail,
      workID,
      workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
    });
    return { detail, resolvedSelection };
  };

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
    const { resolvedSelection } = commitResolvedWorkSelection(
      workID,
      terminalWorkDetail,
    );
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
    const detail = buildTerminalWorkDetail(status, item);
    const { resolvedSelection } = commitResolvedWorkSelection(
      item.traceWorkID,
      detail,
      item.dispatchID,
    );

    commitSelectionState({
      selection: resolvedSelection ?? selection,
      terminalWorkDetail: detail,
    });
  };

  const selectStateWorkItem = (place: DashboardPlaceRef, workItem: DashboardWorkItemRef) => {
    const resolvedSelection = resolveStateWorkItemSelection({
      place,
      projectedWorkstationRequestsByDispatchID,
      snapshot,
      workItem,
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
      terminalWorkDetail: fallbackTerminalWorkDetail(snapshot, terminalStatus, workItem),
    });
  };

  const selectWorkstationRequest = (request: DashboardWorkstationRequest) => {
    commitSelectionState({
      selection: workstationRequestSelection(request),
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

function resolveStateWorkItemSelection({
  place,
  projectedWorkstationRequestsByDispatchID,
  snapshot,
  workItem,
}: {
  place: DashboardPlaceRef;
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  snapshot: DashboardSnapshot | null | undefined;
  workItem: DashboardWorkItemRef;
}): DashboardSelection | null {
  return resolveTrackedWorkSelection({
    nodeID: placeNodeID(snapshot, place),
    snapshot,
    workID: workItem.work_id,
    workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
  });
}

function fallbackTerminalWorkDetail(
  snapshot: DashboardSnapshot | null | undefined,
  status: TerminalWorkStatus,
  workItem: DashboardWorkItemRef,
): TerminalWorkDetail {
  const failedDetail = snapshot?.runtime.session.failed_work_details_by_work_id?.[workItem.work_id];
  return {
    failureMessage: failedDetail?.failure_message,
    failureReason: failedDetail?.failure_reason,
    label: workItem.display_name?.trim() || workItem.work_id,
    status,
    traceWorkID: workItem.work_id,
    workItem,
  };
}

function buildTerminalWorkDetail(
  status: TerminalWorkStatus,
  item: TerminalWorkItem,
): TerminalWorkDetail {
  return {
    attempts: item.attempts,
    dispatchID: item.dispatchID,
    failureMessage: item.failureMessage,
    failureReason: item.failureReason,
    label: item.label,
    preferWorkstationRequest: true,
    status,
    traceWorkID: item.traceWorkID,
    workItem: item.workItem,
  };
}

function workstationRequestSelection(
  request: DashboardWorkstationRequest,
): DashboardSelection {
  return {
    dispatchId: request.dispatch_id,
    kind: "workstation-request",
    nodeId: request.workstation_node_id,
    request,
  };
}
