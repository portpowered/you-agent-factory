import type { Node, NodeProps } from "@xyflow/react";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
export interface FactoryGraphDocNodeData extends Record<string, unknown> {
    activeFlow?: boolean;
    displayLabel: string;
    focused?: boolean;
    factoryGraphNodeId?: string;
    fileType?: string;
    handles: FactoryGraphNodeHandle[];
    interactionOverlay?: FactoryGraphNodeInteractionOverlay;
    kind: "doc";
    locale?: string;
    onSelectDoc?: (targetPath: string) => void;
    selectedDoc: boolean;
    targetPath: string;
    validationError?: boolean;
    muted?: boolean;
    resizeControls?: FactoryGraphNodeResizeControlsProps;
}
export type FactoryGraphDocNode = Node<FactoryGraphDocNodeData, "doc">;
/** Original Factory document node, with host-owned selection callback. */
export declare function FactoryGraphDocNodeView({ data, selected: reactFlowSelected, }: NodeProps<FactoryGraphDocNode>): import("react").JSX.Element;
