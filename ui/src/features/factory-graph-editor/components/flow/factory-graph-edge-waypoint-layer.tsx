import { useReactFlow, useStore, type XYPosition } from "@xyflow/react";
import { useCallback, useRef, useState } from "react";
import { cn } from "../../../../lib/cn";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { FactoryLayoutPoint } from "../../lib/layout/factory-graph-layout-operations";

type DragSession = {
  pointerId: number;
  waypointIndex: number;
};

export function FactoryGraphEdgeWaypointLayer({
  ariaLabel,
  edgeId,
  onMoveWaypoint,
  onRemoveWaypoint,
  waypoints,
}: {
  ariaLabel: (index: number) => string;
  edgeId: string;
  onMoveWaypoint: (
    edgeId: string,
    waypointIndex: number,
    position: FactoryLayoutPoint,
  ) => void;
  onRemoveWaypoint?: (edgeId: string, waypointIndex: number) => void;
  waypoints: readonly FactoryLayoutPoint[];
}) {
  const { screenToFlowPosition } = useReactFlow();
  const transform = useStore((state) => state.transform);
  const dragSessionRef = useRef<DragSession | null>(null);
  const [previewPositions, setPreviewPositions] = useState<
    FactoryLayoutPoint[] | null
  >(null);
  const [translateX, translateY, zoom] = transform;
  const renderedWaypoints = previewPositions ?? waypoints;

  const projectFlowPoint = useCallback(
    (point: FactoryLayoutPoint) => ({
      x: point.x * zoom + translateX,
      y: point.y * zoom + translateY,
    }),
    [translateX, translateY, zoom],
  );

  const handlePointerDown = useCallback(
    (waypointIndex: number) =>
      (event: React.PointerEvent<HTMLButtonElement>) => {
        event.stopPropagation();
        event.preventDefault();
        dragSessionRef.current = {
          pointerId: event.pointerId,
          waypointIndex,
        };
        setPreviewPositions(waypoints.map((point) => ({ ...point })));
        event.currentTarget.setPointerCapture(event.pointerId);
      },
    [waypoints],
  );

  const handlePointerMove = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      const dragSession = dragSessionRef.current;
      if (!dragSession || dragSession.pointerId !== event.pointerId) {
        return;
      }

      const nextPosition = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      setPreviewPositions((current) => {
        const base = current ?? waypoints.map((point) => ({ ...point }));
        return base.map((point, index) =>
          index === dragSession.waypointIndex ? nextPosition : point,
        );
      });
    },
    [screenToFlowPosition, waypoints],
  );

  const handleKeyDown = useCallback(
    (waypointIndex: number) =>
      (event: React.KeyboardEvent<HTMLButtonElement>) => {
        if (
          !onRemoveWaypoint ||
          (event.key !== "Delete" && event.key !== "Backspace")
        ) {
          return;
        }

        event.preventDefault();
        event.stopPropagation();
        onRemoveWaypoint(edgeId, waypointIndex);
      },
    [edgeId, onRemoveWaypoint],
  );

  const handlePointerUp = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      const dragSession = dragSessionRef.current;
      if (!dragSession || dragSession.pointerId !== event.pointerId) {
        return;
      }

      dragSessionRef.current = null;
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }

      const nextPosition = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      setPreviewPositions(null);
      onMoveWaypoint(edgeId, dragSession.waypointIndex, nextPosition);
    },
    [edgeId, onMoveWaypoint, screenToFlowPosition],
  );

  if (renderedWaypoints.length === 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-0 z-20"
      data-factory-edge-waypoint-layer={edgeId}
    >
      {renderedWaypoints.map((waypoint, index) => {
        const projected = projectFlowPoint(waypoint);
        return (
          <GraphNodeButton
            aria-label={ariaLabel(index)}
            className={cn(
              "pointer-events-auto absolute h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-primary bg-surface shadow-sm",
              "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary",
            )}
            data-factory-edge-waypoint={index}
            // biome-ignore lint/suspicious/noArrayIndexKey: waypoint order is the stable identity while dragging updates position.
            key={`${edgeId}:waypoint:${index}`}
            onKeyDown={handleKeyDown(index)}
            onPointerDown={handlePointerDown(index)}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            style={{
              left: projected.x,
              top: projected.y,
            }}
          />
        );
      })}
    </div>
  );
}

export function flowMidpointBetweenNodes(
  source: XYPosition,
  target: XYPosition,
): FactoryLayoutPoint {
  return {
    x: (source.x + target.x) / 2,
    y: (source.y + target.y) / 2,
  };
}
