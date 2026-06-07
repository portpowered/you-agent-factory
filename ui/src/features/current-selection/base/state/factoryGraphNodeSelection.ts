import type { DashboardSnapshot } from "../../../../api/dashboard";
import {
  parseFactoryGraphWorkStateNodeId,
  parseFactoryGraphWorkstationNodeId,
  parseFactoryGraphWorkTypeNodeId,
} from "../../../factory-graph-editor/lib/factory-graph-draft-types";

type DashboardNodeSelection = {
  kind: "node";
  nodeId: string;
};

function workTypeExistsInFactory(
  factory: DashboardSnapshot["factory"],
  workTypeId: string,
): boolean {
  if (!factory) {
    return false;
  }

  type LegacyDashboardFactoryDefinition = NonNullable<
    DashboardSnapshot["factory"]
  > & {
    work_types?: NonNullable<DashboardSnapshot["factory"]>["workTypes"];
  };
  const legacyFactory = factory as LegacyDashboardFactoryDefinition;
  const workTypes = factory.workTypes ?? legacyFactory.work_types ?? [];
  return workTypes.some(
    (workType) => workType.id === workTypeId || workType.name === workTypeId,
  );
}

function workstationExistsInFactory(
  factory: NonNullable<DashboardSnapshot["factory"]>,
  workstationId: string,
): boolean {
  return (
    factory.workstations?.some(
      (workstation) =>
        workstation.id === workstationId || workstation.name === workstationId,
    ) ?? false
  );
}

export function factoryGraphWorkStateNodeExistsInSnapshot(
  snapshot: DashboardSnapshot,
  nodeId: string,
): boolean {
  const subjectId = parseFactoryGraphWorkStateNodeId(nodeId);
  if (!subjectId) {
    return false;
  }
  const separatorIndex = subjectId.indexOf(":");
  if (separatorIndex <= 0 || separatorIndex >= subjectId.length - 1) {
    return false;
  }
  const workTypeId = subjectId.slice(0, separatorIndex);
  const stateId = subjectId.slice(separatorIndex + 1);

  const workType = snapshot.factory?.workTypes?.find(
    (candidate) => candidate.id === workTypeId || candidate.name === workTypeId,
  );

  return (
    workType?.states?.some(
      (state) => state.id === stateId || state.name === stateId,
    ) ?? false
  );
}

export function resolveFactoryGraphNodeSelection(
  snapshot: DashboardSnapshot,
  selection: DashboardNodeSelection,
  factory: DashboardSnapshot["factory"],
): DashboardNodeSelection | null {
  if (snapshot.topology.workstation_nodes_by_id[selection.nodeId]) {
    const workstationName = parseFactoryGraphWorkstationNodeId(
      selection.nodeId,
    );
    if (
      workstationName &&
      factory &&
      !workstationExistsInFactory(factory, workstationName)
    ) {
      return null;
    }

    return selection;
  }

  const workTypeName = parseFactoryGraphWorkTypeNodeId(selection.nodeId);
  if (workTypeName && workTypeExistsInFactory(factory, workTypeName)) {
    return selection;
  }

  if (factoryGraphWorkStateNodeExistsInSnapshot(snapshot, selection.nodeId)) {
    return selection;
  }

  return null;
}
