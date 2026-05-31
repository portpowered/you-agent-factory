import { useCallback, useState } from "react";

import type { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import { buildFactoryGraphSaveSummary } from "../../factory-graph-editor/lib/factory-graph-editor-save-summary";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type { useFactoryGraphAddEntityController } from "../components/react-flow-current-activity-card-editor-chrome";
import type { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";

export type GraphEditorTransientControllerReset = {
  setConnectionNotice: ReturnType<
    typeof useFactoryGraphConnectionController
  >["setConnectionNotice"];
  setPendingRemovalEdgeId: (edgeId: string | null) => void;
  setPendingRemovalNodeId: (nodeId: string | null) => void;
};

export function useGraphEditorSaveFlow({
  activeWorkCount,
  addEntityController,
  draftState,
  editableGraph,
  editorUnavailableClassifierWorkstationName,
  locale,
  saveEditableDefinition,
  setActiveTool,
  setEditorMode,
  transientControllerReset,
}: {
  activeWorkCount: number;
  addEntityController: ReturnType<typeof useFactoryGraphAddEntityController>;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  editorUnavailableClassifierWorkstationName?: string;
  locale?: string | null;
  saveEditableDefinition: ReturnType<typeof useFactoryDocumentSave>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setEditorMode: (mode: boolean) => void;
  transientControllerReset: GraphEditorTransientControllerReset;
}) {
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const [isConfirmingSave, setIsConfirmingSave] = useState(false);
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
  const saveSummary = buildFactoryGraphSaveSummary(draftState.draft, locale);

  const resetTransientEditorState = useCallback(() => {
    addEntityController.reset();
    saveEditableDefinition.reset();
    transientControllerReset.setConnectionNotice(null);
    setIsConfirmingSave(false);
    setIsConfirmingLeaveEditor(false);
    transientControllerReset.setPendingRemovalEdgeId(null);
    transientControllerReset.setPendingRemovalNodeId(null);
  }, [
    addEntityController,
    saveEditableDefinition,
    transientControllerReset,
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
    [canSaveDraft, editableGraph.actions, leaveEditor],
  );

  const handleSaveDraft = useCallback(
    async () => saveDraft({ leaveAfterSave: false }),
    [saveDraft],
  );

  const handleSaveBeforeLeavingEditor = useCallback(async () => {
    return saveDraft({ leaveAfterSave: true });
  }, [saveDraft]);

  return {
    canSaveDraft,
    handleDiscardEditorChanges,
    handleDiscardPendingChanges,
    handleSaveBeforeLeavingEditor,
    handleSaveDraft,
    hasActiveWork,
    isConfirmingLeaveEditor,
    isConfirmingSave,
    isStaleDraft,
    leaveEditor,
    saveBlockedReason,
    saveSummary,
    setIsConfirmingLeaveEditor,
    setIsConfirmingSave,
  };
}
