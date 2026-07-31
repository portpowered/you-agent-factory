import { type Edge } from "@xyflow/react";
import type { FactoryGraphSource } from "./source.js";
import { type FactoryGraphNode } from "./semantic-nodes.js";
export interface FactoryGraphReplaySurfaceProps {
    className?: string;
    onSelectNode?: (nodeId: string) => void;
    selectedNodeId?: string;
    source: FactoryGraphSource;
}
/**
 * Read-only canonical Factory graph for replay and emulator hosts.
 * It consumes the complete Factory and its selected-tick runtime projection,
 * retaining the authored Factory layout rather than re-inventing topology.
 */
export declare function FactoryGraphReplaySurface({ className, onSelectNode, selectedNodeId, source, }: FactoryGraphReplaySurfaceProps): import("react").JSX.Element;
export interface FactoryGraphReplayFlow {
    edges: Edge[];
    nodes: FactoryGraphNode[];
}
/** Project replay data into the original Factory semantic node family. */
export declare function projectFactoryGraphReplayFlow(source: FactoryGraphSource, selectedNodeId?: string): FactoryGraphReplayFlow;
