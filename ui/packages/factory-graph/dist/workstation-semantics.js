import { WorkstationGuardType, WorkstationKind, WorkstationType, } from "@you-agent-factory/client";
export const UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS = {
    controlRole: "UNKNOWN",
    runtimeRole: "UNKNOWN",
    runtimeType: "UNKNOWN",
    schedulingBehavior: "UNKNOWN",
};
const WORKSTATION_TYPE_VALUES = new Set(Object.values(WorkstationType));
const WORKSTATION_KIND_VALUES = new Set(Object.values(WorkstationKind));
/** Resolve the canonical runtime type without inferring it from worker data. */
export function resolveFactoryGraphWorkstationRuntimeType(workstation) {
    const normalized = normalizeEnumValue(workstation?.type);
    return normalized && WORKSTATION_TYPE_VALUES.has(normalized)
        ? normalized
        : "UNKNOWN";
}
/** Map canonical and legacy runtime type values onto graph-level roles. */
export function factoryGraphWorkstationRuntimeRole(runtimeType) {
    switch (runtimeType) {
        case WorkstationType.AGENT_RUN:
        case WorkstationType.MODEL_WORKSTATION:
            return "AGENT";
        case WorkstationType.CLASSIFIER_WORKSTATION:
            return "CLASSIFIER";
        case WorkstationType.INFERENCE_RUN:
        case WorkstationType.MODEL_INVOKE:
            return "INFERENCE";
        case WorkstationType.LOGICAL_MOVE:
            return "LOGICAL_MOVE";
        case WorkstationType.POLLER_RUN:
            return "POLLER";
        case WorkstationType.SCRIPT_RUN:
            return "SCRIPT";
        default:
            return "UNKNOWN";
    }
}
/** Resolve scheduling independently from runtime implementation type. */
export function resolveFactoryGraphWorkstationSchedulingBehavior(workstation) {
    if (!workstation)
        return "UNKNOWN";
    const normalized = normalizeEnumValue(workstation.behavior);
    return normalized && WORKSTATION_KIND_VALUES.has(normalized)
        ? normalized
        : "UNKNOWN";
}
/**
 * Project one authored workstation. Missing or invalid type metadata stays
 * neutral; worker presence, names, and topology shape never fill it in.
 */
export function resolveFactoryGraphWorkstationSemantics(workstation) {
    if (!workstation)
        return UNKNOWN_FACTORY_GRAPH_WORKSTATION_SEMANTICS;
    const runtimeType = resolveFactoryGraphWorkstationRuntimeType(workstation);
    const runtimeRole = factoryGraphWorkstationRuntimeRole(runtimeType);
    return {
        controlRole: workstationControlRole(runtimeRole, workstation),
        runtimeRole,
        runtimeType,
        schedulingBehavior: resolveFactoryGraphWorkstationSchedulingBehavior(workstation),
    };
}
/**
 * Join authored workstation semantics to selected-tick activity by durable
 * workstation identity. Names are a fallback only for id-less legacy authors.
 */
export function projectFactoryGraphWorkstationSemantics(source) {
    const authored = source.factory.workstations ?? [];
    const topologyWorkstations = source.runtime.topology.nodes.filter((node) => node.kind === "workstation");
    return topologyWorkstations.map((node) => {
        const authoredWorkstation = authoredWorkstationForNode(authored, node);
        const semantics = resolveFactoryGraphWorkstationSemantics(authoredWorkstation);
        const activeDispatchIds = source.runtime.activity.activeDispatchOverlays
            .filter((overlay) => overlayMatchesWorkstation(overlay, node.entityId, node.id))
            .map((overlay) => overlay.dispatchId)
            .sort();
        const active = activeDispatchIds.length > 0 ||
            source.runtime.activity.activeWorkstationNodeIds.includes(node.id);
        return {
            ...semantics,
            activity: { active, activeDispatchIds },
            ...(authoredWorkstation
                ? {
                    authoredWorkstationId: authoredWorkstation.id?.trim() || authoredWorkstation.name,
                }
                : {}),
            nodeId: node.id,
            workstationId: node.entityId,
            workstationName: node.label,
        };
    });
}
function authoredWorkstationForNode(workstations, node) {
    return workstations.find((workstation) => {
        const authoredId = workstation.id?.trim();
        return authoredId
            ? authoredId === node.entityId
            : workstation.name === node.label;
    });
}
function overlayMatchesWorkstation(overlay, workstationId, workstationNodeId) {
    return (overlay.workstationNodeId === workstationNodeId ||
        overlay.workstationId === workstationId);
}
function workstationControlRole(runtimeRole, workstation) {
    if (runtimeRole === "UNKNOWN")
        return "UNKNOWN";
    if (runtimeRole === "CLASSIFIER")
        return "CLASSIFIER";
    if (runtimeRole !== "LOGICAL_MOVE")
        return "NONE";
    return hasSupportedVisitCountGuard(workstation.guards)
        ? "LOOP_BREAKER"
        : "LOGICAL_ROUTER";
}
function hasSupportedVisitCountGuard(guards) {
    return (guards ?? []).some((guard) => {
        const type = normalizeEnumValue(guard.type);
        if (type !== WorkstationGuardType.VISIT_COUNT)
            return false;
        if (!guard.workstation?.trim())
            return false;
        const fixedLimit = typeof guard.maxVisits === "number" &&
            Number.isInteger(guard.maxVisits) &&
            guard.maxVisits > 0;
        const argumentLimit = Boolean(guard.maxVisitsArgument?.trim());
        return fixedLimit || argumentLimit;
    });
}
function normalizeEnumValue(value) {
    const normalized = value?.trim().toUpperCase();
    return normalized || undefined;
}
