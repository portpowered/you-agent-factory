// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: selection derivation helpers are kept colocated to avoid splitting shared runtime-selection rules mid-story.
import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardPlaceRef,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../api/dashboard/types";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../../terminal-work/lib/types";
import {
  findWorkItemReference,
  findWorkstationNodeIDForPlace,
} from "../state/dashboardSelection";
import { findDashboardStatePlace } from "../state/dashboardStatePlaces";
import type {
  DashboardSelection,
  StatePositionWorkItem,
  TerminalWorkDetail,
} from "../state/selection-types";
import {
  requestDispatchID,
  requestStartedAt,
  requestTransitionID,
  requestWorkstationNodeID,
  requestWorkstationName,
  requestWorkItems,
  sortWorkstationRequests,
  type DispatchWorkstationRequest,
} from "./useCurrentSelection.request-helpers";

export function buildTerminalWorkItems(
  labels: string[],
  attempts: DashboardProviderSessionAttempt[] | undefined,
  failureDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>,
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>,
): TerminalWorkItem[] {
  const failureDetails = Object.values(failureDetailsByWorkID ?? {});
  const requests = sortWorkstationRequests(
    Object.values(workstationRequestsByDispatchID ?? {}),
  );

  return labels.map((label) => {
    const matchingAttempts =
      attempts?.filter((attempt) =>
        attempt.work_items?.some(
          (workItem) => workItem.display_name === label || workItem.work_id === label,
        ),
      ) ?? [];
    const latestAttempt = matchingAttempts[matchingAttempts.length - 1];
    const matchedWorkItem = matchingAttempts
      .flatMap((attempt) => attempt.work_items ?? [])
      .find((workItem) => workItem.display_name === label || workItem.work_id === label);
    const matchingRequests = requests.filter((request) =>
      requestWorkItems(request).some(
        (workItem) => workItem.display_name === label || workItem.work_id === label,
      ),
    );
    const latestRequest = matchingRequests[0];
    const matchedFailureDetail = failureDetails.find(
      (detail) =>
        detail.work_item.display_name === label ||
        detail.work_item.work_id === label ||
        (matchedWorkItem ? detail.work_item.work_id === matchedWorkItem.work_id : false),
    );

    return {
      attempts: matchingAttempts,
      dispatchID:
        matchedFailureDetail?.dispatch_id ??
        (latestRequest ? requestDispatchID(latestRequest) : undefined) ??
        latestAttempt?.dispatch_id,
      failureMessage: matchedFailureDetail?.failure_message ?? latestAttempt?.failure_message,
      failureReason: matchedFailureDetail?.failure_reason ?? latestAttempt?.failure_reason,
      label,
      traceWorkID: matchedWorkItem?.work_id ?? matchedFailureDetail?.work_item.work_id ?? label,
      workItem: matchedWorkItem ?? matchedFailureDetail?.work_item,
      workstationName: terminalWorkstationName(
        matchedFailureDetail,
        latestAttempt,
        latestRequest,
      ),
    };
  });
}

function terminalWorkstationName(
  failureDetail: DashboardFailedWorkDetail | undefined,
  attempt: DashboardProviderSessionAttempt | undefined,
  request: DispatchWorkstationRequest | undefined,
): string | undefined {
  return (
    (request ? requestWorkstationName(request) : undefined) ??
    (request ? requestTransitionID(request) : undefined) ??
    failureDetail?.workstation_name ??
    failureDetail?.transition_id ??
    attempt?.workstation_name ??
    attempt?.transition_id
  );
}

export function findStatePlace(snapshot: DashboardSnapshot, placeId: string): DashboardPlaceRef | null {
  return findDashboardStatePlace(snapshot, placeId);
}

export function currentWorkItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>,
): StatePositionWorkItem[] {
  return snapshot && placeId
    ? enrichStatePositionWorkItems(
        snapshot.runtime.current_work_items_by_place_id?.[placeId] ?? [],
        snapshot,
        workstationRequestsByDispatchID,
      )
    : [];
}

export function terminalHistoryItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>,
): StatePositionWorkItem[] {
  return snapshot && placeId
    ? enrichStatePositionWorkItems(
        snapshot.runtime.place_occupancy_work_items_by_place_id?.[placeId] ?? [],
        snapshot,
        workstationRequestsByDispatchID,
      )
    : [];
}

function enrichStatePositionWorkItems(
  workItems: DashboardWorkItemRef[],
  snapshot: DashboardSnapshot,
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>,
): StatePositionWorkItem[] {
  const activeExecutions = Object.values(
    snapshot.runtime.active_executions_by_dispatch_id ?? {},
  );
  const sortedRequests = sortWorkstationRequests(
    Object.values(
      workstationRequestsByDispatchID ??
        snapshot.runtime.workstation_requests_by_dispatch_id ??
        {},
    ),
  );

  return workItems.map((workItem) => {
    const matchingRequest = sortedRequests.find((request) =>
      requestWorkItems(request).some(
        (candidate) => candidate.work_id === workItem.work_id,
      ),
    );
    const startedAt =
      activeExecutions.find((execution) =>
        execution.work_items?.some(
          (candidate) => candidate.work_id === workItem.work_id,
        ),
      )?.started_at ??
      (matchingRequest ? requestStartedAt(matchingRequest) : undefined);

    return startedAt ? { ...workItem, started_at: startedAt } : workItem;
  });
}

