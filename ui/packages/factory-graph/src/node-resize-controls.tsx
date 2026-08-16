import {
  type ControlPosition,
  NodeResizeControl,
  type OnResizeEnd,
  ResizeControlVariant,
  useUpdateNodeInternals,
} from "@xyflow/react";
import { type CSSProperties, useCallback } from "react";

import type {
  FactoryGraphNodeDimensionBounds,
  FactoryGraphNodeDimensions,
  FactoryGraphNodeResizeAxes,
} from "./node-family.js";

export interface FactoryGraphNodeResizeControlsProps {
  allowedAxes: FactoryGraphNodeResizeAxes;
  bounds: FactoryGraphNodeDimensionBounds;
  isVisible?: boolean;
  nodeId?: string;
  onResizeEnd?: (dimensions: FactoryGraphNodeDimensions) => void;
}

/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({
  allowedAxes,
  bounds,
  isVisible = false,
  nodeId,
  onResizeEnd,
}: FactoryGraphNodeResizeControlsProps) {
  const updateNodeInternals = useUpdateNodeInternals();
  const refreshNodeInternals = useCallback(() => {
    if (nodeId) {
      updateNodeInternals(nodeId);
    }
  }, [nodeId, updateNodeInternals]);
  const handleResizeEnd = useCallback<OnResizeEnd>(
    (_event, dimensions) => {
      onResizeEnd?.({ height: dimensions.height, width: dimensions.width });
      refreshNodeInternals();
    },
    [onResizeEnd, refreshNodeInternals],
  );
  if (!isVisible) {
    return null;
  }

  const resizePosition = resizeControlPosition(allowedAxes);
  if (!resizePosition) {
    return null;
  }

  return (
    <NodeResizeControl
      className="factory-graph-node-resize-control factory-graph-node-resize-edge nodrag nopan pointer-events-auto"
      maxHeight={bounds.maximum.height}
      maxWidth={bounds.maximum.width}
      minHeight={bounds.minimum.height}
      minWidth={bounds.minimum.width}
      nodeId={nodeId}
      onResizeEnd={handleResizeEnd}
      position={resizePosition}
      style={bottomEdgeResizeControlStyle}
      variant={ResizeControlVariant.Line}
      shouldResize={(_event, dimensions) =>
        isFiniteBoundedDimensions(dimensions, bounds)
      }
    />
  );
}

const bottomEdgeResizeControlStyle: CSSProperties = {
  borderBottomColor: "var(--color-primary)",
  borderBottomStyle: "solid",
  borderBottomWidth: "3px",
  borderLeftWidth: "0px",
  borderRightWidth: "0px",
  borderTopWidth: "0px",
  height: "10px",
  left: "0",
  top: "100%",
  transform: "translateY(-50%)",
  width: "100%",
};

function resizeControlPosition(
  allowedAxes: FactoryGraphNodeResizeAxes,
): ControlPosition | null {
  if (allowedAxes.width && allowedAxes.height) {
    return "bottom-right";
  }
  if (allowedAxes.width) {
    return "right";
  }
  if (allowedAxes.height) {
    return "bottom";
  }
  return null;
}

function isFiniteBoundedDimensions(
  dimensions: FactoryGraphNodeDimensions,
  bounds: FactoryGraphNodeDimensionBounds,
): boolean {
  return (
    Number.isFinite(dimensions.width) &&
    Number.isFinite(dimensions.height) &&
    dimensions.width >= bounds.minimum.width &&
    dimensions.width <= bounds.maximum.width &&
    dimensions.height >= bounds.minimum.height &&
    dimensions.height <= bounds.maximum.height
  );
}
