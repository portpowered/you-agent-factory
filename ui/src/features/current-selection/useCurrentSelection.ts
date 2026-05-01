import {
  useEffect,
  useMemo,
} from "react";

import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardPlaceRef,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
  DashboardWorkstationNode,
} from "../../api/dashboard/types";
import {
  findWorkItemReference,
  findWorkstationNodeIDForPlace,
  resolveDashboardSelection,
} from "../../state/dashboardSelection";
import { useSelectionHistoryStore } from "../../state/selectionHistoryStore";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../terminal-work/terminal-work-card";
import type {
  DashboardSelection,
  TerminalWorkDetail,
} from "./types";

export interface CurrentSelectionState {
  canRedoSelection: boolean;
  canUndoSelection: boolean;
  completedWorkItems: TerminalWorkItem[];
  failedWorkItems: TerminalWorkItem[];
  openTerminalWorkDetail: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  redoSelection: () => void;
  selectedNode: DashboardWorkstationNode | null;
  selectedNodeActiveExecutions: DashboardActiveExecution[];
  selectedNodeProviderSessions: DashboardProviderSessionAttempt[];
  selectedStateCurrentWorkItems: DashboardWorkItemRef[];
  selectedStatePlace: DashboardPlaceRef | null;
  selectedStateTerminalHistoryWorkItems: DashboardWorkItemRef[];
  selectedStateTokenCount: number;
  selectedWorkDispatchAttempts: DashboardProviderSessionAttempt[];
  selectedWorkID: string | null;
  selectedWorkProviderSessions: DashboardProviderSessionAttempt[];
  selectedWorkRequestHistory: WorkstationRequestLike[];
  selectedWorkWorkstationRequests: DashboardWorkstationRequest[];
  selectedWorkstationRequest: DashboardWorkstationRequest | null;
  selectedNodeWorkstationRequests: DashboardWorkstationRequest[];
  selection: DashboardSelection | null;
  selectWorkByID: (workID: string) => void;
  selectStateNode: (placeId: string) => void;
  selectStateWorkItem: (place: DashboardPlaceRef, workItem: DashboardWorkItemRef) => void;
  selectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  selectWorkstation: (nodeId: string) => void;
  selectWorkstationRequest: (request: DashboardWorkstationRequest) => void;
  terminalWorkDetail: TerminalWorkDetail | null;
  undoSelection: () => void;
}

export type UseCurrentSelectionResult = CurrentSelectionState;

type DispatchWorkstationRequest =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

