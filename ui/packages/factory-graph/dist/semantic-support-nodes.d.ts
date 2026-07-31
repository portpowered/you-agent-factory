import type { Node, NodeProps } from "@xyflow/react";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
/** The portion of a Factory place needed by the original semantic node views. */
export interface FactoryGraphPlaceRef {
    kind?: string;
    place_id: string;
    state_value?: string | null;
    type_id?: string | null;
}
export interface FactoryGraphWorkerNodeData extends Record<string, unknown> {
    activeFlow: boolean;
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    kind: "worker";
    locale?: string;
    muted: boolean;
    onSelectWorker?: (workerName: string) => void;
    place: FactoryGraphPlaceRef;
    selectedWorker: boolean;
}
export type FactoryGraphWorkerNode = Node<FactoryGraphWorkerNodeData, "worker">;
export interface FactoryGraphWorkTypeNodeData extends Record<string, unknown> {
    activeFlow: boolean;
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    isDefaultWorkType?: boolean;
    kind: "work-type";
    locale?: string;
    muted: boolean;
    onSelectWorkType?: (workTypeName: string) => void;
    place: FactoryGraphPlaceRef;
    selectedWorkType?: boolean;
    validationError?: boolean;
    validationMessage?: string;
}
export type FactoryGraphWorkTypeNode = Node<FactoryGraphWorkTypeNodeData, "workType">;
export interface FactoryGraphResourceNodeData extends Record<string, unknown> {
    activeFlow: boolean;
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    kind: "resource";
    locale?: string;
    muted: boolean;
    onSelectResource?: (resourceName: string) => void;
    place: FactoryGraphPlaceRef;
    selectedResource: boolean;
    tokenCount: number;
}
export type FactoryGraphResourceNode = Node<FactoryGraphResourceNodeData, "resource">;
/** Original Factory worker node, with host-owned worker selection. */
export declare function FactoryGraphWorkerNodeView({ data, }: NodeProps<FactoryGraphWorkerNode>): import("react").JSX.Element;
/** Original Factory work-type node, with host-owned selection and validation. */
export declare function FactoryGraphWorkTypeNodeView({ data, }: NodeProps<FactoryGraphWorkTypeNode>): import("react").JSX.Element;
/** Original Factory resource node, with host-owned resource selection. */
export declare function FactoryGraphResourceNodeView({ data, }: NodeProps<FactoryGraphResourceNode>): import("react").JSX.Element;
export declare function FactoryGraphNodeBadge({ children, className, tone, weight, ...rest }: ComponentPropsWithoutRef<"span"> & {
    children: ReactNode;
    tone?: "danger" | "info" | "neutral" | "success" | "warning";
    weight?: "body" | "label";
}): import("react").JSX.Element;
