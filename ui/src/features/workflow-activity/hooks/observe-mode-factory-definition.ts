import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { FactoryTimelineMode } from "../../timeline/state/factoryTimelineStore";
import { doesFactoryDefinitionChangeAffectGraphTopology } from "../../factory-graph-editor/lib/factory-graph-topology-impact";

type FactoryDefinitionLike = NonNullable<DashboardSnapshot["factory"]>;

function workTypeStateMap(
  factory: FactoryDefinitionLike,
): Map<string, Set<string>> {
  const statesByWorkType = new Map<string, Set<string>>();

  for (const workType of factory.workTypes ?? []) {
    statesByWorkType.set(
      workType.name,
      new Set((workType.states ?? []).map((state) => state.name)),
    );
  }

  return statesByWorkType;
}

function isSavedDocumentMinimalReplayStub(
  document: CurrentFactoryDocument,
): boolean {
  return (
    (document.workTypes?.length ?? 0) === 0 &&
    (document.workstations?.length ?? 0) === 0 &&
    (document.workers?.length ?? 0) === 0 &&
    (document.resources?.length ?? 0) === 0
  );
}

function isDocumentWorkTypeStructureAheadOfSnapshot(
  document: CurrentFactoryDocument,
  snapshotFactory: FactoryDefinitionLike,
): boolean {
  const documentWorkTypes = workTypeStateMap(document);
  const snapshotWorkTypes = workTypeStateMap(snapshotFactory);

  if (snapshotWorkTypes.size === 0) {
    return documentWorkTypes.size > 0;
  }

  for (const [workTypeName, snapshotStates] of snapshotWorkTypes) {
    const documentStates = documentWorkTypes.get(workTypeName);
    if (!documentStates) {
      return false;
    }

    for (const stateName of snapshotStates) {
      if (!documentStates.has(stateName)) {
        return false;
      }
    }
  }

  if (documentWorkTypes.size > snapshotWorkTypes.size) {
    return true;
  }

  for (const [workTypeName, documentStates] of documentWorkTypes) {
    const snapshotStates = snapshotWorkTypes.get(workTypeName);
    if (!snapshotStates) {
      continue;
    }

    for (const stateName of documentStates) {
      if (!snapshotStates.has(stateName)) {
        return true;
      }
    }
  }

  return false;
}

function isSnapshotWorkTypeReplayProjection(
  document: CurrentFactoryDocument,
  snapshotFactory: FactoryDefinitionLike,
): boolean {
  const documentWorkTypes = workTypeStateMap(document);
  const snapshotWorkTypes = workTypeStateMap(snapshotFactory);

  return [...snapshotWorkTypes].some(([workTypeName, snapshotStates]) => {
    const documentStates = documentWorkTypes.get(workTypeName);
    if (!documentStates) {
      return snapshotStates.size > 0;
    }

    return [...snapshotStates].some(
      (stateName) => !documentStates.has(stateName),
    );
  });
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

  if (documentWorkstations.size === 0) {
    return false;
  }

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

  if (isSavedDocumentMinimalReplayStub(document)) {
    return snapshotFactory;
  }

  if (
    !doesFactoryDefinitionChangeAffectGraphTopology(document, snapshotFactory)
  ) {
    return document;
  }

  if (isDocumentWorkTypeStructureAheadOfSnapshot(document, snapshotFactory)) {
    return document;
  }

  if (isSnapshotWorkTypeReplayProjection(document, snapshotFactory)) {
    return snapshotFactory;
  }

  if (isSnapshotFactoryStrictWorkstationSuperset(snapshotFactory, document)) {
    return document;
  }

  return document;
}
