import type { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { FactoryDocumentSaveState } from "../../current-selection/base/public";
import type {
  FactoryGraphEditorTool,
  FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { useFactoryGraphEdgeWaypointEditor } from "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook";
import type { useFactoryValidation } from "../../factory-graph-editor/hooks/validation/use-factory-validation";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraftValidationError,
  FactoryGraphNode,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { FactoryGraphEditorDirtyState } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state";
import type { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary";
import { factoryLayoutGroupCanvasNodeOptions } from "../../factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import type { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import type { useFactoryGraphAddEntityController } from "./use-current-activity-graph-add-controller";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";
import type { CurrentActivityGraphLayoutState } from "./use-current-activity-graph-render-state";

export type CurrentActivityGraphStatusState = {
  dirtyStateSummary: FactoryGraphEditorDirtyState;
  hasActiveWork: boolean;
  hasDocumentBackedLayoutDraft: boolean;
  hasLayoutChanges: boolean;
  hasTopologyChanges: boolean;
  isDefinitionLoading: boolean;
  isSaving: boolean;
  isStaleDraft: boolean;
  loadErrorMessage?: string;
  saveBlockedReason?: string;
  saveError: CurrentFactoryDefinitionError | null;
};

export type CurrentActivityGraphValidationState = {
  draftErrors: readonly FactoryGraphDraftValidationError[];
  factoryDefinition: CanonicalFactoryDefinition | null;
  projection: ReturnType<typeof useFactoryValidation>["projection"];
  targets: ReturnType<typeof useFactoryValidation>["targets"];
};

type BuildCurrentActivityGraphStateValueArgs = {
  activeTool: FactoryGraphEditorTool;
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  addMenuActions: ReturnType<typeof buildFactoryGraphAddEntityMenuActions>;
  blockedRemovalReason: string | null;
  canDeleteSelection: ReturnType<
    typeof useFactoryGraphRemovalController
  >["canDeleteSelection"];
  cancelSaveConfirmation: () => void;
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  documentSave: FactoryDocumentSaveState;
  edgeWaypointControls: ReturnType<typeof useFactoryGraphEdgeWaypointEditor>;
  connectionNotice: string | null;
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  graphProjection: CurrentActivityGraphFlowProjection;
  layoutState: CurrentActivityGraphLayoutState;
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
  createVisualGroup: (center: {
    x: number;
    y: number;
  }) => { id: string } | null;
  renameVisualGroup: (groupId: string, label: string) => void;
  setVisualGroupColor: (
    groupId: string,
    color: "primary" | "info" | "success" | "warning" | "outline",
  ) => void;
  addNodeToVisualGroup: (groupId: string, nodeId: string) => void;
  removeNodeFromVisualGroup: (groupId: string, nodeId: string) => void;
  moveVisualGroupByDelta: (
    groupId: string,
    delta: { x: number; y: number },
    resolvedNodePositions?: ReadonlyMap<string, { x: number; y: number }>,
  ) => void;
  resizeVisualGroup: (
    groupId: string,
    bounds: { height: number; width: number; x: number; y: number },
  ) => void;
  deleteVisualGroup: (groupId: string) => void;
  topologyNodes: readonly FactoryGraphNode[];
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
  handleSelectionBatchDelete: ReturnType<
    typeof useFactoryGraphRemovalController
  >["handleSelectionBatchDelete"];
  handleSelectionNodeDelete: (nodeId: string) => void;
  handleSaveDraft: () => Promise<boolean>;
  handleSaveBeforeLeavingEditor: () => Promise<boolean>;
  isConfirmingLeaveEditor: boolean;
  isConfirmingSave: boolean;
  pendingConnectionSource: ReturnType<
    typeof useFactoryGraphConnectionController
  >["pendingConnectionSource"];
  pendingRemovalIntent: ReturnType<
    typeof useFactoryGraphRemovalController
  >["pendingRemovalIntent"];
  saveAttemptRevision: number;
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
  statusState: CurrentActivityGraphStatusState;
  validationState: CurrentActivityGraphValidationState;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen: boolean;
  setHideShowMenuOpen: (open: boolean) => void;
  setVisibilityPreset: (preset: FactoryGraphEditorVisibilityPreset) => void;
  toggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
};

function buildCurrentActivityGraphStatus(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  const messages = getFactoryGraphEditorMessages(args.locale);
  const hasTopologyChanges = args.statusState.hasTopologyChanges;
  const hasLayoutChanges = args.statusState.hasLayoutChanges;
  const hasSharedGraphChanges = hasTopologyChanges || hasLayoutChanges;
  const hasAnyChanges =
    hasSharedGraphChanges ||
    args.statusState.dirtyStateSummary.preferencesDirty;

  return {
    dirtySummary: hasAnyChanges
      ? messages.dirtyStateSummary(args.statusState.dirtyStateSummary)
      : null,
    hasDocumentBackedLayoutDraft: args.statusState.hasDocumentBackedLayoutDraft,
    hasActiveWork: args.statusState.hasActiveWork,
    hasLayoutChanges,
    hasSharedGraphChanges,
    hasTopologyChanges,
    isDefinitionLoading: args.statusState.isDefinitionLoading,
    isStaleDraft: args.statusState.isStaleDraft,
    loadErrorMessage: args.statusState.loadErrorMessage,
    preferencesDirty: args.statusState.dirtyStateSummary.preferencesDirty,
    saveBlockedReason: args.statusState.saveBlockedReason,
    saveError: args.statusState.saveError,
    isSaving: args.statusState.isSaving,
  };
}

function buildCurrentActivityGraphLayoutControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    addEdgeWaypoint: args.addEdgeWaypoint,
    canMoveLayout: args.layoutState.canMoveLayout,
    canRedo: args.layoutState.canRedo,
    canUndo: args.layoutState.canUndo,
    canonicalViewport: args.layoutState.canonicalViewport,
    currentLayout: args.layoutState.currentLayout,
    moveEdgeWaypoint: args.moveEdgeWaypoint,
    moveNode: args.moveLayoutNode,
    moveNodesByDelta: args.moveLayoutNodesByDelta,
    redo: args.redoLayout,
    removeEdgeWaypoint: args.removeEdgeWaypoint,
    reset: args.resetLayout,
    undo: args.undoLayout,
    updateViewport: args.updateLayoutViewport,
    createVisualGroup: args.createVisualGroup,
    renameVisualGroup: args.renameVisualGroup,
    setVisualGroupColor: args.setVisualGroupColor,
    addNodeToVisualGroup: args.addNodeToVisualGroup,
    canvasNodeOptions: factoryLayoutGroupCanvasNodeOptions(args.topologyNodes),
    removeNodeFromVisualGroup: args.removeNodeFromVisualGroup,
    moveVisualGroupByDelta: args.moveVisualGroupByDelta,
    resizeVisualGroup: args.resizeVisualGroup,
    deleteVisualGroup: args.deleteVisualGroup,
  };
}

function buildCurrentActivityGraphSaveControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    attemptRevision: args.saveAttemptRevision,
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

function buildCurrentActivityGraphConnectionControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    handleAnchorClick: args.handleConnectionAnchorClick,
    pendingSource: args.pendingConnectionSource,
  };
}

function buildCurrentActivityGraphLeaveControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    cancel: () => {
      args.setIsConfirmingLeaveEditor(false);
    },
    discardChanges: args.handleDiscardEditorChanges,
    isOpen: args.isConfirmingLeaveEditor,
  };
}

function buildCurrentActivityGraphValidationControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    draftErrors: args.validationState.draftErrors,
    factoryDefinition: args.validationState.factoryDefinition,
    projection: args.validationState.projection,
    targets: args.validationState.targets,
  };
}

function buildCurrentActivityGraphAddControls(
  args: BuildCurrentActivityGraphStateValueArgs,
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
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    blockedReason: args.blockedRemovalReason,
    canDeleteSelection: args.canDeleteSelection,
    pendingIntent: args.pendingRemovalIntent,
    cancel: args.handleCancelRemoval,
    confirm: args.handleConfirmRemoval,
    deleteEdge: args.handleEditorEdgeDelete,
    deleteNode: args.handleEditorNodeDelete,
    requestSelectionBatchRemoval: args.handleSelectionBatchDelete,
    requestSelectionNodeRemoval: args.handleSelectionNodeDelete,
  };
}

function buildCurrentActivityGraphVisibilityControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    hiddenNodeClasses: args.hiddenNodeClasses,
    isDirty: args.statusState.dirtyStateSummary.preferencesDirty,
    isMenuOpen: args.hideShowMenuOpen,
    preset: args.visibilityPreset,
    resetPreferences: args.resetPreferences,
    setMenuOpen: args.setHideShowMenuOpen,
    setPreset: args.setVisibilityPreset,
    toggleHiddenNodeClass: args.toggleHiddenNodeClass,
  };
}

