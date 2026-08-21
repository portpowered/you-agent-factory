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

/** Canonical work-state categories may grow without requiring a UI release. */
export type FactoryGraphWorkStateTypeValue =
  | FactoryGraphWorkStateType
  | (string & {});

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
  workStateType: FactoryGraphWorkStateTypeValue | undefined,
): string {
  if (!isFactoryGraphKnownWorkStateType(workStateType)) {
    return factoryGraphNodeSurfaceClassName("neutral");
  }
  return factoryGraphNodeVisualStatusSurfaceClassName(
    resolveFactoryGraphVisualState({
      family: "work-state",
      lifecycle: workStateType,
    }).surface,
  );
}

export function workStatePhaseSemanticIconKind(
  workStateType: FactoryGraphWorkStateTypeValue | undefined,
): GraphSemanticIconKind {
  return isFactoryGraphKnownWorkStateType(workStateType)
    ? ICON_KIND_BY_PHASE[workStateType]
    : "queue";
}

export function workStatePhaseSemanticIconClassName(
  workStateType: FactoryGraphWorkStateTypeValue | undefined,
): string {
  return factoryGraphNodeVisualIconClassName(
    resolveFactoryGraphVisualState({
      family: "work-state",
      lifecycle: workStateType,
    }),
    "text-on-surface-variant",
  );
}

export function isFactoryGraphKnownWorkStateType(
  workStateType: FactoryGraphWorkStateTypeValue | undefined,
): workStateType is FactoryGraphWorkStateType {
  return (
    typeof workStateType === "string" &&
    FACTORY_GRAPH_WORK_STATE_TYPES.includes(
      workStateType.trim() as FactoryGraphWorkStateType,
    )
  );
}

/** Returns an unfamiliar category unchanged for a neutral raw-value label. */
export function factoryGraphUnknownWorkStateType(
  workStateType: FactoryGraphWorkStateTypeValue | undefined,
): string | undefined {
  return typeof workStateType === "string" &&
    workStateType.length > 0 &&
    !isFactoryGraphKnownWorkStateType(workStateType)
    ? workStateType
    : undefined;
}
