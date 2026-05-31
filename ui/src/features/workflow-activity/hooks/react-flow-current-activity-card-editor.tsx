import { useEffect, useRef, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { useFactoryGraphTopologyEditorBridge } from "../state/factory-graph-topology-editor-bridge";
import { useDraftAppliedFactoryValidation } from "./react-flow-current-activity-card-editor-draft-validation";
import { useCurrentActivityEditableGraph } from "./react-flow-current-activity-card-editor-editable-graph";
import { useGraphEditorLeaveEditorBridge } from "./react-flow-current-activity-card-editor-save-flow";
import { buildCurrentActivityGraphEditorValue } from "./react-flow-current-activity-card-editor-value";
import { useGraphEditorControllers } from "./use-graph-editor-controllers";
import { useGraphEditorSaveFlow } from "./use-graph-editor-save-flow";
import { useGraphEditorSession } from "./use-graph-editor-session";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph editor hook wires session, controllers, and save flow into one card value model.
export function useCurrentActivityGraphEditor(
  snapshot: DashboardSnapshot,
  locale?: string | null,
  factoryDocumentScopeKey?: string | null,
  onNodeRemovedFromDraft?: (nodeId: string) => void,
) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const leaveEditorBridge = useGraphEditorLeaveEditorBridge();
  const { currentFactoryQuery, editableGraph, saveEditableDefinition } =
    useCurrentActivityEditableGraph({
      editorMode,
      factoryDocumentScopeKey,
      locale,
      snapshot,
    });
  const draftState = editableGraph.draftState;
  const structuralValidation = useDraftAppliedFactoryValidation(
    draftState,
    editorMode,
  );
  const session = useGraphEditorSession({
    activeTool,
    draftState,
    editableDefinitionQuery: currentFactoryQuery,
    editorMode,
    locale,
    ...leaveEditorBridge.sessionCallbacks,
    projectedFactory: snapshot.factory,
    saveEditableDefinition,
    setActiveTool,
    setEditorMode,
  });
  const controllers = useGraphEditorControllers({
    activeTool,
    canInteractWithEditor: session.canInteractWithEditor,
    currentFactoryDefinition: session.currentFactoryDefinition,
    draftState,
    editableGraph,
    locale,
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
      setConnectionNotice: controllers.setConnectionNotice,
      setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
      setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
    },
  });
  leaveEditorBridge.bindSaveFlow(saveFlow);

  const { resetEditorChromeForScopeChange } = saveFlow;
  const lastFactoryDocumentScopeKeyRef = useRef<string | null>(null);
  useEffect(() => {
    const normalizedScopeKey = factoryDocumentScopeKey ?? null;
    const previousScopeKey = lastFactoryDocumentScopeKeyRef.current;
    const scopeChanged =
      previousScopeKey !== null && previousScopeKey !== normalizedScopeKey;
    lastFactoryDocumentScopeKeyRef.current = normalizedScopeKey;

    if (!scopeChanged) {
      return;
    }

    resetEditorChromeForScopeChange();
    useFactoryGraphTopologyEditorBridge.getState().setHandlers(null);
  }, [factoryDocumentScopeKey, resetEditorChromeForScopeChange]);

  return buildCurrentActivityGraphEditorValue({
    activeTool,
    addEntityController,
    addMenuActions: session.addMenuActions,
    blockedRemovalReason: controllers.blockedRemovalReason,
    canInteractWithEditor: session.canInteractWithEditor,
    cancelSaveConfirmation: saveFlow.cancelSaveConfirmation,
    canSaveDraft: saveFlow.canSaveDraft,
    documentSave: editableGraph.saveState.documentSave,
    connectionNotice: controllers.connectionNotice,
    currentFactoryDefinition: session.currentFactoryDefinition,
    draftState,
    editableDefinitionQuery: currentFactoryQuery,
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
    handleSelectionNodeDelete: controllers.handleSelectionNodeDelete,
    handleSaveDraft: saveFlow.handleSaveDraft,
    handleSaveBeforeLeavingEditor: saveFlow.handleSaveBeforeLeavingEditor,
    hasActiveWork: saveFlow.hasActiveWork,
    isConfirmingLeaveEditor: saveFlow.isConfirmingLeaveEditor,
    isConfirmingSave: saveFlow.isConfirmingSave,
    isStaleDraft: saveFlow.isStaleDraft,
    pendingConnectionSource: controllers.pendingConnectionSource,
    pendingRemovalIntent: controllers.pendingRemovalIntent,
    saveAttemptRevision: saveFlow.saveAttemptRevision,
    saveBlockedReason: saveFlow.saveBlockedReason,
    saveEditableDefinition,
    saveSummary: saveFlow.saveSummary,
    setActiveTool,
    setBlockedRemovalReason: controllers.setBlockedRemovalReason,
    setConnectionNotice: controllers.setConnectionNotice,
    setIsConfirmingSave: saveFlow.setIsConfirmingSave,
    setIsConfirmingLeaveEditor: saveFlow.setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
    structuralValidation,
  });
}
