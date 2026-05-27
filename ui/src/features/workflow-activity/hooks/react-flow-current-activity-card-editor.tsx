import { useCallback, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/factory-graph-editor-save-summary";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/public";
import {
  type EditableFactoryGraphViewModel,
  useEditableFactoryGraph,
} from "../../factory-graph-editor/public";
import { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import { findClassifierGraphEditorUnsupportedWorkstationName } from "./factory-graph-editor-availability";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import { buildCurrentActivityGraphEditorValue } from "./react-flow-current-activity-card-editor-value";

export function useCurrentActivityGraphEditor(snapshot: DashboardSnapshot) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const [isConfirmingSave, setIsConfirmingSave] = useState(false);
  const { currentFactoryQuery, editableGraph, saveEditableDefinition } =
    useCurrentActivityEditableGraph({
      editorMode,
      snapshot,
    });
  const editableDefinitionQuery = currentFactoryQuery;
  const draftState = editableGraph.draftState;
  const {
    addMenuActions,
    canInteractWithEditor,
    canSaveDraft,
    currentFactoryDefinition,
    editorUnavailableClassifierWorkstationName,
    hasActiveWork,
    isStaleDraft,
    saveBlockedReason,
    saveSummary,
  } = useFactoryGraphEditorSessionState({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    draftState,
    editableGraph,
    editorMode,
    editableDefinitionQuery: currentFactoryQuery,
    projectedFactory: snapshot.factory,
    saveEditableDefinition,
  });
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    editableGraph,
    setActiveTool,
  });
  const controllers = useFactoryGraphEditorControllers({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    saveEditableDefinition,
  });
  const {
    handleDiscardEditorChanges,
    handleDiscardPendingChanges,
    handleEditorModeToggle,
    handleSaveDraft,
    handleSaveBeforeLeavingEditor,
  } = useFactoryGraphEditorLeaveHandlers({
    addEntityController,
    canSaveDraft,
    draftState,
    editableGraph,
    editorUnavailableClassifierWorkstationName,
    editorMode,
    saveEditableDefinition,
    setActiveTool,
    setConnectionNotice: controllers.setConnectionNotice,
    setEditorMode,
    setIsConfirmingSave,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
  });

  return buildCurrentActivityGraphEditorValue({
    activeTool,
    addEntityController,
    addMenuActions,
    blockedRemovalReason: controllers.blockedRemovalReason,
    canInteractWithEditor,
    canSaveDraft,
    connectionNotice: controllers.connectionNotice,
    currentFactoryDefinition,
    draftState,
    editableDefinitionQuery,
    editorUnavailableClassifierWorkstationName,
    editorMode,
    handleDiscardPendingChanges,
    handleConfirmRemoval: controllers.handleConfirmRemoval,
    handleConnectionAnchorClick: controllers.handleConnectionAnchorClick,
    handleDiscardEditorChanges,
    handleEditorConnect: controllers.handleEditorConnect,
    handleEditorEdgeDelete: controllers.handleEditorEdgeDelete,
    handleEditorModeToggle,
    handleEditorNodeDelete: controllers.handleEditorNodeDelete,
    handleSaveDraft,
    handleSaveBeforeLeavingEditor,
    hasActiveWork,
    isConfirmingLeaveEditor,
    isConfirmingSave,
    isStaleDraft,
    pendingConnectionSource: controllers.pendingConnectionSource,
    pendingRemovalIntent: controllers.pendingRemovalIntent,
    saveBlockedReason,
    saveEditableDefinition,
    saveSummary,
    setActiveTool,
    setBlockedRemovalReason: controllers.setBlockedRemovalReason,
    setConnectionNotice: controllers.setConnectionNotice,
    setIsConfirmingSave,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
  });
}

function useCurrentActivityEditableGraph({
  editorMode,
  snapshot,
}: {
  editorMode: boolean;
  snapshot: DashboardSnapshot;
}) {
  const currentFactoryQuery = useCurrentFactoryDocument(editorMode);
  const saveEditableDefinition = useSaveCurrentFactory();
  const editableGraph = useEditableFactoryGraph({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    currentFactoryDocument: currentFactoryQuery.data,
    projectedFactory: snapshot.factory,
    saveFactoryDefinition: saveEditableDefinition.mutateAsync,
  });

  return {
    currentFactoryQuery,
    editableGraph,
    saveEditableDefinition,
  };
}

function useFactoryGraphEditorControllers({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  saveEditableDefinition,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentFactory>;
}) {
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    saveEditableDefinition,
  });

  return {
    ...connectionController,
    ...removalController,
  };
}

