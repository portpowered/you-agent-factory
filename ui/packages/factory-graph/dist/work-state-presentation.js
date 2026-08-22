import { factoryGraphNodeSurfaceClassName, factoryGraphNodeVisualIconClassName, factoryGraphNodeVisualStatusSurfaceClassName, } from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
export const FACTORY_GRAPH_WORK_STATE_TYPES = [
    "INITIAL",
    "PROCESSING",
    "TERMINAL",
    "FAILED",
];
export const WORK_STATE_PHASE_LEGEND_ORDER = [
    "INITIAL",
    "PROCESSING",
    "TERMINAL",
    "FAILED",
];
const ICON_KIND_BY_PHASE = {
    INITIAL: "queue",
    PROCESSING: "processing",
    TERMINAL: "terminal",
    FAILED: "failed",
};
export function workStatePhaseSwatchClassName(workStateType) {
    return workStatePhaseSurfaceClassName(workStateType);
}
export function workStatePhaseSurfaceClassName(workStateType) {
    if (!isFactoryGraphKnownWorkStateType(workStateType)) {
        return factoryGraphNodeSurfaceClassName("neutral");
    }
    return factoryGraphNodeVisualStatusSurfaceClassName(resolveFactoryGraphVisualState({
        family: "work-state",
        lifecycle: workStateType,
    }).surface);
}
export function workStatePhaseSemanticIconKind(workStateType) {
    return isFactoryGraphKnownWorkStateType(workStateType)
        ? ICON_KIND_BY_PHASE[workStateType]
        : "queue";
}
export function workStatePhaseSemanticIconClassName(workStateType) {
    const lifecycle = isFactoryGraphKnownWorkStateType(workStateType)
        ? workStateType
        : undefined;
    return factoryGraphNodeVisualIconClassName(resolveFactoryGraphVisualState({
        family: "work-state",
        lifecycle,
    }), "text-on-surface-variant");
}
export function isFactoryGraphKnownWorkStateType(workStateType) {
    return (typeof workStateType === "string" &&
        FACTORY_GRAPH_WORK_STATE_TYPES.includes(workStateType));
}
/** Returns an unfamiliar category unchanged for a neutral raw-value label. */
export function factoryGraphUnknownWorkStateType(workStateType) {
    return typeof workStateType === "string" &&
        workStateType.length > 0 &&
        !isFactoryGraphKnownWorkStateType(workStateType)
        ? workStateType
        : undefined;
}
