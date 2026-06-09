import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import {
  type CurrentActivityGraphViewModelInput,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";

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
  const graph = useCurrentActivityGraphViewModel({
    ...input,
    editor: {
      activeTool: editor.activeTool,
      canInteractWithEditor: editor.canInteractWithEditor,
      editorMode: editor.editorMode,
      graphState: editor.graphState,
      handleConnectionAnchorClick: editor.handleConnectionAnchorClick,
      pendingConnectionSource: editor.pendingConnectionSource,
      validationTargets: structuralValidation.targets,
    },
  });
  const {
    addEntityDraft: _addEntityDraft,
    addEntityErrors: _addEntityErrors,
    addMenuActions: _addMenuActions,
    addMenuOpen: _addMenuOpen,
    addEdgeWaypoint: _addEdgeWaypoint,
    blockedRemovalReason: _blockedRemovalReason,
    cancelSaveConfirmation: _cancelSaveConfirmation,
    canSaveDraft: _canSaveDraft,
    currentFactoryDefinition: _currentFactoryDefinition,
    documentSave: _documentSave,
    graphState: _graphState,
    handleAddEntityAction: _handleAddEntityAction,
    handleAddEntitySubmit: _handleAddEntitySubmit,
    handleCancelRemoval: _handleCancelRemoval,
    handleConfirmRemoval: _handleConfirmRemoval,
    handleEditorEdgeDelete: _handleEditorEdgeDelete,
    handleEditorNodeDelete: _handleEditorNodeDelete,
    handleSelectionNodeDelete: _handleSelectionNodeDelete,
    handleSaveBeforeLeavingEditor: _handleSaveBeforeLeavingEditor,
    handleSaveDraft: _handleSaveDraft,
    isConfirmingSave: _isConfirmingSave,
    moveEdgeWaypoint: _moveEdgeWaypoint,
    moveLayoutNode: _moveLayoutNode,
    moveLayoutNodesByDelta: _moveLayoutNodesByDelta,
    pendingRemovalIntent: _pendingRemovalIntent,
    redoLayout: _redoLayout,
    removeEdgeWaypoint: _removeEdgeWaypoint,
    requestSaveConfirmation: _requestSaveConfirmation,
    resetLayout: _resetLayout,
    saveEditableDefinition: _saveEditableDefinition,
    saveSummary: _saveSummary,
    setAddEntityDraft: _setAddEntityDraft,
    setAddEntityErrors: _setAddEntityErrors,
    setAddMenuOpen: _setAddMenuOpen,
    validationFactoryDefinition: _validationFactoryDefinition,
    validationProjection: _validationProjection,
    validationTargets: _validationTargets,
    undoLayout: _undoLayout,
    updateLayoutViewport: _updateLayoutViewport,
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
