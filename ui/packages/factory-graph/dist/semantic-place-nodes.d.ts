import type { Node, NodeProps } from "@xyflow/react";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
import type { FactoryGraphPlaceRef } from "./semantic-support-nodes.js";
import { type FactoryGraphWorkStateTypeValue } from "./work-state-presentation.js";
export interface FactoryGraphSemanticPlaceRef extends FactoryGraphPlaceRef {
    kind: "constraint" | "limit" | "resource" | "work_state" | (string & {});
    state_category?: FactoryGraphWorkStateTypeValue;
}
export interface FactoryGraphBasePlaceNodeData extends Record<string, unknown> {
    activeFlow: boolean;
    activeItemLabels: string[];
    expanded?: boolean;
    focused?: boolean;
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    interactionOverlay?: FactoryGraphNodeInteractionOverlay;
    kind?: string;
    locale?: string;
    muted: boolean;
    onSelectStateNode?: (placeId: string) => void;
    place: FactoryGraphSemanticPlaceRef;
    selectedStateNode: boolean;
    tokenCount: number;
    validationError?: boolean;
    validationMessage?: string;
    resizeControls?: FactoryGraphNodeResizeControlsProps;
}
export interface FactoryGraphStatePositionNodeData extends FactoryGraphBasePlaceNodeData {
}
export interface FactoryGraphConstraintNodeData extends FactoryGraphBasePlaceNodeData {
}
export type FactoryGraphStatePositionNode = Node<FactoryGraphStatePositionNodeData, "statePosition">;
export type FactoryGraphConstraintNode = Node<FactoryGraphConstraintNodeData, "constraint">;
export type FactoryGraphPlaceNode = FactoryGraphConstraintNode | FactoryGraphStatePositionNode;
export declare function FactoryGraphStatePositionNodeView(props: NodeProps<FactoryGraphStatePositionNode>): import("react/jsx-runtime").JSX.Element;
export declare function FactoryGraphConstraintNodeView(props: NodeProps<FactoryGraphConstraintNode>): import("react/jsx-runtime").JSX.Element;
