import {
  buildFactoryGraphBulkSelectionSummary,
  factoryGraphEditorSelectionSignature,
  resolveGraphSelectionDashboardSyncAction,
  type GraphSelectionDashboardSyncAction,
} from "../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import {
  type FactoryGraphEditorSelectionState,
  type FactoryGraphEditorSelectionTarget,
  resolveFactoryGraphEditorPrimaryTarget,
} from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection";
import {
  type FactoryGraphEditorSelectionBridgeSnapshot,
  useFactoryGraphEditorSelectionBridge,
} from "../state/factory-graph-editor-selection-bridge";
import type { CurrentActivityGraphSelectionDashboardSyncHandlers } from "./use-current-activity-graph-selection-dashboard-sync";

function dispatchGraphSelectionDashboardSyncAction(
  action: GraphSelectionDashboardSyncAction,
  handlers: CurrentActivityGraphSelectionDashboardSyncHandlers,
): void {
  switch (action.kind) {
    case "doc":
      handlers.onSelectDoc(action.targetPath);
      return;
    case "resource":
      handlers.onSelectResource(action.resourceName);
      return;
    case "state-node":
      handlers.onSelectStateNode(action.placeId);
      return;
    case "worker":
      handlers.onSelectWorker(action.workerName);
      return;
    case "work-type":
      handlers.onSelectWorkType(action.workTypeName);
      return;
    case "workstation":
      handlers.onSelectWorkstation(action.nodeId);
      return;
    default:
      return;
  }
}

function resolveDashboardSyncActionForPrimaryTarget(
  primaryTarget: FactoryGraphEditorSelectionTarget | null,
): GraphSelectionDashboardSyncAction | null {
  if (primaryTarget?.kind !== "node") {
    return null;
  }

  return resolveGraphSelectionDashboardSyncAction(primaryTarget.id);
}

function bridgeSnapshotsEqual(
  left: FactoryGraphEditorSelectionBridgeSnapshot | null,
  right: FactoryGraphEditorSelectionBridgeSnapshot,
): boolean {
  if (!left) {
    return false;
  }

  return (
    factoryGraphEditorSelectionSignature({
      primaryTarget: left.primaryTarget,
      selectedEdgeIds: new Set(left.selectedEdgeIds),
      selectedNodeIds: new Set(left.selectedNodeIds),
    }) ===
    factoryGraphEditorSelectionSignature({
      primaryTarget: right.primaryTarget,
      selectedEdgeIds: new Set(right.selectedEdgeIds),
      selectedNodeIds: new Set(right.selectedNodeIds),
    })
  );
}

export function publishCurrentActivityGraphSelectionBridgeState(
  state: FactoryGraphEditorSelectionState,
): void {
  const primaryTarget = resolveFactoryGraphEditorPrimaryTarget(state);
  const bulkSelectionSummary = buildFactoryGraphBulkSelectionSummary(state);
  const nextBridgeSelection: FactoryGraphEditorSelectionBridgeSnapshot = {
    bulkSelectionSummary,
    primaryTarget,
    selectedEdgeIds: [...state.selectedEdgeIds],
    selectedNodeIds: [...state.selectedNodeIds],
  };

  if (
    bridgeSnapshotsEqual(
      useFactoryGraphEditorSelectionBridge.getState().selection,
      nextBridgeSelection,
    )
  ) {
    return;
  }

  useFactoryGraphEditorSelectionBridge
    .getState()
    .setSelection(nextBridgeSelection);
}

export function syncCurrentActivityGraphSelectionToDashboard(
  state: FactoryGraphEditorSelectionState,
  handlers: CurrentActivityGraphSelectionDashboardSyncHandlers,
  lastSyncedSignatureRef: { current: string | null },
): void {
  const totalCount = state.selectedNodeIds.size + state.selectedEdgeIds.size;
  const selectionSignature = factoryGraphEditorSelectionSignature(state);

  if (totalCount !== 1) {
    lastSyncedSignatureRef.current = null;
    return;
  }

  if (lastSyncedSignatureRef.current === selectionSignature) {
    return;
  }

  const syncAction = resolveDashboardSyncActionForPrimaryTarget(
    resolveFactoryGraphEditorPrimaryTarget(state),
  );
  if (!syncAction) {
    return;
  }

  dispatchGraphSelectionDashboardSyncAction(syncAction, handlers);
  lastSyncedSignatureRef.current = selectionSignature;
}

export function createCurrentActivityGraphSelectionBridgePublisher(
  handlers: CurrentActivityGraphSelectionDashboardSyncHandlers,
) {
  const lastSyncedSignatureRef = { current: null as string | null };

  return (state: FactoryGraphEditorSelectionState) => {
    publishCurrentActivityGraphSelectionBridgeState(state);
    queueMicrotask(() => {
      syncCurrentActivityGraphSelectionToDashboard(
        state,
        handlers,
        lastSyncedSignatureRef,
      );
    });
  };
}
