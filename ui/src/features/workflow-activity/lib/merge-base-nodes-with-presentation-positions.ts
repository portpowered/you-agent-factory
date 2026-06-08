import type { CurrentActivityNode } from "../../flowchart/public";

function positionsEqual(
  left: { x: number; y: number },
  right: { x: number; y: number },
) {
  return left.x === right.x && left.y === right.y;
}

export function mergeBaseNodesWithPresentationPositions(
  baseNodes: CurrentActivityNode[],
  previousBaseNodes: CurrentActivityNode[],
  currentPresentationNodes: CurrentActivityNode[],
): CurrentActivityNode[] {
  const currentPositions = new Map(
    currentPresentationNodes.map((node) => [node.id, node.position]),
  );
  const previousBasePositions = new Map(
    previousBaseNodes.map((node) => [node.id, node.position]),
  );

  return baseNodes.map((node) => {
    const previousBasePosition = previousBasePositions.get(node.id);
    const canonicalPositionChanged =
      previousBasePosition !== undefined &&
      !positionsEqual(previousBasePosition, node.position);

    if (canonicalPositionChanged) {
      return node;
    }

    return {
      ...node,
      position: currentPositions.get(node.id) ?? node.position,
    };
  });
}
