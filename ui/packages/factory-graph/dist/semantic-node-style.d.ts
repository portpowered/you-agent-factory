export type FactoryGraphNodeSurfaceTone = "danger" | "info" | "neutral" | "neutralHigh" | "primary" | "resource" | "success" | "warning" | "workState" | "workstation";
export interface FactoryGraphNodeHoverState {
    activeFlow?: boolean;
    muted?: boolean;
    selected?: boolean;
    validationError?: boolean;
}
export type FactoryGraphNodeHoverSurface = "primary" | "warning";
export declare function factoryGraphNodeSurfaceClassName(tone: FactoryGraphNodeSurfaceTone): string;
export declare function factoryGraphNodeTitleClassName(className?: string): string;
/** Accent feedback used by the original Factory graph's semantic node views. */
export declare function factoryGraphNodeHoverClassName(state: FactoryGraphNodeHoverState, surface?: FactoryGraphNodeHoverSurface): string | undefined;
