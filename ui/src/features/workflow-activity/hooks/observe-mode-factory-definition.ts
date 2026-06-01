import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { FactoryTimelineMode } from "../../timeline/state/factoryTimelineStore";
import { doesFactoryDefinitionChangeAffectGraphTopology } from "../../factory-graph-editor/lib/factory-graph-topology-impact";

type FactoryDefinitionLike = NonNullable<DashboardSnapshot["factory"]>;

function factoryWorkTypeStateSkeleton(
  factory: FactoryDefinitionLike,
): string {
  const workTypes = [...(factory.workTypes ?? [])].sort((left, right) =>
    left.name.localeCompare(right.name),
  );

  return workTypes
    .map((workType) => {
      const states = [...(workType.states ?? [])]
        .map((state) => state.name)
        .sort()
        .join(",");
      return `${workType.name}:${states}`;
    })
    .join("|");
}

function factoryWorkstationIds(factory: FactoryDefinitionLike): Set<string> {
  return new Set(
    (factory.workstations ?? [])
      .map((workstation) => workstation.id ?? workstation.name)
      .filter((id): id is string => Boolean(id)),
  );
}

function isSnapshotFactoryStrictWorkstationSuperset(
  snapshotFactory: FactoryDefinitionLike,
  document: CurrentFactoryDocument,
): boolean {
  const snapshotWorkstations = factoryWorkstationIds(snapshotFactory);
  const documentWorkstations = factoryWorkstationIds(document);

  if (snapshotWorkstations.size <= documentWorkstations.size) {
    return false;
  }

  return [...documentWorkstations].every((workstationId) =>
    snapshotWorkstations.has(workstationId),
  );
}

/**
 * Chooses the factory definition that should drive observer-mode graph rendering.
 * Prefers the saved document after current-selection saves and timeline-fixed ticks
 * from the dashboard snapshot when replay projection diverges structurally.
 */
export function resolveObserveModeFactoryDefinition({
  document,
  snapshotFactory,
  timelineMode,
}: {
  document: CurrentFactoryDocument;
  snapshotFactory?: FactoryDefinitionLike;
  timelineMode: FactoryTimelineMode;
}): FactoryDefinitionLike {
  if (!snapshotFactory) {
    return document;
  }

  if (timelineMode === "fixed") {
    return snapshotFactory;
  }

  if (
    !doesFactoryDefinitionChangeAffectGraphTopology(document, snapshotFactory)
  ) {
    return document;
  }

  if (
    factoryWorkTypeStateSkeleton(document) !==
    factoryWorkTypeStateSkeleton(snapshotFactory)
  ) {
    return snapshotFactory;
  }

  if (isSnapshotFactoryStrictWorkstationSuperset(snapshotFactory, document)) {
    return document;
  }

  return document;
}
