import type { FactoryGraphVisualState } from "./visual-state.js";
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
/** Safe wrapping shared by semantic labels and their merged metadata surfaces. */
export declare function factoryGraphNodeWrappedTextClassName(className?: string): string;
/** Surface and emphasis classes for the package-owned visual-state grammar. */
export declare function factoryGraphNodeVisualStateClassName(state: FactoryGraphVisualState): string;
/** Applies the owning node tone to nested guarded workstation content. */
export declare function factoryGraphNodeVisualNestedAccentClassName(state: FactoryGraphVisualState): string;
/** Returns a lifecycle/active icon class, with a family fallback for idle nodes. */
export declare function factoryGraphNodeVisualIconClassName(state: FactoryGraphVisualState, fallbackClassName?: string): string;
/** Plain surface classes used by phase legends and compatibility adapters. */
export declare function factoryGraphNodeVisualStatusSurfaceClassName(status: FactoryGraphVisualState["surface"]): string;
/** Accent feedback used by the original Factory graph's semantic node views. */
export declare function factoryGraphNodeHoverClassName(state: FactoryGraphNodeHoverState, surface?: FactoryGraphNodeHoverSurface): string | undefined;
