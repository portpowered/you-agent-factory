import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { NodeResizeControl, useUpdateNodeInternals, } from "@xyflow/react";
import { Button } from "@you-agent-factory/components/primitives";
import { useCallback } from "react";
/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export function FactoryGraphNodeResizeControls({ allowedAxes, bounds, fitDimensions, isVisible = false, labels, nodeId, onFitToContent, onResetSize, onResizeEnd, }) {
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
    const refreshNodeInternalsAfterCommit = useCallback(() => {
        if (!nodeId) {
            return;
        }
        if (typeof requestAnimationFrame !== "function") {
            updateNodeInternals(nodeId);
            return;
        }
        requestAnimationFrame(() => updateNodeInternals(nodeId));
    }, [nodeId, updateNodeInternals]);
    const handleFitToContent = useCallback(() => {
        if (!onFitToContent) {
            return;
        }
        onFitToContent(fitDimensions);
        refreshNodeInternalsAfterCommit();
    }, [fitDimensions, onFitToContent, refreshNodeInternalsAfterCommit]);
    const handleResetSize = useCallback(() => {
        onResetSize?.();
        refreshNodeInternals();
    }, [onResetSize, refreshNodeInternals]);
    if (!isVisible) {
        return null;
    }
    const resizePositions = resizeControlPositions(allowedAxes);
    const resizeDirection = allowedAxes.width && !allowedAxes.height
        ? "horizontal"
        : allowedAxes.height && !allowedAxes.width
            ? "vertical"
            : undefined;
    return (_jsxs(_Fragment, { children: [resizePositions.map((position) => (_jsx(NodeResizeControl, { className: "factory-graph-node-resize-control nodrag nopan", maxHeight: bounds.maximum.height, maxWidth: bounds.maximum.width, minHeight: bounds.minimum.height, minWidth: bounds.minimum.width, nodeId: nodeId, onResizeEnd: handleResizeEnd, position: position, resizeDirection: resizeDirection, shouldResize: (_event, dimensions) => isFiniteBoundedDimensions(dimensions, bounds) }, position))), _jsxs("div", { className: "pointer-events-auto absolute -top-11 right-0 z-40 flex gap-1 rounded-lg border border-outline bg-surface-container-high p-1 shadow-af-panel", "data-factory-graph-node-resize-actions": true, onPointerDown: (event) => event.stopPropagation(), children: [onFitToContent ? (_jsx(Button, { "aria-label": labels.fitToContent, className: "nodrag nopan min-h-8 rounded-md px-2 py-1 text-[0.68rem]", onClick: handleFitToContent, size: "sm", tone: "outline", children: labels.fitToContent })) : null, onResetSize ? (_jsx(Button, { "aria-label": labels.resetSize, className: "nodrag nopan min-h-8 rounded-md px-2 py-1 text-[0.68rem]", onClick: handleResetSize, size: "sm", tone: "outline", children: labels.resetSize })) : null] })] }));
}
function resizeControlPositions(allowedAxes) {
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
function isFiniteBoundedDimensions(dimensions, bounds) {
    return (Number.isFinite(dimensions.width) &&
        Number.isFinite(dimensions.height) &&
        dimensions.width >= bounds.minimum.width &&
        dimensions.width <= bounds.maximum.width &&
        dimensions.height >= bounds.minimum.height &&
        dimensions.height <= bounds.maximum.height);
}
