import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type {
  EditableWorkstationDraft,
  EditableWorkstationInputDraft,
} from "./workstation-editable-values";

type WorkstationGuard = EditableWorkstationDraft["guards"][number];
type InputGuardBase = EditableWorkstationInputDraft["guards"][number];

export type WorkstationLevelGuard =
  | (WorkstationGuard & { type: "VISIT_COUNT" })
  | (WorkstationGuard & { type: "MATCHES_FIELDS" });

export const WORKSTATION_LEVEL_GUARD_TYPES = [
  "VISIT_COUNT",
  "MATCHES_FIELDS",
] as const satisfies ReadonlyArray<WorkstationGuard["type"]>;

export type WorkstationLevelGuardType =
  (typeof WORKSTATION_LEVEL_GUARD_TYPES)[number];

export const INPUT_GUARD_TYPES = [
  "ALL_CHILDREN_COMPLETE",
  "ANY_CHILD_FAILED",
  "SAME_NAME",
  "SAME_TRACE_ID",
] as const satisfies ReadonlyArray<InputGuardBase["type"]>;

export type InputGuardType = (typeof INPUT_GUARD_TYPES)[number];

export type InputGuard =
  | (InputGuardBase & { type: "ALL_CHILDREN_COMPLETE" })
  | (InputGuardBase & { type: "ANY_CHILD_FAILED" })
  | (InputGuardBase & { type: "SAME_NAME" })
  | (InputGuardBase & { type: "SAME_TRACE_ID" });

export type { InputGuardBase };

export function resolveFactoryWorkstationNameOptions(
  factory: CanonicalFactoryDefinition,
): string[] {
  return (factory.workstations ?? [])
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}

export function formatInputGuardSummary(guard: InputGuardBase): string {
  if (guard.type === "SAME_NAME" || guard.type === "SAME_TRACE_ID") {
    return guard.matchInput?.trim() || "?";
  }

  if (
    guard.type === "ALL_CHILDREN_COMPLETE" ||
    guard.type === "ANY_CHILD_FAILED"
  ) {
    const parentInput = guard.parentInput?.trim() || "?";
    const spawnedBy = guard.spawnedBy?.trim();
    return spawnedBy ? `${parentInput} · ${spawnedBy}` : parentInput;
  }

  return guard.type;
}

export function resolvePeerInputWorkTypes(
  inputs: EditableWorkstationDraft["inputs"],
  slotIndex: number,
): string[] {
  return inputs.flatMap((input, index) =>
    index === slotIndex || input.workType.trim().length === 0
      ? []
      : [input.workType.trim()],
  );
}

export function formatWorkstationGuardSummary(guard: WorkstationGuard): string {
  if (guard.type === "VISIT_COUNT") {
    const workstation = guard.workstation?.trim() || "?";
    const maxVisits =
      guard.maxVisits != null && Number.isFinite(guard.maxVisits)
        ? String(guard.maxVisits)
        : "?";
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return `${workstation} · max ${maxVisits}`;
  }

  if (guard.type === "MATCHES_FIELDS") {
    return guard.matchConfig?.inputKey?.trim() || "?";
  }

  return guard.type;
}

export function normalizeEditableInputGuards(
  guards: InputGuardBase[],
): InputGuardBase[] {
  if (guards.length === 0) {
    return [];
  }

  return [guards[0]];
}

export function createDefaultInputGuard(
  type: InputGuardType,
  peerWorkTypes: string[],
): InputGuardBase {
  const peerWorkType = peerWorkTypes[0] ?? "";

  if (type === "SAME_NAME" || type === "SAME_TRACE_ID") {
    return {
      matchInput: peerWorkType,
      type,
    };
  }

  return {
    parentInput: peerWorkType,
    type,
  };
}

export function setEditableInputSlotGuard(
  input: EditableWorkstationInputDraft,
  guard: InputGuardBase | null,
): EditableWorkstationInputDraft {
  return {
    ...input,
    guards: guard ? [guard] : [],
  };
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

function editableWorkstationCronDraftsEqualOrNull(
  left: EditableWorkstationDraft["cron"],
  right: EditableWorkstationDraft["cron"],
): boolean {
  if (left === null && right === null) {
    return true;
  }
  if (left === null || right === null) {
    return false;
  }
  return (
    left.schedule === right.schedule &&
    left.triggerAtStart === right.triggerAtStart &&
    left.jitter === right.jitter &&
    left.expiryWindow === right.expiryWindow
  );
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
    editableWorkstationCronDraftsEqualOrNull(left.cron, right.cron) &&
    guardsDraftEqual(left.guards, right.guards) &&
    editableWorkstationInputsDraftEqual(left.inputs, right.inputs)
  );
}
