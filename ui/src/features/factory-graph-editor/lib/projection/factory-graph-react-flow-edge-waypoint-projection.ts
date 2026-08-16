import type { Edge } from "@xyflow/react";
import type { FactoryGraphEdgeKind } from "../draft/factory-graph-draft-types";
import { factoryLayoutEdgeWaypoints } from "../layout/factory-graph-layout-edge-waypoints";
import type { FactoryLayout } from "../layout/factory-graph-layout-operations";
import type { FactoryGraphReactFlowEdge } from "./factory-graph-react-flow-projection";

export type FactoryGraphReactFlowEdgeIdentity = {
  id: string;
  kind: FactoryGraphEdgeKind | undefined;
  source: string;
  sourceHandle: string | null | undefined;
  target: string;
  targetHandle: string | null | undefined;
};

type WaypointDecoratedEdgeData = NonNullable<
  FactoryGraphReactFlowEdge["data"]
> & {
  factoryGraphEdgeId?: string;
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
  layout: FactoryLayout;
  waypointPreviews?: ReadonlyMap<string, readonly { x: number; y: number }[]>;
  selectedWaypointEdgeId?: string | null;
}): FactoryGraphReactFlowEdge[] {
  return input.edges.map((edge): FactoryGraphReactFlowEdge => {
    const data = (edge.data ?? {}) as Partial<WaypointDecoratedEdgeData>;
    const layoutEdgeId = data.factoryGraphEdgeId ?? edge.id;
    const previewWaypoints = input.waypointPreviews?.get(layoutEdgeId);
    const waypoints =
      previewWaypoints?.map((point) => ({ x: point.x, y: point.y })) ??
      factoryLayoutEdgeWaypoints(input.layout, layoutEdgeId);
    const selected =
      edge.id === input.selectedWaypointEdgeId ||
      layoutEdgeId === input.selectedWaypointEdgeId;
    if (!waypoints && !selected) {
      return edge;
    }

    return {
      ...edge,
      data: {
        ...data,
        active: data.active ?? false,
        alwaysShowLabel: data.alwaysShowLabel ?? false,
        kind: data.kind,
        label: data.label ?? (typeof edge.label === "string" ? edge.label : ""),
        pendingStatus: data.pendingStatus ?? "none",
        waypoints,
      },
      selected,
      type: "factoryEditorEdge",
    };
  });
}
