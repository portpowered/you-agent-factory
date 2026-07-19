import type {
  FactoryActivityProjection,
  FactoryTopologyConnection,
  FactoryTopologyNode,
} from "@you-agent-factory/factory-replay";

export function connectionHasRenderedEndpoints(
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

export function activityCounts(
  activity: FactoryActivityProjection,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const overlay of activity.activeDispatchOverlays) {
    const nodeIds = new Set([
      overlay.workerNodeId,
      overlay.workstationNodeId,
      ...(overlay.resourceNodeIds ?? []),
    ]);
    for (const nodeId of nodeIds) {
      if (nodeId) counts.set(nodeId, (counts.get(nodeId) ?? 0) + 1);
    }
  }
  return counts;
}
