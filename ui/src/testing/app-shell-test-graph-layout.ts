import type { DashboardPlaceRef, DashboardTopology } from "../api/dashboard";

export function buildDashboardTestGraphLayout(topology: DashboardTopology) {
  const placesById = new Map<string, DashboardPlaceRef>();

  for (const nodeId of topology.workstation_node_ids) {
    const workstation = topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      placesById.set(place.place_id, place);
    }
  }

  for (const edge of topology.edges ?? []) {
    if (!placesById.has(edge.via_place_id)) {
      placesById.set(edge.via_place_id, {
        kind: "work_state",
        place_id: edge.via_place_id,
        state_category: edge.state_category,
        state_value: edge.state_value,
        type_id: edge.work_type_id,
      });
    }
  }

  const placeNodes = [...placesById.values()]
    .sort((left, right) => left.place_id.localeCompare(right.place_id))
    .map((place, index) => ({
      column: 0,
      height: place.kind === "resource" ? 86 : place.kind === "constraint" ? 58 : 86,
      nodeId: `place:${place.place_id}`,
      nodeKind:
        place.kind === "resource"
          ? "resource"
          : place.kind === "constraint"
            ? "constraint"
            : "state_position",
      place,
      row: index,
      width: place.kind === "resource" ? 168 : place.kind === "constraint" ? 156 : 164,
      x: 0,
      y: index * 140,
    }));

  const workstationNodes = topology.workstation_node_ids.map((nodeId, index) => ({
    column: 1,
    height: 196,
    nodeId: `workstation:${nodeId}`,
    nodeKind: "workstation" as const,
    row: index,
    width: 156,
    workstationNodeId: nodeId,
    x: 280,
    y: index * 240,
  }));

  const edges = (topology.edges ?? []).map((edge, index) => ({
    edgeId: `edge:${index}`,
    fromNodeId: `workstation:${edge.from_node_id}`,
    label: edge.state_value ?? edge.work_type_id ?? "",
    labelX: 0,
    labelY: 0,
    outcomeKind: edge.outcome_kind ?? "accepted",
    path: "",
    sourcePlaceKind: undefined,
    stateCategory: edge.state_category,
    targetPlaceKind: "work_state" as const,
    toNodeId: `place:${edge.via_place_id}`,
  }));

  return {
    edges,
    height: Math.max(
      600,
      ...placeNodes.map((node) => node.y + node.height),
      ...workstationNodes.map((node) => node.y + node.height),
    ),
    nodes: [...placeNodes, ...workstationNodes],
    width: 1000,
  };
}
