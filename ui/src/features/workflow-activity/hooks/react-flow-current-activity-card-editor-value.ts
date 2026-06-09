import type { FactoryDocumentSaveState } from "../../current-selection/base/public";
import type {
  FactoryGraphEditorTool,
  FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { useFactoryGraphLayoutDraftState } from "../../factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook";
import type { EditableFactoryGraphSaveMutation } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { useFactoryValidation } from "../../factory-graph-editor/hooks/validation/use-factory-validation";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { FactoryGraphEditorDirtyState } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state";
import type { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import type { CurrentActivityFactoryDocumentQuery } from "./current-activity-factory-document-state";
import type { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import type { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import type { CurrentActivityFactoryGraphViewState } from "./use-current-activity-factory-graph-view-state";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";

type BuildCurrentActivityGraphEditorValueArgs = {
  activeTool: FactoryGraphEditorTool;
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  addMenuActions: ReturnType<typeof buildFactoryGraphAddEntityMenuActions>;
  blockedRemovalReason: string | null;
  cancelSaveConfirmation: () => void;
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  documentSave: FactoryDocumentSaveState;
  connectionNotice: string | null;
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  dirtyStateSummary: FactoryGraphEditorDirtyState;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
  graphState: CurrentActivityGraphFlowProjection;
  locale?: string | null;
  addEdgeWaypoint: (
    edgeId: string,
    position: { x: number; y: number },
    insertIndex?: number,
  ) => void;
  moveEdgeWaypoint: (
    edgeId: string,
    waypointIndex: number,
    position: { x: number; y: number },
  ) => void;
  removeEdgeWaypoint: (edgeId: string, waypointIndex: number) => void;
  moveLayoutNode: (nodeId: string, position: { x: number; y: number }) => void;
  moveLayoutNodesByDelta: (
    nodeIds: readonly string[],
    delta: { x: number; y: number },
    resolvedPositionsByNodeId: ReadonlyMap<string, { x: number; y: number }>,
  ) => void;
  redoLayout: () => void;
  resetLayout: () => void;
  resetPreferences: () => void;
  undoLayout: () => void;
  updateLayoutViewport: (viewport: {
    x: number;
    y: number;
    zoom: number;
  }) => void;
  editableDefinitionQuery: CurrentActivityFactoryDocumentQuery;
  editorUnavailableClassifierWorkstationName?: string;
  editorMode: boolean;
  handleCancelRemoval: () => void;
  handleDiscardPendingChanges: () => void;
  handleConfirmRemoval: () => void;
  handleConnectionAnchorClick: ReturnType<
    typeof useFactoryGraphConnectionController
  >["handleConnectionAnchorClick"];
  handleDiscardEditorChanges: () => void;
  handleEditorConnect: ReturnType<
    typeof useFactoryGraphConnectionController
  >["handleEditorConnect"];
  handleEditorEdgeDelete: (edgeId: string) => void;
  handleEditorModeToggle: () => void;
  handleEditorNodeDelete: (nodeId: string) => void;
  handleSelectionNodeDelete: (nodeId: string) => void;
  handleSaveDraft: () => Promise<boolean>;
  handleSaveBeforeLeavingEditor: () => Promise<boolean>;
  hasActiveWork: boolean;
  isConfirmingLeaveEditor: boolean;
  isConfirmingSave: boolean;
  isStaleDraft: boolean;
  pendingConnectionSource: ReturnType<
    typeof useFactoryGraphConnectionController
  >["pendingConnectionSource"];
  pendingRemovalIntent: ReturnType<
    typeof useFactoryGraphRemovalController
  >["pendingRemovalIntent"];
  saveAttemptRevision: number;
  saveBlockedReason?: string;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  saveSummary: ReturnType<typeof buildFactoryGraphSaveSummary>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setBlockedRemovalReason: (reason: string | null) => void;
  setConnectionNotice: ReturnType<
    typeof useFactoryGraphConnectionController
  >["setConnectionNotice"];
  setIsConfirmingSave: (open: boolean) => void;
  setIsConfirmingLeaveEditor: (open: boolean) => void;
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
  structuralValidation: ReturnType<typeof useFactoryValidation>;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen: boolean;
  setHideShowMenuOpen: (open: boolean) => void;
  setVisibilityPreset: (preset: FactoryGraphEditorVisibilityPreset) => void;
  toggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
  viewState: CurrentActivityFactoryGraphViewState;
};

function buildCurrentActivityGraphEditorStatus(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  const messages = getFactoryGraphEditorMessages(args.locale);
  const hasTopologyChanges = args.draftState.hasChanges;
  const hasLayoutChanges = args.layoutDraftState.layoutDirty;
  const hasSharedGraphChanges = hasTopologyChanges || hasLayoutChanges;
  const hasAnyChanges =
    hasSharedGraphChanges || args.dirtyStateSummary.preferencesDirty;

  return {
    dirtySummary: hasAnyChanges
      ? messages.dirtyStateSummary(args.dirtyStateSummary)
      : null,
    hasDocumentBackedLayoutDraft: args.draftState.source === "current-factory",
    hasLayoutChanges,
    hasSharedGraphChanges,
    hasTopologyChanges,
    isDefinitionLoading: args.editableDefinitionQuery.status === "pending",
    loadErrorMessage: args.editableDefinitionQuery.error?.message,
    preferencesDirty: args.dirtyStateSummary.preferencesDirty,
    saveError: args.saveEditableDefinition.error ?? null,
    isSaving: args.saveEditableDefinition.isPending,
  };
}

function buildCurrentActivityGraphLayoutControls(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  return {
    addEdgeWaypoint: args.addEdgeWaypoint,
    canMoveLayout: args.draftState.source === "current-factory",
    canRedo: args.layoutDraftState.canRedoLayout,
    canUndo: args.layoutDraftState.canUndoLayout,
    currentLayout: args.graphState.renderedLayout,
    moveEdgeWaypoint: args.moveEdgeWaypoint,
    moveNode: args.moveLayoutNode,
    moveNodesByDelta: args.moveLayoutNodesByDelta,
    redo: args.redoLayout,
    removeEdgeWaypoint: args.removeEdgeWaypoint,
    reset: args.resetLayout,
    undo: args.undoLayout,
    updateViewport: args.updateLayoutViewport,
  };
}

function buildCurrentActivityGraphSaveControls(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  return {
    canSave: args.canSaveDraft,
    cancelConfirmation: args.cancelSaveConfirmation,
    confirmSave: args.handleSaveDraft,
    feedback: args.documentSave,
    isConfirming: args.isConfirmingSave,
    requestConfirmation: () => {
      args.setIsConfirmingSave(true);
    },
    saveBeforeLeaving: args.handleSaveBeforeLeavingEditor,
    summary: args.saveSummary,
  };
}

function buildCurrentActivityGraphValidationControls(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  return {
    factoryDefinition:
      args.viewState.pendingFactoryDefinition ??
      args.currentFactoryDefinition ??
      args.graphState.displayFactoryDefinition ??
      null,
    projection: args.structuralValidation.projection,
    targets: args.structuralValidation.targets,
  };
}

function buildCurrentActivityGraphAddControls(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  return {
    actions: args.addMenuActions,
    currentFactoryDefinition: args.currentFactoryDefinition,
    draft: args.addEntityController.addEntityDraft,
    errors: args.addEntityController.addEntityErrors,
    isDialogOpen: args.addEntityController.addEntityDraft !== null,
    isMenuOpen: args.addEntityController.addMenuOpen,
    closeDialog: () => {
      args.addEntityController.setAddEntityDraft(null);
      args.addEntityController.setAddEntityErrors({});
    },
    setMenuOpen: args.addEntityController.setAddMenuOpen,
    startAction: args.addEntityController.handleAddEntityAction,
    submit: args.addEntityController.handleAddEntitySubmit,
    updateDraft: (draft: typeof args.addEntityController.addEntityDraft) => {
      args.addEntityController.setAddEntityDraft(draft);
      args.addEntityController.setAddEntityErrors({});
    },
  };
}

function buildCurrentActivityGraphRemovalControls(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  return {
    blockedReason: args.blockedRemovalReason,
    pendingIntent: args.pendingRemovalIntent,
    cancel: args.handleCancelRemoval,
    confirm: args.handleConfirmRemoval,
    deleteEdge: args.handleEditorEdgeDelete,
    deleteNode: args.handleEditorNodeDelete,
    requestSelectionNodeRemoval: args.handleSelectionNodeDelete,
  };
}

export function buildCurrentActivityGraphEditorValue(
  args: BuildCurrentActivityGraphEditorValueArgs,
) {
  const addControls = buildCurrentActivityGraphAddControls(args);
  const status = buildCurrentActivityGraphEditorStatus(args);
  const layoutControls = buildCurrentActivityGraphLayoutControls(args);
  const removalControls = buildCurrentActivityGraphRemovalControls(args);
  const saveControls = buildCurrentActivityGraphSaveControls(args);
  const validationControls = buildCurrentActivityGraphValidationControls(args);

  return {
    activeTool: args.activeTool,
    addControls,
    addEntityDraft: args.addEntityController.addEntityDraft,
    addEntityErrors: args.addEntityController.addEntityErrors,
    addMenuActions: args.addMenuActions,
    addMenuOpen: args.addEntityController.addMenuOpen,
    blockedRemovalReason: args.blockedRemovalReason,
    cancelSaveConfirmation: args.cancelSaveConfirmation,
    canInteractWithEditor: args.canInteractWithEditor,
    canSaveDraft: args.canSaveDraft,
    documentSave: args.documentSave,
    connectionNotice: args.connectionNotice,
    currentFactoryDefinition: args.currentFactoryDefinition,
    graphState: args.graphState,
    layoutControls,
    addEdgeWaypoint: args.addEdgeWaypoint,
    moveEdgeWaypoint: args.moveEdgeWaypoint,
    removeEdgeWaypoint: args.removeEdgeWaypoint,
    moveLayoutNode: args.moveLayoutNode,
    moveLayoutNodesByDelta: args.moveLayoutNodesByDelta,
    redoLayout: args.redoLayout,
    requestSaveConfirmation: saveControls.requestConfirmation,
    resetLayout: args.resetLayout,
    resetPreferences: args.resetPreferences,
    undoLayout: args.undoLayout,
    updateLayoutViewport: args.updateLayoutViewport,
    editorUnavailableClassifierWorkstationName:
      args.editorUnavailableClassifierWorkstationName,
    editorMode: args.editorMode,
    handleCancelRemoval: args.handleCancelRemoval,
    handleDiscardPendingChanges: args.handleDiscardPendingChanges,
    handleAddEntityAction: args.addEntityController.handleAddEntityAction,
    handleAddEntitySubmit: args.addEntityController.handleAddEntitySubmit,
    handleConfirmRemoval: args.handleConfirmRemoval,
    handleConnectionAnchorClick: args.handleConnectionAnchorClick,
    handleDiscardEditorChanges: args.handleDiscardEditorChanges,
    handleEditorConnect: args.handleEditorConnect,
    handleEditorEdgeDelete: args.handleEditorEdgeDelete,
    handleEditorModeToggle: args.handleEditorModeToggle,
    handleEditorNodeDelete: args.handleEditorNodeDelete,
    handleSelectionNodeDelete: args.handleSelectionNodeDelete,
    handleSaveDraft: args.handleSaveDraft,
    handleSaveBeforeLeavingEditor: args.handleSaveBeforeLeavingEditor,
    hasActiveWork: args.hasActiveWork,
    isConfirmingLeaveEditor: args.isConfirmingLeaveEditor,
    isConfirmingSave: args.isConfirmingSave,
    isStaleDraft: args.isStaleDraft,
    leaveDialogOpen: args.isConfirmingLeaveEditor,
    pendingConnectionSource: args.pendingConnectionSource,
    pendingRemovalIntent: args.pendingRemovalIntent,
    removalControls,
    saveAttemptRevision: args.saveAttemptRevision,
    saveBlockedReason: args.saveBlockedReason,
    saveControls,
    saveEditableDefinition: args.saveEditableDefinition,
    saveSummary: args.saveSummary,
    setActiveTool: args.setActiveTool,
    setAddEntityDraft: args.addEntityController.setAddEntityDraft,
    setAddEntityErrors: args.addEntityController.setAddEntityErrors,
    setAddMenuOpen: args.addEntityController.setAddMenuOpen,
    setBlockedRemovalReason: args.setBlockedRemovalReason,
    setConnectionNotice: args.setConnectionNotice,
    setIsConfirmingLeaveEditor: args.setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: args.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: args.setPendingRemovalNodeId,
    structuralValidation: args.structuralValidation,
    status,
    validationControls,
    validationFactoryDefinition: validationControls.factoryDefinition,
    validationProjection: validationControls.projection,
    validationTargets: validationControls.targets,
    hiddenNodeClasses: args.hiddenNodeClasses,
    hideShowMenuOpen: args.hideShowMenuOpen,
    setHideShowMenuOpen: args.setHideShowMenuOpen,
    setVisibilityPreset: args.setVisibilityPreset,
    toggleHiddenNodeClass: args.toggleHiddenNodeClass,
    visibilityPreset: args.visibilityPreset,
  };
}
