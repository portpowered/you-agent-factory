import { getBezierPath, type Position } from "@xyflow/react";

export type FactoryGraphEdgeWaypoint = {
  x: number;
  y: number;
};

export function buildFactoryGraphEdgePathThroughWaypoints(input: {
  sourceX: number;
  sourceY: number;
  sourcePosition: Position;
  targetX: number;
  targetY: number;
  targetPosition: Position;
  waypoints?: readonly FactoryGraphEdgeWaypoint[];
}): {
  labelX: number;
  labelY: number;
  path: string;
} {
  const routePoints = [
    { x: input.sourceX, y: input.sourceY },
    ...(input.waypoints ?? []),
    { x: input.targetX, y: input.targetY },
  ];

  if (routePoints.length <= 2) {
    const [path, labelX, labelY] = getBezierPath({
      sourcePosition: input.sourcePosition,
      sourceX: input.sourceX,
      sourceY: input.sourceY,
      targetPosition: input.targetPosition,
      targetX: input.targetX,
      targetY: input.targetY,
    });
    return { labelX, labelY, path };
  }

  const path = routePoints
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x} ${point.y}`)
    .join(" ");
  const midpointIndex = Math.floor((routePoints.length - 1) / 2);
  const labelPoint = routePoints[midpointIndex];
  return {
    labelX: labelPoint.x,
    labelY: labelPoint.y,
    path,
  };
}
