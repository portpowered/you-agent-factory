import type { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { FactoryDocumentSaveState } from "../../current-selection/base/public";
import type {
  FactoryGraphEditorTool,
  FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { useFactoryGraphLayoutDraftState } from "../../factory-graph-editor/hooks/factory-graph-layout-draft-hook";
import type { EditableFactoryGraphSaveMutation } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { useFactoryValidation } from "../../factory-graph-editor/hooks/validation/use-factory-validation";
import type { FactoryGraphEditorDirtyState } from "../../factory-graph-editor/lib/factory-graph-editor-dirty-state";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary";
import type { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import type { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import type { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

export function buildCurrentActivityGraphEditorValue(args: {
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
  editableDefinitionQuery: ReturnType<typeof useCurrentFactoryDocument>;
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
}) {
  return {
    activeTool: args.activeTool,
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
    dirtyStateSummary: args.dirtyStateSummary,
    draftState: args.draftState,
    layoutDraftState: args.layoutDraftState,
    moveLayoutNode: args.moveLayoutNode,
    moveLayoutNodesByDelta: args.moveLayoutNodesByDelta,
    redoLayout: args.redoLayout,
    resetLayout: args.resetLayout,
    resetPreferences: args.resetPreferences,
    undoLayout: args.undoLayout,
    updateLayoutViewport: args.updateLayoutViewport,
    editableDefinitionQuery: args.editableDefinitionQuery,
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
    saveAttemptRevision: args.saveAttemptRevision,
    saveBlockedReason: args.saveBlockedReason,
    saveEditableDefinition: args.saveEditableDefinition,
    saveSummary: args.saveSummary,
    setActiveTool: args.setActiveTool,
    setAddEntityDraft: args.addEntityController.setAddEntityDraft,
    setAddEntityErrors: args.addEntityController.setAddEntityErrors,
    setAddMenuOpen: args.addEntityController.setAddMenuOpen,
    setBlockedRemovalReason: args.setBlockedRemovalReason,
    setConnectionNotice: args.setConnectionNotice,
    setIsConfirmingSave: args.setIsConfirmingSave,
    setIsConfirmingLeaveEditor: args.setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: args.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: args.setPendingRemovalNodeId,
    structuralValidation: args.structuralValidation,
    hiddenNodeClasses: args.hiddenNodeClasses,
    hideShowMenuOpen: args.hideShowMenuOpen,
    setHideShowMenuOpen: args.setHideShowMenuOpen,
    setVisibilityPreset: args.setVisibilityPreset,
    toggleHiddenNodeClass: args.toggleHiddenNodeClass,
    visibilityPreset: args.visibilityPreset,
  };
}
