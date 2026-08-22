import type { components, FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryGraphSource } from "./source.js";
type FactoryWorkstation = NonNullable<FactoryDefinition["workstations"]>[number];
export type FactoryGraphWorkstationRuntimeType = components["schemas"]["WorkstationType"] | "UNKNOWN";
export type FactoryGraphWorkstationRuntimeRole = "AGENT" | "CLASSIFIER" | "HUMAN_APPROVAL" | "INFERENCE" | "LOGICAL_MOVE" | "POLLER" | "SCRIPT" | "UNKNOWN";
export type FactoryGraphWorkstationSchedulingBehavior = components["schemas"]["WorkstationKind"] | "UNKNOWN";
export type FactoryGraphWorkstationControlRole = "CLASSIFIER" | "HUMAN_APPROVAL" | "LOGICAL_ROUTER" | "LOOP_BREAKER" | "NONE" | "UNKNOWN";
export interface FactoryGraphWorkstationGuardedControl {
    guardType: "VISIT_COUNT";
    limit: {
        argument?: string;
        fixed?: number;
    };
    targetWorkstation: string;
}
export interface FactoryGraphWorkstationSemantics {
    controlRole: FactoryGraphWorkstationControlRole;
    guardedControl?: FactoryGraphWorkstationGuardedControl;
    runtimeRole: FactoryGraphWorkstationRuntimeRole;
    runtimeType: FactoryGraphWorkstationRuntimeType;
    schedulingBehavior: FactoryGraphWorkstationSchedulingBehavior;
}
export interface FactoryGraphWorkstationActivityProjection {
    active: boolean;
    activeDispatchIds: string[];
}
export interface FactoryGraphWorkstationSemanticProjection extends FactoryGraphWorkstationSemantics {
    activity: FactoryGraphWorkstationActivityProjection;
    authoredWorkstationId?: string;
    nodeId: string;
    workstationId: string;
    workstationName: string;
}
export declare const UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS: {
    readonly controlRole: "UNKNOWN";
    readonly runtimeRole: "UNKNOWN";
    readonly runtimeType: "UNKNOWN";
    readonly schedulingBehavior: "UNKNOWN";
};
/** Resolve the canonical runtime type without inferring it from worker data. */
export declare function resolveFactoryGraphWorkstationRuntimeType(workstation: Pick<FactoryWorkstation, "type"> | undefined): FactoryGraphWorkstationRuntimeType;
/** Map canonical and legacy runtime type values onto graph-level roles. */
export declare function factoryGraphWorkstationRuntimeRole(runtimeType: FactoryGraphWorkstationRuntimeType): FactoryGraphWorkstationRuntimeRole;
/** Resolve scheduling independently from runtime implementation type. */
export declare function resolveFactoryGraphWorkstationSchedulingBehavior(workstation: Pick<FactoryWorkstation, "behavior"> | undefined): FactoryGraphWorkstationSchedulingBehavior;
/**
 * Project one authored workstation. Missing or invalid type metadata stays
 * neutral; worker presence, names, and topology shape never fill it in.
 */
export declare function resolveFactoryGraphWorkstationSemantics(workstation: FactoryWorkstation | undefined): FactoryGraphWorkstationSemantics;
/**
 * Join authored workstation semantics to selected-tick activity by durable
 * workstation identity. Names are a fallback only for id-less legacy authors.
 */
export declare function projectFactoryGraphWorkstationSemantics(source: FactoryGraphSource): FactoryGraphWorkstationSemanticProjection[];
export {};
