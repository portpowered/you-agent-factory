import type {
  DashboardPlaceRef,
  DashboardSnapshot,
  StateCategory,
} from "../../../../api/dashboard/types";

type DashboardFactoryDefinition = NonNullable<DashboardSnapshot["factory"]>;
type LegacyDashboardFactoryDefinition = DashboardFactoryDefinition & {
  work_types?: DashboardFactoryDefinition["workTypes"];
};

function workStatePlaceId(workTypeName: string, stateName: string): string {
  return `${workTypeName}:${stateName}`;
}

function factoryWorkTypes(factory: DashboardFactoryDefinition | undefined) {
  const legacyFactory = factory as LegacyDashboardFactoryDefinition | undefined;
  return factory?.workTypes ?? legacyFactory?.work_types ?? [];
}

function stateCategory(value: string | undefined): StateCategory | undefined {
  return value?.trim() ? value : undefined;
}

function topologyStatePlace(
  snapshot: DashboardSnapshot,
  placeId: string,
): DashboardPlaceRef | null {
  for (const nodeId of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.kind === "work_state" && place.place_id === placeId) {
        return place;
      }
    }
  }

  return null;
}

function factoryStatePlace(
  factory: DashboardFactoryDefinition | undefined,
  placeId: string,
): DashboardPlaceRef | null {
  for (const workType of factoryWorkTypes(factory)) {
    for (const state of workType.states) {
      if (workStatePlaceId(workType.name, state.name) !== placeId) {
        continue;
      }

      return {
        kind: "work_state",
        place_id: placeId,
        state_category: stateCategory(state.type),
        state_value: state.name,
        type_id: workType.name,
      };
    }
  }

  return null;
}

export function findDashboardStatePlace(
  snapshot: DashboardSnapshot,
  placeId: string,
  factoryOverride?: DashboardFactoryDefinition,
): DashboardPlaceRef | null {
  const factory = factoryOverride ?? snapshot.factory;

  return (
    topologyStatePlace(snapshot, placeId) ?? factoryStatePlace(factory, placeId)
  );
}

export function hasDashboardStatePlace(
  snapshot: DashboardSnapshot,
  placeId: string,
  factoryOverride?: DashboardFactoryDefinition,
): boolean {
  return findDashboardStatePlace(snapshot, placeId, factoryOverride) !== null;
}
