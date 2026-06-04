/**
 * Maintainer note: factory graph editor work-state node colors map canonical
 * `WorkStateType` values from the factory definition to semantic tokens:
 * - INITIAL → af-info (blue)
 * - PROCESSING → af-warning (yellow)
 * - TERMINAL → af-success (green)
 * - FAILED → af-danger (red)
 *
 * Dashboard workflow graph work-state nodes reuse these helpers from
 * `current-activity-place-node.tsx` via `place.state_category`.
 */
import type { GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";
import type { FactoryGraphWorkStateType } from "./factory-graph-work-state-type";

export const WORK_STATE_PHASE_LEGEND_ORDER = [
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
  "FAILED",
] as const satisfies readonly FactoryGraphWorkStateType[];

const NEUTRAL_WORK_STATE_SURFACE =
  "border-outline-variant bg-surface-container-high";

const WORK_STATE_PHASE_SURFACE: Record<FactoryGraphWorkStateType, string> = {
  INITIAL: "border-info-border bg-info-container",
  PROCESSING: "border-af-warning-border bg-warning-container",
  TERMINAL: "border-af-success-border bg-success-container",
  FAILED: "border-af-danger-border bg-error-container",
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
    return NEUTRAL_WORK_STATE_SURFACE;
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
