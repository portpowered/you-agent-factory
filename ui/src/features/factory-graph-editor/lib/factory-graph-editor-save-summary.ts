import type { FactoryGraphDraft } from "./factory-graph-draft-types";

export interface FactoryGraphSaveSummary {
  changedEdges: number;
  createdEntities: number;
  description: string;
  removedEntities: number;
}

export function buildFactoryGraphSaveSummary(
  draft: FactoryGraphDraft,
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

  return {
    changedEdges,
    createdEntities,
    description: buildSummaryDescription({
      changedEdges,
      createdEntities,
      removedEntities,
    }),
    removedEntities,
  };
}

function buildSummaryDescription(summary: Omit<FactoryGraphSaveSummary, "description">) {
  const segments = [
    describeCount(summary.createdEntities, "created entity"),
    describeCount(summary.removedEntities, "deleted entity"),
    describeCount(summary.changedEdges, "changed edge"),
  ].filter((segment) => segment !== null);

  if (segments.length === 0) {
    return "No graph changes are pending.";
  }

  if (segments.length === 1) {
    return `This save will apply ${segments[0]}.`;
  }

  const finalSegment = segments[segments.length - 1];
  return `This save will apply ${segments.slice(0, -1).join(", ")} and ${finalSegment}.`;
}

function describeCount(count: number, singular: string) {
  if (count === 0) {
    return null;
  }
  const plural =
    singular === "created entity" || singular === "deleted entity"
      ? `${singular.slice(0, -1)}ies`
      : `${singular}s`;
  return `${count} ${count === 1 ? singular : plural}`;
}
