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
} from "../../api/dashboard/types";
import {
  findWorkItemReference,
  findWorkstationNodeIDForPlace,
} from "../../state/dashboardSelection";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../terminal-work/terminal-work-card";
import type { DashboardSelection, TerminalWorkDetail } from "./types";
import { requestDispatchID, requestWorkItems, toDashboardWorkstationRequest } from "./useCurrentSelection.request-helpers";

export function buildTerminalWorkItems(
  labels: string[],
  attempts: DashboardProviderSessionAttempt[] | undefined,
  failureDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>,
): TerminalWorkItem[] {
  const failureDetails = Object.values(failureDetailsByWorkID ?? {});

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
    const matchedFailureDetail = failureDetails.find(
      (detail) =>
        detail.work_item.display_name === label ||
        detail.work_item.work_id === label ||
        (matchedWorkItem ? detail.work_item.work_id === matchedWorkItem.work_id : false),
    );

    return {
      attempts: matchingAttempts,
      failureMessage: matchedFailureDetail?.failure_message ?? latestAttempt?.failure_message,
      failureReason: matchedFailureDetail?.failure_reason ?? latestAttempt?.failure_reason,
      label,
      traceWorkID: matchedWorkItem?.work_id ?? matchedFailureDetail?.work_item.work_id ?? label,
      workItem: matchedWorkItem ?? matchedFailureDetail?.work_item,
    };
  });
}

export function findStatePlace(snapshot: DashboardSnapshot, placeId: string): DashboardPlaceRef | null {
  const placesById = new Map<string, DashboardPlaceRef>();

  for (const nodeId of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [...(workstation.input_places ?? []), ...(workstation.output_places ?? [])]) {
      if (place.kind === "work_state") {
        placesById.set(place.place_id, place);
      }
    }
  }

  return placesById.get(placeId) ?? null;
}

export function currentWorkItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
): DashboardWorkItemRef[] {
  return snapshot && placeId
    ? snapshot.runtime.current_work_items_by_place_id?.[placeId] ?? []
    : [];
}

export function terminalHistoryItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
): DashboardWorkItemRef[] {
  return snapshot && placeId
    ? snapshot.runtime.place_occupancy_work_items_by_place_id?.[placeId] ?? []
    : [];
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

export function resolveTrackedWorkSelection({
  nodeID,
  snapshot,
  terminalWorkDetail,
  workID,
  workstationRequestsByDispatchID,
}: {
  nodeID?: string;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail?: TerminalWorkDetail | null;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): DashboardSelection | null {
  if (!snapshot) {
    return null;
  }

  const workstationRequest = Object.values(
    workstationRequestsByDispatchID ?? snapshot.runtime.workstation_requests_by_dispatch_id ?? {},
  ).find((request) => requestWorkItems(request).some((item) => item.work_id === workID));

  if (workstationRequest && isScriptBackedWorkstationRequest(workstationRequest)) {
    return {
      dispatchId: requestDispatchID(workstationRequest),
      kind: "workstation-request",
      nodeId: requestWorkstationNodeID(workstationRequest),
      request: toDashboardWorkstationRequest(workstationRequest),
    };
  }

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
      kind: "workstation-request",
      nodeId: requestWorkstationNodeID(workstationRequest),
      request: toDashboardWorkstationRequest(workstationRequest),
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

  const failedDetail = snapshot.runtime.session.failed_work_details_by_work_id?.[workID];
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

function requestWorkstationNodeID(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): string {
  return "workstation_node_id" in request
    ? request.workstation_node_id
    : request.transitionId ?? request.transition_id ?? "";
}

function isScriptBackedWorkstationRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): boolean {
  if ("workstation_node_id" in request) {
    return request.script_request !== undefined || request.script_response !== undefined;
  }

  return (
    request.request.scriptRequest !== undefined ||
    request.request.script_request !== undefined ||
    request.response?.scriptResponse !== undefined ||
    request.response?.script_response !== undefined
  );
}
