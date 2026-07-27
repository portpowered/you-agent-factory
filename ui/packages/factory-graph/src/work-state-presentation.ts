import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { factoryGraphNodeSurfaceClassName } from "./semantic-node-style.js";

export const FACTORY_GRAPH_WORK_STATE_TYPES = [
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
  "FAILED",
] as const;

export type FactoryGraphWorkStateType =
  (typeof FACTORY_GRAPH_WORK_STATE_TYPES)[number];

export const WORK_STATE_PHASE_LEGEND_ORDER = [
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
  "FAILED",
] as const satisfies readonly FactoryGraphWorkStateType[];

const WORK_STATE_SURFACE = factoryGraphNodeSurfaceClassName("workState");

const ICON_KIND_BY_PHASE: Record<
  FactoryGraphWorkStateType,
  GraphSemanticIconKind
> = {
  INITIAL: "queue",
  PROCESSING: "processing",
  TERMINAL: "terminal",
  FAILED: "failed",
};

const ICON_CLASS_BY_PHASE: Record<FactoryGraphWorkStateType, string> = {
  INITIAL: "text-info",
  PROCESSING: "text-warning",
  TERMINAL: "text-success",
  FAILED: "text-error",
};

export function workStatePhaseSwatchClassName(
  _workStateType: FactoryGraphWorkStateType,
): string {
  return WORK_STATE_SURFACE;
}

export function workStatePhaseSurfaceClassName(
  _workStateType: FactoryGraphWorkStateType | undefined,
): string {
  return WORK_STATE_SURFACE;
}

export function workStatePhaseSemanticIconKind(
  workStateType: FactoryGraphWorkStateType | undefined,
): GraphSemanticIconKind {
  return workStateType ? ICON_KIND_BY_PHASE[workStateType] : "queue";
}

export function workStatePhaseSemanticIconClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  return workStateType
    ? ICON_CLASS_BY_PHASE[workStateType]
    : "text-on-surface-variant";
}
