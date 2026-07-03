import { getBezierPath, Position } from "@xyflow/react";

export type GraphEdgeWaypoint = {
  x: number;
  y: number;
};

const CATMULL_ROM_ALPHA = 0.5;
const MIN_PARAMETER_DISTANCE = 0.0001;
const VIRTUAL_ENDPOINT_DISTANCE = 96;

function positionVector(position: Position): GraphEdgeWaypoint {
  switch (position) {
    case Position.Left:
      return { x: -1, y: 0 };
    case Position.Right:
      return { x: 1, y: 0 };
    case Position.Top:
      return { x: 0, y: -1 };
    case Position.Bottom:
      return { x: 0, y: 1 };
  }
}

function pointDistance(
  first: GraphEdgeWaypoint,
  second: GraphEdgeWaypoint,
): number {
  return Math.hypot(second.x - first.x, second.y - first.y);
}

function catmullRomParameterDistance(
  first: GraphEdgeWaypoint,
  second: GraphEdgeWaypoint,
): number {
  return Math.max(
    MIN_PARAMETER_DISTANCE,
    pointDistance(first, second) ** CATMULL_ROM_ALPHA,
  );
}

function formatPathNumber(value: number): string {
  if (Object.is(value, -0)) {
    return "0";
  }

  return Number.isInteger(value) ? `${value}` : value.toFixed(3);
}

function compactConsecutiveRoutePoints(
  routePoints: readonly GraphEdgeWaypoint[],
): GraphEdgeWaypoint[] {
  const compacted: GraphEdgeWaypoint[] = [];

  for (const point of routePoints) {
    const previous = compacted.at(-1);
    if (previous && previous.x === point.x && previous.y === point.y) {
      continue;
    }

    compacted.push(point);
  }

  return compacted;
}

function virtualEndpoint(input: {
  direction: GraphEdgeWaypoint;
  from: GraphEdgeWaypoint;
  mode: "after" | "before";
  neighbor: GraphEdgeWaypoint;
}): GraphEdgeWaypoint {
  const distance = Math.max(
    VIRTUAL_ENDPOINT_DISTANCE,
    pointDistance(input.from, input.neighbor),
  );
  const scalar = input.mode === "before" ? -distance : distance;

  return {
    x: input.from.x + input.direction.x * scalar,
    y: input.from.y + input.direction.y * scalar,
  };
}

function catmullRomTangent(input: {
  current: GraphEdgeWaypoint;
  next: GraphEdgeWaypoint;
  previous: GraphEdgeWaypoint;
  tCurrent: number;
  tNext: number;
  tPrevious: number;
}): GraphEdgeWaypoint {
  const previousSpan = input.tCurrent - input.tPrevious;
  const nextSpan = input.tNext - input.tCurrent;
  const totalSpan = input.tNext - input.tPrevious;

  return {
    x:
      nextSpan *
      ((input.current.x - input.previous.x) / previousSpan -
        (input.next.x - input.previous.x) / totalSpan +
        (input.next.x - input.current.x) / nextSpan),
    y:
      nextSpan *
      ((input.current.y - input.previous.y) / previousSpan -
        (input.next.y - input.previous.y) / totalSpan +
        (input.next.y - input.current.y) / nextSpan),
  };
}

function buildWaypointCatmullRomPath(input: {
  routePoints: readonly GraphEdgeWaypoint[];
  sourcePosition: Position;
  targetPosition: Position;
}): string {
  const routePoints = compactConsecutiveRoutePoints(input.routePoints);
  if (routePoints.length <= 1) {
    const [point] = routePoints;
    return point
      ? `M ${formatPathNumber(point.x)} ${formatPathNumber(point.y)}`
      : "";
  }

  const lastPointIndex = routePoints.length - 1;
  const [source] = routePoints;
  const points = [
    virtualEndpoint({
      direction: positionVector(input.sourcePosition),
      from: source,
      mode: "before",
      neighbor: routePoints[1],
    }),
    ...routePoints,
    virtualEndpoint({
      direction: {
        x: -positionVector(input.targetPosition).x,
        y: -positionVector(input.targetPosition).y,
      },
      from: routePoints[lastPointIndex],
      mode: "after",
      neighbor: routePoints[lastPointIndex - 1],
    }),
  ];
  const segments = [
    `M ${formatPathNumber(source.x)} ${formatPathNumber(source.y)}`,
  ];

  for (let index = 1; index <= lastPointIndex; index += 1) {
    const previous = points[index - 1];
    const start = points[index];
    const end = points[index + 1];
    const next = points[index + 2];
    const tPrevious = 0;
    const tStart = tPrevious + catmullRomParameterDistance(previous, start);
    const tEnd = tStart + catmullRomParameterDistance(start, end);
    const tNext = tEnd + catmullRomParameterDistance(end, next);
    const startTangent = catmullRomTangent({
      current: start,
      next: end,
      previous,
      tCurrent: tStart,
      tNext: tEnd,
      tPrevious,
    });
    const endTangent = catmullRomTangent({
      current: end,
      next,
      previous: start,
      tCurrent: tEnd,
      tNext,
      tPrevious: tStart,
    });
    const firstControl = {
      x: start.x + startTangent.x / 3,
      y: start.y + startTangent.y / 3,
    };
    const secondControl = {
      x: end.x - endTangent.x / 3,
      y: end.y - endTangent.y / 3,
    };

    segments.push(
      [
        "C",
        formatPathNumber(firstControl.x),
        formatPathNumber(firstControl.y),
        `${formatPathNumber(secondControl.x)},`,
        formatPathNumber(secondControl.y),
        `${formatPathNumber(end.x)},`,
        formatPathNumber(end.y),
      ].join(" "),
    );
  }

  return segments.join(" ");
}

export function buildGraphEdgePathThroughWaypoints(input: {
  sourceX: number;
  sourceY: number;
  sourcePosition: Position;
  targetX: number;
  targetY: number;
  targetPosition: Position;
  waypoints?: readonly GraphEdgeWaypoint[];
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

  const path = buildWaypointCatmullRomPath({
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
