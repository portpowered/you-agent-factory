// biome-ignore-all lint/style/noExcessiveLinesPerFile: selection derivation helpers are kept colocated to avoid splitting shared runtime-selection rules mid-story.
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
} from "../../../../api/dashboard/types";
import {
  type TerminalWorkItem,
  type TerminalWorkStatus,
  terminalWorkStatusFromOutcome,
} from "../../../terminal-work/lib/types";
import {
  findWorkItemReference,
  findWorkstationNodeIDForPlace,
} from "../../state/dashboardSelection";
import { findDashboardStatePlace } from "../../state/dashboardStatePlaces";
import type {
  DashboardSelection,
  StatePositionWorkItem,
  TerminalWorkDetail,
} from "../../state/selection-types";
import {
  type DispatchWorkstationRequest,
  requestDispatchID,
  requestStartedAt,
  requestTransitionID,
  requestWorkItems,
  requestWorkstationName,
  requestWorkstationNodeID,
  sortWorkstationRequests,
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

  return labels.flatMap((label) => {
    const canonicalItems = uniqueTerminalWorkItemsForLabel(
      label,
      attempts,
      failureDetails,
      requests,
    );
    return (canonicalItems.length > 1 ? canonicalItems : [undefined]).map(
      (canonicalItem) => {
        const matchingAttempts =
          attempts?.filter((attempt) =>
            attempt.work_items?.some((workItem) =>
              canonicalItem
                ? workItem.work_id === canonicalItem.work_id
                : workItem.display_name === label || workItem.work_id === label,
            ),
          ) ?? [];
        const latestAttempt = matchingAttempts[matchingAttempts.length - 1];
        const matchedWorkItem =
          canonicalItem ??
          matchingAttempts
            .flatMap((attempt) => attempt.work_items ?? [])
            .find(
              (workItem) =>
                workItem.display_name === label || workItem.work_id === label,
            );
        const matchingRequests = requests.filter((request) =>
          requestWorkItems(request).some((workItem) =>
            matchedWorkItem
              ? workItem.work_id === matchedWorkItem.work_id
              : workItem.display_name === label || workItem.work_id === label,
          ),
        );
        const latestRequest = matchingRequests[0];
        const matchedFailureDetail = failureDetails.find((detail) =>
          matchedWorkItem
            ? detail.work_item.work_id === matchedWorkItem.work_id
            : detail.work_item.display_name === label ||
              detail.work_item.work_id === label,
        );

        return {
          attempts: matchingAttempts,
          dispatchID:
            matchedFailureDetail?.dispatch_id ??
            (latestRequest ? requestDispatchID(latestRequest) : undefined) ??
            latestAttempt?.dispatch_id,
          failureMessage:
            matchedFailureDetail?.failure_message ??
            (latestRequest
              ? requestFailureMessage(latestRequest)
              : undefined) ??
            latestAttempt?.failure_message,
          failureReason:
            matchedFailureDetail?.failure_reason ??
            (latestRequest ? requestFailureReason(latestRequest) : undefined) ??
            latestAttempt?.failure_reason,
          label: matchedWorkItem?.display_name?.trim() || label,
          traceWorkID:
            matchedWorkItem?.work_id ??
            matchedFailureDetail?.work_item.work_id ??
            label,
          workItem: matchedWorkItem ?? matchedFailureDetail?.work_item,
          workstationName: terminalWorkstationName(
            matchedFailureDetail,
            latestAttempt,
            latestRequest,
          ),
        };
      },
    );
  });
}

