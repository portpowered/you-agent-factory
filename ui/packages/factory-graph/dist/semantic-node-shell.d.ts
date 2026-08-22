import { type GraphNodeHandle } from "@you-agent-factory/components/graphs";
import type { ReactNode } from "react";
import { type FactoryGraphNodeFamily } from "./node-family.js";
import { type FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import { type FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { type FactoryGraphVisualStateInput } from "./visual-state.js";
export type FactoryGraphPlaceNodeType = "constraint" | "doc" | "resource" | "statePosition" | "worker" | "workType";
export type FactoryGraphNodeHandle = GraphNodeHandle;
export interface FactoryGraphZAxisIncompleteHints {
    accessibleLabel: string;
    title: string;
}
export interface FactoryGraphNodeShellProps {
    children: ReactNode;
    className?: string;
    handles: FactoryGraphNodeHandle[];
    interactionOverlay?: FactoryGraphNodeInteractionOverlay;
    nodeType: "workstation" | FactoryGraphPlaceNodeType;
    resizeControls?: FactoryGraphNodeResizeControlsProps;
    visualState?: Omit<FactoryGraphVisualStateInput, "family">;
    zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
}
/** Shared secondary surface rendered when a semantic node has been resized. */
export declare function FactoryGraphNodeExpandedContent({ children, family, }: {
    children: ReactNode;
    family: FactoryGraphNodeFamily;
}): import("react/jsx-runtime").JSX.Element;
/** Original semantic Factory node frame, including its typed connection rails. */
export declare function FactoryGraphNodeShell({ children, className, handles, interactionOverlay, nodeType, resizeControls, visualState: visualStateInput, zAxisIncompleteHints, }: FactoryGraphNodeShellProps): import("react/jsx-runtime").JSX.Element;
export declare function factoryGraphHandleToneFromId(handleId: string): NonNullable<GraphNodeHandle["tone"]>;
