import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import {
  type CurrentActivityGraphRenderProjection,
  type CurrentActivityGraphViewModelInput,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";

type CurrentActivityGraphCardViewModelInput = Omit<
  CurrentActivityGraphViewModelInput,
  "editor"
> & {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
};

export function useCurrentActivityGraphCardViewModel(
  input: CurrentActivityGraphCardViewModelInput,
) {
  const { structuralValidation, ...editor } = input.editor;
  const graphProjection = currentActivityGraphRenderProjection(
    editor.graphState,
  );
  const graph = useCurrentActivityGraphViewModel({
    ...input,
    editor: {
      activeTool: editor.activeTool,
      canInteractWithEditor: editor.canInteractWithEditor,
      editorMode: editor.editorMode,
      graphProjection,
      handleConnectionAnchorClick: editor.handleConnectionAnchorClick,
      pendingConnectionSource: editor.pendingConnectionSource,
      validationTargets: structuralValidation.targets,
    },
  });
  const {
    activeTool: _activeTool,
    addEntityDraft: _addEntityDraft,
    addEntityErrors: _addEntityErrors,
    addMenuActions: _addMenuActions,
    addMenuOpen: _addMenuOpen,
    addEdgeWaypoint: _addEdgeWaypoint,
    blockedRemovalReason: _blockedRemovalReason,
    cancelSaveConfirmation: _cancelSaveConfirmation,
    canInteractWithEditor: _canInteractWithEditor,
    canSaveDraft: _canSaveDraft,
    connectionNotice: _connectionNotice,
    currentFactoryDefinition: _currentFactoryDefinition,
    documentSave: _documentSave,
    editorMode: _editorMode,
    editorUnavailableClassifierWorkstationName:
      _editorUnavailableClassifierWorkstationName,
    graphState: _graphState,
    handleAddEntityAction: _handleAddEntityAction,
    handleAddEntitySubmit: _handleAddEntitySubmit,
    handleCancelRemoval: _handleCancelRemoval,
    handleConfirmRemoval: _handleConfirmRemoval,
    handleDiscardEditorChanges: _handleDiscardEditorChanges,
    handleDiscardPendingChanges: _handleDiscardPendingChanges,
    handleEditorEdgeDelete: _handleEditorEdgeDelete,
    handleEditorModeToggle: _handleEditorModeToggle,
    handleEditorNodeDelete: _handleEditorNodeDelete,
    handleSelectionNodeDelete: _handleSelectionNodeDelete,
    handleSaveBeforeLeavingEditor: _handleSaveBeforeLeavingEditor,
    handleSaveDraft: _handleSaveDraft,
    hasActiveWork: _hasActiveWork,
    hiddenNodeClasses: _hiddenNodeClasses,
    hideShowMenuOpen: _hideShowMenuOpen,
    isConfirmingSave: _isConfirmingSave,
    isConfirmingLeaveEditor: _isConfirmingLeaveEditor,
    isStaleDraft: _isStaleDraft,
    leaveDialogOpen: _leaveDialogOpen,
    moveEdgeWaypoint: _moveEdgeWaypoint,
    moveLayoutNode: _moveLayoutNode,
    moveLayoutNodesByDelta: _moveLayoutNodesByDelta,
    pendingRemovalIntent: _pendingRemovalIntent,
    redoLayout: _redoLayout,
    removeEdgeWaypoint: _removeEdgeWaypoint,
    requestSaveConfirmation: _requestSaveConfirmation,
    resetLayout: _resetLayout,
    saveBlockedReason: _saveBlockedReason,
    saveEditableDefinition: _saveEditableDefinition,
    saveSummary: _saveSummary,
    resetPreferences: _resetPreferences,
    setActiveTool: _setActiveTool,
    setAddEntityDraft: _setAddEntityDraft,
    setAddEntityErrors: _setAddEntityErrors,
    setAddMenuOpen: _setAddMenuOpen,
    setHideShowMenuOpen: _setHideShowMenuOpen,
    setIsConfirmingLeaveEditor: _setIsConfirmingLeaveEditor,
    setVisibilityPreset: _setVisibilityPreset,
    toggleHiddenNodeClass: _toggleHiddenNodeClass,
    validationFactoryDefinition: _validationFactoryDefinition,
    validationProjection: _validationProjection,
    validationTargets: _validationTargets,
    undoLayout: _undoLayout,
    updateLayoutViewport: _updateLayoutViewport,
    visibilityPreset: _visibilityPreset,
    ...publicEditor
  } = editor;

  return {
    ...publicEditor,
    ...graph,
  };
}

export type CurrentActivityGraphCardViewModel = ReturnType<
  typeof useCurrentActivityGraphCardViewModel
>;

function currentActivityGraphRenderProjection(
  graphState: CurrentActivityGraphFlowProjection,
): CurrentActivityGraphRenderProjection {
  return {
    canonicalLayoutViewport: graphState.canonicalLayoutViewport,
    displayFactoryDefinition: graphState.displayFactoryDefinition,
    graphLayout: graphState.graphLayout,
    pendingAdditionEdgeIds: graphState.pendingAdditionEdgeIds,
    positionedGraphLayout: graphState.positionedGraphLayout,
    visibleGraphEdges: graphState.visibleGraphEdges,
  };
}
