import type { Edge, Node } from "@xyflow/react";
import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import type {
  FactoryActivityProjection,
  FactoryLoadProjection,
  FactoryTopologyConnection,
  FactoryTopologyNode,
} from "@you-agent-factory/factory-replay";
import { activeWorkByWorkstationNode } from "./factory-topology-active-work";
import type {
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayProjection,
} from "./factory-topology-replay";
import type {
  AnnotationNodeData,
  TopologyNodeData,
} from "./factory-topology-replay-nodes";
import { FactoryVisualizerInternalError } from "./visualizer-error";

export interface FactoryTopologyFlowProjection {
  edges: Edge[];
  nodes: Node<TopologyNodeData | AnnotationNodeData>[];
  validEndpoints: boolean;
}

const columnByKind: Record<FactoryTopologyNode["kind"], number> = {
  resource: 0,
  worker: 1,
  "work-type": 2,
  "work-state": 3,
  workstation: 4,
};

/** Convert immutable replay projections into disposable React Flow view data. */
export function projectFactoryTopologyFlow(
  projection: FactoryTopologyReplayProjection,
  messages: FactoryTopologyReplayMessages,
  selectedNodeId: string | undefined,
  onSelectNode: ((node: FactoryTopologyNode) => void) | undefined,
  prefersReducedMotion = false,
  layout?: FactoryVisualizationLayoutV1,
): FactoryTopologyFlowProjection {
  try {
    const { connections, nodes: topologyNodes } = projection.topology;
    const nodeById = new Map(topologyNodes.map((node) => [node.id, node]));
    const validEndpoints = connections.every((connection) =>
      connectionHasRenderedEndpoints(connection, nodeById),
    );
    const nodeData = nodePresentationData(projection, connections, layout);
    return {
      edges: validEndpoints
        ? projectEdges(connections, projection.activity, prefersReducedMotion)
        : [],
      nodes: [
        ...projectTopologyNodes(
          topologyNodes,
          messages,
          selectedNodeId,
          onSelectNode,
          nodeData,
        ),
        ...projectAnnotations(layout, messages),
      ],
      validEndpoints,
    };
  } catch (error) {
    if (error instanceof FactoryVisualizerInternalError) throw error;
    throw new FactoryVisualizerInternalError("projection", error);
  }
}

function nodePresentationData(
  projection: FactoryTopologyReplayProjection,
  connections: readonly FactoryTopologyConnection[],
  layout: FactoryVisualizationLayoutV1 | undefined,
) {
  return {
    activeWorkByNode: activeWorkByWorkstationNode(projection.activity),
    activeDetailNodeIds: activityDetailNodeIds(
      projection.activity,
      connections,
      projection.load.workStateCounts,
    ),
    activityCountByNode: activityCounts(projection.activity),
    emptyStateByNode: new Map(
      (layout?.nodeEmptyStates ?? []).map((state) => [
        state.nodeId,
        state.content,
      ]),
    ),
    occupancyByNode: new Map(
      projection.load.resourceOccupancy.map((occupancy) => [
        occupancy.resourceNodeId,
        occupancy,
      ]),
    ),
    workStateCountByNode: new Map(
      projection.load.workStateCounts.map((count) => [
        count.workStateNodeId,
        count,
      ]),
    ),
  };
}

function projectTopologyNodes(
  topologyNodes: readonly FactoryTopologyNode[],
  messages: FactoryTopologyReplayMessages,
  selectedNodeId: string | undefined,
  onSelectNode: ((node: FactoryTopologyNode) => void) | undefined,
  data: ReturnType<typeof nodePresentationData>,
): Node<TopologyNodeData>[] {
  const rowByKind = new Map<FactoryTopologyNode["kind"], number>();
  return topologyNodes.map((node) => {
    const row = rowByKind.get(node.kind) ?? 0;
    rowByKind.set(node.kind, row + 1);
    const occupancy = data.occupancyByNode.get(node.id);
    const workStateCount = data.workStateCountByNode.get(node.id);
    return {
      data: {
        activityCount: data.activityCountByNode.get(node.id) ?? 0,
        activeWorkItems: data.activeWorkByNode.get(node.id) ?? [],
        ...(data.activeDetailNodeIds.has(node.id)
          ? {}
          : { emptyState: data.emptyStateByNode.get(node.id) }),
        messages,
        node,
        ...(occupancy
          ? {
              occupancy: {
                capacity: occupancy.capacity,
                evidence: occupancy.evidence,
                occupied: occupancy.occupiedQuantity,
              },
            }
          : {}),
        onSelectNode,
        selected: selectedNodeId === node.id,
        showNodeKinds: true,
        ...(workStateCount
          ? {
              workStateCount: {
                count: workStateCount.count,
                evidence: workStateCount.evidence,
              },
            }
          : {}),
      },
      draggable: false,
      id: node.id,
      position: layoutNode(node.kind, row),
      selectable: false,
      type: "factoryTopologyNode",
    };
  });
}

