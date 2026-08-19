import type { Node, NodeProps } from "@xyflow/react";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { type FactoryGraphNodeHandle, type FactoryGraphZAxisIncompleteHints } from "./semantic-node-shell.js";
import { type FactoryGraphWorkItemRef, type FactoryGraphWorkstationRef, type FactoryGraphWorkstationPresentation as WorkstationPresentation } from "./semantic-workstation-presentation.js";
import type { FactoryGraphWorkstationSemantics } from "./workstation-semantics.js";
export type { FactoryGraphWorkItemRef, FactoryGraphWorkstationRef, } from "./semantic-workstation-presentation.js";
export interface FactoryGraphActiveExecution {
    dispatch_id: string;
    started_at: string;
    work_items?: FactoryGraphWorkItemRef[];
}
export interface FactoryGraphWorkstationNodeData extends Record<string, unknown> {
    active: boolean;
    activeFlow: boolean;
    focused?: boolean;
    executions: FactoryGraphActiveExecution[];
    factoryGraphNodeId?: string;
    handles: FactoryGraphNodeHandle[];
    interactionOverlay?: FactoryGraphNodeInteractionOverlay;
    kind?: "workstation";
    locale?: string;
    muted: boolean;
    now: number;
    progressOutcomeRouteWorkstation?: unknown;
    runtimeStatus?: string | null;
    selectedWorkID: string | null;
    selectedWorkstation: boolean;
    resizeControls?: FactoryGraphNodeResizeControlsProps;
    summaryOnly?: boolean;
    workstation: FactoryGraphWorkstationRef;
    workstationSemantics?: FactoryGraphWorkstationSemantics;
    zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
    onSelectWorkstation?: (nodeId: string) => void;
    onSelectWorkID?: (workID: string, hint?: {
        dispatchID?: string;
        nodeID?: string;
    }) => void;
    validationError?: boolean;
}
export type FactoryGraphWorkstationNode = Node<FactoryGraphWorkstationNodeData, "workstation">;
/** Original Factory workstation presentation, with host-owned selection callbacks. */
export declare function FactoryGraphWorkstationNodeView({ data, selected: reactFlowSelected, }: NodeProps<FactoryGraphWorkstationNode>): import("react").JSX.Element;
export declare function FactoryGraphWorkstationGuardedControlCard({ locale, presentation, }: {
    locale?: string;
    presentation: WorkstationPresentation;
}): import("react").JSX.Element | null;
