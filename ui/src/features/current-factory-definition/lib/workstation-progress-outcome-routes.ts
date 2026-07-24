import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  WorkstationKind,
  WorkstationType,
} from "../../../api/generated/openapi";
import {
  isAgentRunWorkstationType,
  isInferenceRunWorkstationType,
} from "./worker-workstation-taxonomy";
import { resolveEditableWorkstationType } from "./workstation/workstation-type";
import { resolveEditableWorkstationBehavior } from "./workstation-behavior";

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
    workstation.type === WorkstationType.CLASSIFIER_WORKSTATION ||
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
    resolveEditableWorkstationType(workstation) === WorkstationType.LOGICAL_MOVE
  ) {
    return false;
  }

  const behavior = resolveEditableWorkstationBehavior(workstation);
  if (behavior === WorkstationKind.REPEATER) {
    return true;
  }

  if (behavior !== WorkstationKind.STANDARD) {
    return true;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    !isAgentRunWorkstationType(workstationType) &&
    !isInferenceRunWorkstationType(workstationType)
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
    resolveEditableWorkstationType(workstation) !== WorkstationType.LOGICAL_MOVE
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
  if (behavior !== WorkstationKind.STANDARD) {
    return false;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    !isAgentRunWorkstationType(workstationType) &&
    !isInferenceRunWorkstationType(workstationType)
  ) {
    return false;
  }

  return !workstationHasEffectiveStopWords(workstation);
}
