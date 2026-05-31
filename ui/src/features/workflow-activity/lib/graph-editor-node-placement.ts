import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND } from "../../factory-graph-editor/lib/factory-graph-editor-layout";

export interface AxisAlignedRect {
  height: number;
  width: number;
  x: number;
  y: number;
}

export interface FlowPoint {
  x: number;
  y: number;
}

export interface NodePlacementSize {
  height: number;
  width: number;
}

export interface ViewportCenterPlacementInput {
  candidateSize: NodePlacementSize;
  maxAttempts?: number;
  occupiedRects: readonly AxisAlignedRect[];
  paddingGap?: number;
  viewportCenter: FlowPoint;
}

export interface ViewportCenterPlacementResult {
  attemptsUsed: number;
  center: FlowPoint;
  collidesAtCenter: boolean;
  exhaustedSearch: boolean;
}

/** Default gap between candidate and occupied node bounds when testing overlap. */
export const GRAPH_EDITOR_NODE_PLACEMENT_PADDING_GAP = 12;

/**
 * Maximum placement candidates tried after the viewport center, including the
 * center itself (48 total attempts: center + 47 nudges).
 */
export const GRAPH_EDITOR_NODE_PLACEMENT_MAX_ATTEMPTS = 48;

export function graphEditorNodeDimensionsForKind(
  kind: FactoryGraphNodeKind,
): NodePlacementSize {
  return { ...FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND[kind] };
}

export function axisAlignedRectFromTopLeft(
  topLeft: FlowPoint,
  size: NodePlacementSize,
): AxisAlignedRect {
  return {
    height: size.height,
    width: size.width,
    x: topLeft.x,
    y: topLeft.y,
  };
}

export function topLeftFromAxisAlignedRectCenter(
  center: FlowPoint,
  size: NodePlacementSize,
): FlowPoint {
  return {
    x: center.x - size.width / 2,
    y: center.y - size.height / 2,
  };
}

function axisAlignedRectFromCenter(
  center: FlowPoint,
  size: NodePlacementSize,
): AxisAlignedRect {
  return axisAlignedRectFromTopLeft(
    topLeftFromAxisAlignedRectCenter(center, size),
    size,
  );
}

function expandedRect(
  rect: AxisAlignedRect,
  paddingGap: number,
): AxisAlignedRect {
  return {
    height: rect.height + paddingGap * 2,
    width: rect.width + paddingGap * 2,
    x: rect.x - paddingGap,
    y: rect.y - paddingGap,
  };
}

function rectsOverlap(
  left: AxisAlignedRect,
  right: AxisAlignedRect,
  paddingGap: number,
): boolean {
  const expandedLeft = expandedRect(left, paddingGap);
  const expandedRight = expandedRect(right, paddingGap);
  const leftRight = expandedLeft.x + expandedLeft.width;
  const leftBottom = expandedLeft.y + expandedLeft.height;
  const rightRight = expandedRight.x + expandedRight.width;
  const rightBottom = expandedRight.y + expandedRight.height;

  return (
    expandedLeft.x < rightRight &&
    leftRight > expandedRight.x &&
    expandedLeft.y < rightBottom &&
    leftBottom > expandedRight.y
  );
}

function collidesWithOccupied(
  candidateRect: AxisAlignedRect,
  occupiedRects: readonly AxisAlignedRect[],
  paddingGap: number,
): boolean {
  return occupiedRects.some((occupiedRect) =>
    rectsOverlap(candidateRect, occupiedRect, paddingGap),
  );
}

function distanceSquared(from: FlowPoint, to: FlowPoint): number {
  const deltaX = from.x - to.x;
  const deltaY = from.y - to.y;
  return deltaX * deltaX + deltaY * deltaY;
}

function* viewportCenterNudgeOffsets(
  stepX: number,
  stepY: number,
): Generator<FlowPoint> {
  yield { x: 0, y: 0 };

  let offsetX = 0;
  let offsetY = 0;
  let stepsInSegment = 1;
  const directions = [
    { dx: 1, dy: 0 },
    { dx: 0, dy: 1 },
    { dx: -1, dy: 0 },
    { dx: 0, dy: -1 },
  ] as const;
  let directionIndex = 0;

  while (true) {
    const direction = directions[directionIndex % directions.length];
    for (let step = 0; step < stepsInSegment; step += 1) {
      offsetX += direction.dx * stepX;
      offsetY += direction.dy * stepY;
      yield { x: offsetX, y: offsetY };
    }
    directionIndex += 1;
    if (directionIndex % 2 === 0) {
      stepsInSegment += 1;
    }
  }
}

export function resolveViewportCenterNodePlacement(
  input: ViewportCenterPlacementInput,
): ViewportCenterPlacementResult {
  const paddingGap =
    input.paddingGap ?? GRAPH_EDITOR_NODE_PLACEMENT_PADDING_GAP;
  const maxAttempts =
    input.maxAttempts ?? GRAPH_EDITOR_NODE_PLACEMENT_MAX_ATTEMPTS;
  const stepX = input.candidateSize.width + paddingGap;
  const stepY = input.candidateSize.height + paddingGap;
  const centerRect = axisAlignedRectFromCenter(
    input.viewportCenter,
    input.candidateSize,
  );
  const collidesAtCenter = collidesWithOccupied(
    centerRect,
    input.occupiedRects,
    paddingGap,
  );

  let attemptsUsed = 1;
  if (!collidesAtCenter) {
    return {
      attemptsUsed,
      center: input.viewportCenter,
      collidesAtCenter: false,
      exhaustedSearch: false,
    };
  }

  let bestCenter: FlowPoint | undefined;
  let bestDistance = Number.POSITIVE_INFINITY;
  const nudgeOffsets = viewportCenterNudgeOffsets(stepX, stepY);
  nudgeOffsets.next();

  while (attemptsUsed < maxAttempts) {
    const nextOffset = nudgeOffsets.next();
    if (nextOffset.done) {
      break;
    }

    attemptsUsed += 1;
    const candidateCenter = {
      x: input.viewportCenter.x + nextOffset.value.x,
      y: input.viewportCenter.y + nextOffset.value.y,
    };
    const candidateRect = axisAlignedRectFromCenter(
      candidateCenter,
      input.candidateSize,
    );
    if (collidesWithOccupied(candidateRect, input.occupiedRects, paddingGap)) {
      continue;
    }

    const candidateDistance = distanceSquared(
      input.viewportCenter,
      candidateCenter,
    );
    if (candidateDistance < bestDistance) {
      bestCenter = candidateCenter;
      bestDistance = candidateDistance;
    }
  }

  if (bestCenter) {
    return {
      attemptsUsed,
      center: bestCenter,
      collidesAtCenter: true,
      exhaustedSearch: false,
    };
  }

  return {
    attemptsUsed,
    center: input.viewportCenter,
    collidesAtCenter: true,
    exhaustedSearch: true,
  };
}
