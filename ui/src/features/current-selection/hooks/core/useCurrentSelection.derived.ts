import { useEffect, useMemo } from "react";

import type {
  DashboardPlaceRef,
  DashboardSnapshot,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import {
  type FactoryBundledDocFile,
  findFactoryBundledDocFile,
} from "../../../workflow-activity/lib/factory-bundled-docs";
import {
  resourceTokenCountFromSnapshot,
  workerNamesReferencingResourceInFactoryDefinition,
  workstationNamesReferencingResourceInFactoryDefinition,
} from "../../resource-selection/lib/resource-detail-values";
import { findFactoryResourceInSnapshot } from "../../state/dashboardSelection";
import type {
  DashboardSelection,
  TerminalWorkDetail,
} from "../../state/selection-types";
import {
  buildSelectedWorkOperationHistory,
  type SelectedWorkOperationHistoryItem,
} from "../helpers/selected-work-operation-history";
import {
  activeExecutionsForSelectedWorkstation,
  buildSelectedWorkDispatchAttempts,
  buildTerminalWorkItems,
  currentWorkItemsForPlace,
  filterProviderSessionAttempts,
  findStatePlace,
  selectLatestProviderSessionAttemptsByDispatch,
  selectWorkstationRequestsForWork,
  sortWorkstationRequests,
  terminalHistoryItemsForPlace,
  type WorkstationRequestLike,
} from "./useCurrentSelection.helpers";
import {
  resolveCurrentFactoryDocumentFromSnapshot,
  selectWorkerAndWorkTypeData,
} from "./useCurrentSelection.selection-metadata";

function selectSelectedNode(
  selection: DashboardSelection | null,
  snapshot: DashboardSnapshot | null | undefined,
): DashboardWorkstationNode | null {
  if (!snapshot) {
    return null;
  }
  if (
    selection?.kind === "node" ||
    selection?.kind === "workstation-request" ||
    selection?.kind === "work-item"
  ) {
    return snapshot.topology.workstation_nodes_by_id[selection.nodeId] ?? null;
  }
  return null;
}

function selectSelectedStatePlaceData({
  projectedWorkstationRequestsByDispatchID,
  selectedStatePlace,
  snapshot,
}: {
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  selectedStatePlace: DashboardPlaceRef | null;
  snapshot: DashboardSnapshot | null | undefined;
}) {
  const selectedStateCurrentWorkItems = currentWorkItemsForPlace(
    snapshot,
    selectedStatePlace?.place_id,
    projectedWorkstationRequestsByDispatchID,
  );
  const selectedStateTerminalHistoryWorkItems = terminalHistoryItemsForPlace(
    snapshot,
    selectedStatePlace?.place_id,
    projectedWorkstationRequestsByDispatchID,
  );
  const selectedStateTokenCount =
    selectedStatePlace && snapshot
      ? (snapshot.runtime.place_token_counts?.[selectedStatePlace.place_id] ??
        0)
      : 0;

  return {
    selectedStateCurrentWorkItems,
    selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount,
  };
}

function selectSelectedResourceRuntime(
  selection: DashboardSelection | null,
  snapshot: DashboardSnapshot | null | undefined,
) {
  const selectedResourceName =
    selection?.kind === "resource" ? selection.resourceName : null;

  return (() => {
    if (!selectedResourceName) {
      return {
        selectedResource: null,
        selectedResourceName: null,
        selectedResourceTokenCount: null,
        selectedResourceWorkerNames: [],
        selectedResourceWorkstationNames: [],
      };
    }

    const selectedResource = snapshot
      ? (findFactoryResourceInSnapshot(snapshot, selectedResourceName) ?? null)
      : null;
    const factory = snapshot?.factory;

    return {
      selectedResource,
      selectedResourceName,
      selectedResourceTokenCount: resourceTokenCountFromSnapshot(
        snapshot,
        selectedResourceName,
      ),
      selectedResourceWorkerNames: factory
        ? workerNamesReferencingResourceInFactoryDefinition(
            factory,
            selectedResourceName,
          )
        : [],
      selectedResourceWorkstationNames: factory
        ? workstationNamesReferencingResourceInFactoryDefinition(
            factory,
            selectedResourceName,
          )
        : [],
    };
  })();
}

function selectSelectedWorkData({
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
}: {
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
}) {
  const selectedWorkRequestHistory = (() => {
    if (selection?.kind !== "work-item") {
      return [];
    }

    return selectWorkstationRequestsForWork(
      projectedWorkstationRequestsByDispatchID as
        | Record<string, WorkstationRequestLike>
        | undefined,
      selection.workItem.work_id,
    );
  })();
  const selectedWorkWorkstationRequests = (() => {
    if (selection?.kind !== "work-item") {
      return [];
    }

    return selectWorkstationRequestsForWork(
      projectedWorkstationRequestsByDispatchID,
      selection.workItem.work_id,
    );
  })();
  const selectedWorkProviderSessions = (() => {
    if (!snapshot || selection?.kind !== "work-item") {
      return [];
    }

    if (selectedWorkRequestHistory.length === 0) {
      return filterProviderSessionAttempts(
        snapshot.runtime.session.provider_sessions,
        (attempt) =>
          attempt.work_items?.some(
            (workItem) => workItem.work_id === selection.workItem.work_id,
          ) ?? false,
      );
    }

    return selectLatestProviderSessionAttemptsByDispatch(
      snapshot.runtime.session.provider_sessions,
      selectedWorkRequestHistory,
    );
  })();
  const selectedWorkDispatchAttempts =
    selection?.kind === "work-item" && snapshot
      ? buildSelectedWorkDispatchAttempts({
          attempts: snapshot.runtime.session.provider_sessions,
          workID: selection.workItem.work_id,
          workstationRequestsByDispatchID:
            projectedWorkstationRequestsByDispatchID,
        })
      : [];
  const selectedWorkOperationHistory: SelectedWorkOperationHistoryItem[] =
    selection?.kind !== "work-item"
      ? []
      : buildSelectedWorkOperationHistory({
          moveOperations:
            snapshot?.runtime.work_move_operations_by_work_id?.[
              selection.workItem.work_id
            ],
          snapshot,
          workID: selection.workItem.work_id,
          workstationRequests: selectedWorkRequestHistory,
        });

  return {
    selectedWorkDispatchAttempts,
    selectedWorkOperationHistory,
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
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  return useMemo(
    () =>
      deriveCurrentSelectionState({
        projectedWorkstationRequestsByDispatchID,
        selection,
        snapshot,
        terminalWorkDetail,
      }),
    [
      projectedWorkstationRequestsByDispatchID,
      selection,
      snapshot,
      terminalWorkDetail,
    ],
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: pure derivation intentionally exposes one testable projection boundary.
export function deriveCurrentSelectionState({
  projectedWorkstationRequestsByDispatchID,
  selection,
  snapshot,
  terminalWorkDetail,
}: {
  projectedWorkstationRequestsByDispatchID:
    | Record<string, DashboardWorkstationRequest>
    | undefined;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail: TerminalWorkDetail | null;
}) {
  const selectedNode = selectSelectedNode(selection, snapshot);
  const currentFactoryDefinition =
    resolveCurrentFactoryDocumentFromSnapshot(snapshot);
  const selectedWorkstationRequest =
    selection?.kind === "workstation-request"
      ? (projectedWorkstationRequestsByDispatchID?.[selection.dispatchId] ??
        selection.request)
      : null;
  const selectedStatePlace =
    selection?.kind === "state-node" && snapshot
      ? findStatePlace(snapshot, selection.placeId)
      : null;
  const {
    selectedStateCurrentWorkItems,
    selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount,
  } = selectSelectedStatePlaceData({
    projectedWorkstationRequestsByDispatchID,
    selectedStatePlace,
    snapshot,
  });
  const selectedNodeProviderSessions =
    selection?.kind && selectedNode && snapshot
      ? filterProviderSessionAttempts(
          snapshot.runtime.session.provider_sessions,
          (attempt) =>
            attempt.transition_id === selectedNode.transition_id ||
            attempt.workstation_name === selectedNode.workstation_name,
        )
      : [];
  const selectedNodeActiveExecutions = activeExecutionsForSelectedWorkstation(
    snapshot,
    selection,
    selectedNode,
  );
  const selectedNodeWorkstationRequests = (() => {
    if (!selectedNode) {
      return [];
    }

    return sortWorkstationRequests(
      Object.values(projectedWorkstationRequestsByDispatchID ?? {}).filter(
        (request) => request.workstation_node_id === selectedNode.node_id,
      ),
    );
  })();
  const selectedWorkID =
    selection?.kind === "work-item"
      ? selection.workItem.work_id
      : selectedWorkstationRequest
        ? (selectedWorkstationRequest.work_items[0]?.work_id ?? null)
        : (terminalWorkDetail?.traceWorkID ?? null);
  const selectedDocTargetPath =
    selection?.kind === "doc" ? selection.targetPath : null;
  const selectedDocBundledFile = ((): FactoryBundledDocFile | null => {
    if (!snapshot || !selectedDocTargetPath) {
      return null;
    }

    return (
      findFactoryBundledDocFile(snapshot.factory, selectedDocTargetPath) ?? null
    );
  })();
  const {
    selectedResource,
    selectedResourceName,
    selectedResourceTokenCount,
    selectedResourceWorkerNames,
    selectedResourceWorkstationNames,
  } = selectSelectedResourceRuntime(selection, snapshot);
  const {
    selectedWorker,
    selectedWorkerName,
    selectedWorkerWorkstationNames,
    selectedWorkType,
    selectedWorkTypeName,
  } = selectWorkerAndWorkTypeData(selection, snapshot);
  const work = selectSelectedWorkData({
    projectedWorkstationRequestsByDispatchID,
    selection,
    snapshot,
  });
  const completedWorkLabels =
    snapshot?.runtime.session.completed_work_labels ?? [];
  const failedWorkLabels = snapshot?.runtime.session.failed_work_labels ?? [];
  const completedWorkItems = buildTerminalWorkItems(
    completedWorkLabels,
    snapshot?.runtime.session.provider_sessions,
    undefined,
    projectedWorkstationRequestsByDispatchID,
  );
  const failedWorkItems = buildTerminalWorkItems(
    failedWorkLabels,
    snapshot?.runtime.session.provider_sessions,
    snapshot?.runtime.session.failed_work_details_by_work_id,
    projectedWorkstationRequestsByDispatchID,
  );

  return {
    completedWorkItems,
    completedWorkLabels,
    currentFactoryDefinition,
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
    selectedWorkOperationHistory: work.selectedWorkOperationHistory,
    selectedWorkProviderSessions: work.selectedWorkProviderSessions,
    selectedWorkRequestHistory: work.selectedWorkRequestHistory,
    selectedWorkWorkstationRequests: work.selectedWorkWorkstationRequests,
    selectedDocBundledFile,
    selectedDocTargetPath,
    selectedResource,
    selectedResourceName,
    selectedResourceTokenCount,
    selectedResourceWorkerNames,
    selectedResourceWorkstationNames,
    selectedWorker,
    selectedWorkerName,
    selectedWorkerWorkstationNames,
    selectedWorkType,
    selectedWorkTypeName,
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
  replacePresent: (state: {
    selection: DashboardSelection | null;
    terminalWorkDetail: TerminalWorkDetail | null;
  }) => void;
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
