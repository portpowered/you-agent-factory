import {
  type ControlPosition,
  NodeResizeControl,
  type OnResizeEnd,
  useUpdateNodeInternals,
} from "@xyflow/react";
import { Button } from "@you-agent-factory/components/primitives";
import { useCallback } from "react";

import type {
  FactoryGraphNodeDimensionBounds,
  FactoryGraphNodeDimensions,
  FactoryGraphNodeResizeAxes,
} from "./node-family.js";

export interface FactoryGraphNodeResizeLabels {
  fitToContent: string;
  resetSize: string;
}

export interface FactoryGraphNodeResizeControlsProps {
  allowedAxes: FactoryGraphNodeResizeAxes;
  bounds: FactoryGraphNodeDimensionBounds;
  fitDimensions: FactoryGraphNodeDimensions;
  isVisible?: boolean;
  labels: FactoryGraphNodeResizeLabels;
  nodeId?: string;
  onFitToContent?: (dimensions: FactoryGraphNodeDimensions) => void;
  onResetSize?: () => void;
  onResizeEnd?: (dimensions: FactoryGraphNodeDimensions) => void;
}

/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({
  allowedAxes,
  bounds,
  fitDimensions,
  isVisible = false,
  labels,
  nodeId,
  onFitToContent,
  onResetSize,
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
  const handleFitToContent = useCallback(() => {
    onFitToContent?.(fitDimensions);
    refreshNodeInternals();
  }, [fitDimensions, onFitToContent, refreshNodeInternals]);
  const handleResetSize = useCallback(() => {
    onResetSize?.();
    refreshNodeInternals();
  }, [onResetSize, refreshNodeInternals]);

  if (!isVisible) {
    return null;
  }

  const resizePositions = resizeControlPositions(allowedAxes);
  const resizeDirection =
    allowedAxes.width && !allowedAxes.height
      ? "horizontal"
      : allowedAxes.height && !allowedAxes.width
        ? "vertical"
        : undefined;

  return (
    <>
      {resizePositions.map((position) => (
        <NodeResizeControl
          className="factory-graph-node-resize-control nodrag nopan"
          key={position}
          maxHeight={bounds.maximum.height}
          maxWidth={bounds.maximum.width}
          minHeight={bounds.minimum.height}
          minWidth={bounds.minimum.width}
          nodeId={nodeId}
          onResizeEnd={handleResizeEnd}
          position={position}
          resizeDirection={resizeDirection}
          shouldResize={(_event, dimensions) =>
            isFiniteBoundedDimensions(dimensions, bounds)
          }
        />
      ))}
      <div
        className="pointer-events-auto absolute -top-11 right-0 z-40 flex gap-1 rounded-lg border border-outline bg-surface-container-high p-1 shadow-af-panel"
        data-factory-graph-node-resize-actions
        onPointerDown={(event) => event.stopPropagation()}
      >
        {onFitToContent ? (
          <Button
            aria-label={labels.fitToContent}
            className="nodrag nopan min-h-8 rounded-md px-2 py-1 text-[0.68rem]"
            onClick={handleFitToContent}
            size="sm"
            tone="outline"
          >
            {labels.fitToContent}
          </Button>
        ) : null}
        {onResetSize ? (
          <Button
            aria-label={labels.resetSize}
            className="nodrag nopan min-h-8 rounded-md px-2 py-1 text-[0.68rem]"
            onClick={handleResetSize}
            size="sm"
            tone="outline"
          >
            {labels.resetSize}
          </Button>
        ) : null}
      </div>
    </>
  );
}

function resizeControlPositions(
  allowedAxes: FactoryGraphNodeResizeAxes,
): ControlPosition[] {
  if (allowedAxes.width && allowedAxes.height) {
    return ["top-left", "top-right", "bottom-left", "bottom-right"];
  }
  if (allowedAxes.width) {
    return ["left", "right"];
  }
  if (allowedAxes.height) {
    return ["top", "bottom"];
  }
  return [];
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
