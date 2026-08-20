import { jsx as _jsx } from "react/jsx-runtime";
import { NodeResizeControl, ResizeControlVariant, useUpdateNodeInternals, } from "@xyflow/react";
import { useCallback } from "react";
/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({ allowedAxes, bounds, isVisible = false, nodeId, onResize, onResizeEnd, }) {
    const updateNodeInternals = useUpdateNodeInternals();
    const refreshNodeInternals = useCallback(() => {
        if (nodeId) {
            updateNodeInternals(nodeId);
        }
    }, [nodeId, updateNodeInternals]);
    const handleResize = useCallback((_event, dimensions) => {
        onResize?.({ height: dimensions.height, width: dimensions.width });
    }, [onResize]);
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
    return (_jsx(NodeResizeControl, { className: RESIZE_GRIP_CONTROL_CLASS_NAME, maxHeight: bounds.maximum.height, maxWidth: bounds.maximum.width, minHeight: bounds.minimum.height, minWidth: bounds.minimum.width, nodeId: nodeId, onResize: handleResize, onResizeEnd: handleResizeEnd, position: resizePosition, style: RESIZE_GRIP_STYLE_BY_POSITION[resizePosition], variant: ResizeControlVariant.Handle, shouldResize: (_event, dimensions) => isFiniteBoundedDimensions(dimensions, bounds), children: _jsx("span", { "aria-hidden": "true", className: RESIZE_GRIP_MARK_CLASS_NAME[resizePosition], "data-factory-graph-node-resize-grip": resizePosition }) }));
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
const RESIZE_GRIP_MARK_BASE_CLASS_NAME = "pointer-events-none block h-full w-full border-af-text-subtle";
const RESIZE_GRIP_MARK_CLASS_NAME = {
    bottom: `${RESIZE_GRIP_MARK_BASE_CLASS_NAME} border-b-2`,
    "bottom-right": `${RESIZE_GRIP_MARK_BASE_CLASS_NAME} rounded-br-sm border-b-2 border-r-2`,
    right: `${RESIZE_GRIP_MARK_BASE_CLASS_NAME} border-r-2`,
};
/**
 * Small enough to read as a hint rather than a divider, and inset from the
 * node edge so the mark sits inside the card the way a bento card grip does.
 */
const RESIZE_GRIP_SIZE = "14px";
const RESIZE_GRIP_INSET = "calc(-100% - 4px)";
function resizeGripStyle(left, top, translate) {
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
const RESIZE_GRIP_STYLE_BY_POSITION = {
    bottom: resizeGripStyle("50%", "100%", `-50% ${RESIZE_GRIP_INSET}`),
    "bottom-right": resizeGripStyle("100%", "100%", `${RESIZE_GRIP_INSET} ${RESIZE_GRIP_INSET}`),
    right: resizeGripStyle("100%", "50%", `${RESIZE_GRIP_INSET} -50%`),
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
