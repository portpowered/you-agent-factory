import {
  type ControlPosition,
  NodeResizeControl,
  type OnResize,
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
  /**
   * Reports every intermediate size while the pointer is still down so the
   * host can paint the node at its in-progress size instead of snapping to
   * the committed size once the drag settles.
   */
  onResize?: (dimensions: FactoryGraphNodeDimensions) => void;
  onResizeEnd?: (dimensions: FactoryGraphNodeDimensions) => void;
}

/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({
  allowedAxes,
  bounds,
  isVisible = false,
  nodeId,
  onResize,
  onResizeEnd,
}: FactoryGraphNodeResizeControlsProps) {
  const updateNodeInternals = useUpdateNodeInternals();
  const refreshNodeInternals = useCallback(() => {
    if (nodeId) {
      updateNodeInternals(nodeId);
    }
  }, [nodeId, updateNodeInternals]);
  const handleResize = useCallback<OnResize>(
    (_event, dimensions) => {
      onResize?.({ height: dimensions.height, width: dimensions.width });
    },
    [onResize],
  );
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
  const resizeDirection = resizeControlDirection(allowedAxes);

  return (
    <NodeResizeControl
      className={RESIZE_GRIP_CONTROL_CLASS_NAME}
      maxHeight={bounds.maximum.height}
      maxWidth={bounds.maximum.width}
      minHeight={bounds.minimum.height}
      minWidth={bounds.minimum.width}
      nodeId={nodeId}
      onResize={handleResize}
      onResizeEnd={handleResizeEnd}
      position={resizePosition}
      resizeDirection={resizeDirection}
      style={RESIZE_GRIP_STYLE}
      variant={ResizeControlVariant.Handle}
      shouldResize={(_event, dimensions) =>
        isFiniteBoundedDimensions(dimensions, bounds)
      }
    >
      <span
        aria-hidden="true"
        className={RESIZE_GRIP_MARK_CLASS_NAME}
        data-factory-graph-node-resize-grip={resizePosition}
      />
    </NodeResizeControl>
  );
}

/**
 * The grip stays out of sight until the node is hovered or focused, matching
 * the bento card resize affordance. `semantic-node-shell` owns the
 * `group/factory-graph-node` ancestor these variants read.
 */
const RESIZE_GRIP_CONTROL_CLASS_NAME = [
  "factory-graph-node-resize-control",
  "factory-graph-node-resize-grip",
  "nodrag",
  "nopan",
  "pointer-events-auto",
  "opacity-0",
  "transition-opacity",
  "group-hover/factory-graph-node:opacity-100",
  "group-focus-within/factory-graph-node:opacity-100",
].join(" ");

/** Neutral, unaccented tick marks; an accent tone would read as node status. */
const RESIZE_GRIP_MARK_BASE_CLASS_NAME =
  "pointer-events-none block h-full w-full border-af-text-subtle";

const RESIZE_GRIP_MARK_CLASS_NAME = `${RESIZE_GRIP_MARK_BASE_CLASS_NAME} rounded-br-sm border-b-2 border-r-2`;

type ResizeGripPosition = Extract<ControlPosition, "bottom-right">;

/**
 * Small enough to read as a hint rather than a divider, and inset from the
 * node edge so the mark sits inside the card the way a bento card grip does.
 */
const RESIZE_GRIP_SIZE = "14px";
const RESIZE_GRIP_INSET = "calc(-100% - 4px)";

function resizeGripStyle(
  left: string,
  top: string,
  translate: string,
): CSSProperties {
  return {
    background: "none",
    border: "none",
    borderRadius: "0",
    height: RESIZE_GRIP_SIZE,
    left,
    top,
    translate,
    width: RESIZE_GRIP_SIZE,
  };
}

const RESIZE_GRIP_STYLE: CSSProperties = resizeGripStyle(
  "100%",
  "100%",
  `${RESIZE_GRIP_INSET} ${RESIZE_GRIP_INSET}`,
);

function resizeControlPosition(
  allowedAxes: FactoryGraphNodeResizeAxes,
): ResizeGripPosition | null {
  return allowedAxes.width || allowedAxes.height ? "bottom-right" : null;
}

function resizeControlDirection(
  allowedAxes: FactoryGraphNodeResizeAxes,
): "horizontal" | "vertical" | undefined {
  if (allowedAxes.width && !allowedAxes.height) {
    return "horizontal";
  }
  if (allowedAxes.height && !allowedAxes.width) {
    return "vertical";
  }
  return undefined;
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
