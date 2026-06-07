import { BaseEdge, type EdgeProps, getBezierPath } from "@xyflow/react";
import { useEffect, useRef, useState } from "react";

type FactoryGraphEdgeData = {
  alwaysShowLabel?: boolean;
  label?: string;
};

export const FACTORY_GRAPH_EDGE_TYPES = {
  factoryEditorEdge: FactoryGraphEdge,
};

function FactoryGraphEdge({
  data,
  id,
  interactionWidth,
  markerEnd,
  selected,
  sourcePosition,
  sourceX,
  sourceY,
  style,
  targetPosition,
  targetX,
  targetY,
}: EdgeProps) {
  const edgeRef = useRef<SVGGElement | null>(null);
  const [inspected, setInspected] = useState(false);
  const edgeData = (data ?? {}) as FactoryGraphEdgeData;
  const [edgePath, labelX, labelY] = getBezierPath({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });

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

  return (
    <g
      className="agent-factory-editor-edge"
      data-edge-id={id}
      data-label-visible={revealLabel ? "true" : "false"}
      ref={edgeRef}
    >
      <BaseEdge
        interactionWidth={interactionWidth}
        markerEnd={markerEnd}
        path={edgePath}
        style={style}
      />
      {edgeData.label ? (
        <text
          className="agent-factory-editor-edge-label pointer-events-none fill-on-surface-subtle text-[11px] font-semibold"
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
