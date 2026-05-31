import { useMemo, useRef, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { buildDraftAppliedFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-apply";
import { useFactoryValidation } from "../../factory-graph-editor/hooks/use-factory-validation";
import { buildCurrentActivityGraphEditorValue } from "./react-flow-current-activity-card-editor-value";
import { useCurrentActivityEditableGraph } from "./react-flow-current-activity-card-editor-editable-graph";
import { useGraphEditorControllers } from "./use-graph-editor-controllers";
import { useGraphEditorSaveFlow } from "./use-graph-editor-save-flow";
import { useGraphEditorSession } from "./use-graph-editor-session";

export function useCurrentActivityGraphEditor(
  snapshot: DashboardSnapshot,
  locale?: string | null,
  factoryDocumentScopeKey?: string | null,
) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const leaveEditorRef = useRef<() => void>(() => {});
  const setIsConfirmingLeaveEditorRef = useRef<(open: boolean) => void>(
    () => {},
  );
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
    onAttemptLeaveEditor: () => setIsConfirmingLeaveEditorRef.current(true),
    onLeaveEditor: () => {
      leaveEditorRef.current();
    },
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
    saveEditableDefinition,
    setActiveTool,
  });
  const { addEntityController } = controllers;
  const saveFlow = useGraphEditorSaveFlow({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    addEntityController,
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
  setIsConfirmingLeaveEditorRef.current = saveFlow.setIsConfirmingLeaveEditor;
  leaveEditorRef.current = saveFlow.leaveEditor;

  return buildCurrentActivityGraphEditorValue({
    activeTool,
    addEntityController,
    addMenuActions: session.addMenuActions,
    blockedRemovalReason: controllers.blockedRemovalReason,
    canInteractWithEditor: session.canInteractWithEditor,
    canSaveDraft: saveFlow.canSaveDraft,
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
    handleSaveDraft: saveFlow.handleSaveDraft,
    handleSaveBeforeLeavingEditor: saveFlow.handleSaveBeforeLeavingEditor,
    graphDraftSaveSucceeded: editableGraph.saveState.lastSuccess,
    hasActiveWork: saveFlow.hasActiveWork,
    isConfirmingLeaveEditor: saveFlow.isConfirmingLeaveEditor,
    isConfirmingSave: saveFlow.isConfirmingSave,
    isStaleDraft: saveFlow.isStaleDraft,
    pendingConnectionSource: controllers.pendingConnectionSource,
    pendingRemovalIntent: controllers.pendingRemovalIntent,
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

function useDraftAppliedFactoryValidation(
  draftState: EditableFactoryGraphViewModel["draftState"],
  editorMode: boolean,
) {
  const draftAppliedFactoryDefinition = useMemo(() => {
    const baseDocument =
      draftState.latestDocument ?? draftState.baseDocument ?? null;
    if (!baseDocument) {
      return null;
    }

    return buildDraftAppliedFactoryDefinition(baseDocument, draftState.draft);
  }, [draftState.baseDocument, draftState.draft, draftState.latestDocument]);

  return useFactoryValidation(draftAppliedFactoryDefinition, editorMode);
}