function useFactoryGraphEditorSessionState({
  activeWorkCount,
  draftState,
  editableGraph,
  editorMode,
  editableDefinitionQuery,
  projectedFactory,
  saveEditableDefinition,
}: {
  activeWorkCount: number;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  editorMode: boolean;
  editableDefinitionQuery: ReturnType<typeof useCurrentFactoryDocument>;
  projectedFactory?: CanonicalFactoryDefinition;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentFactory>;
}) {
  const hasActiveWork = activeWorkCount > 0;
  const currentFactoryDefinition =
    draftState.pendingFactoryDefinition ??
    draftState.latestDocument ??
    draftState.baseDocument ??
    null;
  const editorUnavailableClassifierWorkstationName =
    findClassifierGraphEditorUnsupportedWorkstationName(
      editorMode
        ? (currentFactoryDefinition ?? projectedFactory ?? null)
        : (projectedFactory ?? currentFactoryDefinition),
    );
  const isStaleDraft = editableGraph.saveState.isStale;
  const canInteractWithEditor =
    editorMode &&
    editableDefinitionQuery.status === "success" &&
    editorUnavailableClassifierWorkstationName === undefined &&
    saveEditableDefinition.status !== "pending";
  const canSaveDraft =
    editableGraph.saveState.canSave &&
    editorUnavailableClassifierWorkstationName === undefined &&
    !hasActiveWork;
  const saveBlockedReason = hasActiveWork
    ? "Topology save is unavailable while active work is still running in this factory."
    : isStaleDraft
      ? "A newer factory topology arrived while this draft was open. Refresh or discard before saving."
      : undefined;

  return {
    addMenuActions: buildFactoryGraphAddEntityMenuActions(
      currentFactoryDefinition,
    ),
    canInteractWithEditor,
    canSaveDraft,
    currentFactoryDefinition,
    editorUnavailableClassifierWorkstationName,
    hasActiveWork,
    isStaleDraft,
    saveBlockedReason,
    saveSummary: buildFactoryGraphSaveSummary(draftState.draft),
  };
}

function useFactoryGraphEditorLeaveHandlers({
  addEntityController,
  canSaveDraft,
  draftState,
  editableGraph,
  editorUnavailableClassifierWorkstationName,
  editorMode,
  saveEditableDefinition,
  setActiveTool,
  setConnectionNotice,
  setEditorMode,
  setIsConfirmingSave,
  setIsConfirmingLeaveEditor,
  setPendingRemovalEdgeId,
  setPendingRemovalNodeId,
}: {
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  canSaveDraft: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  editorUnavailableClassifierWorkstationName?: string;
  editorMode: boolean;
  saveEditableDefinition: ReturnType<typeof useSaveCurrentFactory>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setConnectionNotice: ReturnType<
    typeof useFactoryGraphConnectionController
  >["setConnectionNotice"];
  setEditorMode: (mode: boolean) => void;
  setIsConfirmingSave: (open: boolean) => void;
  setIsConfirmingLeaveEditor: (open: boolean) => void;
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
}) {
  const resetTransientEditorState = useCallback(() => {
    addEntityController.reset();
    saveEditableDefinition.reset();
    setConnectionNotice(null);
    setIsConfirmingSave(false);
    setIsConfirmingLeaveEditor(false);
    setPendingRemovalEdgeId(null);
    setPendingRemovalNodeId(null);
  }, [
    addEntityController,
    saveEditableDefinition,
    setConnectionNotice,
    setIsConfirmingLeaveEditor,
    setIsConfirmingSave,
    setPendingRemovalEdgeId,
    setPendingRemovalNodeId,
  ]);
  const leaveEditor = useCallback(() => {
    setEditorMode(false);
    setActiveTool(null);
    resetTransientEditorState();
  }, [resetTransientEditorState, setActiveTool, setEditorMode]);
  const handleEditorModeToggle = useCallback(() => {
    if (!editorMode) {
      if (editorUnavailableClassifierWorkstationName) {
        return;
      }
      setEditorMode(true);
      return;
    }
    if (draftState.hasChanges) {
      setIsConfirmingLeaveEditor(true);
      return;
    }
    leaveEditor();
  }, [
    draftState.hasChanges,
    editorMode,
    editorUnavailableClassifierWorkstationName,
    leaveEditor,
    setEditorMode,
    setIsConfirmingLeaveEditor,
  ]);
  const handleDiscardPendingChanges = useCallback(() => {
    editableGraph.actions.discard();
    resetTransientEditorState();
  }, [editableGraph.actions, resetTransientEditorState]);
  const handleDiscardEditorChanges = useCallback(() => {
    handleDiscardPendingChanges();
    leaveEditor();
  }, [handleDiscardPendingChanges, leaveEditor]);
  const saveDraft = useCallback(
    async ({ leaveAfterSave }: { leaveAfterSave: boolean }) => {
      if (!canSaveDraft) {
        return false;
      }

      try {
        const didSave = await editableGraph.actions.save();
        if (!didSave) {
          setIsConfirmingSave(false);
          return false;
        }
        setIsConfirmingSave(false);
        setIsConfirmingLeaveEditor(false);
        if (leaveAfterSave) {
          leaveEditor();
        }
        return true;
      } catch {
        setIsConfirmingSave(false);
        return false;
      }
    },
    [
      canSaveDraft,
      editableGraph.actions,
      leaveEditor,
      setIsConfirmingLeaveEditor,
      setIsConfirmingSave,
    ],
  );
  const handleSaveDraft = useCallback(
    async () => saveDraft({ leaveAfterSave: false }),
    [saveDraft],
  );
  const handleSaveBeforeLeavingEditor = useCallback(async () => {
    return saveDraft({ leaveAfterSave: true });
  }, [saveDraft]);

  return {
    handleDiscardEditorChanges,
    handleDiscardPendingChanges,
    handleEditorModeToggle,
    handleSaveDraft,
    handleSaveBeforeLeavingEditor,
  };
}
