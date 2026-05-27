import type { FactoryGraphDraft } from "./factory-graph-draft-types";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export interface FactoryGraphSaveSummary {
  changedEdges: number;
  createdEntities: number;
  description: string;
  removedEntities: number;
}

export function buildFactoryGraphSaveSummary(
  draft: FactoryGraphDraft,
  locale?: string | null,
): FactoryGraphSaveSummary {
  const createdEntities =
    draft.additions.resources.length +
    draft.additions.workers.length +
    draft.additions.workStates.length +
    draft.additions.workTypes.length +
    draft.additions.workstations.length;
  const removedEntities =
    draft.removals.resources.length +
    draft.removals.workers.length +
    draft.removals.workStates.length +
    draft.removals.workTypes.length +
    draft.removals.workstations.length;
  const changedEdges =
    draft.edgeChanges.additions.length + draft.edgeChanges.removals.length;

  const messages = getFactoryGraphEditorMessages(locale);
  return {
    changedEdges,
    createdEntities,
    description: messages.saveSummaryDescription({
      changedEdges,
      createdEntities,
      removedEntities,
    }),
    removedEntities,
  };
}
