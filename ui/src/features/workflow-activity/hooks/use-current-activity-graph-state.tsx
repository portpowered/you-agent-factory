import { useEffect, useMemo, useRef, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { areFactorySessionIDsEquivalent } from "../../../api/session-routing";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { useFactoryGraphEdgeWaypointEditor } from "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook";
import { useFactoryGraphTopologyEditorBridge } from "../state/factory-graph-topology-editor-bridge";
import { useGraphEditorPendingFactoryBridge } from "../state/graph-editor-pending-factory-bridge";
import { useCurrentActivityFactoryDocumentState } from "./current-activity-factory-document-state";
import { buildCurrentActivityGraphStatusState } from "./current-activity-graph-editor-status-state";
import {
  buildCurrentActivityGraphStateValue,
  type CurrentActivityGraphStateValue,
} from "./current-activity-graph-state-value";
import { useDraftAppliedFactoryValidation } from "./react-flow-current-activity-card-editor-draft-validation";
import { useCurrentActivityEditableGraph } from "./react-flow-current-activity-card-editor-editable-graph";
import { useGraphEditorLeaveEditorBridge } from "./react-flow-current-activity-card-editor-save-flow";
import { useCurrentActivityGraphRenderState } from "./use-current-activity-graph-render-state";
import { useGraphEditorControllers } from "./use-graph-editor-controllers";
import { useGraphEditorSaveFlow } from "./use-graph-editor-save-flow";
import { useGraphEditorSession } from "./use-graph-editor-session";
import { useHiddenFactoryGraphNodeClasses } from "./use-hidden-factory-graph-node-classes";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph state wires session, controllers, and save flow into one card value model.
export function useCurrentActivityGraphState(
  snapshot: DashboardSnapshot,
  locale?: string | null,
  factoryDocumentScopeKey?: string | null,
  onDocAdded?: (targetPath: string) => void,
  onNodeRemovedFromDraft?: (nodeId: string) => void,
): CurrentActivityGraphStateValue {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const {
    hiddenNodeClasses,
    hideShowMenuOpen,
    preferencesDirty,
    resetPreferences,
    setHideShowMenuOpen,
    setVisibilityPreset,
    toggleHiddenNodeClass,
    visibilityPreset,
  } = useHiddenFactoryGraphNodeClasses(factoryDocumentScopeKey);
  const leaveEditorBridge = useGraphEditorLeaveEditorBridge();
  const factoryDocumentState = useCurrentActivityFactoryDocumentState({
    eventFactory: snapshot.factory,
  });
  const {
    documentDraft,
    editableDefinitionQuery,
    editableGraph,
    saveEditableDefinition,
  } = useCurrentActivityEditableGraph({
    editorMode,
    factoryDocumentState,
    factoryDocumentScopeKey,
    hasPreferenceChanges: preferencesDirty,
    locale,
    snapshot,
  });
  const draftState = editableGraph.draftState;
  const structuralValidation = useDraftAppliedFactoryValidation(
    draftState,
    documentDraft.layout,
    editorMode,
  );
  const graphRenderState = useCurrentActivityGraphRenderState({
    definitionStatus: editableDefinitionQuery.status,
    documentDraft,
    editorMode,
    factoryDocumentState,
    hiddenNodeClasses,
    isSaving: saveEditableDefinition.isPending,
    structuralValidation,
    snapshot,
    visibilityPreset,
  });
  const graphProjection = graphRenderState.flowState;
  const viewState = graphRenderState.viewState;
  const session = useGraphEditorSession({
    activeTool,
    editorMode,
    locale,
    ...leaveEditorBridge.sessionCallbacks,
    sessionState: graphRenderState.sessionState,
    setActiveTool,
    setEditorMode,
  });
  const controllers = useGraphEditorControllers({
    activeTool,
    canInteractWithEditor: session.canInteractWithEditor,
    currentFactoryDefinition: session.currentFactoryDefinition,
    draftState,
    editableGraph,
    hiddenNodeClasses,
    locale,
    onDocAdded,
    onNodeRemovedFromDraft,
    saveEditableDefinition,
    setActiveTool,
  });
  const { addEntityController } = controllers;
  const saveFlow = useGraphEditorSaveFlow({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    addEntityController,
    documentSave: editableGraph.saveState.documentSave,
    documentSaveControls: editableGraph.documentSaveControls,
    draftState,
    editableGraph,
    editorUnavailableClassifierWorkstationName:
      session.editorUnavailableClassifierWorkstationName,
    locale,
    saveEditableDefinition,
    setActiveTool,
    setEditorMode,
    transientControllerReset: {
      setBlockedRemovalReason: controllers.setBlockedRemovalReason,
      setConnectionNotice: controllers.setConnectionNotice,
      setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
      setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
    },
  });
  leaveEditorBridge.bindSaveFlow(saveFlow);

  const edgeWaypointNodes = useMemo(
    () =>
      graphProjection.positionedGraphLayout.nodes.map((node) => ({
        id: node.nodeId,
        position: { x: node.x, y: node.y },
      })),
    [graphProjection.positionedGraphLayout.nodes],
  );
  const edgeWaypointControls = useFactoryGraphEdgeWaypointEditor({
    activeTool,
    addEdgeWaypoint: editableGraph.actions.addEdgeWaypoint,
    canInteractWithEditor: session.canInteractWithEditor,
    editorMode,
    handleEditorEdgeDelete: controllers.handleEditorEdgeDelete,
    layout: graphRenderState.layoutState.currentLayout,
    locale,
    moveEdgeWaypoint: editableGraph.actions.moveEdgeWaypoint,
    nodes: edgeWaypointNodes,
    removeEdgeWaypoint: editableGraph.actions.removeEdgeWaypoint,
  });

  const { resetEditorChromeForScopeChange } = saveFlow;
  const lastFactoryDocumentScopeKeyRef = useRef<string | null>(null);
  const statusState = buildCurrentActivityGraphStatusState({
    definitionError: editableDefinitionQuery.error,
    definitionStatus: editableDefinitionQuery.status,
    dirtyStateSummary: documentDraft.dirtyState,
    draftHasChanges: documentDraft.hasTopologyChanges,
    draftSource: documentDraft.source,
    hasActiveWork: saveFlow.hasActiveWork,
    isSaving: saveEditableDefinition.isPending,
    isStaleDraft: saveFlow.isStaleDraft,
    layoutDirty: documentDraft.hasLayoutChanges,
    saveBlockedReason: saveFlow.saveBlockedReason,
    saveError: saveEditableDefinition.error ?? null,
  });

  useEffect(() => {
    useFactoryGraphTopologyEditorBridge
      .getState()
      .setGraphDraftHasPendingChanges(documentDraft.hasTopologyChanges);

    return () => {
      useFactoryGraphTopologyEditorBridge
        .getState()
        .setGraphDraftHasPendingChanges(false);
    };
  }, [documentDraft.hasTopologyChanges]);

  useEffect(() => {
    const pendingFactoryBridge = useGraphEditorPendingFactoryBridge.getState();
    pendingFactoryBridge.setPendingFactoryDefinition(
      editorMode ? (viewState.currentFactoryDefinition ?? null) : null,
    );

    return () => {
      pendingFactoryBridge.setPendingFactoryDefinition(null);
    };
  }, [editorMode, viewState.currentFactoryDefinition]);

  useEffect(() => {
    const normalizedScopeKey = factoryDocumentScopeKey ?? null;
    const previousScopeKey = lastFactoryDocumentScopeKeyRef.current;
    const scopeChanged =
      previousScopeKey !== null &&
      !areFactorySessionIDsEquivalent(previousScopeKey, normalizedScopeKey);
    lastFactoryDocumentScopeKeyRef.current = normalizedScopeKey;

    if (!scopeChanged) {
      return;
    }

    resetEditorChromeForScopeChange();
    const bridge = useFactoryGraphTopologyEditorBridge.getState();
    bridge.setHandlers(null);
    bridge.setGraphDraftHasPendingChanges(false);
  }, [factoryDocumentScopeKey, resetEditorChromeForScopeChange]);

  return buildCurrentActivityGraphStateValue({
    activeTool,
    addEntityController,
    addMenuActions: session.addMenuActions,
    blockedRemovalReason: controllers.blockedRemovalReason,
    canDeleteSelection: controllers.canDeleteSelection,
    canInteractWithEditor: session.canInteractWithEditor,
    cancelSaveConfirmation: saveFlow.cancelSaveConfirmation,
    canSaveDraft: saveFlow.canSaveDraft,
    documentSave: editableGraph.saveState.documentSave,
    edgeWaypointControls,
    connectionNotice: controllers.connectionNotice,
    currentFactoryDefinition: viewState.currentFactoryDefinition ?? null,
    graphProjection,
    layoutState: graphRenderState.layoutState,
    locale,
    addEdgeWaypoint: editableGraph.actions.addEdgeWaypoint,
    moveEdgeWaypoint: editableGraph.actions.moveEdgeWaypoint,
    removeEdgeWaypoint: editableGraph.actions.removeEdgeWaypoint,
    moveLayoutNode: editableGraph.actions.moveLayoutNode,
    moveLayoutNodesByDelta: editableGraph.actions.moveLayoutNodesByDelta,
    resizeLayoutNode: editableGraph.actions.resizeLayoutNode,
    fitLayoutNode: editableGraph.actions.fitLayoutNode,
    resetLayoutNodeSize: editableGraph.actions.resetLayoutNodeSize,
    resetLayout: editableGraph.actions.resetLayout,
    redoLayout: editableGraph.actions.redoLayout,
    resetPreferences,
    undoLayout: editableGraph.actions.undoLayout,
    updateLayoutViewport: editableGraph.actions.updateLayoutViewport,
    createVisualGroup: editableGraph.actions.createVisualGroup,
    fitVisualGroup: editableGraph.actions.fitVisualGroup,
    renameVisualGroup: editableGraph.actions.renameVisualGroup,
    setVisualGroupColor: editableGraph.actions.setVisualGroupColor,
    addNodeToVisualGroup: editableGraph.actions.addNodeToVisualGroup,
    removeNodeFromVisualGroup: editableGraph.actions.removeNodeFromVisualGroup,
    moveVisualGroupByDelta: editableGraph.actions.moveVisualGroupByDelta,
    resizeVisualGroup: editableGraph.actions.resizeVisualGroup,
    deleteVisualGroup: editableGraph.actions.deleteVisualGroup,
    topologyNodes: editableGraph.draftState.graph?.nodes ?? [],
    editorUnavailableClassifierWorkstationName:
      session.editorUnavailableClassifierWorkstationName,
    editorMode,
    handleCancelRemoval: controllers.handleCancelRemoval,
    handleDiscardPendingChanges: saveFlow.handleDiscardPendingChanges,
    handleConfirmRemoval: controllers.handleConfirmRemoval,
    handleConnectionAnchorClick: controllers.handleConnectionAnchorClick,
    handleDiscardEditorChanges: saveFlow.handleDiscardEditorChanges,
    handleEditorConnect: controllers.handleEditorConnect,
    handleEditorEdgeDelete: controllers.handleEditorEdgeDelete,
    handleEditorModeToggle: session.handleEditorModeToggle,
    handleEditorNodeDelete: controllers.handleEditorNodeDelete,
    handleSelectionBatchDelete: controllers.handleSelectionBatchDelete,
    handleSelectionNodeDelete: controllers.handleSelectionNodeDelete,
    handleSaveDraft: saveFlow.handleSaveDraft,
    handleSaveBeforeLeavingEditor: saveFlow.handleSaveBeforeLeavingEditor,
    isConfirmingLeaveEditor: saveFlow.isConfirmingLeaveEditor,
    isConfirmingSave: saveFlow.isConfirmingSave,
    pendingConnectionSource: controllers.pendingConnectionSource,
    pendingRemovalIntent: controllers.pendingRemovalIntent,
    saveAttemptRevision: saveFlow.saveAttemptRevision,
    saveSummary: saveFlow.saveSummary,
    setActiveTool,
    setBlockedRemovalReason: controllers.setBlockedRemovalReason,
    setConnectionNotice: controllers.setConnectionNotice,
    setIsConfirmingSave: saveFlow.setIsConfirmingSave,
    setIsConfirmingLeaveEditor: saveFlow.setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
    statusState,
    validationState: graphRenderState.validationState,
    hiddenNodeClasses,
    hideShowMenuOpen,
    setHideShowMenuOpen,
    setVisibilityPreset,
    toggleHiddenNodeClass,
    visibilityPreset,
  });
}

export type CurrentActivityGraphState = CurrentActivityGraphStateValue;
