import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { resolveEditableWorkstationBehavior } from "./workstation-behavior";
import { resolveEditableWorkstationType } from "./workstation-type";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export type WorkstationProgressOutcomeRouteContext = Pick<
  CanonicalWorkstation,
  "type" | "behavior" | "stopWords" | "classificationRoutes"
>;

export function workstationHasConfiguredStopWords(
  stopWords: readonly string[] | undefined,
): boolean {
  return (stopWords ?? []).some((word) => word.trim().length > 0);
}

export function isClassifierWorkstation(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  return (
    workstation.type === "CLASSIFIER_WORKSTATION" ||
    (workstation.classificationRoutes?.length ?? 0) > 0
  );
}

export function workstationSupportsProgressOutcomeRoutes(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  if (isClassifierWorkstation(workstation)) {
    return false;
  }

  if (resolveEditableWorkstationType(workstation) === "LOGICAL_MOVE") {
    return false;
  }

  const behavior = resolveEditableWorkstationBehavior(workstation);
  if (behavior === "REPEATER") {
    return true;
  }

  if (behavior !== "STANDARD") {
    return true;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    workstationType !== "MODEL_WORKSTATION" &&
    workstationType !== "MODEL_INVOKE"
  ) {
    return true;
  }

  return workstationHasConfiguredStopWords(workstation.stopWords);
}

/** True when the Failure progress-outcome connect anchor may render (false for logical-move only). */
export function workstationSupportsProgressOutcomeFailureRoute(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  return resolveEditableWorkstationType(workstation) !== "LOGICAL_MOVE";
}

/** True when Continue/Reject connect anchors are omitted for missing stopWords on a standard model processor. */
export function workstationHasZAxisIncompleteForConnections(
  workstation: WorkstationProgressOutcomeRouteContext,
): boolean {
  if (isClassifierWorkstation(workstation)) {
    return false;
  }

  const behavior = resolveEditableWorkstationBehavior(workstation);
  if (behavior !== "STANDARD") {
    return false;
  }

  const workstationType = resolveEditableWorkstationType(workstation);
  if (
    workstationType !== "MODEL_WORKSTATION" &&
    workstationType !== "MODEL_INVOKE"
  ) {
    return false;
  }

  return !workstationHasConfiguredStopWords(workstation.stopWords);
}
