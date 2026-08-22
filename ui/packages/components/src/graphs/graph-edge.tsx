import {
  BaseEdge,
  type EdgeProps,
  getBezierPath,
  useReactFlow,
  type XYPosition,
} from "@xyflow/react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { useEffect, useRef, useState } from "react";

import {
  buildGraphEdgePathThroughWaypoints,
  type GraphEdgeWaypoint,
} from "./graph-edge-path";

export type GraphEdgeData = {
  alwaysShowLabel?: boolean;
  interaction?: GraphEdgeInteraction;
  label?: string;
  waypoints?: GraphEdgeWaypoint[];
};

export type GraphEdgePointerInteraction = (
  event: ReactPointerEvent<SVGPathElement>,
  flowPosition: XYPosition,
) => void;

export type GraphEdgeInteraction = {
  onPointerCancel?: GraphEdgePointerInteraction;
  onPointerDown?: GraphEdgePointerInteraction;
  onPointerMove?: GraphEdgePointerInteraction;
  onPointerUp?: GraphEdgePointerInteraction;
};

export const GRAPH_EDGE_TYPES = {
  graphEdge: GraphEdge,
};

export type GraphEdgeProps = EdgeProps & {
  edgeClassName?: string;
  labelClassName?: string;
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph edge rendering keeps routing, labels, and opt-in pointer interactions together.
export function GraphEdge({
  data,
  edgeClassName = "graph-edge",
  id,
  interactionWidth,
  labelClassName = "graph-edge-label pointer-events-none fill-on-surface-subtle text-[11px] font-semibold",
  markerEnd,
  selected,
  sourcePosition,
  sourceX,
  sourceY,
  style,
  targetPosition,
  targetX,
  targetY,
}: GraphEdgeProps) {
  const { screenToFlowPosition } = useReactFlow();
  const edgeRef = useRef<SVGGElement | null>(null);
  const [inspected, setInspected] = useState(false);
  const edgeData = (data ?? {}) as GraphEdgeData;
  const routedPath = buildGraphEdgePathThroughWaypoints({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
    waypoints: edgeData.waypoints,
  });
  const [fallbackPath, fallbackLabelX, fallbackLabelY] = getBezierPath({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });
  const edgePath =
    edgeData.waypoints && edgeData.waypoints.length > 0
      ? routedPath.path
      : fallbackPath;
  const labelX =
    edgeData.waypoints && edgeData.waypoints.length > 0
      ? routedPath.labelX
      : fallbackLabelX;
  const labelY =
    edgeData.waypoints && edgeData.waypoints.length > 0
      ? routedPath.labelY
      : fallbackLabelY;

  useEffect(() => {
    const edgeElement = edgeRef.current?.parentElement;
    if (!edgeElement) {
      return;
    }

    const show = () => setInspected(true);
    const hide = () => setInspected(false);

    edgeElement.addEventListener("mouseenter", show);
    edgeElement.addEventListener("mouseleave", hide);
    edgeElement.addEventListener("focusin", show);
    edgeElement.addEventListener("focusout", hide);

    return () => {
      edgeElement.removeEventListener("mouseenter", show);
      edgeElement.removeEventListener("mouseleave", hide);
      edgeElement.removeEventListener("focusin", show);
      edgeElement.removeEventListener("focusout", hide);
    };
  }, []);

  const revealLabel = Boolean(
    edgeData.label && (edgeData.alwaysShowLabel || inspected || selected),
  );
  const handlePointerInteraction = (
    handler: GraphEdgePointerInteraction | undefined,
  ) =>
    handler
      ? (event: ReactPointerEvent<SVGPathElement>) =>
          handler(
            event,
            screenToFlowPosition({
              x: event.clientX,
              y: event.clientY,
            }),
          )
      : undefined;

  return (
    <g
      className={edgeClassName}
      data-edge-id={id}
      data-label-visible={revealLabel ? "true" : "false"}
      ref={edgeRef}
    >
      <BaseEdge
        interactionWidth={0}
        markerEnd={markerEnd}
        onPointerCancel={handlePointerInteraction(
          edgeData.interaction?.onPointerCancel,
        )}
        onPointerDown={handlePointerInteraction(
          edgeData.interaction?.onPointerDown,
        )}
        onPointerMove={handlePointerInteraction(
          edgeData.interaction?.onPointerMove,
        )}
        onPointerUp={handlePointerInteraction(
          edgeData.interaction?.onPointerUp,
        )}
        path={edgePath}
        style={style}
      />
      {interactionWidth !== 0 ? (
        <path
          className="react-flow__edge-interaction"
          d={edgePath}
          fill="none"
          onPointerCancel={handlePointerInteraction(
            edgeData.interaction?.onPointerCancel,
          )}
          onPointerDown={handlePointerInteraction(
            edgeData.interaction?.onPointerDown,
          )}
          onPointerMove={handlePointerInteraction(
            edgeData.interaction?.onPointerMove,
          )}
          onPointerUp={handlePointerInteraction(
            edgeData.interaction?.onPointerUp,
          )}
          strokeOpacity={0}
          strokeWidth={interactionWidth ?? 20}
        />
      ) : null}
      {edgeData.label ? (
        <text
          className={labelClassName}
          style={{
            paintOrder: "stroke",
            stroke: "var(--color-surface)",
            strokeLinejoin: "round",
            strokeWidth: 8,
          }}
          textAnchor="middle"
          x={labelX}
          y={labelY}
        >
          {edgeData.label}
        </text>
      ) : null}
    </g>
  );
}
