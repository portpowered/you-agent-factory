import type { Node, NodeProps } from "@xyflow/react";
import { type FactoryGraphNodeHandle, type FactoryGraphZAxisIncompleteHints } from "./semantic-node-shell.js";
import { type FactoryGraphWorkItemRef, type FactoryGraphWorkstationRef } from "./semantic-workstation-presentation.js";
export type { FactoryGraphWorkItemRef, FactoryGraphWorkstationRef, } from "./semantic-workstation-presentation.js";
export interface FactoryGraphActiveExecution {
    dispatch_id: string;
    started_at: string;
    work_items?: FactoryGraphWorkItemRef[];
}
export interface FactoryGraphWorkstationNodeData extends Record<string, unknown> {
    active: boolean;
    activeFlow: boolean;
    executions: FactoryGraphActiveExecution[];
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    kind?: "workstation";
    locale?: string;
    muted: boolean;
    now: number;
    progressOutcomeRouteWorkstation?: unknown;
    selectedWorkID: string | null;
    selectedWorkstation: boolean;
    summaryOnly?: boolean;
    workstation: FactoryGraphWorkstationRef;
    zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
    onSelectWorkstation?: (nodeId: string) => void;
    onSelectWorkID?: (workID: string, hint?: {
        dispatchID?: string;
        nodeID?: string;
    }) => void;
}
export type FactoryGraphWorkstationNode = Node<FactoryGraphWorkstationNodeData, "workstation">;
/** Original Factory workstation presentation, with host-owned selection callbacks. */
export declare function FactoryGraphWorkstationNodeView({ data, }: NodeProps<FactoryGraphWorkstationNode>): import("react").JSX.Element;
