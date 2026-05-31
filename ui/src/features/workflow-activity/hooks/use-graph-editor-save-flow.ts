import { useCallback, useState } from "react";

import {
  isFactoryDocumentSaveConfirming,
  type FactoryDocumentSaveState,
} from "../../current-selection/base/hooks/factory-document-save-types";
import type { EditableFactoryGraphDocumentSaveControls } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { EditableFactoryGraphSaveMutation } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
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
  documentSave,
  documentSaveControls,
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
  documentSave: FactoryDocumentSaveState;
  documentSaveControls: EditableFactoryGraphDocumentSaveControls;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  editorUnavailableClassifierWorkstationName?: string;
  locale?: string | null;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setEditorMode: (mode: boolean) => void;
  transientControllerReset: GraphEditorTransientControllerReset;
}) {
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const [saveAttemptRevision, setSaveAttemptRevision] = useState(0);
  const hasActiveWork = activeWorkCount > 0;
  const isStaleDraft = editableGraph.saveState.isStale;
  const isConfirmingSave = isFactoryDocumentSaveConfirming(documentSave);
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

  const setIsConfirmingSave = useCallback(
    (open: boolean) => {
      if (open) {
        documentSaveControls.beginConfirmation();
        return;
      }
      documentSaveControls.cancelConfirmation();
    },
    [documentSaveControls],
  );

  const resetTransientEditorState = useCallback(() => {
    addEntityController.reset();
    saveEditableDefinition.reset();
    documentSaveControls.clearSaveFeedback();
    transientControllerReset.setConnectionNotice(null);
    setIsConfirmingLeaveEditor(false);
    transientControllerReset.setPendingRemovalEdgeId(null);
    transientControllerReset.setPendingRemovalNodeId(null);
  }, [
    addEntityController,
    documentSaveControls,
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

      setSaveAttemptRevision((revision) => revision + 1);

      try {
        const didSave = await editableGraph.actions.save();
        if (!didSave) {
          documentSaveControls.clearSaveFeedback();
          return false;
        }
        setIsConfirmingLeaveEditor(false);
        if (leaveAfterSave) {
          leaveEditor();
        }
        return true;
      } catch {
        documentSaveControls.clearSaveFeedback();
        return false;
      }
    },
    [
      canSaveDraft,
      documentSaveControls,
      editableGraph.actions,
      leaveEditor,
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
    beginSaveConfirmation: documentSaveControls.beginConfirmation,
    cancelSaveConfirmation: documentSaveControls.cancelConfirmation,
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
    saveAttemptRevision,
    saveBlockedReason,
    saveSummary,
    setIsConfirmingLeaveEditor,
    setIsConfirmingSave,
  };
}
