import type { GraphLayout } from "../../flowchart/lib/layout";

export function currentActivityGraphKey(graphLayout: GraphLayout): string {
  if (graphLayout.nodes.length === 0) {
    return "";
  }

  const nodeIds = graphLayout.nodes
    .map((node) => node.nodeId)
    .sort()
    .join("|");
  const edgeIds = graphLayout.edges
    .map((edge) => edge.edgeId)
    .sort()
    .join("|");
  return `${nodeIds}::${edgeIds}`;
}

export function graphKeyAfterAddingNode(
  graphKey: string,
  newNodeId: string,
): string {
  if (!graphKey) {
    return newNodeId;
  }

  const separatorIndex = graphKey.indexOf("::");
  const nodePart =
    separatorIndex === -1 ? graphKey : graphKey.slice(0, separatorIndex);
  const edgePart =
    separatorIndex === -1 ? "" : graphKey.slice(separatorIndex + 2);
  const nodeIds = nodePart.split("|").filter((nodeId) => nodeId.length > 0);

  nodeIds.push(newNodeId);
  nodeIds.sort();

  return `${nodeIds.join("|")}::${edgePart}`;
}
