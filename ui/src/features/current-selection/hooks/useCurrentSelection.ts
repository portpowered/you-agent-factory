import { useEffect, useRef } from "react";

import type {
  DashboardActiveExecution,
  DashboardPlaceRef,
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../api/dashboard/types";
import { useSelectionHistoryStore } from "../state/selectionHistoryStore";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../../terminal-work/public";
import type {
  DashboardSelection,
  StatePositionWorkItem,
  TerminalWorkDetail,
} from "../state/selection-types";
import { useCurrentSelectionActions } from "./useCurrentSelection.actions";
import type { WorkSelectionHint } from "./useCurrentSelection.actions";
import {
  useCurrentSelectionDerivedState,
  useSelectionSynchronization,
  useTerminalWorkDetailCleanup,
} from "./useCurrentSelection.derived";
import { resolveProjectedWorkstationRequestsByDispatchID, type WorkstationRequestLike } from "./useCurrentSelection.helpers";

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
  selectedNodeWorkstationRequests: DashboardWorkstationRequest[];
  selectedStateCurrentWorkItems: StatePositionWorkItem[];
  selectedStatePlace: DashboardPlaceRef | null;
  selectedStateTerminalHistoryWorkItems: StatePositionWorkItem[];
  selectedStateTokenCount: number;
  selectedWorkDispatchAttempts: DashboardProviderSessionAttempt[];
  selectedWorkID: string | null;
  selectedWorkProviderSessions: DashboardProviderSessionAttempt[];
  selectedWorkRequestHistory: WorkstationRequestLike[];
  selectedWorkWorkstationRequests: DashboardWorkstationRequest[];
  selectedWorkstationRequest: DashboardWorkstationRequest | null;
  selection: DashboardSelection | null;
  selectStateNode: (placeId: string) => void;
  selectStateWorkItem: (place: DashboardPlaceRef, workItem: StatePositionWorkItem) => void;
  selectWorkByID: (workID: string, hint?: WorkSelectionHint) => void;
  selectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: StatePositionWorkItem,
  ) => void;
  selectWorkstation: (nodeId: string) => void;
  selectWorkstationRequest: (request: DashboardWorkstationRequest) => void;
  terminalWorkDetail: TerminalWorkDetail | null;
  undoSelection: () => void;
}

function useCurrentSelectionStoreState() {
  return {
    canRedoSelection: useSelectionHistoryStore((state) => state.future.length > 0),
    canUndoSelection: useSelectionHistoryStore((state) => state.past.length > 0),
    commitSelectionState: useSelectionHistoryStore((state) => state.commitSelectionState),
    redoSelection: useSelectionHistoryStore((state) => state.redo),
    replacePresent: useSelectionHistoryStore((state) => state.replacePresent),
    resetSelectionHistory: useSelectionHistoryStore((state) => state.clear),
    selection: useSelectionHistoryStore((state) => state.present.selection),
    terminalWorkDetail: useSelectionHistoryStore((state) => state.present.terminalWorkDetail),
    undoSelection: useSelectionHistoryStore((state) => state.undo),
  };
}

export function useCurrentSelection({
  sessionID,
  snapshot,
  workstationRequestsByDispatchID,
}: {
  sessionID: string;
  snapshot: DashboardSnapshot | null | undefined;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}): CurrentSelectionState {
  const store = useCurrentSelectionStoreState();
  const previousSessionIDRef = useRef(sessionID);
  const sessionChanged = previousSessionIDRef.current !== sessionID;
  const selection = sessionChanged ? null : store.selection;
  const terminalWorkDetail = sessionChanged ? null : store.terminalWorkDetail;
  const projectedWorkstationRequestsByDispatchID = resolveProjectedWorkstationRequestsByDispatchID(
    snapshot,
    workstationRequestsByDispatchID,
  );

  useEffect(() => {
    if (sessionChanged) {
      store.resetSelectionHistory();
      previousSessionIDRef.current = sessionID;
    }
  }, [sessionChanged, sessionID, store.resetSelectionHistory]);

  useSelectionSynchronization({
    projectedWorkstationRequestsByDispatchID,
    replacePresent: store.replacePresent,
    resetSelectionHistory: store.resetSelectionHistory,
    selection,
    snapshot,
    terminalWorkDetail,
  });

  const derived = useCurrentSelectionDerivedState({
    projectedWorkstationRequestsByDispatchID,
    selection,
    snapshot,
    terminalWorkDetail,
  });

  useTerminalWorkDetailCleanup({
    completedWorkLabels: derived.completedWorkLabels,
    failedWorkLabels: derived.failedWorkLabels,
    replacePresent: store.replacePresent,
    selection,
    terminalWorkDetail,
  });

  const actions = useCurrentSelectionActions({
    commitSelectionState: store.commitSelectionState,
    completedWorkItems: derived.completedWorkItems,
    failedWorkItems: derived.failedWorkItems,
    projectedWorkstationRequestsByDispatchID,
    selection,
    snapshot,
    terminalWorkDetail,
  });

  return {
    canRedoSelection: store.canRedoSelection,
    canUndoSelection: store.canUndoSelection,
    completedWorkItems: derived.completedWorkItems,
    failedWorkItems: derived.failedWorkItems,
    openTerminalWorkDetail: actions.openTerminalWorkDetail,
    redoSelection: store.redoSelection,
    selectedNode: derived.selectedNode,
    selectedNodeActiveExecutions: derived.selectedNodeActiveExecutions,
    selectedNodeProviderSessions: derived.selectedNodeProviderSessions,
    selectedNodeWorkstationRequests: derived.selectedNodeWorkstationRequests,
    selectedStateCurrentWorkItems: derived.selectedStateCurrentWorkItems,
    selectedStatePlace: derived.selectedStatePlace,
    selectedStateTerminalHistoryWorkItems: derived.selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount: derived.selectedStateTokenCount,
    selectedWorkDispatchAttempts: derived.selectedWorkDispatchAttempts,
    selectedWorkID: derived.selectedWorkID,
    selectedWorkProviderSessions: derived.selectedWorkProviderSessions,
    selectedWorkRequestHistory: derived.selectedWorkRequestHistory,
    selectedWorkWorkstationRequests: derived.selectedWorkWorkstationRequests,
    selectedWorkstationRequest: derived.selectedWorkstationRequest,
    selection,
    selectStateNode: actions.selectStateNode,
    selectStateWorkItem: actions.selectStateWorkItem,
    selectWorkByID: actions.selectWorkByID,
    selectWorkItem: actions.selectWorkItem,
    selectWorkstation: actions.selectWorkstation,
    selectWorkstationRequest: actions.selectWorkstationRequest,
    terminalWorkDetail,
    undoSelection: store.undoSelection,
  };
}
