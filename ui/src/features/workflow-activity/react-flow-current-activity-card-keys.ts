import type { DashboardTopology } from "../../api/dashboard/types";
import type { GraphLayout } from "../flowchart/layout";

export function currentActivityGraphKey(graphLayout: GraphLayout): string {
  if (graphLayout.nodes.length === 0) {
    return "";
  }

  const nodeIds = graphLayout.nodes.map((node) => node.nodeId).sort().join("|");
  const edgeIds = graphLayout.edges.map((edge) => edge.edgeId).sort().join("|");
  return `${nodeIds}::${edgeIds}`;
}

export function currentActivityTopologyKey(topology: DashboardTopology): string {
  return JSON.stringify({
    edges: [...(topology.edges ?? [])]
      .map((edge) => ({
        from_node_id: edge.from_node_id,
        outcome_kind: edge.outcome_kind ?? "accepted",
        state_category: edge.state_category ?? "",
        state_value: edge.state_value ?? "",
        to_node_id: edge.to_node_id,
        via_place_id: edge.via_place_id,
        work_type_id: edge.work_type_id ?? "",
      }))
      .sort((left, right) => JSON.stringify(left).localeCompare(JSON.stringify(right))),
    workstations: [...topology.workstation_node_ids]
      .sort()
      .map((nodeId) => {
        const workstation = topology.workstation_nodes_by_id[nodeId];
        return {
          input_places: [...(workstation?.input_places ?? [])]
            .map((place) => ({
              kind: place.kind,
              place_id: place.place_id,
              state_category: place.state_category ?? "",
              state_value: place.state_value ?? "",
              type_id: place.type_id ?? "",
            }))
            .sort((left, right) => left.place_id.localeCompare(right.place_id)),
          node_id: workstation?.node_id ?? nodeId,
          output_places: [...(workstation?.output_places ?? [])]
            .map((place) => ({
              kind: place.kind,
              place_id: place.place_id,
              state_category: place.state_category ?? "",
              state_value: place.state_value ?? "",
              type_id: place.type_id ?? "",
            }))
            .sort((left, right) => left.place_id.localeCompare(right.place_id)),
          transition_id: workstation?.transition_id ?? "",
          workstation_kind: workstation?.workstation_kind ?? "",
          workstation_name: workstation?.workstation_name ?? "",
        };
      }),
  });
}

