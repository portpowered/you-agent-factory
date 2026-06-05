import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  WorkstationKind,
  WorkstationType,
} from "../../../api/generated/openapi";
import { resolveEditableWorkstationBehavior } from "./workstation-behavior";
import { resolveEditableWorkstationType } from "./workstation-type";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export type WorkstationProgressOutcomeRouteContext = Pick<
  CanonicalWorkstation,
  "type" | "behavior" | "stopWords" | "classificationRoutes"
> & {
  assignedWorkerStopToken?: string;
};

export function workstationHasConfiguredStopWords(
  stopWords: readonly string[] | undefined,
): boolean {
  return (stopWords ?? []).some((word) => word.trim().length > 0);
}

export function workerHasConfiguredStopToken(
  stopToken: string | undefined,
): boolean {
  return (stopToken ?? "").trim().length > 0;
}

export function workstationHasEffectiveStopWords(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  return (
    workstationHasConfiguredStopWords(workstation.stopWords) ||
    workerHasConfiguredStopToken(workstation.assignedWorkerStopToken)
  );
}

export function isClassifierWorkstation(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  return (
    workstation.type === WorkstationType.WorkstationTypeClassifierWorkstation ||
    (workstation.classificationRoutes?.length ?? 0) > 0
  );
}

export function workstationSupportsProgressOutcomeRoutes(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  if (isClassifierWorkstation(workstation)) {
    return false;
  }

  if (
    resolveEditableWorkstationType(workstation) ===
    WorkstationType.WorkstationTypeLogicalMove
  ) {
    return false;
  }

  const behavior = resolveEditableWorkstationBehavior(workstation);
  if (behavior === WorkstationKind.WorkstationKindRepeater) {
    return true;
  }

  if (behavior !== WorkstationKind.WorkstationKindStandard) {
    return true;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    workstationType !== WorkstationType.WorkstationTypeModelWorkstation &&
    workstationType !== WorkstationType.WorkstationTypeModelInvoke
  ) {
    return true;
  }

  return workstationHasEffectiveStopWords(workstation);
}

/** True when the Failure progress-outcome connect anchor may render (false for logical-move only). */
export function workstationSupportsProgressOutcomeFailureRoute(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  return (
    resolveEditableWorkstationType(workstation) !==
    WorkstationType.WorkstationTypeLogicalMove
  );
}

/** True when Continue/Reject connect anchors are omitted for missing stop markers on a standard model processor. */
export function workstationHasZAxisIncompleteForConnections(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  if (isClassifierWorkstation(workstation)) {
    return false;
  }

  const behavior = resolveEditableWorkstationBehavior(workstation);
  if (behavior !== WorkstationKind.WorkstationKindStandard) {
    return false;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    workstationType !== WorkstationType.WorkstationTypeModelWorkstation &&
    workstationType !== WorkstationType.WorkstationTypeModelInvoke
  ) {
    return false;
  }

  return !workstationHasEffectiveStopWords(workstation);
}
