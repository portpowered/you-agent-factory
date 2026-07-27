import type { Node, NodeProps } from "@xyflow/react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
import { type FactoryGraphPlaceRef } from "./semantic-support-nodes.js";
import { type FactoryGraphWorkStateType } from "./work-state-presentation.js";
export interface FactoryGraphSemanticPlaceRef extends FactoryGraphPlaceRef {
    kind: "constraint" | "limit" | "resource" | "work_state" | (string & {});
    state_category?: FactoryGraphWorkStateType;
}
export interface FactoryGraphBasePlaceNodeData extends Record<string, unknown> {
    activeFlow: boolean;
    activeItemLabels: string[];
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    kind?: string;
    locale?: string;
    muted: boolean;
    onSelectStateNode?: (placeId: string) => void;
    place: FactoryGraphSemanticPlaceRef;
    selectedStateNode: boolean;
    tokenCount: number;
    validationError?: boolean;
    validationMessage?: string;
}
export interface FactoryGraphStatePositionNodeData extends FactoryGraphBasePlaceNodeData {
}
export interface FactoryGraphConstraintNodeData extends FactoryGraphBasePlaceNodeData {
}
export type FactoryGraphStatePositionNode = Node<FactoryGraphStatePositionNodeData, "statePosition">;
export type FactoryGraphConstraintNode = Node<FactoryGraphConstraintNodeData, "constraint">;
export type FactoryGraphPlaceNode = FactoryGraphConstraintNode | FactoryGraphStatePositionNode;
export declare function FactoryGraphStatePositionNodeView(props: NodeProps<FactoryGraphStatePositionNode>): import("react").JSX.Element;
export declare function FactoryGraphConstraintNodeView(props: NodeProps<FactoryGraphConstraintNode>): import("react").JSX.Element;
export declare function FactoryGraphWorkProgressMarker(props: ({
    ariaLabel: string;
    className?: string;
    count: number;
    kind: "numeric";
} & ComponentPropsWithoutRef<"span">) | ({
    ariaLabel: string;
    className?: string;
    dotClassName?: string;
    dotCount: number;
    dotDataAttribute: string;
    kind: "dots";
    suffix?: ReactNode;
} & ComponentPropsWithoutRef<"span">)): import("react").JSX.Element;
