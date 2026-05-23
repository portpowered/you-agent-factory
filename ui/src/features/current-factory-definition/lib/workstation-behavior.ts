import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<CanonicalFactoryDefinition["workers"]>[number];

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
  workstation: Pick<CanonicalWorkstation, "behavior">,
): EditableWorkstationBehavior {
  return workstation.behavior ?? DEFAULT_WORKSTATION_BEHAVIOR;
}

export function resolveEditableWorkstationBehaviorOptions(
  currentBehavior: EditableWorkstationBehavior,
): EditableWorkstationBehavior[] {
  return currentBehavior === "CRON"
    ? [...BASE_EDITABLE_WORKSTATION_BEHAVIORS, "CRON"]
    : BASE_EDITABLE_WORKSTATION_BEHAVIORS;
}

export function workerSupportsPollerBehavior(
  worker: Pick<CanonicalWorker, "type"> | null | undefined,
): boolean {
  return (
    worker?.type === "HOSTED_WORKER" || worker?.type === "SCRIPT_WORKER"
  );
}

export function workstationBehaviorRequiresPrompt(
  behavior: EditableWorkstationBehavior,
): boolean {
  return behavior !== "POLLER";
}