export function useCurrentSelection({
  snapshot,
  workstationRequestsByDispatchID,
}: {
  snapshot: DashboardSnapshot | null | undefined;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): CurrentSelectionState {
  const canRedoSelection = useSelectionHistoryStore((state) => state.future.length > 0);
  const canUndoSelection = useSelectionHistoryStore((state) => state.past.length > 0);
  const commitSelectionState = useSelectionHistoryStore((state) => state.commitSelectionState);
  const replacePresent = useSelectionHistoryStore((state) => state.replacePresent);
  const resetSelectionHistory = useSelectionHistoryStore((state) => state.clear);
  const redoSelection = useSelectionHistoryStore((state) => state.redo);
  const selection = useSelectionHistoryStore((state) => state.present.selection);
  const terminalWorkDetail = useSelectionHistoryStore(
    (state) => state.present.terminalWorkDetail,
  );
  const undoSelection = useSelectionHistoryStore((state) => state.undo);
  const projectedWorkstationRequestsByDispatchID = resolveProjectedWorkstationRequestsByDispatchID(
    snapshot,
    workstationRequestsByDispatchID,
  );

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

  const selectedNode =
    selection?.kind === "node" && snapshot
      ? snapshot.topology.workstation_nodes_by_id[selection.nodeId]
      : selection?.kind === "workstation-request" && snapshot
        ? snapshot.topology.workstation_nodes_by_id[selection.nodeId]
      : selection?.kind === "work-item" && snapshot
        ? snapshot.topology.workstation_nodes_by_id[selection.nodeId]
        : null;
  const selectedWorkstationRequest =
    selection?.kind === "workstation-request" ? selection.request : null;
  const selectedStatePlace =
    selection?.kind === "state-node" && snapshot
      ? findStatePlace(snapshot, selection.placeId)
      : null;
  const selectedStateCurrentWorkItems = useMemo(
    () => currentWorkItemsForPlace(snapshot, selectedStatePlace?.place_id),
    [snapshot, selectedStatePlace?.place_id],
  );
  const selectedStateTerminalHistoryWorkItems = useMemo(
    () => terminalHistoryItemsForPlace(snapshot, selectedStatePlace?.place_id),
    [snapshot, selectedStatePlace?.place_id],
  );
  const selectedStateTokenCount =
    selectedStatePlace && snapshot
      ? snapshot.runtime.place_token_counts?.[selectedStatePlace.place_id] ?? 0
      : 0;
  const selectedWorkRequestHistory = useMemo(() => {
    if (selection?.kind !== "work-item") {
      return [];
    }

    return selectWorkstationRequestsForWork(
      projectedWorkstationRequestsByDispatchID as
        | Record<string, WorkstationRequestLike>
        | undefined,
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
          attempt.work_items?.some(
            (workItem) => workItem.work_id === selection.workItem.work_id,
          ) ?? false,
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
  const completedWorkLabels = snapshot?.runtime.session.completed_work_labels ?? [];
  const failedWorkLabels = snapshot?.runtime.session.failed_work_labels ?? [];
  const selectedWorkID =
    selection?.kind === "work-item"
      ? selection.workItem.work_id
      : selection?.kind === "workstation-request"
        ? selection.request.work_items[0]?.work_id ?? null
      : terminalWorkDetail?.traceWorkID ?? null;

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
        kind: "work-item",
        dispatchId,
        nodeId,
        execution,
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

  const selectStateWorkItem = (
    place: DashboardPlaceRef,
    workItem: DashboardWorkItemRef,
  ) => {
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
          snapshot?.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]
            ?.failure_message,
        failureReason:
          snapshot?.runtime.session.failed_work_details_by_work_id?.[workItem.work_id]
            ?.failure_reason,
        label: resolveWorkItemDisplayLabel(workItem),
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

  const openTerminalWorkDetail = (status: TerminalWorkStatus, item: TerminalWorkItem) => {
    const resolvedSelection = resolveTrackedWorkSelection({
      snapshot,
      terminalWorkDetail: {
        attempts: item.attempts,
        failureMessage: item.failureMessage,
        failureReason: item.failureReason,
        label: item.label,
        status,
        traceWorkID: item.traceWorkID,
        workItem: item.workItem,
      },
      workID: item.traceWorkID,
      workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
    });
    commitSelectionState({
      selection: resolvedSelection ?? selection,
      terminalWorkDetail: {
        attempts: item.attempts,
        failureMessage: item.failureMessage,
        failureReason: item.failureReason,
        label: item.label,
        status,
        traceWorkID: item.traceWorkID,
        workItem: item.workItem,
      },
    });
  };

  return {
    canRedoSelection,
    canUndoSelection,
    completedWorkItems,
    failedWorkItems,
    openTerminalWorkDetail,
    redoSelection,
    selectedNode,
    selectedNodeActiveExecutions,
    selectedNodeProviderSessions,
    selectedStateCurrentWorkItems,
    selectedStatePlace,
    selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount,
    selectedWorkDispatchAttempts,
    selectedWorkID,
    selectedWorkProviderSessions,
    selectedWorkRequestHistory,
    selectedWorkWorkstationRequests,
    selectedWorkstationRequest,
    selectedNodeWorkstationRequests,
    selection,
    selectWorkByID,
    selectStateNode,
    selectStateWorkItem,
    selectWorkItem,
    selectWorkstation,
    selectWorkstationRequest,
    terminalWorkDetail,
    undoSelection,
  };
}

function resolveProjectedWorkstationRequestsByDispatchID(
  snapshot: DashboardSnapshot | null | undefined,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined,
): Record<string, DashboardWorkstationRequest> | undefined {
  if (workstationRequestsByDispatchID && Object.keys(workstationRequestsByDispatchID).length > 0) {
    return workstationRequestsByDispatchID;
  }

  if (!snapshot?.runtime.workstation_requests_by_dispatch_id) {
    return undefined;
  }

  return Object.fromEntries(
    Object.entries(snapshot.runtime.workstation_requests_by_dispatch_id).map(
      ([dispatchID, request]) => [dispatchID, toDashboardWorkstationRequest(request)],
    ),
  );
}

function filterProviderSessionAttempts(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  predicate: (attempt: DashboardProviderSessionAttempt) => boolean,
): DashboardProviderSessionAttempt[] {
  if (!attempts) {
    return [];
  }

  return attempts.filter(
    (attempt: DashboardProviderSessionAttempt) =>
      attempt.provider_session?.id !== undefined && predicate(attempt),
  );
}

function filterDispatchAttempts(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  predicate: (attempt: DashboardProviderSessionAttempt) => boolean,
): DashboardProviderSessionAttempt[] {
  if (!attempts) {
    return [];
  }

  return attempts.filter(predicate);
}

function buildSelectedWorkDispatchAttempts({
  attempts,
  workID,
  workstationRequestsByDispatchID,
}: {
  attempts: DashboardProviderSessionAttempt[] | undefined;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>;
}): DashboardProviderSessionAttempt[] {
  const matchingAttempts = filterDispatchAttempts(
    attempts,
    (attempt) =>
      attempt.work_items?.some((workItem) => workItem.work_id === workID) ?? false,
  );
  const requests = sortDispatchRequests(
    Object.values(workstationRequestsByDispatchID ?? {}).filter((request) =>
      requestWorkItems(request).some((workItem) => workItem.work_id === workID),
    ),
  );

  if (requests.length === 0) {
    return matchingAttempts;
  }

  const attemptsByDispatchID = new Map<string, DashboardProviderSessionAttempt>();
  for (const attempt of matchingAttempts) {
    attemptsByDispatchID.set(attempt.dispatch_id, attempt);
  }

  for (const request of requests) {
    const dispatchAttempt = dispatchAttemptFromRequest(request);
    const existingAttempt = attemptsByDispatchID.get(dispatchAttempt.dispatch_id);

    attemptsByDispatchID.set(
      dispatchAttempt.dispatch_id,
      existingAttempt
        ? mergeDispatchAttempts(existingAttempt, dispatchAttempt)
        : dispatchAttempt,
    );
  }

  const orderedDispatchIDs = [
    ...requests.map((request) => request.dispatch_id),
    ...matchingAttempts.map((attempt) => attempt.dispatch_id),
  ];

  return [...new Set(orderedDispatchIDs)]
    .map((dispatchID) => attemptsByDispatchID.get(dispatchID))
    .filter((attempt): attempt is DashboardProviderSessionAttempt => attempt !== undefined);
}

function sortDispatchRequests(
  requests: DispatchWorkstationRequest[],
): DispatchWorkstationRequest[] {
  return [...requests].sort((left, right) => {
    if (requestStartedAt(left) !== requestStartedAt(right)) {
      return requestStartedAt(right).localeCompare(requestStartedAt(left));
    }

    return left.dispatch_id.localeCompare(right.dispatch_id);
  });
}

function dispatchAttemptFromRequest(
  request: DispatchWorkstationRequest,
): DashboardProviderSessionAttempt {
  return {
    diagnostics: requestDiagnostics(request),
    dispatch_id: request.dispatch_id,
    failure_message: requestFailureMessage(request),
    failure_reason: requestFailureReason(request),
    outcome: requestOutcome(request),
    provider_session: requestProviderSession(request),
    transition_id: request.transition_id,
    workstation_name: request.workstation_name,
    work_items: requestWorkItems(request),
  };
}

function mergeDispatchAttempts(
  existingAttempt: DashboardProviderSessionAttempt,
  derivedAttempt: DashboardProviderSessionAttempt,
): DashboardProviderSessionAttempt {
  return {
    ...derivedAttempt,
    ...existingAttempt,
    diagnostics: existingAttempt.diagnostics ?? derivedAttempt.diagnostics,
    failure_message: existingAttempt.failure_message ?? derivedAttempt.failure_message,
    failure_reason: existingAttempt.failure_reason ?? derivedAttempt.failure_reason,
    outcome: existingAttempt.outcome || derivedAttempt.outcome,
    provider_session: existingAttempt.provider_session ?? derivedAttempt.provider_session,
    workstation_name: existingAttempt.workstation_name ?? derivedAttempt.workstation_name,
    work_items: existingAttempt.work_items ?? derivedAttempt.work_items,
  };
}

function buildTerminalWorkItems(
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

function collectStatePlacesById(snapshot: DashboardSnapshot): Map<string, DashboardPlaceRef> {
  const placesById = new Map<string, DashboardPlaceRef>();

  for (const nodeId of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.kind === "work_state") {
        placesById.set(place.place_id, place);
      }
    }
  }

  return placesById;
}

function findStatePlace(
  snapshot: DashboardSnapshot,
  placeId: string,
): DashboardPlaceRef | null {
  return collectStatePlacesById(snapshot).get(placeId) ?? null;
}

function currentWorkItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
): DashboardWorkItemRef[] {
  if (!snapshot || !placeId) {
    return [];
  }

  return snapshot.runtime.current_work_items_by_place_id?.[placeId] ?? [];
}

function terminalHistoryItemsForPlace(
  snapshot: DashboardSnapshot | null | undefined,
  placeId: string | undefined,
): DashboardWorkItemRef[] {
  if (!snapshot || !placeId) {
    return [];
  }

  return snapshot.runtime.place_occupancy_work_items_by_place_id?.[placeId] ?? [];
}

function activeExecutionMatchesWorkstation(
  execution: DashboardActiveExecution,
  workstation: DashboardWorkstationNode,
): boolean {
  return (
    execution.workstation_node_id === workstation.node_id ||
    execution.transition_id === workstation.transition_id ||
    execution.workstation_name === workstation.workstation_name
  );
}

function activeExecutionsForSelectedWorkstation(
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
    (execution) => activeExecutionMatchesWorkstation(execution, selectedNode),
  );
}

type WorkstationRequestLike =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

function isProjectedWorkstationRequest(
  request: WorkstationRequestLike,
): request is DashboardWorkstationRequest {
  return "workstation_node_id" in request;
}

export function sortWorkstationRequests<TRequest extends WorkstationRequestLike>(
  requests: TRequest[],
): TRequest[] {
  return [...requests].sort((left, right) => {
    const leftStartedAt = requestStartedAt(left);
    const rightStartedAt = requestStartedAt(right);
    if (leftStartedAt !== rightStartedAt) {
      return rightStartedAt.localeCompare(leftStartedAt);
    }
    return left.dispatch_id.localeCompare(right.dispatch_id);
  });
}

export function selectWorkstationRequestsForWork<TRequest extends WorkstationRequestLike>(
  workstationRequestsByDispatchID: Record<string, TRequest> | undefined,
  workID: string,
): TRequest[] {
  return sortWorkstationRequests(
    Object.values(workstationRequestsByDispatchID ?? {}).filter((request) =>
      requestReferencesWorkItem(request, workID),
    ),
  );
}

function resolveActiveWorkItemSelection(
  snapshot: DashboardSnapshot | null | undefined,
  workItem: DashboardWorkItemRef,
): DashboardSelection | null {
  if (!snapshot) {
    return null;
  }

  for (const execution of Object.values(snapshot.runtime.active_executions_by_dispatch_id ?? {})) {
    const matchedWorkItem = execution.work_items?.find(
      (candidate) => candidate.work_id === workItem.work_id,
    );

    if (!matchedWorkItem) {
      continue;
    }

    return {
      dispatchId: execution.dispatch_id,
      execution,
      kind: "work-item",
      nodeId: execution.workstation_node_id,
      workItem: matchedWorkItem,
    };
  }

  return null;
}

interface ResolveTrackedWorkSelectionInput {
  nodeID?: string;
  snapshot: DashboardSnapshot | null | undefined;
  terminalWorkDetail?: TerminalWorkDetail | null;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

function resolveTrackedWorkSelection({
  nodeID,
  snapshot,
  terminalWorkDetail,
  workID,
  workstationRequestsByDispatchID,
}: ResolveTrackedWorkSelectionInput): DashboardSelection | null {
  if (!snapshot) {
    return null;
  }

  const workstationRequest = findMatchingWorkstationRequest(
    workstationRequestsByDispatchID ?? snapshot.runtime.workstation_requests_by_dispatch_id,
    workID,
  );
  if (workstationRequest && isScriptBackedWorkstationRequest(workstationRequest)) {
    return workstationRequestSelection(workstationRequest);
  }

  const activeSelection = resolveActiveWorkItemSelection(snapshot, { work_id: workID });
  if (activeSelection) {
    return activeSelection;
  }

  const fallbackWorkItem =
    findWorkItemReference(snapshot, workID) ??
    terminalWorkDetail?.workItem ??
    snapshot.runtime.session.failed_work_details_by_work_id?.[workID]?.work_item;
  if (!fallbackWorkItem) {
    return null;
  }

  if (workstationRequest) {
    return workstationRequestSelection(workstationRequest);
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

function findTrackedWorkNodeID(
  snapshot: DashboardSnapshot,
  workID: string,
): string | undefined {
  for (const [placeID, workItems] of Object.entries(snapshot.runtime.current_work_items_by_place_id ?? {})) {
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

function placeNodeID(
  snapshot: DashboardSnapshot | null | undefined,
  place: DashboardPlaceRef,
): string | undefined {
  if (!snapshot) {
    return undefined;
  }

  return findWorkstationNodeIDForPlace(snapshot, place.place_id);
}

function requestWorkItems(
  request: DispatchWorkstationRequest,
): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.work_items
    : [
        ...requestInputWorkItems(request),
        ...requestOutputWorkItems(request),
      ];
}

function requestWorkstationNodeID(
  request: DispatchWorkstationRequest,
): string {
  return isProjectedWorkstationRequest(request) ? request.workstation_node_id : request.transition_id;
}

function requestStartedAt(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): string {
  return isProjectedWorkstationRequest(request) ? request.started_at ?? "" : request.request.started_at ?? "";
}

function requestReferencesWorkItem(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
  workID: string,
): boolean {
  return requestRelatedWorkItems(request).some((workItem) => workItem.work_id === workID);
}

function requestRelatedWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return dedupeWorkItems([
    ...requestWorkItems(request),
    ...requestInputWorkItems(request),
    ...requestOutputWorkItems(request),
  ]);
}

function dedupeWorkItems(workItems: DashboardWorkItemRef[]): DashboardWorkItemRef[] {
  const workItemsByID = new Map<string, DashboardWorkItemRef>();

  for (const workItem of workItems) {
    workItemsByID.set(workItem.work_id, workItem);
  }

  return [...workItemsByID.values()];
}

function requestInputWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.request_view?.input_work_items ?? []
    : request.request.input_work_items ?? [];
}

function requestOutputWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.output_work_items ?? []
    : request.response?.output_work_items ?? [];
}

function selectLatestProviderSessionAttemptsByDispatch(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  requests: WorkstationRequestLike[],
): DashboardProviderSessionAttempt[] {
  if (!attempts) {
    return [];
  }

  const latestAttemptsByDispatchID = new Map<string, DashboardProviderSessionAttempt>();
  for (const attempt of attempts) {
    if (!attempt.provider_session?.id) {
      continue;
    }

    latestAttemptsByDispatchID.set(attempt.dispatch_id, attempt);
  }

  return requests.flatMap((request) => {
    const matchingAttempt = latestAttemptsByDispatchID.get(request.dispatch_id);
    return matchingAttempt ? [matchingAttempt] : [];
  });
}

function findMatchingWorkstationRequest(
  requests:
    | Record<string, DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest>
    | undefined,
  workID: string,
): DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest | undefined {
  return Object.values(requests ?? {}).find((request) =>
    requestWorkItems(request).some((item) => item.work_id === workID),
  );
}

function workstationRequestSelection(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardSelection {
  return {
    dispatchId: request.dispatch_id,
    kind: "workstation-request",
    nodeId: requestWorkstationNodeID(request),
    request: toDashboardWorkstationRequest(request),
  };
}

function isScriptBackedWorkstationRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): boolean {
  if ("workstation_node_id" in request) {
    return request.script_request !== undefined || request.script_response !== undefined;
  }

  return (
    request.request.script_request !== undefined ||
    request.response?.script_response !== undefined
  );
}

function toDashboardWorkstationRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkstationRequest {
  if ("workstation_node_id" in request) {
    return request;
  }

  return {
    dispatch_id: request.dispatch_id,
    dispatched_request_count: request.counts.dispatched_count,
    errored_request_count: request.counts.errored_count,
    failure_message: request.response?.failure_message,
    failure_reason: request.response?.failure_reason,
    inference_attempts: [],
    model: request.request.model,
    outcome: request.response?.outcome,
    prompt: request.request.prompt,
    provider: request.request.provider,
    provider_session: request.response?.provider_session,
    request_metadata: request.request.request_metadata,
    responded_request_count: request.counts.responded_count,
    response: request.response?.response_text,
    response_metadata: request.response?.response_metadata,
    script_request: request.request.script_request,
    script_response: request.response?.script_response,
    started_at: request.request.started_at,
    total_duration_millis: request.response?.duration_millis,
    trace_ids: request.request.trace_ids,
    transition_id: request.transition_id,
    work_items: requestWorkItems(request),
    working_directory: request.request.working_directory,
    workstation_name: request.workstation_name,
    workstation_node_id: request.transition_id,
    worktree: request.request.worktree,
  };
}

function requestProviderSession(
  request: DispatchWorkstationRequest,
) {
  return "request" in request ? request.response?.provider_session : request.provider_session;
}

function requestOutcome(request: DispatchWorkstationRequest): string {
  return "request" in request ? request.response?.outcome ?? "PENDING" : request.outcome ?? "PENDING";
}

function requestFailureReason(request: DispatchWorkstationRequest): string | undefined {
  return "request" in request ? request.response?.failure_reason : request.failure_reason;
}

function requestFailureMessage(request: DispatchWorkstationRequest): string | undefined {
  return "request" in request ? request.response?.failure_message : request.failure_message;
}

function requestDiagnostics(
  request: DispatchWorkstationRequest,
) {
  return "request" in request ? request.response?.diagnostics : undefined;
}

function inferStateWorkTerminalStatus(
  snapshot: DashboardSnapshot | null | undefined,
  place: DashboardPlaceRef,
  workItem: DashboardWorkItemRef,
): TerminalWorkStatus | null {
  if (!snapshot) {
    return null;
  }

  const failedDetail = snapshot.runtime.session.failed_work_details_by_work_id?.[workItem.work_id];
  if (failedDetail) {
    return "failed";
  }

  const workLabels = new Set([
    workItem.work_id,
    resolveWorkItemDisplayLabel(workItem),
  ]);

  const failedWorkLabels = new Set(snapshot.runtime.session.failed_work_labels ?? []);
  for (const label of workLabels) {
    if (failedWorkLabels.has(label)) {
      return "failed";
    }
  }

  const completedWorkLabels = new Set(snapshot.runtime.session.completed_work_labels ?? []);
  for (const label of workLabels) {
    if (completedWorkLabels.has(label)) {
      return "completed";
    }
  }

  if (place.state_category === "FAILED") {
    return "failed";
  }

  if (place.state_category === "TERMINAL") {
    return "completed";
  }

  return null;
}

function findTerminalWorkItem(
  items: TerminalWorkItem[],
  workItem: DashboardWorkItemRef,
): TerminalWorkItem | undefined {
  const workLabel = resolveWorkItemDisplayLabel(workItem);

  return items.find((item) => {
    if (item.traceWorkID === workItem.work_id) {
      return true;
    }

    if (item.workItem?.work_id === workItem.work_id) {
      return true;
    }

    return item.label === workLabel;
  });
}

function resolveWorkItemDisplayLabel(workItem: DashboardWorkItemRef): string {
  const displayName = workItem.display_name?.trim();
  return displayName && displayName.length > 0 ? displayName : workItem.work_id;
}
