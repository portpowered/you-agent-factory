import { type ReactNode, useCallback, useState } from "react";

import type { DashboardTopology } from "../../api/dashboard/types";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../current-factory-definition";
import {
  createEmptyFactoryGraphDraft,
  useFactoryGraphDraftState,
} from "../factory-graph-editor/factory-graph-draft";
import type { CanonicalFactoryDefinition } from "../factory-graph-editor/factory-graph-draft-types";
import {
  applyFactoryGraphAddEntityDraft,
  buildFactoryGraphAddEntityMenuActions,
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  validateFactoryGraphAddEntityDraft,
} from "../factory-graph-editor/factory-graph-editor-additions";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../factory-graph-editor/factory-graph-editor-controls";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";

const CURRENT_ACTIVITY_HEADER_ROW_CLASS =
  "flex items-start justify-between gap-3";

export function useCurrentActivityGraphEditor(
  projectedTopology: DashboardTopology,
) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] =
    useState(false);
  const editableDefinitionQuery = useCurrentEditableFactoryDefinitionDocument(editorMode);
  const draftState = useFactoryGraphDraftState({
    editableDefinitionDocument: editableDefinitionQuery.data,
    projectedTopology,
  });
  const saveEditableDefinition = useSaveCurrentEditableFactoryDefinition();
  const {
    addMenuActions,
    canInteractWithEditor,
    canSaveDraft,
    currentFactoryDefinition,
  } = useFactoryGraphEditorSessionState({
    draftState,
    editorMode,
    editableDefinitionQuery,
    saveEditableDefinition,
  });
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    draftState,
    setActiveTool,
  });
  const {
    connectionNotice,
    handleConnectionAnchorClick,
    handleEditorConnect,
    pendingConnectionSource,
    setConnectionNotice,
  } = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
  });
  const {
    blockedRemovalReason,
    handleConfirmRemoval,
    handleEditorEdgeDelete,
    handleEditorNodeDelete,
    pendingRemovalIntent,
    setBlockedRemovalReason,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  } = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    saveEditableDefinition,
  });
  const {
    handleDiscardEditorChanges,
    handleEditorModeToggle,
    handleSaveBeforeLeavingEditor,
  } = useFactoryGraphEditorLeaveHandlers({
    addEntityController,
    canSaveDraft,
    draftState,
    editorMode,
    saveEditableDefinition,
    setActiveTool,
    setConnectionNotice,
    setEditorMode,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  });

  return buildCurrentActivityGraphEditorValue({
    activeTool,
    addEntityController,
    addMenuActions,
    blockedRemovalReason,
    canInteractWithEditor,
    canSaveDraft,
    connectionNotice,
    currentFactoryDefinition,
    draftState,
    editableDefinitionQuery,
    editorMode,
    handleConfirmRemoval,
    handleConnectionAnchorClick,
    handleDiscardEditorChanges,
    handleEditorConnect,
    handleEditorEdgeDelete,
    handleEditorModeToggle,
    handleEditorNodeDelete,
    handleSaveBeforeLeavingEditor,
    isConfirmingLeaveEditor,
    pendingConnectionSource,
    pendingRemovalIntent,
    saveEditableDefinition,
    setActiveTool,
    setBlockedRemovalReason,
    setConnectionNotice,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  });
}

function useFactoryGraphEditorSessionState({
  draftState,
  editorMode,
  editableDefinitionQuery,
  saveEditableDefinition,
}: {
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  editorMode: boolean;
  editableDefinitionQuery: ReturnType<
    typeof useCurrentEditableFactoryDefinitionDocument
  >;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentEditableFactoryDefinition>;
}) {
  const canInteractWithEditor =
    editorMode &&
    editableDefinitionQuery.status === "success" &&
    saveEditableDefinition.status !== "pending";
  const canSaveDraft =
    draftState.hasChanges &&
    draftState.pendingFactoryDefinition !== null &&
    draftState.validationErrors.length === 0 &&
    draftState.latestDocument !== null;
  const currentFactoryDefinition =
    draftState.pendingFactoryDefinition ??
    draftState.latestDocument?.factoryDefinition ??
    draftState.baseDocument?.factoryDefinition ??
    null;

  return {
    addMenuActions:
      buildFactoryGraphAddEntityMenuActions(currentFactoryDefinition),
    canInteractWithEditor,
    canSaveDraft,
    currentFactoryDefinition,
  };
}

