import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../api/dashboard";

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

export type DashboardSelection =
  | DashboardNodeSelection
  | DashboardStateNodeSelection
  | DashboardWorkItemSelection
  | DashboardWorkstationRequestSelection;

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
    return hasStatePlace(snapshot, selection.placeId)
      ? selection
      : selectDefaultSelection(snapshot);
  }

  if (selection.kind === "work-item") {
    return resolveWorkItemSelection(snapshot, selection);
  }

  return resolveWorkstationRequestSelection(
    snapshot,
    selection,
    workstationRequestsByDispatchID,
  );
}

function hasStatePlace(snapshot: DashboardSnapshot, placeId: string): boolean {
  for (const nodeId of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.kind === "work_state" && place.place_id === placeId) {
        return true;
      }
    }
  }

  return false;
}

function resolveWorkItemSelection(
  snapshot: DashboardSnapshot,
  selection: DashboardWorkItemSelection,
): DashboardSelection | null {
  const currentExecution =
    selection.dispatchId === undefined
      ? undefined
      : snapshot.runtime.active_executions_by_dispatch_id?.[selection.dispatchId];
  const currentWorkItem =
    currentExecution?.work_items?.find(
      (workItem) => workItem.work_id === selection.workItem.work_id,
    ) ?? findWorkItemReference(snapshot, selection.workItem.work_id);
  if (!currentWorkItem) {
    return snapshot.topology.workstation_nodes_by_id[selection.nodeId]
      ? { kind: "node", nodeId: selection.nodeId }
      : selectDefaultSelection(snapshot);
  }

  return {
    dispatchId: currentExecution?.dispatch_id ?? selection.dispatchId,
    execution: currentExecution,
    kind: "work-item",
    nodeId: selection.nodeId,
    workItem: currentWorkItem,
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
    .flatMap((request) => [
      ...(request.request.input_work_items ?? []),
      ...(request.response?.output_work_items ?? []),
    ])
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