function uniqueTerminalWorkItemsForLabel(
  label: string,
  attempts: DashboardProviderSessionAttempt[] | undefined,
  failureDetails: DashboardFailedWorkDetail[],
  requests: DispatchWorkstationRequest[],
): DashboardWorkItemRef[] {
  const byWorkID = new Map<string, DashboardWorkItemRef>();
  const addIfMatching = (workItem: DashboardWorkItemRef) => {
    if (
      (workItem.display_name === label || workItem.work_id === label) &&
      !byWorkID.has(workItem.work_id)
    ) {
      byWorkID.set(workItem.work_id, workItem);
    }
  };
  for (const detail of failureDetails) {
    addIfMatching(detail.work_item);
  }
  for (const request of requests) {
    for (const workItem of requestWorkItems(request)) {
      addIfMatching(workItem);
    }
  }
  for (const attempt of attempts ?? []) {
    for (const workItem of attempt.work_items ?? []) {
      addIfMatching(workItem);
    }
  }
  return [...byWorkID.values()];
}

export function buildNonStandardTerminalWorkItems(
  workstationRequestsByDispatchID:
    | Record<string, DispatchWorkstationRequest>
    | undefined,
  attempts: DashboardProviderSessionAttempt[] | undefined,
): Record<"canceled" | "terminated" | "unknown", TerminalWorkItem[]> {
  const itemsByStatus: Record<
    "canceled" | "terminated" | "unknown",
    TerminalWorkItem[]
  > = {
    canceled: [],
    terminated: [],
    unknown: [],
  };
  const seen = new Set<string>();

  for (const request of sortWorkstationRequests(
    Object.values(workstationRequestsByDispatchID ?? {}),
  )) {
    const status = terminalWorkStatusFromOutcome(requestOutcome(request));
    if (!status || status === "completed" || status === "failed") {
      continue;
    }

    const requestID = requestDispatchID(request);
    const requestAttempts =
      attempts?.filter((attempt) =>
        attempt.work_items?.some(
          (workItem) =>
            requestWorkItems(request).some(
              (requestWorkItem) => requestWorkItem.work_id === workItem.work_id,
            ) && attempt.dispatch_id === requestID,
        ),
      ) ?? [];
    for (const workItem of requestWorkItems(request)) {
      const identity = `${requestID}:${workItem.work_id}`;
      if (seen.has(identity)) {
        continue;
      }
      seen.add(identity);
      itemsByStatus[status].push({
        attempts: requestAttempts,
        dispatchID: requestID,
        failureMessage: requestFailureMessage(request),
        failureReason: requestFailureReason(request),
        label: workItem.display_name?.trim() || workItem.work_id,
        traceWorkID: workItem.work_id,
        workItem,
        workstationName: requestWorkstationName(request),
      });
    }
  }

  return itemsByStatus;
}

function requestFailureMessage(
  request: DispatchWorkstationRequest,
): string | undefined {
  return "workstation_node_id" in request
    ? request.failure_message
    : request.response?.failureDetail?.message;
}

function requestFailureReason(
  request: DispatchWorkstationRequest,
): string | undefined {
  return "workstation_node_id" in request
    ? request.failure_reason
    : request.response?.failureDetail?.reason;
}

