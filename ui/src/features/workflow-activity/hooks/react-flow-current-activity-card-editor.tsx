import { useCallback, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../../current-factory-definition/public";
import {
  createEmptyFactoryGraphDraft,
  useFactoryGraphDraftState,
} from "../../factory-graph-editor/public";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/factory-graph-editor-save-summary";
import { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import {
  findClassifierGraphEditorUnsupportedWorkstationName,
  findClassifierGraphEditorUnsupportedWorkstationNameFromTopology,
} from "./factory-graph-editor-availability";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import { buildCurrentActivityGraphEditorValue } from "./react-flow-current-activity-card-editor-value";

export function useCurrentActivityGraphEditor(snapshot: DashboardSnapshot) {
  const projectedTopology = snapshot.topology;
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const [isConfirmingSave, setIsConfirmingSave] = useState(false);
  const editableDefinitionQuery =
    useCurrentEditableFactoryDefinitionDocument(editorMode);
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
    editorUnavailableClassifierWorkstationName,
    hasActiveWork,
    isStaleDraft,
    saveBlockedReason,
    saveSummary,
  } = useFactoryGraphEditorSessionState({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    draftState,
    editorMode,
    editableDefinitionQuery,
    projectedTopology,
    saveEditableDefinition,
  });
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition,
    draftState,
    setActiveTool,
  });
  const controllers = useFactoryGraphEditorControllers({
    activeTool,
    canInteractWithEditor,
    draftState,
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

function useFactoryGraphEditorControllers({
  activeTool,
  canInteractWithEditor,
  draftState,
  saveEditableDefinition,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  saveEditableDefinition: ReturnType<
    typeof useSaveCurrentEditableFactoryDefinition
  >;
}) {
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
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
  editorMode,
  editableDefinitionQuery,
  projectedTopology,
  saveEditableDefinition,
}: {
  activeWorkCount: number;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  editorMode: boolean;
  editableDefinitionQuery: ReturnType<
    typeof useCurrentEditableFactoryDefinitionDocument
  >;
  projectedTopology: DashboardSnapshot["topology"];
  saveEditableDefinition: ReturnType<
    typeof useSaveCurrentEditableFactoryDefinition
  >;
}) {
  const hasActiveWork = activeWorkCount > 0;
  const editorUnavailableClassifierWorkstationName = editorMode
    ? findClassifierGraphEditorUnsupportedWorkstationName(
        draftState.pendingFactoryDefinition ??
          draftState.latestDocument?.factoryDefinition ??
          draftState.baseDocument?.factoryDefinition ??
          null,
      ) ??
      findClassifierGraphEditorUnsupportedWorkstationNameFromTopology(
        projectedTopology,
      )
    : findClassifierGraphEditorUnsupportedWorkstationNameFromTopology(
        projectedTopology,
      ) ??
      findClassifierGraphEditorUnsupportedWorkstationName(
        draftState.pendingFactoryDefinition ??
          draftState.latestDocument?.factoryDefinition ??
          draftState.baseDocument?.factoryDefinition ??
          null,
      );
  const isStaleDraft =
    draftState.hasChanges &&
    draftState.baseDocument !== null &&
    draftState.latestDocument !== null &&
    (draftState.baseDocument.version.logical !==
      draftState.latestDocument.version.logical ||
      draftState.baseDocument.version.physical !==
        draftState.latestDocument.version.physical);
  const canInteractWithEditor =
    editorMode &&
    editableDefinitionQuery.status === "success" &&
    editorUnavailableClassifierWorkstationName === undefined &&
    saveEditableDefinition.status !== "pending";
  const canSaveDraft =
    draftState.hasChanges &&
    draftState.pendingFactoryDefinition !== null &&
    draftState.validationErrors.length === 0 &&
    draftState.latestDocument !== null &&
    editorUnavailableClassifierWorkstationName === undefined &&
    !hasActiveWork &&
    !isStaleDraft;
  const currentFactoryDefinition =
    draftState.pendingFactoryDefinition ??
    draftState.latestDocument?.factoryDefinition ??
    draftState.baseDocument?.factoryDefinition ??
    null;
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
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  editorUnavailableClassifierWorkstationName?: string;
  editorMode: boolean;
  saveEditableDefinition: ReturnType<
    typeof useSaveCurrentEditableFactoryDefinition
  >;
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
    draftState.resetDraft();
    resetTransientEditorState();
  }, [draftState, resetTransientEditorState]);
  const handleDiscardEditorChanges = useCallback(() => {
    handleDiscardPendingChanges();
    leaveEditor();
  }, [handleDiscardPendingChanges, leaveEditor]);
  const saveDraft = useCallback(
    async ({ leaveAfterSave }: { leaveAfterSave: boolean }) => {
      if (!canSaveDraft || draftState.pendingFactoryDefinition == null) {
        return false;
      }

      try {
        await saveEditableDefinition.mutateAsync({
          baseVersion: draftState.latestDocument?.version,
          factoryDefinition: draftState.pendingFactoryDefinition,
        });
        draftState.replaceDraft(createEmptyFactoryGraphDraft());
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
      draftState,
      leaveEditor,
      saveEditableDefinition,
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
