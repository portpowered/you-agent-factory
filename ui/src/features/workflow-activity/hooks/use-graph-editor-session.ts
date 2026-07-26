import { useCallback, useMemo } from "react";

import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { buildFactoryGraphAddEntityMenuActions } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import { findClassifierGraphEditorUnsupportedWorkstationName } from "./factory-graph-editor-availability";
import { resolveGraphEditorModeToggleAction } from "./state/graph-editor-session-state";

type CurrentActivityGraphSessionDefinitionStatus =
  | "error"
  | "pending"
  | "success";

interface CurrentActivityGraphSessionState {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  definitionStatus: CurrentActivityGraphSessionDefinitionStatus;
  hasPendingGraphChanges: boolean;
  isSaving: boolean;
  projectedFactory?: CanonicalFactoryDefinition;
}

export function useGraphEditorSession({
  activeTool,
  editorMode,
  locale,
  onAttemptLeaveEditor,
  onLeaveEditor,
  sessionState,
  setActiveTool,
  setEditorMode,
}: {
  activeTool: FactoryGraphEditorTool;
  editorMode: boolean;
  locale?: string | null;
  onAttemptLeaveEditor: () => void;
  onLeaveEditor: () => void;
  sessionState: CurrentActivityGraphSessionState;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
  setEditorMode: (mode: boolean) => void;
}) {
  const currentFactoryDefinition = sessionState.currentFactoryDefinition;
  const classifierEvaluationDefinition = useMemo(() => {
    if (editorMode || sessionState.definitionStatus === "success") {
      return currentFactoryDefinition ?? null;
    }

    return sessionState.projectedFactory ?? currentFactoryDefinition ?? null;
  }, [
    currentFactoryDefinition,
    editorMode,
    sessionState.definitionStatus,
    sessionState.projectedFactory,
  ]);
  const editorUnavailableClassifierWorkstationName =
    findClassifierGraphEditorUnsupportedWorkstationName(
      classifierEvaluationDefinition,
    );
  const canInteractWithEditor =
    editorMode &&
    sessionState.definitionStatus === "success" &&
    editorUnavailableClassifierWorkstationName === undefined &&
    !sessionState.isSaving;

  const handleEditorModeToggle = useCallback(() => {
    const action = resolveGraphEditorModeToggleAction({
      editorMode,
      hasPendingGraphChanges: sessionState.hasPendingGraphChanges,
      unavailableClassifierWorkstationName:
        editorUnavailableClassifierWorkstationName,
    });
    if (action === "enter") {
      setEditorMode(true);
    } else if (action === "confirm-leave") {
      onAttemptLeaveEditor();
    } else if (action === "leave") {
      onLeaveEditor();
    }
  }, [
    editorMode,
    editorUnavailableClassifierWorkstationName,
    onAttemptLeaveEditor,
    onLeaveEditor,
    sessionState.hasPendingGraphChanges,
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
