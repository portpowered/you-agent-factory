import { useEffect, useMemo } from "react";

import type {
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import { resolveDashboardSelection } from "../../state/dashboardSelection";
import type { DashboardSelection, TerminalWorkDetail } from "./types";
import {
  activeExecutionsForSelectedWorkstation,
  buildTerminalWorkItems,
  currentWorkItemsForPlace,
  findStatePlace,
  filterProviderSessionAttempts,
  selectLatestProviderSessionAttemptsByDispatch,
  selectWorkstationRequestsForWork,
  sortWorkstationRequests,
  terminalHistoryItemsForPlace,
  type WorkstationRequestLike,
  buildSelectedWorkDispatchAttempts,
} from "./useCurrentSelection.helpers";

export function useSelectionSynchronization({
  projectedWorkstationRequestsByDispatchID,
  replacePresent,
  resetSelectionHistory,
  selection,
  snapshot,
  terminalWorkDetail,
}: {
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  replacePresent: (state: { selection: DashboardSelection | null; terminalWorkDetail: TerminalWorkDetail | null }) => void;
  resetSelectionHistory: () => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  useEffect(() => {
    if (!snapshot) {
      resetSelectionHistory();
      return;
    }

    replacePresent({
      selection: resolveDashboardSelection({
        selection,
        snapshot,
        workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
      }),
      terminalWorkDetail,
    });
  }, [
    projectedWorkstationRequestsByDispatchID,
    replacePresent,
    resetSelectionHistory,
    selection,
    snapshot,
    terminalWorkDetail,
  ]);
}

function useSelectedNode(
  selection: DashboardSelection | null,
  snapshot: DashboardSnapshot | null | undefined,
): DashboardWorkstationNode | null {
  if (!snapshot) {
    return null;
  }
  if (selection?.kind === "node" || selection?.kind === "workstation-request" || selection?.kind === "work-item") {
    return snapshot.topology.workstation_nodes_by_id[selection.nodeId] ?? null;
  }
  return null;
}

function useSelectedWorkData({
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
}: {
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
}) {
  const selectedWorkRequestHistory = useMemo(() => {
    if (selection?.kind !== "work-item") {
      return [];
    }

    return selectWorkstationRequestsForWork(
      projectedWorkstationRequestsByDispatchID as Record<string, WorkstationRequestLike> | undefined,
      selection.workItem.work_id,
    );
  }, [projectedWorkstationRequestsByDispatchID, selection]);
  const selectedWorkWorkstationRequests = useMemo(() => {
    if (selection?.kind !== "work-item") {
      return [];
    }

    return selectWorkstationRequestsForWork(
      projectedWorkstationRequestsByDispatchID,
      selection.workItem.work_id,
    );
  }, [projectedWorkstationRequestsByDispatchID, selection]);
  const selectedWorkProviderSessions = useMemo(() => {
    if (!snapshot || selection?.kind !== "work-item") {
      return [];
    }

    if (selectedWorkRequestHistory.length === 0) {
      return filterProviderSessionAttempts(
        snapshot.runtime.session.provider_sessions,
        (attempt) =>
          attempt.work_items?.some((workItem) => workItem.work_id === selection.workItem.work_id) ?? false,
      );
    }

    return selectLatestProviderSessionAttemptsByDispatch(
      snapshot.runtime.session.provider_sessions,
      selectedWorkRequestHistory,
    );
  }, [selectedWorkRequestHistory, selection, snapshot]);
  const selectedWorkDispatchAttempts =
    selection?.kind === "work-item" && snapshot
      ? buildSelectedWorkDispatchAttempts({
          attempts: snapshot.runtime.session.provider_sessions,
          workID: selection.workItem.work_id,
          workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
        })
      : [];

  return {
    selectedWorkDispatchAttempts,
    selectedWorkProviderSessions,
    selectedWorkRequestHistory,
    selectedWorkWorkstationRequests,
  };
}

export function useCurrentSelectionDerivedState({
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
  terminalWorkDetail,
}: {
  projectedWorkstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  const selectedNode = useSelectedNode(selection, snapshot);
  const selectedWorkstationRequest =
    selection?.kind === "workstation-request" ? selection.request : null;
  const selectedStatePlace =
    selection?.kind === "state-node" && snapshot ? findStatePlace(snapshot, selection.placeId) : null;
  const selectedStateCurrentWorkItems = useMemo(
    () => currentWorkItemsForPlace(snapshot, selectedStatePlace?.place_id),
    [snapshot, selectedStatePlace?.place_id],
  );
  const selectedStateTerminalHistoryWorkItems = useMemo(
    () => terminalHistoryItemsForPlace(snapshot, selectedStatePlace?.place_id),
    [snapshot, selectedStatePlace?.place_id],
  );
  const selectedStateTokenCount =
    selectedStatePlace && snapshot ? snapshot.runtime.place_token_counts?.[selectedStatePlace.place_id] ?? 0 : 0;
  const selectedNodeProviderSessions =
    selection?.kind && selectedNode && snapshot
      ? filterProviderSessionAttempts(
          snapshot.runtime.session.provider_sessions,
          (attempt) =>
            attempt.transition_id === selectedNode.transition_id ||
            attempt.workstation_name === selectedNode.workstation_name,
        )
      : [];
  const selectedNodeActiveExecutions = useMemo(
    () => activeExecutionsForSelectedWorkstation(snapshot, selection, selectedNode),
    [selection, selectedNode, snapshot],
  );
  const selectedNodeWorkstationRequests = useMemo(() => {
    if (!selectedNode) {
      return [];
    }

    return sortWorkstationRequests(
      Object.values(projectedWorkstationRequestsByDispatchID ?? {}).filter(
        (request) => request.workstation_node_id === selectedNode.node_id,
      ),
    );
  }, [projectedWorkstationRequestsByDispatchID, selectedNode]);
  const selectedWorkID =
    selection?.kind === "work-item"
      ? selection.workItem.work_id
      : selection?.kind === "workstation-request"
        ? selection.request.work_items[0]?.work_id ?? null
        : terminalWorkDetail?.traceWorkID ?? null;
  const work = useSelectedWorkData({
    projectedWorkstationRequestsByDispatchID,
    selection,
    snapshot,
  });
  const completedWorkLabels = snapshot?.runtime.session.completed_work_labels ?? [];
  const failedWorkLabels = snapshot?.runtime.session.failed_work_labels ?? [];
  const completedWorkItems = useMemo(
    () => buildTerminalWorkItems(completedWorkLabels, snapshot?.runtime.session.provider_sessions),
    [completedWorkLabels, snapshot],
  );
  const failedWorkItems = useMemo(
    () =>
      buildTerminalWorkItems(
        failedWorkLabels,
        snapshot?.runtime.session.provider_sessions,
        snapshot?.runtime.session.failed_work_details_by_work_id,
      ),
    [failedWorkLabels, snapshot],
  );

  return {
    completedWorkItems,
    completedWorkLabels,
    failedWorkItems,
    failedWorkLabels,
    selectedNode,
    selectedNodeActiveExecutions,
    selectedNodeProviderSessions,
    selectedNodeWorkstationRequests,
    selectedStateCurrentWorkItems,
    selectedStatePlace,
    selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount,
    selectedWorkDispatchAttempts: work.selectedWorkDispatchAttempts,
    selectedWorkID,
    selectedWorkProviderSessions: work.selectedWorkProviderSessions,
    selectedWorkRequestHistory: work.selectedWorkRequestHistory,
    selectedWorkWorkstationRequests: work.selectedWorkWorkstationRequests,
    selectedWorkstationRequest,
  };
}

export function useTerminalWorkDetailCleanup({
  completedWorkLabels,
  failedWorkLabels,
  replacePresent,
  selection,
  terminalWorkDetail,
}: {
  completedWorkLabels: string[];
  failedWorkLabels: string[];
  replacePresent: (state: { selection: DashboardSelection | null; terminalWorkDetail: TerminalWorkDetail | null }) => void;
  selection: DashboardSelection | null;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  useEffect(() => {
    if (
      terminalWorkDetail &&
      !completedWorkLabels.includes(terminalWorkDetail.label) &&
      !failedWorkLabels.includes(terminalWorkDetail.label)
    ) {
      replacePresent({
        selection,
        terminalWorkDetail: null,
      });
    }
  }, [
    completedWorkLabels,
    failedWorkLabels,
    replacePresent,
    selection,
    terminalWorkDetail,
  ]);
}
