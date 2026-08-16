import type { GraphSemanticIconKind } from "./semantic-icon.js";
import {
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeVisualIconClassName,
  factoryGraphNodeVisualStatusSurfaceClassName,
} from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";

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

const ICON_KIND_BY_PHASE: Record<
  FactoryGraphWorkStateType,
  GraphSemanticIconKind
> = {
  INITIAL: "queue",
  PROCESSING: "processing",
  TERMINAL: "terminal",
  FAILED: "failed",
};

export function workStatePhaseSwatchClassName(
  workStateType: FactoryGraphWorkStateType,
): string {
  return workStatePhaseSurfaceClassName(workStateType);
}

export function workStatePhaseSurfaceClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  if (!workStateType) return factoryGraphNodeSurfaceClassName("workState");
  return factoryGraphNodeVisualStatusSurfaceClassName(
    resolveFactoryGraphVisualState({
      family: "work-state",
      lifecycle: workStateType,
    }).surface,
  );
}

export function workStatePhaseSemanticIconKind(
  workStateType: FactoryGraphWorkStateType | undefined,
): GraphSemanticIconKind {
  return workStateType ? ICON_KIND_BY_PHASE[workStateType] : "queue";
}

export function workStatePhaseSemanticIconClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  return factoryGraphNodeVisualIconClassName(
    resolveFactoryGraphVisualState({
      family: "work-state",
      lifecycle: workStateType,
    }),
    "text-on-surface-variant",
  );
}