function useFactoryGraphEditorLeaveHandlers({
  addEntityController,
  canSaveDraft,
  draftState,
  editorMode,
  saveEditableDefinition,
  setActiveTool,
  setConnectionNotice,
  setEditorMode,
  setIsConfirmingLeaveEditor,
  setPendingRemovalEdgeId,
  setPendingRemovalNodeId,
}: {
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  canSaveDraft: boolean;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  editorMode: boolean;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentEditableFactoryDefinition>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setConnectionNotice: ReturnType<
    typeof useFactoryGraphConnectionController
  >["setConnectionNotice"];
  setEditorMode: (mode: boolean) => void;
  setIsConfirmingLeaveEditor: (open: boolean) => void;
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
}) {
  const leaveEditor = useCallback(() => {
    setEditorMode(false);
    setActiveTool(null);
    addEntityController.reset();
    setIsConfirmingLeaveEditor(false);
    setConnectionNotice(null);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    addEntityController,
    setActiveTool,
    setConnectionNotice,
    setEditorMode,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);
  const handleEditorModeToggle = useCallback(() => {
    if (!editorMode) {
      setEditorMode(true);
      return;
    }
    if (draftState.hasChanges) {
      setIsConfirmingLeaveEditor(true);
      return;
    }
    leaveEditor();
  }, [draftState.hasChanges, editorMode, leaveEditor, setEditorMode, setIsConfirmingLeaveEditor]);
  const handleDiscardEditorChanges = useCallback(() => {
    draftState.resetDraft();
    leaveEditor();
  }, [draftState, leaveEditor]);
  const handleSaveBeforeLeavingEditor = useCallback(async () => {
    if (!canSaveDraft || draftState.pendingFactoryDefinition == null) {
      return false;
    }

    try {
      await saveEditableDefinition.mutateAsync({
        baseVersion: draftState.latestDocument?.version,
        factoryDefinition: draftState.pendingFactoryDefinition,
      });
      draftState.replaceDraft(createEmptyFactoryGraphDraft());
      leaveEditor();
      return true;
    } catch {
      return false;
    }
  }, [canSaveDraft, draftState, leaveEditor, saveEditableDefinition]);

  return {
    handleDiscardEditorChanges,
    handleEditorModeToggle,
    handleSaveBeforeLeavingEditor,
  };
}

function useFactoryGraphAddEntityController({
  currentFactoryDefinition,
  draftState,
  setActiveTool,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addEntityDraft, setAddEntityDraft] =
    useState<FactoryGraphAddEntityDraft | null>(null);
  const [addEntityErrors, setAddEntityErrors] =
    useState<FactoryGraphAddEntityFieldErrors>({});

  const handleAddEntityAction = useCallback(
    (actionID: string) => {
      setActiveTool("add");
      setAddEntityDraft(
        createFactoryGraphAddEntityDraft(
          actionID as FactoryGraphAddEntityKind,
          currentFactoryDefinition,
        ),
      );
      setAddEntityErrors({});
      setAddMenuOpen(false);
    },
    [currentFactoryDefinition, setActiveTool],
  );

  const handleAddEntitySubmit = useCallback(() => {
    if (addEntityDraft === null) {
      return;
    }

    const validationErrors = validateFactoryGraphAddEntityDraft(
      addEntityDraft,
      currentFactoryDefinition,
    );
    if (Object.keys(validationErrors).length > 0) {
      setAddEntityErrors(validationErrors);
      return;
    }

    draftState.updateDraft((currentDraft) =>
      applyFactoryGraphAddEntityDraft(currentDraft, addEntityDraft),
    );
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, [addEntityDraft, currentFactoryDefinition, draftState]);

  const reset = useCallback(() => {
    setAddMenuOpen(false);
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, []);

  return {
    addEntityDraft,
    addEntityErrors,
    addMenuOpen,
    handleAddEntityAction,
    handleAddEntitySubmit,
    reset,
    setAddEntityDraft,
    setAddEntityErrors,
    setAddMenuOpen,
  };
}

function buildCurrentActivityGraphEditorValue(args: {
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
    typeof useCurrentEditableFactoryDefinitionDocument
  >;
  editorMode: boolean;
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
  handleSaveBeforeLeavingEditor: () => Promise<boolean>;
  isConfirmingLeaveEditor: boolean;
  pendingConnectionSource: ReturnType<
    typeof useFactoryGraphConnectionController
  >["pendingConnectionSource"];
  pendingRemovalIntent: ReturnType<
    typeof useFactoryGraphRemovalController
  >["pendingRemovalIntent"];
  saveEditableDefinition: ReturnType<typeof useSaveCurrentEditableFactoryDefinition>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setBlockedRemovalReason: (reason: string | null) => void;
  setConnectionNotice: ReturnType<
    typeof useFactoryGraphConnectionController
  >["setConnectionNotice"];
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
    editorMode: args.editorMode,
    handleAddEntityAction: args.addEntityController.handleAddEntityAction,
    handleAddEntitySubmit: args.addEntityController.handleAddEntitySubmit,
    handleConfirmRemoval: args.handleConfirmRemoval,
    handleConnectionAnchorClick: args.handleConnectionAnchorClick,
    handleDiscardEditorChanges: args.handleDiscardEditorChanges,
    handleEditorConnect: args.handleEditorConnect,
    handleEditorEdgeDelete: args.handleEditorEdgeDelete,
    handleEditorModeToggle: args.handleEditorModeToggle,
    handleEditorNodeDelete: args.handleEditorNodeDelete,
    handleSaveBeforeLeavingEditor: args.handleSaveBeforeLeavingEditor,
    isConfirmingLeaveEditor: args.isConfirmingLeaveEditor,
    leaveDialogOpen: args.isConfirmingLeaveEditor,
    pendingConnectionSource: args.pendingConnectionSource,
    pendingRemovalIntent: args.pendingRemovalIntent,
    saveEditableDefinition: args.saveEditableDefinition,
    setActiveTool: args.setActiveTool,
    setAddEntityDraft: args.addEntityController.setAddEntityDraft,
    setAddEntityErrors: args.addEntityController.setAddEntityErrors,
    setAddMenuOpen: args.addEntityController.setAddMenuOpen,
    setBlockedRemovalReason: args.setBlockedRemovalReason,
    setConnectionNotice: args.setConnectionNotice,
    setIsConfirmingLeaveEditor: args.setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: args.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: args.setPendingRemovalNodeId,
  };
}

export function CurrentActivityGraphEditorHeader({
  editorMode,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  onToggle,
  title,
}: {
  editorMode: boolean;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
  onToggle: () => void;
  title: ReactNode;
}) {
  return (
    <div className={CURRENT_ACTIVITY_HEADER_ROW_CLASS}>
      <div className="min-w-0">
        {title}
        <div className="mt-2">
          <FactoryGraphEditorStatus
            editorMode={editorMode}
            hasChanges={hasChanges}
            isDefinitionLoading={isDefinitionLoading}
            loadErrorMessage={loadErrorMessage}
          />
        </div>
      </div>
      <FactoryGraphEditorModeToggle
        editorMode={editorMode}
        onClick={onToggle}
      />
    </div>
  );
}
