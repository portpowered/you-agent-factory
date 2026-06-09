import {
  type FactoryLayout,
  moveFactoryLayoutNode,
  resolveProjectedLayoutPositions,
} from "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import type { GraphLayout } from "../../../flowchart/lib/layout";
import type {
  GraphNodePosition,
  GraphNodePositions,
} from "../../state/currentActivityGraphStore";

function finitePosition(
  position: GraphNodePosition | undefined,
): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

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

export function overlayGraphNodePositions(
  canonicalPositions: GraphNodePositions,
  overlayPositions: GraphNodePositions,
): GraphNodePositions {
  return {
    ...canonicalPositions,
    ...overlayPositions,
  };
}

export function selectHydratableGraphNodePositions(
  canonicalPositions: GraphNodePositions,
  overlayPositions: GraphNodePositions,
): GraphNodePositions {
  const hydratablePositions: GraphNodePositions = {};

  for (const [nodeId, overlayPosition] of Object.entries(overlayPositions)) {
    if (!finitePosition(overlayPosition)) {
      continue;
    }

    const canonicalPosition = canonicalPositions[nodeId];
    if (
      finitePosition(canonicalPosition) &&
      canonicalPosition.x === overlayPosition.x &&
      canonicalPosition.y === overlayPosition.y
    ) {
      continue;
    }

    hydratablePositions[nodeId] = overlayPosition;
  }

  return hydratablePositions;
}

export function mergeFactoryLayoutWithNodePositions(
  layout: FactoryLayout,
  positions: GraphNodePositions,
): FactoryLayout {
  let nextLayout = structuredClone(layout);

  for (const [nodeId, position] of Object.entries(positions)) {
    if (!finitePosition(position)) {
      continue;
    }

    nextLayout = moveFactoryLayoutNode(nextLayout, nodeId, position);
  }

  return nextLayout;
}
