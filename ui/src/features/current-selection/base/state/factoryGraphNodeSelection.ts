import type { DashboardSnapshot } from "../../../../api/dashboard";
import { parseFactoryGraphWorkTypeNodeId } from "../../../factory-graph-editor/lib/factory-validation-graph-projection";

type DashboardNodeSelection = {
  kind: "node";
  nodeId: string;
};

function parseFactoryGraphWorkstationNodeId(nodeId: string): string | null {
  const prefix = "workstation:";
  if (!nodeId.startsWith(prefix)) {
    return null;
  }

  const name = nodeId.slice(prefix.length);
  return name.length > 0 ? name : null;
}

function workTypeExistsInFactory(
  factory: DashboardSnapshot["factory"],
  workTypeName: string,
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
  return workTypes.some((workType) => workType.name === workTypeName);
}

function workstationExistsInFactory(
  factory: NonNullable<DashboardSnapshot["factory"]>,
  workstationName: string,
): boolean {
  return (
    factory.workstations?.some(
      (workstation) => workstation.name === workstationName,
    ) ?? false
  );
}

export function factoryGraphWorkStateNodeExistsInSnapshot(
  snapshot: DashboardSnapshot,
  nodeId: string,
): boolean {
  const match = /^work-state:([^:]+):(.+)$/.exec(nodeId);
  if (!match?.[1] || !match[2]) {
    return false;
  }

  const workType = snapshot.factory?.workTypes?.find(
    (candidate) => candidate.name === match[1],
  );

  return (
    workType?.states?.some((state) => state.name === match[2]) ?? false
  );
}

export function resolveFactoryGraphNodeSelection(
  snapshot: DashboardSnapshot,
  selection: DashboardNodeSelection,
  factory: DashboardSnapshot["factory"],
): DashboardNodeSelection | null {
  if (snapshot.topology.workstation_nodes_by_id[selection.nodeId]) {
    const workstationName = parseFactoryGraphWorkstationNodeId(selection.nodeId);
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
