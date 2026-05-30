import type {
  DashboardActiveExecution,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type { FactoryWorker } from "../../../../api/events/types";
import { hasDashboardStatePlace } from "./dashboardStatePlaces";

export interface DashboardNodeSelection {
  kind: "node";
  nodeId: string;
}

export interface DashboardStateNodeSelection {
  kind: "state-node";
  placeId: string;
}

export interface DashboardWorkItemSelection {
  dispatchId?: string;
  execution?: DashboardActiveExecution;
  kind: "work-item";
  nodeId: string;
  workItem: DashboardWorkItemRef;
}

export interface DashboardWorkstationRequestSelection {
  dispatchId: string;
  kind: "workstation-request";
  nodeId: string;
  request: DashboardWorkstationRequest;
}

export interface DashboardWorkerSelection {
  kind: "worker";
  workerName: string;
}

export type DashboardSelection =
  | DashboardNodeSelection
  | DashboardStateNodeSelection
  | DashboardWorkItemSelection
  | DashboardWorkstationRequestSelection
  | DashboardWorkerSelection;

export function selectDefaultSelection(snapshot: DashboardSnapshot): DashboardSelection | null {
  const firstActiveNodeId = snapshot.runtime.active_workstation_node_ids?.[0];
  if (firstActiveNodeId) {
    return { kind: "node", nodeId: firstActiveNodeId };
  }

  const firstNodeId = snapshot.topology.workstation_node_ids[0];
  return firstNodeId ? { kind: "node", nodeId: firstNodeId } : null;
}

interface ResolveDashboardSelectionInput {
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

export function resolveDashboardSelection({
  selection,
  snapshot,
  workstationRequestsByDispatchID,
}: ResolveDashboardSelectionInput): DashboardSelection | null {
  if (selection === null) {
    return selectDefaultSelection(snapshot);
  }

  if (selection.kind === "node") {
    return snapshot.topology.workstation_nodes_by_id[selection.nodeId]
      ? selection
      : selectDefaultSelection(snapshot);
  }

  if (selection.kind === "state-node") {
    return hasDashboardStatePlace(snapshot, selection.placeId)
      ? selection
      : selectDefaultSelection(snapshot);
  }

  if (selection.kind === "work-item") {
    return resolveWorkItemSelection(
      snapshot,
      selection,
      workstationRequestsByDispatchID,
    );
  }

  if (selection.kind === "worker") {
    return workerExistsInSnapshotFactory(snapshot, selection.workerName)
      ? selection
      : selectDefaultSelection(snapshot);
  }

  return resolveWorkstationRequestSelection(
    snapshot,
    selection,
    workstationRequestsByDispatchID,
  );
}

function resolveWorkItemSelection(
  snapshot: DashboardSnapshot,
  selection: DashboardWorkItemSelection,
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>,
): DashboardSelection | null {
  const currentWorkID = selection.workItem.work_id;
  const currentExecution =
    selection.dispatchId === undefined
      ? undefined
      : snapshot.runtime.active_executions_by_dispatch_id?.[selection.dispatchId];
  const currentWorkItem = findWorkItemReference(snapshot, currentWorkID);
  const resolvedSelection =
    selectionFromExecution(currentExecution, currentWorkID) ??
    findAnyActiveExecutionSelection(snapshot, currentWorkID) ??
    findRetainedRequestSelection(
      snapshot,
      currentWorkID,
      workstationRequestsByDispatchID,
    ) ??
    findFailedWorkSelection(snapshot, currentWorkID) ??
    findTrackedWorkSelection(snapshot, currentWorkID, currentWorkItem) ??
    findProviderWorkSelection(snapshot, currentWorkID);
  if (resolvedSelection) {
    return resolvedSelection;
  }

  if (!currentWorkItem) {
    return snapshot.topology.workstation_nodes_by_id[selection.nodeId]
      ? { kind: "node", nodeId: selection.nodeId }
      : selectDefaultSelection(snapshot);
  }

  return snapshot.topology.workstation_nodes_by_id[selection.nodeId]
    ? {
        dispatchId: selection.dispatchId,
        execution: currentExecution,
        kind: "work-item",
        nodeId: selection.nodeId,
        workItem: currentWorkItem,
      }
    : selectDefaultSelection(snapshot);
}

function selectionFromExecution(
  execution: DashboardActiveExecution | undefined,
  workID: string,
): DashboardWorkItemSelection | null {
  const workItem = execution?.work_items?.find(
    (candidate) => candidate.work_id === workID,
  );
  if (!execution || !workItem) {
    return null;
  }

  return {
    dispatchId: execution.dispatch_id,
    execution,
    kind: "work-item",
    nodeId: execution.workstation_node_id,
    workItem,
  };
}

function findAnyActiveExecutionSelection(
  snapshot: DashboardSnapshot,
  workID: string,
): DashboardWorkItemSelection | null {
  const execution = Object.values(
    snapshot.runtime.active_executions_by_dispatch_id ?? {},
  ).find((candidate) =>
    candidate.work_items?.some((workItem) => workItem.work_id === workID),
  );

  return selectionFromExecution(execution, workID);
}

function findRetainedRequestSelection(
  snapshot: DashboardSnapshot,
  workID: string,
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>,
): DashboardWorkItemSelection | null {
  const retainedRequest =
    findWorkstationRequestForWork(
      workstationRequestsByDispatchID,
      workID,
    ) ??
    findWorkstationRequestForWork(
      snapshot.runtime.workstation_requests_by_dispatch_id,
      workID,
    );
  const nodeID = retainedRequest
    ? resolveRetainedRequestNodeID(snapshot, retainedRequest)
    : undefined;
  const workItem = retainedRequest
    ? workItemsFromRetainedRequest(retainedRequest).find(
        (candidate) => candidate.work_id === workID,
      )
    : undefined;
  if (!retainedRequest || !nodeID || !workItem) {
    return null;
  }

  return {
    dispatchId: retainedRequest.dispatch_id,
    kind: "work-item",
    nodeId: nodeID,
    workItem,
  };
}

function findFailedWorkSelection(
  snapshot: DashboardSnapshot,
  workID: string,
): DashboardWorkItemSelection | null {
  const failedDetail =
    snapshot.runtime.session.failed_work_details_by_work_id?.[workID];
  const nodeID = failedDetail
    ? resolveTransitionNodeID(
        snapshot,
        failedDetail.transition_id,
        failedDetail.workstation_name,
      )
    : undefined;
  if (!failedDetail || !nodeID) {
    return null;
  }

  return {
    dispatchId: failedDetail.dispatch_id,
    kind: "work-item",
    nodeId: nodeID,
    workItem: failedDetail.work_item,
  };
}

function findTrackedWorkSelection(
  snapshot: DashboardSnapshot,
  workID: string,
  workItem: DashboardWorkItemRef | undefined,
): DashboardWorkItemSelection | null {
  const nodeID = findTrackedWorkNodeID(snapshot, workID);
  if (!nodeID || !workItem) {
    return null;
  }

  return {
    kind: "work-item",
    nodeId: nodeID,
    workItem,
  };
}

function findProviderWorkSelection(
  snapshot: DashboardSnapshot,
  workID: string,
): DashboardWorkItemSelection | null {
  const providerAttempt = snapshot.runtime.session.provider_sessions?.find((attempt) =>
    attempt.work_items?.some((workItem) => workItem.work_id === workID),
  );
  const nodeID = providerAttempt
    ? resolveTransitionNodeID(
        snapshot,
        providerAttempt.transition_id,
        providerAttempt.workstation_name,
      )
    : undefined;
  const workItem = providerAttempt?.work_items?.find(
    (candidate) => candidate.work_id === workID,
  );
  if (!providerAttempt || !nodeID || !workItem) {
    return null;
  }

  return {
    dispatchId: providerAttempt.dispatch_id,
    kind: "work-item",
    nodeId: nodeID,
    workItem,
  };
}

function resolveWorkstationRequestSelection(
  snapshot: DashboardSnapshot,
  selection: DashboardWorkstationRequestSelection,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined,
): DashboardSelection | null {
  const currentRequest = workstationRequestsByDispatchID?.[selection.dispatchId];
  if (!currentRequest) {
    return selectDefaultSelection(snapshot);
  }

  if (!snapshot.topology.workstation_nodes_by_id[currentRequest.workstation_node_id]) {
    return selectDefaultSelection(snapshot);
  }

  return {
    dispatchId: currentRequest.dispatch_id,
    kind: "workstation-request",
    nodeId: currentRequest.workstation_node_id,
    request: currentRequest,
  };
}

export function findWorkItemReference(
  snapshot: DashboardSnapshot,
  workID: string,
): DashboardWorkItemRef | undefined {
  const activeWorkItem = Object.values(snapshot.runtime.active_executions_by_dispatch_id ?? {})
    .flatMap((execution) => execution.work_items ?? [])
    .find((workItem) => workItem.work_id === workID);
  if (activeWorkItem) {
    return activeWorkItem;
  }

  const currentWorkItem = Object.values(snapshot.runtime.current_work_items_by_place_id ?? {})
    .flat()
    .find((workItem) => workItem.work_id === workID);
  if (currentWorkItem) {
    return currentWorkItem;
  }

  const retainedWorkItem = Object.values(snapshot.runtime.place_occupancy_work_items_by_place_id ?? {})
    .flat()
    .find((workItem) => workItem.work_id === workID);
  if (retainedWorkItem) {
    return retainedWorkItem;
  }

  const workstationRequestWorkItem = Object.values(
    snapshot.runtime.workstation_requests_by_dispatch_id ?? {},
  )
    .flatMap((request) => workItemsFromRetainedRequest(request))
    .find((workItem) => workItem.work_id === workID);
  if (workstationRequestWorkItem) {
    return workstationRequestWorkItem;
  }

  return snapshot.runtime.session.provider_sessions
    ?.flatMap((attempt) => attempt.work_items ?? [])
    .find((workItem) => workItem.work_id === workID);
}

export function findWorkstationNodeIDForPlace(
  snapshot: DashboardSnapshot,
  placeID: string,
): string | undefined {
  for (const nodeID of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeID];
    if (!workstation) {
      continue;
    }

    const matchingPlace = [...(workstation.input_places ?? []), ...(workstation.output_places ?? [])]
      .some((place) => place.place_id === placeID);
    if (matchingPlace) {
      return nodeID;
    }
  }

  return undefined;
}

function workItemsFromRetainedRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  const runtimeRequest = "request" in request ? request.request : undefined;
  const runtimeResponse =
    "response" in request && typeof request.response === "object"
      ? request.response
      : undefined;
  const projectedRequestView = "request_view" in request ? request.request_view : undefined;
  const projectedResponseView =
    "response_view" in request ? request.response_view : undefined;
  const projectedWorkItems = "work_items" in request ? request.work_items : undefined;

  return [
    ...(runtimeRequest?.input_work_items ?? []),
    ...(runtimeResponse?.output_work_items ?? []),
    ...(projectedRequestView?.input_work_items ?? []),
    ...(projectedResponseView?.output_work_items ?? []),
    ...(projectedWorkItems ?? []),
  ];
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

function findWorkstationRequestForWork(
  workstationRequestsByDispatchID:
    | Record<string, DashboardRuntimeWorkstationRequest>
    | Record<string, DashboardWorkstationRequest>
    | undefined,
  workID: string,
): DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest | undefined {
  return Object.values(workstationRequestsByDispatchID ?? {}).find((request) =>
    workItemsFromRetainedRequest(request).some(
      (workItem) => workItem.work_id === workID,
    ),
  );
}

function resolveRetainedRequestNodeID(
  snapshot: DashboardSnapshot,
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): string | undefined {
  if ("workstation_node_id" in request) {
    return request.workstation_node_id;
  }

  return resolveTransitionNodeID(
    snapshot,
    request.transition_id,
    request.workstation_name,
  );
}

function workerExistsInSnapshotFactory(
  snapshot: DashboardSnapshot,
  workerName: string,
): boolean {
  return findFactoryWorkerInSnapshot(snapshot, workerName) !== undefined;
}

export function findFactoryWorkerInSnapshot(
  snapshot: DashboardSnapshot,
  workerName: string,
): FactoryWorker | undefined {
  return snapshot.factory?.workers?.find((worker) => worker.name === workerName);
}

export function workstationNamesReferencingWorkerInSnapshot(
  snapshot: DashboardSnapshot,
  workerName: string,
): string[] {
  return (snapshot.factory?.workstations ?? [])
    .filter((workstation) => workstation.worker === workerName)
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}

function resolveTransitionNodeID(
  snapshot: DashboardSnapshot,
  transitionID: string | undefined,
  workstationName: string | undefined,
): string | undefined {
  if (transitionID && snapshot.topology.workstation_nodes_by_id[transitionID]) {
    return snapshot.topology.workstation_nodes_by_id[transitionID]?.node_id;
  }

  return Object.values(snapshot.topology.workstation_nodes_by_id).find(
    (node) => node.workstation_name === workstationName,
  )?.node_id;
}
