import type { FactoryGraphNodeDimensionBounds, FactoryGraphNodeDimensions, FactoryGraphNodeResizeAxes } from "./node-family.js";
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
export declare function FactoryGraphNodeResizeControls({ allowedAxes, bounds, isVisible, nodeId, onResize, onResizeEnd, }: FactoryGraphNodeResizeControlsProps): import("react").JSX.Element | null;
