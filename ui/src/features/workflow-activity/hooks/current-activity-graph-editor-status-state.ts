import type { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { FactoryGraphEditorDirtyState } from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state";
import type { CurrentActivityGraphStatusState } from "./current-activity-graph-state-value";

type BuildCurrentActivityGraphStatusStateArgs = {
  definitionError?: Error | null;
  definitionStatus: "error" | "pending" | "success";
  dirtyStateSummary: FactoryGraphEditorDirtyState;
  draftHasChanges: boolean;
  draftSource: "current-factory" | "projection";
  hasActiveWork: boolean;
  isSaving: boolean;
  isStaleDraft: boolean;
  layoutDirty: boolean;
  saveBlockedReason?: string;
  saveError: CurrentFactoryDefinitionError | null;
};

export function buildCurrentActivityGraphStatusState({
  definitionError,
  definitionStatus,
  dirtyStateSummary,
  draftHasChanges,
  draftSource,
  hasActiveWork,
  isSaving,
  isStaleDraft,
  layoutDirty,
  saveBlockedReason,
  saveError,
}: BuildCurrentActivityGraphStatusStateArgs): CurrentActivityGraphStatusState {
  return {
    dirtyStateSummary,
    hasActiveWork,
    hasDocumentBackedLayoutDraft: draftSource === "current-factory",
    hasLayoutChanges: layoutDirty,
    hasTopologyChanges: draftHasChanges,
    isDefinitionLoading: definitionStatus === "pending",
    isSaving,
    isStaleDraft,
    loadErrorMessage: definitionError?.message,
    saveBlockedReason,
    saveError,
  };
}
