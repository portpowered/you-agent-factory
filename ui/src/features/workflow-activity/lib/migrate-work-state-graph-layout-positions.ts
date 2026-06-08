import type { FactoryGraphWorkStateReference } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { nodeKeyId } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { GraphNodePositions } from "../state/currentActivityGraphStore";

export interface MigrateWorkStateGraphLayoutPositionsInput {
  nextStateName: string;
  positionsByGraphKey: Record<string, GraphNodePositions>;
  previousStateName: string;
  workTypeName: string;
}

export function workStateFactoryGraphNodeId(
  workTypeName: string,
  stateName: string,
): string {
  const key: FactoryGraphWorkStateReference = {
    kind: "work-state",
    stateName,
    workTypeName,
  };
  return nodeKeyId(key);
}

export function migrateWorkStateGraphLayoutPositions({
  nextStateName,
  positionsByGraphKey,
  previousStateName,
  workTypeName,
}: MigrateWorkStateGraphLayoutPositionsInput): Record<
  string,
  GraphNodePositions
> {
  const previousNodeId = workStateFactoryGraphNodeId(
    workTypeName,
    previousStateName,
  );
  const nextNodeId = workStateFactoryGraphNodeId(workTypeName, nextStateName);
  if (previousNodeId === nextNodeId) {
    return positionsByGraphKey;
  }

  const migratedPositionsByGraphKey: Record<string, GraphNodePositions> = {};

  for (const [graphKey, positions] of Object.entries(positionsByGraphKey)) {
    const hasStoredPreviousPosition = positions[previousNodeId] !== undefined;
    const graphKeyReferencesPreviousNode = graphKey.includes(previousNodeId);
    if (!hasStoredPreviousPosition && !graphKeyReferencesPreviousNode) {
      migratedPositionsByGraphKey[graphKey] = positions;
      continue;
    }

    const migratedGraphKey = graphKeyReferencesPreviousNode
      ? graphKey.replaceAll(previousNodeId, nextNodeId)
      : graphKey;
    const migratedPositions = migrateWorkStateNodePositionEntry(
      positions,
      previousNodeId,
      nextNodeId,
    );
    const existingPositions = migratedPositionsByGraphKey[migratedGraphKey];
    migratedPositionsByGraphKey[migratedGraphKey] = existingPositions
      ? { ...existingPositions, ...migratedPositions }
      : migratedPositions;
  }

  return migratedPositionsByGraphKey;
}

function migrateWorkStateNodePositionEntry(
  positions: GraphNodePositions,
  previousNodeId: string,
  nextNodeId: string,
): GraphNodePositions {
  const previousPosition = positions[previousNodeId];
  if (previousPosition === undefined) {
    return positions;
  }

  const migratedPositions = { ...positions, [nextNodeId]: previousPosition };
  delete migratedPositions[previousNodeId];
  return migratedPositions;
}
