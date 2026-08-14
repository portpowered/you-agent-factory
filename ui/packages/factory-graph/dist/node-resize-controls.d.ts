import type { FactoryGraphNodeDimensionBounds, FactoryGraphNodeDimensions, FactoryGraphNodeResizeAxes } from "./node-family.js";
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
export declare function FactoryGraphNodeResizeControls({ allowedAxes, bounds, fitDimensions, isVisible, labels, nodeId, onFitToContent, onResetSize, onResizeEnd, }: FactoryGraphNodeResizeControlsProps): import("react/jsx-runtime").JSX.Element | null;