function buildCurrentActivityGraphEditorControls(
  args: BuildCurrentActivityGraphStateValueArgs,
) {
  return {
    activeTool: args.activeTool,
    canInteract: args.canInteractWithEditor,
    connect: args.handleEditorConnect,
    connectionNotice: args.connectionNotice,
    discardPendingChanges: args.handleDiscardPendingChanges,
    isEditing: args.editorMode,
    selectTool: args.setActiveTool,
    toggleMode: args.handleEditorModeToggle,
    unavailableClassifierWorkstationName:
      args.editorUnavailableClassifierWorkstationName,
  };
}

export function buildCurrentActivityGraphStateValue(
  args: BuildCurrentActivityGraphStateValueArgs,
): CurrentActivityGraphStateValue {
  const addControls = buildCurrentActivityGraphAddControls(args);
  const editorControls = buildCurrentActivityGraphEditorControls(args);
  const connectionControls = buildCurrentActivityGraphConnectionControls(args);
  const status = buildCurrentActivityGraphStatus(args);
  const layoutControls = buildCurrentActivityGraphLayoutControls(args);
  const leaveControls = buildCurrentActivityGraphLeaveControls(args);
  const removalControls = buildCurrentActivityGraphRemovalControls(args);
  const saveControls = buildCurrentActivityGraphSaveControls(args);
  const validationControls = buildCurrentActivityGraphValidationControls(args);
  const visibilityControls = buildCurrentActivityGraphVisibilityControls(args);

  return {
    addControls,
    connectionControls,
    editorControls,
    edgeWaypointControls: args.edgeWaypointControls,
    graphProjection: args.graphProjection,
    layoutControls,
    leaveControls,
    removalControls,
    saveControls,
    status,
    validationControls,
    visibilityControls,
  };
}

export type CurrentActivityGraphStatus = ReturnType<
  typeof buildCurrentActivityGraphStatus
>;
export type CurrentActivityGraphLayoutControls = ReturnType<
  typeof buildCurrentActivityGraphLayoutControls
>;
export type CurrentActivityGraphSaveControls = ReturnType<
  typeof buildCurrentActivityGraphSaveControls
>;
export type CurrentActivityGraphConnectionControls = ReturnType<
  typeof buildCurrentActivityGraphConnectionControls
>;
export type CurrentActivityGraphLeaveControls = ReturnType<
  typeof buildCurrentActivityGraphLeaveControls
>;
export type CurrentActivityGraphValidationControls = ReturnType<
  typeof buildCurrentActivityGraphValidationControls
>;
export type CurrentActivityGraphAddControls = ReturnType<
  typeof buildCurrentActivityGraphAddControls
>;
export type CurrentActivityGraphRemovalControls = ReturnType<
  typeof buildCurrentActivityGraphRemovalControls
>;
export type CurrentActivityGraphVisibilityControls = ReturnType<
  typeof buildCurrentActivityGraphVisibilityControls
>;
export type CurrentActivityGraphEditorControls = ReturnType<
  typeof buildCurrentActivityGraphEditorControls
>;

export interface CurrentActivityGraphStateValue {
  addControls: CurrentActivityGraphAddControls;
  connectionControls: CurrentActivityGraphConnectionControls;
  editorControls: CurrentActivityGraphEditorControls;
  edgeWaypointControls: ReturnType<typeof useFactoryGraphEdgeWaypointEditor>;
  graphProjection: CurrentActivityGraphFlowProjection;
  layoutControls: CurrentActivityGraphLayoutControls;
  leaveControls: CurrentActivityGraphLeaveControls;
  removalControls: CurrentActivityGraphRemovalControls;
  saveControls: CurrentActivityGraphSaveControls;
  status: CurrentActivityGraphStatus;
  validationControls: CurrentActivityGraphValidationControls;
  visibilityControls: CurrentActivityGraphVisibilityControls;
}
