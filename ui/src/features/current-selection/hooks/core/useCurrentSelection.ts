import { useEffect, useRef } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import type {
  DashboardActiveExecution,
  DashboardPlaceRef,
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import type {
  FactoryResource,
  FactoryWorker,
  FactoryWorkType,
} from "../../../../api/events/types";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
} from "../../../terminal-work/lib/types";
import type { FactoryBundledDocFile } from "../../../workflow-activity/lib/factory-bundled-docs";
import { useGraphEditorPendingFactoryBridge } from "../../../workflow-activity/state/graph-editor-pending-factory-bridge";
import type {
  DashboardSelection,
  StatePositionWorkItem,
  TerminalWorkDetail,
} from "../../state/selection-types";
import { useSelectionHistoryStore } from "../../state/selectionHistoryStore";
import type { SelectedWorkOperationHistoryItem } from "../helpers/selected-work-operation-history";
import { useSelectionSynchronization } from "../useCurrentSelection.synchronization";
import type { WorkSelectionHint } from "./useCurrentSelection.actions";
import { useCurrentSelectionActions } from "./useCurrentSelection.actions";
import {
  useCurrentSelectionDerivedState,
  useTerminalWorkDetailCleanup,
} from "./useCurrentSelection.derived";
import {
  resolveProjectedWorkstationRequestsByDispatchID,
  type WorkstationRequestLike,
} from "./useCurrentSelection.helpers";

export interface CurrentSelectionState {
  canRedoSelection: boolean;
  canUndoSelection: boolean;
  canceledWorkItems?: TerminalWorkItem[];
  completedWorkItems: TerminalWorkItem[];
  currentFactoryDefinition?: CurrentFactoryDocument | null;
  failedWorkItems: TerminalWorkItem[];
  terminatedWorkItems?: TerminalWorkItem[];
  unknownWorkItems?: TerminalWorkItem[];
  openTerminalWorkDetail: (
    status: TerminalWorkStatus,
    item: TerminalWorkItem,
  ) => void;
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
  selectedWorkOperationHistory: SelectedWorkOperationHistoryItem[];
  selectedWorkProviderSessions: DashboardProviderSessionAttempt[];
  selectedWorkRequestHistory: WorkstationRequestLike[];
  selectedWorkWorkstationRequests: DashboardWorkstationRequest[];
  selectedResource?: FactoryResource | null;
  selectedResourceName: string | null;
  selectedResourceTokenCount: number | null;
  selectedResourceWorkerNames?: string[];
  selectedResourceWorkstationNames?: string[];
  selectedWorker: FactoryWorker | null;
  selectedWorkerName: string | null;
  selectedWorkerWorkstationNames: string[];
  selectedWorkType: FactoryWorkType | null;
  selectedWorkTypeName: string | null;
  selectedWorkstationRequest: DashboardWorkstationRequest | null;
  selection: DashboardSelection | null;
  selectStateNode: (placeId: string) => void;
  selectStateWorkItem: (
    place: DashboardPlaceRef,
    workItem: StatePositionWorkItem,
  ) => void;
  selectWorkByID: (workID: string, hint?: WorkSelectionHint) => void;
  selectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: StatePositionWorkItem,
  ) => void;
  selectWorkstation: (nodeId: string) => void;
  selectWorkstationRequest: (request: DashboardWorkstationRequest) => void;
  selectDoc: (targetPath: string) => void;
  selectResource: (resourceName: string) => void;
  selectWorker: (workerName: string) => void;
  selectedDocBundledFile?: FactoryBundledDocFile | null;
  selectedDocTargetPath: string | null;
  clearSelectedDocIfMatching: (targetPath: string) => void;
  clearSelectedFactoryGraphNodeIfMatching: (nodeId: string) => void;
  clearSelectedStateNodeIfMatching: (placeId: string) => void;
  clearSelectedWorkerIfMatching: (workerName: string) => void;
  selectWorkType: (workTypeName: string) => void;
  terminalWorkDetail: TerminalWorkDetail | null;
  undoSelection: () => void;
}