export function activeExecutionsForSelectedWorkstation(
  snapshot: DashboardSnapshot | null | undefined,
  selection: DashboardSelection | null,
  selectedNode: DashboardWorkstationNode | null,
): DashboardActiveExecution[] {
  if (
    !snapshot ||
    !selectedNode ||
    (selection?.kind !== "node" && selection?.kind !== "workstation-request")
  ) {
    return [];
  }

  return Object.values(snapshot.runtime.active_executions_by_dispatch_id ?? {}).filter(
    (execution) =>
      execution.workstation_node_id === selectedNode.node_id ||
      execution.transition_id === selectedNode.transition_id ||
      execution.workstation_name === selectedNode.workstation_name,
  );
}

export function resolveWorkItemSelectionByWorkID({
  dispatchID,
  nodeID,
  snapshot,
  terminalWorkDetail,
  workID,
  workstationRequestsByDispatchID,
}: {
  dispatchID?: string;
  nodeID?: string;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail?: TerminalWorkDetail | null;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): DashboardSelection | null {
  if (!snapshot) {
    return null;
  }

  const failedDetail = snapshot.runtime.session.failed_work_details_by_work_id?.[workID];
  const preferredFailureDispatchID =
    dispatchID ??
    terminalWorkDetail?.dispatchID ??
    failedDetail?.dispatch_id;
  const preferredSelection = resolvePreferredWorkItemSelection({
    failedDetail,
    nodeID,
    preferredFailureDispatchID,
    snapshot,
    terminalWorkDetail,
    workID,
    workstationRequestsByDispatchID,
  });
  if (preferredSelection) {
    return preferredSelection;
  }

  const workstationRequest = Object.values(
    workstationRequestsByDispatchID ?? snapshot.runtime.workstation_requests_by_dispatch_id ?? {},
  ).find((request) => requestWorkItems(request).some((item) => item.work_id === workID));

  for (const execution of Object.values(snapshot.runtime.active_executions_by_dispatch_id ?? {})) {
    const matchedWorkItem = execution.work_items?.find((candidate) => candidate.work_id === workID);
    if (matchedWorkItem) {
      return {
        dispatchId: execution.dispatch_id,
        execution,
        kind: "work-item",
        nodeId: execution.workstation_node_id,
        workItem: matchedWorkItem,
      };
    }
  }

  const fallbackWorkItem =
    findWorkItemReference(snapshot, workID) ??
    terminalWorkDetail?.workItem ??
    snapshot.runtime.session.failed_work_details_by_work_id?.[workID]?.work_item;
  if (!fallbackWorkItem) {
    return null;
  }

  if (workstationRequest) {
    return {
      dispatchId: requestDispatchID(workstationRequest),
      kind: "work-item",
      nodeId: requestWorkstationNodeID(workstationRequest),
      workItem: fallbackWorkItem,
    };
  }

  const providerAttempt = snapshot.runtime.session.provider_sessions?.find((attempt) =>
    attempt.work_items?.some((item) => item.work_id === workID),
  );
  const providerNodeID =
    providerAttempt?.transition_id && snapshot.topology.workstation_nodes_by_id[providerAttempt.transition_id]
      ? providerAttempt.transition_id
      : Object.values(snapshot.topology.workstation_nodes_by_id).find(
          (node) => node.workstation_name === providerAttempt?.workstation_name,
        )?.node_id;
  if (providerAttempt && providerNodeID) {
    return {
      dispatchId: providerAttempt.dispatch_id,
      kind: "work-item",
      nodeId: providerNodeID,
      workItem: providerAttempt.work_items?.find((item) => item.work_id === workID) ?? fallbackWorkItem,
    };
  }

  if (failedDetail) {
    const failedNodeID =
      snapshot.topology.workstation_nodes_by_id[failedDetail.transition_id]?.node_id ??
      Object.values(snapshot.topology.workstation_nodes_by_id).find(
        (node) => node.workstation_name === failedDetail.workstation_name,
      )?.node_id;
    if (failedNodeID) {
      return {
        dispatchId: failedDetail.dispatch_id,
        kind: "work-item",
        nodeId: failedNodeID,
        workItem: failedDetail.work_item,
      };
    }
  }

  const retainedNodeID = findTrackedWorkNodeID(snapshot, workID);
  if (retainedNodeID) {
    return {
      kind: "work-item",
      nodeId: retainedNodeID,
      workItem: fallbackWorkItem,
    };
  }

  if (nodeID && snapshot.topology.workstation_nodes_by_id[nodeID]) {
    return {
      kind: "work-item",
      nodeId: nodeID,
      workItem: fallbackWorkItem,
    };
  }

  return null;
}

function resolvePreferredWorkItemSelection({
  failedDetail,
  nodeID,
  preferredFailureDispatchID,
  snapshot,
  terminalWorkDetail,
  workID,
  workstationRequestsByDispatchID,
}: {
  failedDetail: DashboardFailedWorkDetail | undefined;
  nodeID: string | undefined;
  preferredFailureDispatchID: string | undefined;
  snapshot: DashboardSnapshot;
  terminalWorkDetail: TerminalWorkDetail | null | undefined;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): DashboardSelection | null {
  if (!preferredFailureDispatchID) {
    return null;
  }

  const preferredExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[preferredFailureDispatchID];
  if (
    preferredExecution &&
    (!nodeID || preferredExecution.workstation_node_id === nodeID)
  ) {
    const matchedWorkItem = preferredExecution.work_items?.find(
      (candidate) => candidate.work_id === workID,
    );
    const resolvedWorkItem =
      matchedWorkItem ?? failedDetail?.work_item ?? terminalWorkDetail?.workItem;
    if (!resolvedWorkItem) {
      return null;
    }
    return {
      dispatchId: preferredExecution.dispatch_id,
      execution: preferredExecution,
      kind: "work-item",
      nodeId: preferredExecution.workstation_node_id,
      workItem: resolvedWorkItem,
    };
  }

  const preferredRequest = (
    workstationRequestsByDispatchID ??
    snapshot.runtime.workstation_requests_by_dispatch_id
  )?.[preferredFailureDispatchID];
  if (preferredRequest) {
    return selectionFromWorkstationRequest(
      preferredRequest,
      workID,
      failedDetail?.work_item ?? terminalWorkDetail?.workItem,
    );
  }

  if (failedDetail?.dispatch_id === preferredFailureDispatchID) {
    return selectionFromFailedDetail(snapshot, failedDetail);
  }

  return null;
}

function selectionFromWorkstationRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
  workID: string,
  fallbackWorkItem: DashboardWorkItemRef | undefined,
): DashboardSelection | null {
  const resolvedWorkItem =
    requestWorkItems(request).find((candidate) => candidate.work_id === workID) ??
    fallbackWorkItem;
  if (!resolvedWorkItem) {
    return null;
  }
  return {
    dispatchId: requestDispatchID(request),
    kind: "work-item",
    nodeId: requestWorkstationNodeID(request),
    workItem: resolvedWorkItem,
  };
}

function selectionFromFailedDetail(
  snapshot: DashboardSnapshot,
  failedDetail: DashboardFailedWorkDetail,
): DashboardSelection | null {
  const failedNodeID =
    snapshot.topology.workstation_nodes_by_id[failedDetail.transition_id]?.node_id ??
    Object.values(snapshot.topology.workstation_nodes_by_id).find(
      (node) => node.workstation_name === failedDetail.workstation_name,
    )?.node_id;
  if (!failedNodeID) {
    return null;
  }

  return {
    dispatchId: failedDetail.dispatch_id,
    kind: "work-item",
    nodeId: failedNodeID,
    workItem: failedDetail.work_item,
  };
}

export function placeNodeID(
  snapshot: DashboardSnapshot | null | undefined,
  place: DashboardPlaceRef,
): string | undefined {
  return snapshot ? findWorkstationNodeIDForPlace(snapshot, place.place_id) : undefined;
}

export function inferStateWorkTerminalStatus(
  snapshot: DashboardSnapshot | null | undefined,
  place: DashboardPlaceRef,
  workItem: DashboardWorkItemRef,
): TerminalWorkStatus | null {
  if (!snapshot) {
    return null;
  }

  if (snapshot.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]) {
    return "failed";
  }

  const displayLabel = workItem.display_name?.trim() || workItem.work_id;
  const labels = [workItem.work_id, displayLabel];
  if (labels.some((label) => (snapshot.runtime.session.failed_work_labels ?? []).includes(label))) {
    return "failed";
  }
  if (labels.some((label) => (snapshot.runtime.session.completed_work_labels ?? []).includes(label))) {
    return "completed";
  }
  if (place.state_category === "FAILED") {
    return "failed";
  }
  if (place.state_category === "TERMINAL") {
    return "completed";
  }
  return null;
}

export function findTerminalWorkItem(
  items: TerminalWorkItem[],
  workItem: DashboardWorkItemRef,
): TerminalWorkItem | undefined {
  const workLabel = workItem.display_name?.trim() || workItem.work_id;
  return items.find((item) => (
    item.traceWorkID === workItem.work_id ||
    item.workItem?.work_id === workItem.work_id ||
    item.label === workLabel
  ));
}

function findTrackedWorkNodeID(snapshot: DashboardSnapshot, workID: string): string | undefined {
  for (const [placeID, workItems] of Object.entries(snapshot.runtime.current_work_items_by_place_id ?? {})) {
    if (workItems.some((workItem) => workItem.work_id === workID)) {
      return findWorkstationNodeIDForPlace(snapshot, placeID);
    }
  }

  for (const [placeID, workItems] of Object.entries(snapshot.runtime.place_occupancy_work_items_by_place_id ?? {})) {
    if (workItems.some((workItem) => workItem.work_id === workID)) {
      return findWorkstationNodeIDForPlace(snapshot, placeID);
    }
  }

  return undefined;
}
