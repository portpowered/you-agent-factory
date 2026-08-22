import { type FactoryGraphGroupRegionInput } from "./group-region-presentation.js";
import type { FactoryGraphNodeFamily } from "./node-family.js";
import { type FactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
import { type FactoryGraphWorkstationSemantics } from "./workstation-semantics.js";
/** Canonical hosts that must retain the package-owned Factory graph contract. */
export declare const FACTORY_GRAPH_HOST_PARITY_HOSTS: readonly ["current-activity", "editor", "replay", "trace"];
export type FactoryGraphHostParityHost = (typeof FACTORY_GRAPH_HOST_PARITY_HOSTS)[number];
export type FactoryGraphHostParityField = "dimensions" | "family" | "groups" | "handles" | "semanticNodeId" | "type" | "visual" | "workProgress" | "workstationSemantics";
/**
 * Handles that are part of the shared rendered graph contract. Structural
 * work-type links and the approval output alias remain in host projections so
 * their edges keep working, but they are not interchangeable customer-facing
 * connection rails and are therefore excluded from cross-host comparisons.
 */
export declare const FACTORY_GRAPH_HOST_PARITY_HANDLE_CONTRACT: {
    readonly constraint: {
        readonly excluded: readonly [];
        readonly shared: readonly [];
    };
    readonly doc: {
        readonly excluded: readonly [];
        readonly shared: readonly [];
    };
    readonly resource: {
        readonly excluded: readonly [];
        readonly shared: readonly ["worker-resource-source", "workstation-resource-source"];
    };
    readonly worker: {
        readonly excluded: readonly [];
        readonly shared: readonly ["worker-assignment-source", "worker-input-target"];
    };
    readonly "work-state": {
        readonly excluded: readonly ["work-type-state-target"];
        readonly shared: readonly ["work-state-input-target", "workstation-input-source"];
    };
    readonly "work-type": {
        readonly excluded: readonly ["work-type-state-source"];
        readonly shared: readonly [];
    };
    readonly workstation: {
        readonly excluded: readonly ["workstation-approval-source"];
        readonly shared: readonly ["worker-assignment-target", "workstation-input-target", "workstation-on-continue-source", "workstation-on-failure-source", "workstation-on-rejection-source", "workstation-output-source", "workstation-resource-target"];
    };
};
export interface FactoryGraphHostParityHandle {
    id: string;
    side: "left" | "right";
    type: "source" | "target";
}
export interface FactoryGraphHostParityDimensions {
    height: number | undefined;
    initialHeight: number | undefined;
    initialWidth: number | undefined;
    measured: {
        height: number | undefined;
        width: number | undefined;
    };
    width: number | undefined;
}
export interface FactoryGraphHostParityWorkProgress {
    count: number;
    mode: ReturnType<typeof factoryGraphWorkProgressMode>;
}
export type FactoryGraphHostParityWorkstationSemantics = Pick<FactoryGraphWorkstationSemantics, "controlRole" | "runtimeRole" | "runtimeType" | "schedulingBehavior">;
export interface FactoryGraphHostParityNode {
    dimensions: FactoryGraphHostParityDimensions;
    family: FactoryGraphNodeFamily;
    handles: FactoryGraphHostParityHandle[];
    semanticNodeId: string;
    type: string;
    visual: FactoryGraphVisualState;
    workProgress: FactoryGraphHostParityWorkProgress;
    workstationSemantics?: FactoryGraphHostParityWorkstationSemantics;
}
export interface FactoryGraphHostParityGroup {
    bounds: {
        height: number;
        width: number;
        x: number;
        y: number;
    };
    color: string;
    id: string;
    label: string;
    nodeIds: string[];
}
export interface FactoryGraphHostParityNodeInput {
    data?: Record<string, unknown>;
    height?: number;
    id: string;
    initialHeight?: number;
    initialWidth?: number;
    measured?: {
        height?: number;
        width?: number;
    };
    selected?: boolean;
    type?: string;
    width?: number;
}
export interface FactoryGraphHostParityProjection {
    groups: FactoryGraphHostParityGroup[];
    host: string;
    nodeTypes: Readonly<Record<string, unknown>>;
    nodes: FactoryGraphHostParityNode[];
}
export interface FactoryGraphHostParityComparison {
    fields: readonly FactoryGraphHostParityField[];
    hosts: readonly string[];
    nodeIds?: readonly string[];
}
export interface AssertFactoryGraphHostParityInput {
    comparisons: readonly FactoryGraphHostParityComparison[];
    hosts: Readonly<Record<string, FactoryGraphHostParityProjection>>;
}
/**
 * Convert a host's React Flow nodes into the package-owned semantic contract.
 *
 * Interaction callbacks, editor draft badges, and trace-only labels are
 * deliberately omitted. They are overlays; the fields retained here are the
 * semantic surface that every Factory graph host must preserve.
 */
export declare function projectFactoryGraphHostParity(input: {
    groups?: readonly (FactoryGraphGroupRegionInput & {
        nodeIds?: readonly string[];
    })[];
    host: string;
    nodeTypes: Readonly<Record<string, unknown>>;
    nodes: readonly FactoryGraphHostParityNodeInput[];
}): FactoryGraphHostParityProjection;
/**
 * Assert one canonical behavioral contract across selected graph hosts.
 *
 * The comparisons are explicit because trace dispatch nodes intentionally own
 * different runtime overlays and IDs. A caller can compare their shared
 * semantic fields without pretending that trace history is live activity.
 */
export declare function assertFactoryGraphHostParity(input: AssertFactoryGraphHostParityInput): void;
