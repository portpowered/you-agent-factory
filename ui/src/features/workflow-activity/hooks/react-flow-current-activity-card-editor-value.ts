import type { useCurrentFactoryDocument } from "../../current-factory-definition/public";
import type { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import type { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/factory-graph-editor-save-summary";
import type { useSaveCurrentFactory } from "../../current-factory-definition/public";
import type { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import type { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import type { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

export function buildCurrentActivityGraphEditorValue(args: {
  activeTool: FactoryGraphEditorTool;
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  addMenuActions: ReturnType<typeof buildFactoryGraphAddEntityMenuActions>;
  blockedRemovalReason: string | null;
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  connectionNotice: string | null;
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  editableDefinitionQuery: ReturnType<
    typeof useCurrentFactoryDocument
  >;
  editorUnavailableClassifierWorkstationName?: string;
  editorMode: boolean;
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
  saveBlockedReason?: string;
  saveEditableDefinition: ReturnType<
    typeof useSaveCurrentFactory
  >;
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
}) {
  return {
    activeTool: args.activeTool,
    addEntityDraft: args.addEntityController.addEntityDraft,
    addEntityErrors: args.addEntityController.addEntityErrors,
    addMenuActions: args.addMenuActions,
    addMenuOpen: args.addEntityController.addMenuOpen,
    blockedRemovalReason: args.blockedRemovalReason,
    canInteractWithEditor: args.canInteractWithEditor,
    canSaveDraft: args.canSaveDraft,
    connectionNotice: args.connectionNotice,
    currentFactoryDefinition: args.currentFactoryDefinition,
    draftState: args.draftState,
    editableDefinitionQuery: args.editableDefinitionQuery,
    editorUnavailableClassifierWorkstationName:
      args.editorUnavailableClassifierWorkstationName,
    editorMode: args.editorMode,
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
    handleSaveDraft: args.handleSaveDraft,
    handleSaveBeforeLeavingEditor: args.handleSaveBeforeLeavingEditor,
    hasActiveWork: args.hasActiveWork,
    isConfirmingLeaveEditor: args.isConfirmingLeaveEditor,
    isConfirmingSave: args.isConfirmingSave,
    isStaleDraft: args.isStaleDraft,
    leaveDialogOpen: args.isConfirmingLeaveEditor,
    pendingConnectionSource: args.pendingConnectionSource,
    pendingRemovalIntent: args.pendingRemovalIntent,
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
  };
}
