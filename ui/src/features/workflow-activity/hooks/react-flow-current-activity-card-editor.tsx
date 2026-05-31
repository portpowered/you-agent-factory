import { useCallback, useMemo, useRef, useState } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/factory-graph-editor-save-summary";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useCurrentActivityEditableGraph } from "./react-flow-current-activity-card-editor-editable-graph";
import { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { useFactoryGraphRemovalController } from "./react-flow-current-activity-card-editor-removals";
import { buildDraftAppliedFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-apply";
import { useFactoryValidation } from "../../factory-graph-editor/hooks/use-factory-validation";
import { buildCurrentActivityGraphEditorValue } from "./react-flow-current-activity-card-editor-value";
import { useGraphEditorSession } from "./use-graph-editor-session";

export function useCurrentActivityGraphEditor(
  snapshot: DashboardSnapshot,
  locale?: string | null,
  factoryDocumentScopeKey?: string | null,
) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const [isConfirmingSave, setIsConfirmingSave] = useState(false);
  const leaveEditorRef = useRef<() => void>(() => {});
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
    onAttemptLeaveEditor: () => setIsConfirmingLeaveEditor(true),
    onLeaveEditor: () => {
      leaveEditorRef.current();
    },
    projectedFactory: snapshot.factory,
    saveEditableDefinition,
    setActiveTool,
    setEditorMode,
  });
  const {
    canSaveDraft,
    hasActiveWork,
    isStaleDraft,
    saveBlockedReason,
    saveSummary,
  } = useFactoryGraphEditorSaveGuards({
    activeWorkCount: snapshot.runtime.in_flight_dispatch_count,
    draftState,
    editableGraph,
    editorUnavailableClassifierWorkstationName:
      session.editorUnavailableClassifierWorkstationName,
    locale,
  });
  const addEntityController = useFactoryGraphAddEntityController({
    currentFactoryDefinition: session.currentFactoryDefinition,
    editableGraph,
    setActiveTool,
  });
  const { controllers, leaveEditor, ...leaveHandlers } =
    useFactoryGraphEditorInteraction({
      activeTool,
      addEntityController,
      canInteractWithEditor: session.canInteractWithEditor,
      canSaveDraft,
      draftState,
      editableGraph,
      locale,
      saveEditableDefinition,
      setActiveTool,
      setEditorMode,
      setIsConfirmingLeaveEditor,
      setIsConfirmingSave,
    });
  leaveEditorRef.current = leaveEditor;

  return buildCurrentActivityGraphEditorValue({
    activeTool,
    addEntityController,
    addMenuActions: session.addMenuActions,
    blockedRemovalReason: controllers.blockedRemovalReason,
    canInteractWithEditor: session.canInteractWithEditor,
    canSaveDraft,
    connectionNotice: controllers.connectionNotice,
    currentFactoryDefinition: session.currentFactoryDefinition,
    draftState,
    editableDefinitionQuery: currentFactoryQuery,
    editorUnavailableClassifierWorkstationName:
      session.editorUnavailableClassifierWorkstationName,
    editorMode,
    handleDiscardPendingChanges: leaveHandlers.handleDiscardPendingChanges,
    handleConfirmRemoval: controllers.handleConfirmRemoval,
    handleConnectionAnchorClick: controllers.handleConnectionAnchorClick,
    handleDiscardEditorChanges: leaveHandlers.handleDiscardEditorChanges,
    handleEditorConnect: controllers.handleEditorConnect,
    handleEditorEdgeDelete: controllers.handleEditorEdgeDelete,
    handleEditorModeToggle: session.handleEditorModeToggle,
    handleEditorNodeDelete: controllers.handleEditorNodeDelete,
    handleSaveDraft: leaveHandlers.handleSaveDraft,
    handleSaveBeforeLeavingEditor: leaveHandlers.handleSaveBeforeLeavingEditor,
    graphDraftSaveSucceeded: editableGraph.saveState.lastSuccess,
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

function useFactoryGraphEditorInteraction({
  activeTool,
  addEntityController,
  canInteractWithEditor,
  canSaveDraft,
  draftState,
  editableGraph,
  locale,
  saveEditableDefinition,
  setActiveTool,
  setEditorMode,
  setIsConfirmingLeaveEditor,
  setIsConfirmingSave,
}: {
  activeTool: FactoryGraphEditorTool;
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  canInteractWithEditor: boolean;
  canSaveDraft: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
  saveEditableDefinition: ReturnType<typeof useFactoryDocumentSave>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setEditorMode: (mode: boolean) => void;
  setIsConfirmingLeaveEditor: (open: boolean) => void;
  setIsConfirmingSave: (open: boolean) => void;
}) {
  const controllers = useFactoryGraphEditorControllers({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    locale,
    saveEditableDefinition,
  });
  const leaveHandlers = useFactoryGraphEditorLeaveHandlers({
    addEntityController,
    canSaveDraft,
    editableGraph,
    saveEditableDefinition,
    setActiveTool,
    setConnectionNotice: controllers.setConnectionNotice,
    setEditorMode,
    setIsConfirmingSave,
    setIsConfirmingLeaveEditor,
    setPendingRemovalEdgeId: controllers.setPendingRemovalEdgeId,
    setPendingRemovalNodeId: controllers.setPendingRemovalNodeId,
  });

  return { controllers, ...leaveHandlers };
}

function useFactoryGraphEditorControllers({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  locale,
  saveEditableDefinition,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
  saveEditableDefinition: ReturnType<typeof useFactoryDocumentSave>;
}) {
  const connectionController = useFactoryGraphConnectionController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    locale,
  });
  const removalController = useFactoryGraphRemovalController({
    activeTool,
    canInteractWithEditor,
    draftState,
    editableGraph,
    locale,
    saveEditableDefinition,
  });

  return {
    ...connectionController,
    ...removalController,
  };
}

function useFactoryGraphEditorSaveGuards({
  activeWorkCount,
  draftState,
  editableGraph,
  editorUnavailableClassifierWorkstationName,
  locale,
}: {
  activeWorkCount: number;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  editorUnavailableClassifierWorkstationName?: string;
  locale?: string | null;
}) {
  const hasActiveWork = activeWorkCount > 0;
  const isStaleDraft = editableGraph.saveState.isStale;
  const canSaveDraft =
    editableGraph.saveState.canSave &&
    editorUnavailableClassifierWorkstationName === undefined &&
    !hasActiveWork;
  const messages = getFactoryGraphEditorMessages(locale);
  const saveBlockedReason = hasActiveWork
    ? messages.saveBlockedActiveWork
    : isStaleDraft
      ? messages.saveBlockedStaleDraft
      : undefined;

  return {
    canSaveDraft,
    hasActiveWork,
    isStaleDraft,
    saveBlockedReason,
    saveSummary: buildFactoryGraphSaveSummary(draftState.draft, locale),
  };
}

function useFactoryGraphEditorLeaveHandlers({
  addEntityController,
  canSaveDraft,
  editableGraph,
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
  editableGraph: EditableFactoryGraphViewModel;
  saveEditableDefinition: ReturnType<typeof useFactoryDocumentSave>;
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
    handleSaveDraft,
    handleSaveBeforeLeavingEditor,
    leaveEditor,
  };
}
