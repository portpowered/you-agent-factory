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
    if (!workStateType)
        return factoryGraphNodeSurfaceClassName("workState");
    return factoryGraphNodeVisualStatusSurfaceClassName(resolveFactoryGraphVisualState({
        family: "work-state",
        lifecycle: workStateType,
    }).surface);
}
export function workStatePhaseSemanticIconKind(workStateType) {
    return workStateType ? ICON_KIND_BY_PHASE[workStateType] : "queue";
}
export function workStatePhaseSemanticIconClassName(workStateType) {
    return factoryGraphNodeVisualIconClassName(resolveFactoryGraphVisualState({
        family: "work-state",
        lifecycle: workStateType,
    }), "text-on-surface-variant");
}
