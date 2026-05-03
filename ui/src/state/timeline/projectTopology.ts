import type {
  DashboardPlaceKind,
  DashboardSnapshot,
  DashboardWorkstationEdge,
  StateCategory,
} from "../../api/dashboard";
import type { FactoryPlace } from "../../api/events";
import { uniqueSorted } from "./shared";
import { isSystemTimeWorkType } from "./systemTime";
import type { ProjectedInitialStructure } from "./types";

type FactoryWorkstationShape = NonNullable<ProjectedInitialStructure["workstations"]>[number];

export function projectTopology(topology: ProjectedInitialStructure): DashboardSnapshot["topology"] {
  const workstations = [...(topology.workstations ?? [])].sort((left, right) =>
    left.id.localeCompare(right.id),
  );
  const placesByID = Object.fromEntries(
    (topology.places ?? []).map((place) => [place.id, place]),
  );
  const workTypeIDs = new Set((topology.work_types ?? []).map((workType) => workType.id));
  const resourceIDs = new Set((topology.resources ?? []).map((resource) => resource.id));

  return {
    edges: buildTopologyEdges(workstations, placesByID, workTypeIDs),
    submit_work_types: projectSubmitWorkTypes(topology),
    workstation_node_ids: workstations.map((workstation) => workstation.id),
    workstation_nodes_by_id: Object.fromEntries(
      workstations.map((workstation) => [
        workstation.id,
        projectWorkstation(workstation, placesByID, workTypeIDs, resourceIDs),
      ]),
    ),
  };
}

export function projectSubmitWorkTypes(
  topology: ProjectedInitialStructure,
): NonNullable<DashboardSnapshot["topology"]["submit_work_types"]> {
  return uniqueSorted(
    (topology.work_types ?? [])
      .filter(isSubmitEligibleWorkType)
      .map((workType) => resolveConfiguredWorkTypeName(workType.id, workType.name)),
  )
    .map((workTypeName) => ({ work_type_name: workTypeName }));
}

export function resolveConfiguredWorkTypeName(id: string, name?: string): string {
  return name || id;
}

function isSubmitEligibleWorkType(
  workType: NonNullable<ProjectedInitialStructure["work_types"]>[number],
): boolean {
  if (isSystemTimeWorkType(workType.id)) {
    return false;
  }

  return (workType.states ?? []).some((state) => state.category === "INITIAL");
}

function buildTopologyEdges(
  workstations: FactoryWorkstationShape[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
): DashboardWorkstationEdge[] {
  const inputsByPlace = new Map<string, string[]>();
  for (const workstation of workstations) {
    for (const placeID of workstation.input_place_ids ?? []) {
      const place = placesByID[placeID];
      if (!place || !workTypeIDs.has(place.type_id)) {
        continue;
      }

      inputsByPlace.set(
        placeID,
        [...(inputsByPlace.get(placeID) ?? []), workstation.id].sort(),
      );
    }
  }

  const edges: DashboardWorkstationEdge[] = [];
  const seen = new Set<string>();
  for (const workstation of workstations) {
    edges.push(
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.output_place_ids ?? [],
        "accepted",
        inputsByPlace,
        placesByID,
        seen,
      ),
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.rejection_place_ids ?? [],
        "rejected",
        inputsByPlace,
        placesByID,
        seen,
      ),
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.failure_place_ids ?? [],
        "failed",
        inputsByPlace,
        placesByID,
        seen,
      ),
    );
  }

  return edges;
}

function buildTopologyEdgesForPlaces(
  sourceID: string,
  placeIDs: string[],
  outcome: NonNullable<DashboardWorkstationEdge["outcome_kind"]>,
  inputsByPlace: Map<string, string[]>,
  placesByID: Record<string, FactoryPlace>,
  seen: Set<string>,
): DashboardWorkstationEdge[] {
  return uniqueSorted(placeIDs).flatMap((placeID) => {
    const place = placesByID[placeID];
    if (!place) {
      return [];
    }

    return (inputsByPlace.get(placeID) ?? []).flatMap((destID) => {
      const edgeID = `${sourceID}:${destID}:${placeID}:${outcome}`;
      if (seen.has(edgeID)) {
        return [];
      }

      seen.add(edgeID);
      return [
        {
          edge_id: edgeID,
          from_node_id: sourceID,
          outcome_kind: outcome,
          state_category: place.category as StateCategory | undefined,
          state_value: place.state,
          to_node_id: destID,
          via_place_id: placeID,
          work_type_id: place.type_id,
        },
      ];
    });
  });
}

function projectWorkstation(
  workstation: FactoryWorkstationShape,
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
  resourceIDs: Set<string>,
): DashboardSnapshot["topology"]["workstation_nodes_by_id"][string] {
  const outputPlaceIDs = [
    ...(workstation.output_place_ids ?? []),
    ...(workstation.rejection_place_ids ?? []),
    ...(workstation.failure_place_ids ?? []),
  ];

  return {
    input_place_ids: workstation.input_place_ids,
    input_places: placeRefs(
      workstation.input_place_ids ?? [],
      placesByID,
      workTypeIDs,
      resourceIDs,
    ),
    input_work_type_ids: workTypeIDsForPlaces(
      workstation.input_place_ids ?? [],
      placesByID,
      workTypeIDs,
    ),
    node_id: workstation.id,
    output_place_ids: outputPlaceIDs,
    output_places: placeRefs(outputPlaceIDs, placesByID, workTypeIDs, resourceIDs),
    output_work_type_ids: workTypeIDsForPlaces(
      outputPlaceIDs,
      placesByID,
      workTypeIDs,
    ),
    transition_id: workstation.id,
    worker_type: workstation.worker_id,
    workstation_kind: workstation.kind,
    workstation_name: workstation.name,
  };
}

function placeRefs(
  ids: string[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
  resourceIDs: Set<string>,
): NonNullable<DashboardSnapshot["topology"]["workstation_nodes_by_id"][string]["input_places"]> {
  return uniqueSorted(ids).flatMap((id) => {
    const place = placesByID[id];
    if (!place) {
      return [];
    }

    const kind: DashboardPlaceKind = workTypeIDs.has(place.type_id)
      ? "work_state"
      : resourceIDs.has(place.type_id)
        ? "resource"
        : "constraint";

    return [
      {
        kind,
        place_id: place.id,
        state_category: place.category as StateCategory | undefined,
        state_value: place.state,
        type_id: place.type_id,
      },
    ];
  });
}

function workTypeIDsForPlaces(
  ids: string[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
): string[] {
  return uniqueSorted(
    ids
      .map((id) => placesByID[id]?.type_id ?? "")
      .filter((id) => workTypeIDs.has(id)),
  );
}
