/** Pure state decision consumed by the editor session hook. */
export type GraphEditorModeToggleAction =
  | "blocked"
  | "confirm-leave"
  | "enter"
  | "leave";

export function resolveGraphEditorModeToggleAction({
  editorMode,
  hasPendingGraphChanges,
  unavailableClassifierWorkstationName,
}: {
  editorMode: boolean;
  hasPendingGraphChanges: boolean;
  unavailableClassifierWorkstationName?: string;
}): GraphEditorModeToggleAction {
  if (!editorMode) {
    return unavailableClassifierWorkstationName ? "blocked" : "enter";
  }
  return hasPendingGraphChanges ? "confirm-leave" : "leave";
}