function projectAnnotations(
  layout: FactoryVisualizationLayoutV1 | undefined,
  messages: FactoryTopologyReplayMessages,
): Node<AnnotationNodeData>[] {
  return (layout?.annotations ?? []).map((annotation) => ({
    data: { annotation, messages },
    draggable: false,
    id: `annotation:${annotation.id}`,
    position: annotation.position,
    selectable: false,
    type: "factoryTopologyAnnotation",
    ...(annotation.size
      ? {
          style: {
            height: annotation.size.height,
            width: annotation.size.width,
          },
        }
      : {}),
  }));
}

function projectEdges(
  connections: readonly FactoryTopologyConnection[],
  activity: FactoryActivityProjection,
  prefersReducedMotion: boolean,
): Edge[] {
  return connections.map((connection) => ({
    animated:
      !prefersReducedMotion &&
      activity.activeDispatchOverlays.some((overlay) =>
        overlay.connectionIds.includes(connection.id),
      ),
    data: { relationship: connection.kind },
    id: connection.id,
    source: connection.source.nodeId,
    sourceHandle: connection.source.handleId,
    target: connection.target.nodeId,
    targetHandle: connection.target.handleId,
  }));
}

function connectionHasRenderedEndpoints(
  connection: FactoryTopologyConnection,
  nodeById: ReadonlyMap<string, FactoryTopologyNode>,
): boolean {
  const source = nodeById.get(connection.source.nodeId);
  const target = nodeById.get(connection.target.nodeId);
  return Boolean(
    source?.handles.some(
      (handle) =>
        handle.id === connection.source.handleId && handle.role === "source",
    ) &&
      target?.handles.some(
        (handle) =>
          handle.id === connection.target.handleId && handle.role === "target",
      ),
  );
}

function activityCounts(
  activity: FactoryActivityProjection,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const overlay of activity.activeDispatchOverlays) {
    for (const nodeId of new Set([
      overlay.workerNodeId,
      overlay.workstationNodeId,
      ...(overlay.resourceNodeIds ?? []),
    ])) {
      if (nodeId) counts.set(nodeId, (counts.get(nodeId) ?? 0) + 1);
    }
  }
  return counts;
}

function activityDetailNodeIds(
  activity: FactoryActivityProjection,
  connections: readonly FactoryTopologyConnection[],
  workStateCounts: FactoryLoadProjection["workStateCounts"],
): Set<string> {
  const nodeIds = new Set(activity.activeWorkstationNodeIds);
  const connectionById = new Map(
    connections.map((connection) => [connection.id, connection]),
  );
  for (const state of workStateCounts)
    if (
      state.evidence === "known" &&
      typeof state.count === "number" &&
      state.count > 0
    )
      nodeIds.add(state.workStateNodeId);
  for (const overlay of activity.activeDispatchOverlays) {
    for (const nodeId of [
      overlay.workerNodeId,
      overlay.workstationNodeId,
      ...(overlay.resourceNodeIds ?? []),
    ])
      if (nodeId) nodeIds.add(nodeId);
    for (const connectionId of overlay.connectionIds) {
      const connection = connectionById.get(connectionId);
      if (connection) {
        nodeIds.add(connection.source.nodeId);
        nodeIds.add(connection.target.nodeId);
      }
    }
  }
  return nodeIds;
}

function layoutNode(
  kind: FactoryTopologyNode["kind"],
  row: number,
): { x: number; y: number } {
  const column = columnByKind[kind];
  if (column === undefined || !Number.isSafeInteger(row) || row < 0)
    throw new FactoryVisualizerInternalError("layout");
  return { x: column * 260, y: row * 170 };
}
