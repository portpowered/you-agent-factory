import type { components } from "../../../../api/generated/openapi";
import {
  FACTORY_LAYOUT_SCHEMA_VERSION,
  type FactoryLayout,
  type FactoryLayoutPoint,
} from "./factory-graph-layout-operations";

export type FactoryLayoutEdge = NonNullable<
  components["schemas"]["Factory"]["layout"]
>["edges"] extends (infer TEdge)[] | undefined
  ? TEdge
  : never;

export function isValidFactoryLayoutPoint(
  point: FactoryLayoutPoint | null | undefined,
): point is FactoryLayoutPoint {
  return (
    point !== null &&
    point !== undefined &&
    Number.isFinite(point.x) &&
    Number.isFinite(point.y)
  );
}

export function factoryLayoutEdgeWaypoints(
  layout: FactoryLayout,
  edgeId: string,
): FactoryLayoutPoint[] | undefined {
  const edge = layout.edges?.find((entry) => entry.id === edgeId);
  if (!edge?.waypoints || edge.waypoints.length === 0) {
    return undefined;
  }

  const validWaypoints = edge.waypoints.filter(isValidFactoryLayoutPoint);
  return validWaypoints.length > 0 ? validWaypoints : undefined;
}

export function setFactoryLayoutEdgeWaypoints(
  layout: FactoryLayout,
  edgeId: string,
  waypoints: readonly FactoryLayoutPoint[] | null,
): FactoryLayout {
  const edges = [...(layout.edges ?? [])];
  const existingIndex = edges.findIndex((entry) => entry.id === edgeId);
  const sanitizedWaypoints =
    waypoints === null
      ? undefined
      : waypoints.filter(isValidFactoryLayoutPoint).map((point) => ({
          x: point.x,
          y: point.y,
        }));

  if (!sanitizedWaypoints || sanitizedWaypoints.length === 0) {
    if (existingIndex < 0) {
      return layout;
    }

    const { waypoints: _removed, ...remainingEdge } = edges[existingIndex];
    if (
      Object.keys(remainingEdge).length === 1 &&
      remainingEdge.id === edgeId
    ) {
      edges.splice(existingIndex, 1);
    } else {
      edges[existingIndex] = remainingEdge as FactoryLayoutEdge;
    }

    return {
      ...layout,
      edges: edges.length > 0 ? edges : undefined,
      schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
    };
  }

  const nextEdge: FactoryLayoutEdge = {
    ...(existingIndex >= 0 ? edges[existingIndex] : { id: edgeId }),
    id: edgeId,
    waypoints: sanitizedWaypoints,
  };

  if (existingIndex >= 0) {
    edges[existingIndex] = nextEdge;
  } else {
    edges.push(nextEdge);
  }

  return {
    ...layout,
    edges,
    schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
  };
}

export function addFactoryLayoutEdgeWaypoint(
  layout: FactoryLayout,
  edgeId: string,
  position: FactoryLayoutPoint,
  insertIndex?: number,
): FactoryLayout {
  if (!isValidFactoryLayoutPoint(position)) {
    return layout;
  }

  const currentWaypoints = [
    ...(factoryLayoutEdgeWaypoints(layout, edgeId) ?? []),
  ];
  const nextPoint = { x: position.x, y: position.y };
  const index =
    insertIndex === undefined
      ? currentWaypoints.length
      : Math.max(0, Math.min(insertIndex, currentWaypoints.length));
  currentWaypoints.splice(index, 0, nextPoint);

  return setFactoryLayoutEdgeWaypoints(layout, edgeId, currentWaypoints);
}

export function removeFactoryLayoutEdgeWaypoint(
  layout: FactoryLayout,
  edgeId: string,
  waypointIndex: number,
): FactoryLayout {
  const currentWaypoints = factoryLayoutEdgeWaypoints(layout, edgeId);
  if (
    !currentWaypoints ||
    waypointIndex < 0 ||
    waypointIndex >= currentWaypoints.length
  ) {
    return layout;
  }

  const nextWaypoints = currentWaypoints.filter(
    (_, index) => index !== waypointIndex,
  );

  return setFactoryLayoutEdgeWaypoints(
    layout,
    edgeId,
    nextWaypoints.length > 0 ? nextWaypoints : null,
  );
}

export function moveFactoryLayoutEdgeWaypoint(
  layout: FactoryLayout,
  edgeId: string,
  waypointIndex: number,
  position: FactoryLayoutPoint,
): FactoryLayout {
  if (!isValidFactoryLayoutPoint(position)) {
    return layout;
  }

  const currentWaypoints = factoryLayoutEdgeWaypoints(layout, edgeId);
  if (
    !currentWaypoints ||
    waypointIndex < 0 ||
    waypointIndex >= currentWaypoints.length
  ) {
    return layout;
  }

  const nextWaypoints = currentWaypoints.map((point, index) =>
    index === waypointIndex ? { x: position.x, y: position.y } : point,
  );

  return setFactoryLayoutEdgeWaypoints(layout, edgeId, nextWaypoints);
}

export function factoryLayoutPointsEqual(
  left: FactoryLayoutPoint,
  right: FactoryLayoutPoint,
): boolean {
  return left.x === right.x && left.y === right.y;
}

export function factoryLayoutWaypointArraysEqual(
  left: readonly FactoryLayoutPoint[] | null | undefined,
  right: readonly FactoryLayoutPoint[] | null | undefined,
): boolean {
  const normalizedLeft = left ?? [];
  const normalizedRight = right ?? [];
  if (normalizedLeft.length !== normalizedRight.length) {
    return false;
  }

  return normalizedLeft.every((point, index) =>
    factoryLayoutPointsEqual(point, normalizedRight[index]),
  );
}
