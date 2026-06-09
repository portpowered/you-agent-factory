import { getBezierPath, Position } from "@xyflow/react";

export type FactoryGraphEdgeWaypoint = {
  x: number;
  y: number;
};

function bezierHandleDistance(
  start: FactoryGraphEdgeWaypoint,
  end: FactoryGraphEdgeWaypoint,
): number {
  return Math.max(
    40,
    Math.min(160, Math.hypot(end.x - start.x, end.y - start.y) / 2),
  );
}

function controlPointFromPosition(
  point: FactoryGraphEdgeWaypoint,
  position: Position,
  distance: number,
): FactoryGraphEdgeWaypoint {
  switch (position) {
    case Position.Left:
      return { x: point.x - distance, y: point.y };
    case Position.Right:
      return { x: point.x + distance, y: point.y };
    case Position.Top:
      return { x: point.x, y: point.y - distance };
    case Position.Bottom:
      return { x: point.x, y: point.y + distance };
  }
}

function segmentPositions(input: {
  end: FactoryGraphEdgeWaypoint;
  start: FactoryGraphEdgeWaypoint;
}): {
  endPosition: Position;
  startPosition: Position;
} {
  const deltaX = input.end.x - input.start.x;
  const deltaY = input.end.y - input.start.y;
  if (Math.abs(deltaX) >= Math.abs(deltaY)) {
    return deltaX >= 0
      ? { endPosition: Position.Left, startPosition: Position.Right }
      : { endPosition: Position.Right, startPosition: Position.Left };
  }

  return deltaY >= 0
    ? { endPosition: Position.Top, startPosition: Position.Bottom }
    : { endPosition: Position.Bottom, startPosition: Position.Top };
}

function buildWaypointBezierPath(input: {
  routePoints: readonly FactoryGraphEdgeWaypoint[];
  sourcePosition: Position;
  targetPosition: Position;
}): string {
  const lastPointIndex = input.routePoints.length - 1;
  const [source] = input.routePoints;
  const segments = [`M ${source.x} ${source.y}`];

  for (let index = 0; index < lastPointIndex; index += 1) {
    const start = input.routePoints[index];
    const end = input.routePoints[index + 1];
    const distance = bezierHandleDistance(start, end);
    const positions = segmentPositions({ end, start });
    const firstControl =
      index === 0
        ? controlPointFromPosition(start, input.sourcePosition, distance)
        : controlPointFromPosition(start, positions.startPosition, distance);
    const secondControl =
      index + 1 === lastPointIndex
        ? controlPointFromPosition(end, input.targetPosition, distance)
        : controlPointFromPosition(end, positions.endPosition, distance);

    segments.push(
      `C ${firstControl.x} ${firstControl.y}, ${secondControl.x} ${secondControl.y}, ${end.x} ${end.y}`,
    );
  }

  return segments.join(" ");
}

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

  const path = buildWaypointBezierPath({
    routePoints,
    sourcePosition: input.sourcePosition,
    targetPosition: input.targetPosition,
  });
  const midpointIndex = Math.floor((routePoints.length - 1) / 2);
  const labelPoint = routePoints[midpointIndex];
  return {
    labelX: labelPoint.x,
    labelY: labelPoint.y,
    path,
  };
}
