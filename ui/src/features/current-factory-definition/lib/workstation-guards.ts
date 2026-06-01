import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { EditableWorkstationDraft } from "./workstation-editable-values";

type WorkstationGuard = EditableWorkstationDraft["guards"][number];

export type WorkstationLevelGuard =
  | (WorkstationGuard & { type: "VISIT_COUNT" })
  | (WorkstationGuard & { type: "MATCHES_FIELDS" });

export const WORKSTATION_LEVEL_GUARD_TYPES = [
  "VISIT_COUNT",
  "MATCHES_FIELDS",
] as const satisfies ReadonlyArray<WorkstationGuard["type"]>;

export type WorkstationLevelGuardType =
  (typeof WORKSTATION_LEVEL_GUARD_TYPES)[number];

export function resolveFactoryWorkstationNameOptions(
  factory: CanonicalFactoryDefinition,
): string[] {
  return (factory.workstations ?? [])
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}

export function formatWorkstationGuardSummary(guard: WorkstationGuard): string {
  if (guard.type === "VISIT_COUNT") {
    const workstation = guard.workstation?.trim() || "?";
    const maxVisits =
      guard.maxVisits != null && Number.isFinite(guard.maxVisits)
        ? String(guard.maxVisits)
        : "?";
    return `${workstation} · max ${maxVisits}`;
  }

  if (guard.type === "MATCHES_FIELDS") {
    return guard.matchConfig?.inputKey?.trim() || "?";
  }

  return guard.type;
}

export function createDefaultWorkstationGuard(
  type: WorkstationLevelGuardType,
  workstationNames: string[],
): WorkstationLevelGuard {
  if (type === "VISIT_COUNT") {
    return {
      maxVisits: 1,
      type: "VISIT_COUNT",
      workstation: workstationNames[0] ?? "",
    };
  }

  return {
    matchConfig: { inputKey: ".Name" },
    type: "MATCHES_FIELDS",
  };
}

export function guardsDraftEqual(
  left: WorkstationGuard[],
  right: WorkstationGuard[],
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function editableWorkstationInputsDraftEqual(
  left: EditableWorkstationDraft["inputs"],
  right: EditableWorkstationDraft["inputs"],
): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function editableWorkstationDraftsEqual(
  left: EditableWorkstationDraft,
  right: EditableWorkstationDraft,
): boolean {
  return (
    left.behavior === right.behavior &&
    left.prompt === right.prompt &&
    left.runnerName === right.runnerName &&
    left.workerName === right.workerName &&
    guardsDraftEqual(left.guards, right.guards) &&
    editableWorkstationInputsDraftEqual(left.inputs, right.inputs)
  );
}
