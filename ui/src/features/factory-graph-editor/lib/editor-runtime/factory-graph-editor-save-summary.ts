import { getFactoryGraphEditorMessages } from "../../messages/editor";
import type { FactoryGraphDraft } from "../draft/factory-graph-draft-types";
import {
  type FactoryGraphEditorDirtyState,
  type FactoryGraphSaveSummaryKind,
  resolveFactoryGraphEditorDirtyState,
  resolveFactoryGraphSaveSummaryKind,
} from "./factory-graph-editor-dirty-state";

export interface FactoryGraphSaveSummary {
  changedEdges: number;
  confirmActionLabel: string;
  createdEntities: number;
  description: string;
  dirtyState: FactoryGraphEditorDirtyState;
  kind: FactoryGraphSaveSummaryKind;
  removedEntities: number;
}

export interface FactoryGraphSaveSummaryInput {
  dirtyState?: FactoryGraphEditorDirtyState;
  draft: FactoryGraphDraft;
  hasLayoutChanges?: boolean;
  hasPreferenceChanges?: boolean;
  hasTopologyChanges?: boolean;
}

export function buildFactoryGraphSaveSummary(
  input: FactoryGraphDraft | FactoryGraphSaveSummaryInput,
  locale?: string | null,
): FactoryGraphSaveSummary {
  const summaryInput = normalizeFactoryGraphSaveSummaryInput(input);
  const createdEntities =
    summaryInput.draft.additions.docs.length +
    summaryInput.draft.additions.resources.length +
    summaryInput.draft.additions.workers.length +
    summaryInput.draft.additions.workStates.length +
    summaryInput.draft.additions.workTypes.length +
    summaryInput.draft.additions.workstations.length;
  const removedEntities =
    summaryInput.draft.removals.docs.length +
    summaryInput.draft.removals.resources.length +
    summaryInput.draft.removals.workers.length +
    summaryInput.draft.removals.workStates.length +
    summaryInput.draft.removals.workTypes.length +
    summaryInput.draft.removals.workstations.length;
  const changedEdges =
    summaryInput.draft.edgeChanges.additions.length +
    summaryInput.draft.edgeChanges.removals.length;
  const dirtyState =
    summaryInput.dirtyState ??
    resolveFactoryGraphEditorDirtyState({
      hasLayoutChanges: summaryInput.hasLayoutChanges ?? false,
      hasPreferenceChanges: summaryInput.hasPreferenceChanges ?? false,
      hasTopologyChanges:
        summaryInput.hasTopologyChanges ??
        hasTopologyDraftChanges(summaryInput.draft),
    });
  const kind = resolveFactoryGraphSaveSummaryKind(dirtyState);
  const messages = getFactoryGraphEditorMessages(locale);
  const topologySummary = messages.saveSummaryDescription({
    changedEdges,
    createdEntities,
    removedEntities,
  });

  return {
    changedEdges,
    confirmActionLabel: messages.saveConfirmAction(kind),
    createdEntities,
    description: messages.saveSummaryForDirtyState({
      changedEdges,
      createdEntities,
      dirtyState,
      kind,
      removedEntities,
      topologySummary,
    }),
    dirtyState,
    kind,
    removedEntities,
  };
}

function normalizeFactoryGraphSaveSummaryInput(
  input: FactoryGraphDraft | FactoryGraphSaveSummaryInput,
): FactoryGraphSaveSummaryInput {
  if ("draft" in input) {
    return input;
  }

  return {
    draft: input,
  };
}

function hasTopologyDraftChanges(draft: FactoryGraphDraft): boolean {
  return (
    draft.additions.resources.length > 0 ||
    draft.additions.workers.length > 0 ||
    draft.additions.workStates.length > 0 ||
    draft.additions.workTypes.length > 0 ||
    draft.additions.workstations.length > 0 ||
    draft.removals.resources.length > 0 ||
    draft.removals.workers.length > 0 ||
    draft.removals.workStates.length > 0 ||
    draft.removals.workTypes.length > 0 ||
    draft.removals.workstations.length > 0 ||
    draft.edgeChanges.additions.length > 0 ||
    draft.edgeChanges.removals.length > 0
  );
}
