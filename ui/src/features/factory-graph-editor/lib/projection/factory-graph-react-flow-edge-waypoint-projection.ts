import type { Edge } from "@xyflow/react";

import { factoryLayoutEdgeWaypoints } from "../layout/factory-graph-layout-edge-waypoints";
import type { FactoryLayout } from "../layout/factory-graph-layout-operations";
import type { FactoryGraphEdgeKind } from "../draft/factory-graph-draft-types";
import type { FactoryGraphReactFlowEdge } from "./factory-graph-react-flow-projection";

export type FactoryGraphReactFlowEdgeIdentity = {
  id: string;
  kind: FactoryGraphEdgeKind | undefined;
  source: string;
  sourceHandle: string | null | undefined;
  target: string;
  targetHandle: string | null | undefined;
};

export function factoryGraphReactFlowEdgeIdentity(
  edge: FactoryGraphReactFlowEdge | Edge,
): FactoryGraphReactFlowEdgeIdentity {
  const data = edge.data as FactoryGraphReactFlowEdge["data"] | undefined;

  return {
    id: edge.id,
    kind: data?.kind,
    source: edge.source,
    sourceHandle: edge.sourceHandle,
    target: edge.target,
    targetHandle: edge.targetHandle,
  };
}

export function decorateProjectedEdgesWithWaypoints(input: {
  edges: readonly FactoryGraphReactFlowEdge[];
  editorMode: boolean;
  layout: FactoryLayout;
  selectedWaypointEdgeId?: string | null;
}): FactoryGraphReactFlowEdge[] {
  if (!input.editorMode) {
    return [...input.edges];
  }

  return input.edges.map((edge) => {
    const waypoints = factoryLayoutEdgeWaypoints(input.layout, edge.id);
    return {
      ...edge,
      data: {
        ...(edge.data ?? {}),
        waypoints,
      },
      selected: edge.id === input.selectedWaypointEdgeId,
      type: "factoryEditorEdge",
    };
  });
}
