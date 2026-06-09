/**
 * Maintainer note: work-state node surfaces intentionally use one color across
 * lifecycle phases. Icons still carry phase semantics without turning node
 * backgrounds into status colors.
 */
import { activityGraphNodeSurfaceClassName } from "../../../flowchart/components/current-activity-node-chrome";
import type { GraphSemanticIconKind } from "../../../flowchart/components/graph-semantic-icon";
import type { FactoryGraphWorkStateType } from "./factory-graph-work-state-type";

export const WORK_STATE_PHASE_LEGEND_ORDER = [
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
  "FAILED",
] as const satisfies readonly FactoryGraphWorkStateType[];

const WORK_STATE_SURFACE = activityGraphNodeSurfaceClassName("workState");

const WORK_STATE_PHASE_SURFACE: Record<FactoryGraphWorkStateType, string> = {
  INITIAL: WORK_STATE_SURFACE,
  PROCESSING: WORK_STATE_SURFACE,
  TERMINAL: WORK_STATE_SURFACE,
  FAILED: WORK_STATE_SURFACE,
};

const WORK_STATE_PHASE_ICON_KIND: Record<
  FactoryGraphWorkStateType,
  GraphSemanticIconKind
> = {
  INITIAL: "queue",
  PROCESSING: "processing",
  TERMINAL: "terminal",
  FAILED: "failed",
};

const WORK_STATE_PHASE_ICON_CLASS: Record<FactoryGraphWorkStateType, string> = {
  INITIAL: "text-info",
  PROCESSING: "text-warning",
  TERMINAL: "text-success",
  FAILED: "text-error",
};

export function workStatePhaseSwatchClassName(
  workStateType: FactoryGraphWorkStateType,
): string {
  return WORK_STATE_PHASE_SURFACE[workStateType];
}

export function workStatePhaseSurfaceClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  if (!workStateType) {
    return WORK_STATE_SURFACE;
  }

  return WORK_STATE_PHASE_SURFACE[workStateType];
}

export function workStatePhaseSemanticIconKind(
  workStateType: FactoryGraphWorkStateType | undefined,
): GraphSemanticIconKind {
  if (!workStateType) {
    return "queue";
  }

  return WORK_STATE_PHASE_ICON_KIND[workStateType];
}

export function workStatePhaseSemanticIconClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  if (!workStateType) {
    return "text-on-surface-variant";
  }

  return WORK_STATE_PHASE_ICON_CLASS[workStateType];
}
