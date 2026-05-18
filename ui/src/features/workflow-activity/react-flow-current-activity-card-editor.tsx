import { useCallback, useState, type ReactNode } from "react";

import type { DashboardTopology } from "../../api/dashboard/types";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../current-factory-definition";
import {
  createEmptyFactoryGraphDraft,
  useFactoryGraphDraftState,
} from "../factory-graph-editor/factory-graph-draft";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../factory-graph-editor/factory-graph-editor-controls";

const CURRENT_ACTIVITY_HEADER_ROW_CLASS =
  "flex items-start justify-between gap-3";

export function useCurrentActivityGraphEditor(projectedTopology: DashboardTopology) {
  const [editorMode, setEditorMode] = useState(false);
  const [activeTool, setActiveTool] = useState<FactoryGraphEditorTool>(null);
  const [isConfirmingLeaveEditor, setIsConfirmingLeaveEditor] = useState(false);
  const editableDefinitionQuery =
    useCurrentEditableFactoryDefinitionDocument(editorMode);
  const draftState = useFactoryGraphDraftState({
    editableDefinitionDocument: editableDefinitionQuery.data,
    projectedTopology,
  });
  const saveEditableDefinition = useSaveCurrentEditableFactoryDefinition();
  const canInteractWithEditor =
    editorMode &&
    editableDefinitionQuery.status === "success" &&
    saveEditableDefinition.status !== "pending";
  const canSaveDraft =
    draftState.hasChanges &&
    draftState.pendingFactoryDefinition !== null &&
    draftState.validationErrors.length === 0 &&
    draftState.latestDocument !== null;

  const leaveEditor = useCallback(() => {
    setEditorMode(false);
    setActiveTool(null);
    setIsConfirmingLeaveEditor(false);
  }, []);

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
  }, [draftState.hasChanges, editorMode, leaveEditor]);

  const handleDiscardEditorChanges = useCallback(() => {
    draftState.resetDraft();
    leaveEditor();
  }, [draftState, leaveEditor]);

  const handleSaveBeforeLeavingEditor = useCallback(async () => {
    if (!canSaveDraft || draftState.pendingFactoryDefinition == null) {
      return;
    }

    await saveEditableDefinition.mutateAsync({
      baseVersion: draftState.latestDocument?.version,
      factoryDefinition: draftState.pendingFactoryDefinition,
    });
    draftState.replaceDraft(createEmptyFactoryGraphDraft());
    leaveEditor();
  }, [canSaveDraft, draftState, leaveEditor, saveEditableDefinition]);

  return {
    activeTool,
    canInteractWithEditor,
    canSaveDraft,
    draftState,
    editorMode,
    editableDefinitionQuery,
    handleDiscardEditorChanges,
    handleEditorModeToggle,
    handleSaveBeforeLeavingEditor,
    isConfirmingLeaveEditor,
    leaveDialogOpen: isConfirmingLeaveEditor,
    saveEditableDefinition,
    setActiveTool,
    setIsConfirmingLeaveEditor,
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
      <FactoryGraphEditorModeToggle editorMode={editorMode} onClick={onToggle} />
    </div>
  );
}
