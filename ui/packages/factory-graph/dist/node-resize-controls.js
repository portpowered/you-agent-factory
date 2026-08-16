import { jsx as _jsx } from "react/jsx-runtime";
import { NodeResizeControl, ResizeControlVariant, useUpdateNodeInternals, } from "@xyflow/react";
import { useCallback } from "react";
/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({ allowedAxes, bounds, isVisible = false, nodeId, onResizeEnd, }) {
    const updateNodeInternals = useUpdateNodeInternals();
    const refreshNodeInternals = useCallback(() => {
        if (nodeId) {
            updateNodeInternals(nodeId);
        }
    }, [nodeId, updateNodeInternals]);
    const handleResizeEnd = useCallback((_event, dimensions) => {
        onResizeEnd?.({ height: dimensions.height, width: dimensions.width });
        refreshNodeInternals();
    }, [onResizeEnd, refreshNodeInternals]);
    if (!isVisible) {
        return null;
    }
    const resizePosition = resizeControlPosition(allowedAxes);
    if (!resizePosition) {
        return null;
    }
    return (_jsx(NodeResizeControl, { className: "factory-graph-node-resize-control factory-graph-node-resize-edge nodrag nopan pointer-events-auto", maxHeight: bounds.maximum.height, maxWidth: bounds.maximum.width, minHeight: bounds.minimum.height, minWidth: bounds.minimum.width, nodeId: nodeId, onResizeEnd: handleResizeEnd, position: resizePosition, style: bottomEdgeResizeControlStyle, variant: ResizeControlVariant.Line, shouldResize: (_event, dimensions) => isFiniteBoundedDimensions(dimensions, bounds) }));
}
const bottomEdgeResizeControlStyle = {
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
function resizeControlPosition(allowedAxes) {
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
function isFiniteBoundedDimensions(dimensions, bounds) {
    return (Number.isFinite(dimensions.width) &&
        Number.isFinite(dimensions.height) &&
        dimensions.width >= bounds.minimum.width &&
        dimensions.width <= bounds.maximum.width &&
        dimensions.height >= bounds.minimum.height &&
        dimensions.height <= bounds.maximum.height);
}
