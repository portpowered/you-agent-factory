import type { Node, NodeProps } from "@xyflow/react";
import { type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
export interface FactoryGraphDocNodeData extends Record<string, unknown> {
    displayLabel: string;
    factoryGraphNodeId?: string;
    fileType?: string;
    handles: FactoryGraphNodeHandle[];
    kind: "doc";
    locale?: string;
    onSelectDoc?: (targetPath: string) => void;
    selectedDoc: boolean;
    targetPath: string;
}
export type FactoryGraphDocNode = Node<FactoryGraphDocNodeData, "doc">;
/** Original Factory document node, with host-owned selection callback. */
export declare function FactoryGraphDocNodeView({ data, }: NodeProps<FactoryGraphDocNode>): import("react").JSX.Element;
