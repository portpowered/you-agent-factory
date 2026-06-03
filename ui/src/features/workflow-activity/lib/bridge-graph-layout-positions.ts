import type {
  GraphNodePosition,
  GraphNodePositions,
} from "../state/currentActivityGraphStore";

function finitePosition(
  position: GraphNodePosition | undefined,
): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

export function nodeIdsFromGraphKey(graphKey: string): ReadonlySet<string> {
  const separatorIndex = graphKey.indexOf("::");
  const nodePart =
    separatorIndex === -1 ? graphKey : graphKey.slice(0, separatorIndex);
  return new Set(nodePart.split("|").filter((nodeId) => nodeId.length > 0));
}

export function findBridgedNodePosition(
  positionsByGraphKey: Record<string, GraphNodePositions>,
  targetGraphKey: string,
  nodeId: string,
): GraphNodePosition | undefined {
  const direct = positionsByGraphKey[targetGraphKey]?.[nodeId];
  if (finitePosition(direct)) {
    return direct;
  }

  const targetNodeIds = nodeIdsFromGraphKey(targetGraphKey);
  let bestMatch: { overlap: number; position: GraphNodePosition } | undefined;

  for (const [graphKey, positions] of Object.entries(positionsByGraphKey)) {
    if (graphKey === targetGraphKey) {
      continue;
    }

    if (!nodeIdsFromGraphKey(graphKey).has(nodeId)) {
      continue;
    }

    const position = positions[nodeId];
    if (!finitePosition(position)) {
      continue;
    }

    const overlap = [...targetNodeIds].filter((candidateNodeId) =>
      nodeIdsFromGraphKey(graphKey).has(candidateNodeId),
    ).length;
    if (!bestMatch || overlap > bestMatch.overlap) {
      bestMatch = { overlap, position };
    }
  }

  return bestMatch?.position;
}

export function resolveStoredNodePositionsForGraphKey(
  positionsByGraphKey: Record<string, GraphNodePositions>,
  graphKey: string,
  nodeIds: readonly string[],
): GraphNodePositions {
  const direct = positionsByGraphKey[graphKey] ?? {};
  const resolved: GraphNodePositions = { ...direct };

  for (const nodeId of nodeIds) {
    if (finitePosition(resolved[nodeId])) {
      continue;
    }

    const bridged = findBridgedNodePosition(
      positionsByGraphKey,
      graphKey,
      nodeId,
    );
    if (bridged) {
      resolved[nodeId] = bridged;
    }
  }

  return resolved;
}

export function bridgeGraphLayoutPositions({
  nodeIds,
  positionsByGraphKey,
  targetGraphKey,
}: {
  nodeIds: readonly string[];
  positionsByGraphKey: Record<string, GraphNodePositions>;
  targetGraphKey: string;
}): Record<string, GraphNodePositions> | null {
  if (!targetGraphKey) {
    return null;
  }

  const targetPositions = { ...(positionsByGraphKey[targetGraphKey] ?? {}) };
  let changed = false;

  for (const nodeId of nodeIds) {
    if (finitePosition(targetPositions[nodeId])) {
      continue;
    }

    const bridged = findBridgedNodePosition(
      positionsByGraphKey,
      targetGraphKey,
      nodeId,
    );
    if (!bridged) {
      continue;
    }

    targetPositions[nodeId] = bridged;
    changed = true;
  }

  if (!changed) {
    return null;
  }

  return {
    ...positionsByGraphKey,
    [targetGraphKey]: targetPositions,
  };
}
