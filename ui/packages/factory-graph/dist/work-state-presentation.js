import { factoryGraphNodeSurfaceClassName } from "./semantic-node-style.js";
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
const WORK_STATE_SURFACE = factoryGraphNodeSurfaceClassName("workState");
const ICON_KIND_BY_PHASE = {
    INITIAL: "queue",
    PROCESSING: "processing",
    TERMINAL: "terminal",
    FAILED: "failed",
};
const ICON_CLASS_BY_PHASE = {
    INITIAL: "text-info",
    PROCESSING: "text-warning",
    TERMINAL: "text-success",
    FAILED: "text-error",
};
export function workStatePhaseSwatchClassName(_workStateType) {
    return WORK_STATE_SURFACE;
}
export function workStatePhaseSurfaceClassName(_workStateType) {
    return WORK_STATE_SURFACE;
}
export function workStatePhaseSemanticIconKind(workStateType) {
    return workStateType ? ICON_KIND_BY_PHASE[workStateType] : "queue";
}
export function workStatePhaseSemanticIconClassName(workStateType) {
    return workStateType
        ? ICON_CLASS_BY_PHASE[workStateType]
        : "text-on-surface-variant";
}
