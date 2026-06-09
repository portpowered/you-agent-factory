import { useCallback, useMemo } from "react";

import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type {
  EditableFactoryGraphSaveMutation,
  EditableFactoryGraphViewModel,
} from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { CurrentActivityFactoryDocumentQuery } from "./current-activity-factory-document-state";
import { findClassifierGraphEditorUnsupportedWorkstationName } from "./factory-graph-editor-availability";

export function useGraphEditorSession({
  activeTool,
  draftState,
  editableDefinitionQuery,
  editorMode,
  layoutDraftState,
  locale,
  onAttemptLeaveEditor,
  onLeaveEditor,
  projectedFactory,
  saveEditableDefinition,
  setActiveTool,
  setEditorMode,
}: {
  activeTool: FactoryGraphEditorTool;
  draftState: EditableFactoryGraphViewModel["draftState"];
  layoutDraftState: EditableFactoryGraphViewModel["layoutDraftState"];
  editableDefinitionQuery: CurrentActivityFactoryDocumentQuery;
  editorMode: boolean;
  locale?: string | null;
  onAttemptLeaveEditor: () => void;
  onLeaveEditor: () => void;
  projectedFactory?: CanonicalFactoryDefinition;
  saveEditableDefinition: EditableFactoryGraphSaveMutation;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setEditorMode: (mode: boolean) => void;
}) {
  const currentFactoryDefinition =
    draftState.pendingFactoryDefinition ??
    draftState.latestDocument ??
    draftState.baseDocument ??
    null;
  const classifierEvaluationDefinition = useMemo(() => {
    if (editorMode || editableDefinitionQuery.status === "success") {
      return currentFactoryDefinition ?? null;
    }

    return projectedFactory ?? currentFactoryDefinition ?? null;
  }, [
    currentFactoryDefinition,
    editableDefinitionQuery.status,
    editorMode,
    projectedFactory,
  ]);
  const editorUnavailableClassifierWorkstationName =
    findClassifierGraphEditorUnsupportedWorkstationName(
      classifierEvaluationDefinition,
    );
  const canInteractWithEditor =
    editorMode &&
    editableDefinitionQuery.status === "success" &&
    editorUnavailableClassifierWorkstationName === undefined &&
    !saveEditableDefinition.isPending;

  const handleEditorModeToggle = useCallback(() => {
    if (!editorMode) {
      if (editorUnavailableClassifierWorkstationName) {
        return;
      }
      setEditorMode(true);
      return;
    }
    if (
      draftState.hasChanges ||
      layoutDraftState.layoutDirty ||
      layoutDraftState.hasChanges
    ) {
      onAttemptLeaveEditor();
      return;
    }
    onLeaveEditor();
  }, [
    draftState.hasChanges,
    editorMode,
    layoutDraftState.hasChanges,
    layoutDraftState.layoutDirty,
    editorUnavailableClassifierWorkstationName,
    onAttemptLeaveEditor,
    onLeaveEditor,
    setEditorMode,
  ]);

  return {
    activeTool,
    addMenuActions: buildFactoryGraphAddEntityMenuActions(
      currentFactoryDefinition,
      locale,
    ),
    canInteractWithEditor,
    currentFactoryDefinition,
    editorMode,
    editorUnavailableClassifierWorkstationName,
    handleEditorModeToggle,
    setActiveTool,
    setEditorMode,
  };
}
