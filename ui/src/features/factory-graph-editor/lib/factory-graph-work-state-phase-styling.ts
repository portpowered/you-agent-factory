import type { GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";
import type { FactoryGraphWorkStateType } from "./factory-graph-work-state-type";

const NEUTRAL_WORK_STATE_SURFACE =
  "border-af-border-strong bg-af-surface-raised";

const WORK_STATE_PHASE_SURFACE: Record<
  FactoryGraphWorkStateType,
  string
> = {
  INITIAL: "border-af-info-border bg-af-info-surface",
  PROCESSING: "border-af-warning-border bg-af-warning-surface",
  TERMINAL: "border-af-success-border bg-af-success-surface",
  FAILED: "border-af-danger-border bg-af-danger-surface",
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

const WORK_STATE_PHASE_ICON_CLASS: Record<FactoryGraphWorkStateType, string> =
  {
    INITIAL: "text-af-info",
    PROCESSING: "text-af-warning",
    TERMINAL: "text-af-success",
    FAILED: "text-af-danger",
  };

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
    return "text-af-text-muted";
  }

  return WORK_STATE_PHASE_ICON_CLASS[workStateType];
}
