import { useCallback, useMemo } from "react";

import type { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import type {
  EditableFactoryGraphSaveMutation,
  EditableFactoryGraphViewModel,
} from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import { findClassifierGraphEditorUnsupportedWorkstationName } from "./factory-graph-editor-availability";

export function useGraphEditorSession({
  activeTool,
  draftState,
  editableDefinitionQuery,
  editorMode,
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
  editableDefinitionQuery: ReturnType<typeof useCurrentFactoryDocument>;
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
    if (draftState.hasChanges) {
      onAttemptLeaveEditor();
      return;
    }
    onLeaveEditor();
  }, [
    draftState.hasChanges,
    editorMode,
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
