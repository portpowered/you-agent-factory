import { type GraphNodeHandle } from "@you-agent-factory/components/graphs";
import type { ReactNode } from "react";
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
    nodeType: "workstation" | FactoryGraphPlaceNodeType;
    zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
}
/** Original semantic Factory node frame, including its typed connection rails. */
export declare function FactoryGraphNodeShell({ children, className, handles, nodeType, zAxisIncompleteHints, }: FactoryGraphNodeShellProps): import("react").JSX.Element;
export declare function factoryGraphHandleToneFromId(handleId: string): NonNullable<GraphNodeHandle["tone"]>;
