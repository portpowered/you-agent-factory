import type { GraphSemanticIconKind } from "./semantic-icon.js";
export declare const FACTORY_GRAPH_WORK_STATE_TYPES: readonly ["INITIAL", "PROCESSING", "TERMINAL", "FAILED"];
export type FactoryGraphWorkStateType = (typeof FACTORY_GRAPH_WORK_STATE_TYPES)[number];
export declare const WORK_STATE_PHASE_LEGEND_ORDER: readonly ["INITIAL", "PROCESSING", "TERMINAL", "FAILED"];
export declare function workStatePhaseSwatchClassName(_workStateType: FactoryGraphWorkStateType): string;
export declare function workStatePhaseSurfaceClassName(_workStateType: FactoryGraphWorkStateType | undefined): string;
export declare function workStatePhaseSemanticIconKind(workStateType: FactoryGraphWorkStateType | undefined): GraphSemanticIconKind;
export declare function workStatePhaseSemanticIconClassName(workStateType: FactoryGraphWorkStateType | undefined): string;
