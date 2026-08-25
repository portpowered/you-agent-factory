import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  isPollerRunWorkstationType,
  isPollerWorkerType,
  isScriptWorkerType,
} from "./worker-workstation-taxonomy";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];

export type EditableWorkstationBehavior = NonNullable<
  CanonicalWorkstation["behavior"]
>;

export const DEFAULT_WORKSTATION_BEHAVIOR: EditableWorkstationBehavior =
  "STANDARD";

const BASE_EDITABLE_WORKSTATION_BEHAVIORS: EditableWorkstationBehavior[] = [
  "STANDARD",
  "REPEATER",
  "POLLER",
];

export function resolveEditableWorkstationBehavior(
  workstation: Pick<CanonicalWorkstation, "behavior"> &
    Partial<Pick<CanonicalWorkstation, "type">>,
): EditableWorkstationBehavior {
  if (isPollerRunWorkstationType(workstation.type)) {
    return "POLLER";
  }

  return workstation.behavior ?? DEFAULT_WORKSTATION_BEHAVIOR;
}

export function resolveEditableWorkstationBehaviorOptions(
  currentBehavior: EditableWorkstationBehavior,
  workstationType?: CanonicalWorkstation["type"] | string | null,
): EditableWorkstationBehavior[] {
  if (isPollerRunWorkstationType(workstationType)) {
    return ["POLLER"];
  }

  return currentBehavior === "CRON"
    ? [...BASE_EDITABLE_WORKSTATION_BEHAVIORS, "CRON"]
    : BASE_EDITABLE_WORKSTATION_BEHAVIORS;
}

/** Graph add-workstation exposes CRON at creation time, not only when editing cron workstations. */
export function resolveFactoryGraphAddWorkstationBehaviorOptions(): EditableWorkstationBehavior[] {
  return [...BASE_EDITABLE_WORKSTATION_BEHAVIORS, "CRON"];
}

export function workerSupportsPollerBehavior(
  worker: Pick<CanonicalWorker, "type"> | null | undefined,
): boolean {
  return isPollerWorkerType(worker?.type) || isScriptWorkerType(worker?.type);
}

export function workstationBehaviorRequiresPrompt(
  behavior: EditableWorkstationBehavior,
): boolean {
  return behavior !== "POLLER";
}

export function workstationUsesRunnerEditing(
  workstationType: CanonicalWorkstation["type"] | string | null | undefined,
  behavior: EditableWorkstationBehavior,
): boolean {
  return behavior !== "POLLER" && !isPollerRunWorkstationType(workstationType);
}
