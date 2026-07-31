import {
  workStatePhaseSemanticIconClassName as packageWorkStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind as packageWorkStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName as packageWorkStatePhaseSurfaceClassName,
  workStatePhaseSwatchClassName as packageWorkStatePhaseSwatchClassName,
  WORK_STATE_PHASE_LEGEND_ORDER as packageWorkStatePhaseLegendOrder,
} from "@you-agent-factory/factory-graph";
import type { GraphSemanticIconKind } from "@you-agent-factory/factory-graph";
import type { FactoryGraphWorkStateType } from "./factory-graph-work-state-type";

export const WORK_STATE_PHASE_LEGEND_ORDER =
  packageWorkStatePhaseLegendOrder satisfies readonly FactoryGraphWorkStateType[];

export function workStatePhaseSwatchClassName(
  workStateType: FactoryGraphWorkStateType,
): string {
  return packageWorkStatePhaseSwatchClassName(workStateType);
}

export function workStatePhaseSurfaceClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  return packageWorkStatePhaseSurfaceClassName(workStateType);
}

export function workStatePhaseSemanticIconKind(
  workStateType: FactoryGraphWorkStateType | undefined,
): GraphSemanticIconKind {
  return packageWorkStatePhaseSemanticIconKind(workStateType);
}

export function workStatePhaseSemanticIconClassName(
  workStateType: FactoryGraphWorkStateType | undefined,
): string {
  return packageWorkStatePhaseSemanticIconClassName(workStateType);
}
