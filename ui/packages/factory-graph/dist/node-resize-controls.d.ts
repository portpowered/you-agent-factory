import type { FactoryGraphNodeDimensionBounds, FactoryGraphNodeDimensions, FactoryGraphNodeResizeAxes } from "./node-family.js";
export interface FactoryGraphNodeResizeControlsProps {
    allowedAxes: FactoryGraphNodeResizeAxes;
    bounds: FactoryGraphNodeDimensionBounds;
    isVisible?: boolean;
    nodeId?: string;
    onResizeEnd?: (dimensions: FactoryGraphNodeDimensions) => void;
}
/** Shared edit-host-controlled node size affordance for Factory graph nodes. */
export declare function FactoryGraphNodeResizeControls({ allowedAxes, bounds, isVisible, nodeId, onResizeEnd, }: FactoryGraphNodeResizeControlsProps): import("react/jsx-runtime").JSX.Element | null;
