import {
  type FactoryLayout,
  resolveProjectedLayoutPositions,
} from "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import type { GraphLayout } from "../../../flowchart/lib/layout";
import type { GraphNodePositions } from "../../state/currentActivityGraphStore";

export function graphNodePositionsFromCanonicalLayout(
  graphLayout: GraphLayout,
  canonicalLayout: FactoryLayout,
): GraphNodePositions {
  const autoLayoutPositions = new Map(
    graphLayout.nodes.map((node) => [node.nodeId, { x: node.x, y: node.y }]),
  );
  const projected = resolveProjectedLayoutPositions({
    autoLayoutPositionsByNodeId: autoLayoutPositions,
    canonicalLayout,
    nodeIds: graphLayout.nodes.map((node) => node.nodeId),
  });

  return Object.fromEntries(projected);
}

export function mergeLegacyStoredGraphNodePositions(
  canonicalPositions: GraphNodePositions,
  legacyStoredPositions: GraphNodePositions,
): GraphNodePositions {
  return {
    ...legacyStoredPositions,
    ...canonicalPositions,
  };
}