function useCurrentSelectionStoreState() {
  return {
    canRedoSelection: useSelectionHistoryStore(
      (state) => state.future.length > 0,
    ),
    canUndoSelection: useSelectionHistoryStore(
      (state) => state.past.length > 0,
    ),
    commitSelectionState: useSelectionHistoryStore(
      (state) => state.commitSelectionState,
    ),
    redoSelection: useSelectionHistoryStore((state) => state.redo),
    reconcilePresent: useSelectionHistoryStore(
      (state) => state.reconcilePresent,
    ),
    replacePresent: useSelectionHistoryStore((state) => state.replacePresent),
    resetSelectionHistory: useSelectionHistoryStore((state) => state.clear),
    selection: useSelectionHistoryStore((state) => state.present.selection),
    terminalWorkDetail: useSelectionHistoryStore(
      (state) => state.present.terminalWorkDetail,
    ),
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
  const projectedWorkstationRequestsByDispatchID =
    resolveProjectedWorkstationRequestsByDispatchID(
      snapshot,
      workstationRequestsByDispatchID,
    );
  const pendingFactoryDefinition = useGraphEditorPendingFactoryBridge(
    (state) => state.pendingFactoryDefinition,
  );

  useEffect(() => {
    if (sessionChanged) {
      store.resetSelectionHistory();
      previousSessionIDRef.current = sessionID;
    }
  }, [sessionChanged, sessionID, store.resetSelectionHistory]);

  useSelectionSynchronization({
    pendingFactoryDefinition: pendingFactoryDefinition ?? undefined,
    projectedWorkstationRequestsByDispatchID,
    reconcilePresent: store.reconcilePresent,
    resetSelectionHistory: store.resetSelectionHistory,
    snapshot,
  });

  const derived = useCurrentSelectionDerivedState({
    projectedWorkstationRequestsByDispatchID,
    selection,
    snapshot,
    terminalWorkDetail,
  });

  useTerminalWorkDetailCleanup({
    replacePresent: store.replacePresent,
    selection,
    terminalWorkItems: terminalWorkItemsForCleanup(derived),
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
    canceledWorkItems: derived.canceledWorkItems,
    completedWorkItems: derived.completedWorkItems,
    currentFactoryDefinition: derived.currentFactoryDefinition,
    failedWorkItems: derived.failedWorkItems,
    terminatedWorkItems: derived.terminatedWorkItems,
    unknownWorkItems: derived.unknownWorkItems,
    openTerminalWorkDetail: actions.openTerminalWorkDetail,
    redoSelection: store.redoSelection,
    selectedNode: derived.selectedNode,
    selectedNodeActiveExecutions: derived.selectedNodeActiveExecutions,
    selectedNodeProviderSessions: derived.selectedNodeProviderSessions,
    selectedNodeWorkstationRequests: derived.selectedNodeWorkstationRequests,
    selectedStateCurrentWorkItems: derived.selectedStateCurrentWorkItems,
    selectedStatePlace: derived.selectedStatePlace,
    selectedStateTerminalHistoryWorkItems:
      derived.selectedStateTerminalHistoryWorkItems,
    selectedStateTokenCount: derived.selectedStateTokenCount,
    selectedWorkDispatchAttempts: derived.selectedWorkDispatchAttempts,
    selectedWorkID: derived.selectedWorkID,
    selectedWorkOperationHistory: derived.selectedWorkOperationHistory,
    selectedWorkProviderSessions: derived.selectedWorkProviderSessions,
    selectedWorkRequestHistory: derived.selectedWorkRequestHistory,
    selectedWorkWorkstationRequests: derived.selectedWorkWorkstationRequests,
    selectedDocBundledFile: derived.selectedDocBundledFile,
    selectedDocTargetPath: derived.selectedDocTargetPath,
    selectedResource: derived.selectedResource,
    selectedResourceName: derived.selectedResourceName,
    selectedResourceTokenCount: derived.selectedResourceTokenCount,
    selectedResourceWorkerNames: derived.selectedResourceWorkerNames,
    selectedResourceWorkstationNames: derived.selectedResourceWorkstationNames,
    selectedWorker: derived.selectedWorker,
    selectedWorkerName: derived.selectedWorkerName,
    selectedWorkerWorkstationNames: derived.selectedWorkerWorkstationNames,
    selectedWorkType: derived.selectedWorkType,
    selectedWorkTypeName: derived.selectedWorkTypeName,
    selectedWorkstationRequest: derived.selectedWorkstationRequest,
    selection,
    selectStateNode: actions.selectStateNode,
    selectStateWorkItem: actions.selectStateWorkItem,
    selectWorkByID: actions.selectWorkByID,
    selectWorkItem: actions.selectWorkItem,
    selectWorkstation: actions.selectWorkstation,
    selectWorkstationRequest: actions.selectWorkstationRequest,
    clearSelectedDocIfMatching: actions.clearSelectedDocIfMatching,
    clearSelectedFactoryGraphNodeIfMatching:
      actions.clearSelectedFactoryGraphNodeIfMatching,
    clearSelectedStateNodeIfMatching: actions.clearSelectedStateNodeIfMatching,
    clearSelectedWorkerIfMatching: actions.clearSelectedWorkerIfMatching,
    selectDoc: actions.selectDoc,
    selectResource: actions.selectResource,
    selectWorker: actions.selectWorker,
    selectWorkType: actions.selectWorkType,
    terminalWorkDetail,
    undoSelection: store.undoSelection,
  };
}

function terminalWorkItemsForCleanup(
  derived: ReturnType<typeof useCurrentSelectionDerivedState>,
): TerminalWorkItem[] {
  return [
    ...(derived.canceledWorkItems ?? []),
    ...derived.completedWorkItems,
    ...derived.failedWorkItems,
    ...(derived.terminatedWorkItems ?? []),
    ...(derived.unknownWorkItems ?? []),
  ];
}
