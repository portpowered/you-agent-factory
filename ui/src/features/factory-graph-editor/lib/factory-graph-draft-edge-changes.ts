import {
  edgeChangeId,
  type FactoryGraphDraftEdgeChange,
} from "./factory-graph-draft-types";

export function appendUniqueFactoryGraphEdgeChange(
  edges: FactoryGraphDraftEdgeChange[],
  edgeChange: FactoryGraphDraftEdgeChange,
) {
  const nextEdgeId = edgeChangeId(edgeChange);
  if (edges.some((entry) => edgeChangeId(entry) === nextEdgeId)) {
    return edges;
  }
  return [...edges, edgeChange];
}