function requestOutcome(
  request: DispatchWorkstationRequest,
): string | undefined {
  return "workstation_node_id" in request
    ? request.outcome
    : request.response?.outcome;
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

export function findStatePlace(
  snapshot: DashboardSnapshot,
  placeId: string,
): DashboardPlaceRef | null {
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
        snapshot.runtime.place_occupancy_work_items_by_place_id?.[placeId] ??
          [],
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

  return Object.values(
    snapshot.runtime.active_executions_by_dispatch_id ?? {},
  ).filter(
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

  const failedDetail =
    snapshot.runtime.session.failed_work_details_by_work_id?.[workID];
  const preferredFailureDispatchID =
    dispatchID ?? terminalWorkDetail?.dispatchID ?? failedDetail?.dispatch_id;
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
    workstationRequestsByDispatchID ??
      snapshot.runtime.workstation_requests_by_dispatch_id ??
      {},
  ).find((request) =>
    requestWorkItems(request).some((item) => item.work_id === workID),
  );

  for (const execution of Object.values(
    snapshot.runtime.active_executions_by_dispatch_id ?? {},
  )) {
    const matchedWorkItem = execution.work_items?.find(
      (candidate) => candidate.work_id === workID,
    );
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
    snapshot.runtime.session.failed_work_details_by_work_id?.[workID]
      ?.work_item;
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

  const providerAttempt = snapshot.runtime.session.provider_sessions?.find(
    (attempt) => attempt.work_items?.some((item) => item.work_id === workID),
  );
  const providerNodeID =
    providerAttempt?.transition_id &&
    snapshot.topology.workstation_nodes_by_id[providerAttempt.transition_id]
      ? providerAttempt.transition_id
      : Object.values(snapshot.topology.workstation_nodes_by_id).find(
          (node) => node.workstation_name === providerAttempt?.workstation_name,
        )?.node_id;
  if (providerAttempt && providerNodeID) {
    return {
      dispatchId: providerAttempt.dispatch_id,
      kind: "work-item",
      nodeId: providerNodeID,
      workItem:
        providerAttempt.work_items?.find((item) => item.work_id === workID) ??
        fallbackWorkItem,
    };
  }

  if (failedDetail) {
    const failedNodeID =
      snapshot.topology.workstation_nodes_by_id[failedDetail.transition_id]
        ?.node_id ??
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
    snapshot.runtime.active_executions_by_dispatch_id?.[
      preferredFailureDispatchID
    ];
  if (
    preferredExecution &&
    (!nodeID || preferredExecution.workstation_node_id === nodeID)
  ) {
    const matchedWorkItem = preferredExecution.work_items?.find(
      (candidate) => candidate.work_id === workID,
    );
    const resolvedWorkItem =
      matchedWorkItem ??
      failedDetail?.work_item ??
      terminalWorkDetail?.workItem;
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

  const preferredRequest = (workstationRequestsByDispatchID ??
    snapshot.runtime.workstation_requests_by_dispatch_id)?.[
    preferredFailureDispatchID
  ];
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
    requestWorkItems(request).find(
      (candidate) => candidate.work_id === workID,
    ) ?? fallbackWorkItem;
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
    snapshot.topology.workstation_nodes_by_id[failedDetail.transition_id]
      ?.node_id ??
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
  return snapshot
    ? findWorkstationNodeIDForPlace(snapshot, place.place_id)
    : undefined;
}

export function inferStateWorkTerminalStatus(
  snapshot: DashboardSnapshot | null | undefined,
  place: DashboardPlaceRef,
  workItem: DashboardWorkItemRef,
): TerminalWorkStatus | null {
  if (!snapshot) {
    return null;
  }

  if (
    snapshot.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]
  ) {
    return "failed";
  }

  const displayLabel = workItem.display_name?.trim() || workItem.work_id;
  const labels = [workItem.work_id, displayLabel];
  if (
    labels.some((label) =>
      (snapshot.runtime.session.failed_work_labels ?? []).includes(label),
    )
  ) {
    return "failed";
  }
  if (
    labels.some((label) =>
      (snapshot.runtime.session.completed_work_labels ?? []).includes(label),
    )
  ) {
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
  dispatchID?: string,
): TerminalWorkItem | undefined {
  const matchingItems = items.filter(
    (item) =>
      item.traceWorkID === workItem.work_id ||
      item.workItem?.work_id === workItem.work_id,
  );
  if (!dispatchID) {
    return matchingItems[0];
  }
  return (
    matchingItems.find((item) => item.dispatchID === dispatchID) ??
    matchingItems[0]
  );
}

function findTrackedWorkNodeID(
  snapshot: DashboardSnapshot,
  workID: string,
): string | undefined {
  for (const [placeID, workItems] of Object.entries(
    snapshot.runtime.current_work_items_by_place_id ?? {},
  )) {
    if (workItems.some((workItem) => workItem.work_id === workID)) {
      return findWorkstationNodeIDForPlace(snapshot, placeID);
    }
  }

  for (const [placeID, workItems] of Object.entries(
    snapshot.runtime.place_occupancy_work_items_by_place_id ?? {},
  )) {
    if (workItems.some((workItem) => workItem.work_id === workID)) {
      return findWorkstationNodeIDForPlace(snapshot, placeID);
    }
  }

  return undefined;
}
