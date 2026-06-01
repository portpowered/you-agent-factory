import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
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

  return workstationHasEffectiveStopWords(workstation);
}

/** True when Continue/Reject connect anchors are omitted for missing stop markers on a standard model processor. */
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

  return !workstationHasEffectiveStopWords(workstation);
}
