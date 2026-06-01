import {
  type FactorySaveValidationErrorLike,
  type FactoryValidationTargetLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableWorkstationSaveValidationErrors } from "./detail-card-types";

const WORKSTATION_GUARD_TARGET_FIELD_BY_CODE: Record<string, string> = {
  "guard-matches-fields-input-key": "matchConfig.inputKey",
  "guard-visit-count-max-visits": "maxVisits",
  "guard-visit-count-workstation": "workstation",
  "per-input-guard-match-input": "matchInput",
  "per-input-guard-parent-input": "parentInput",
  "per-input-guard-same-trace-match-input": "matchInput",
  "per-input-guard-same-trace-self-ref": "matchInput",
  "per-input-guard-self-ref": "parentInput",
  "per-input-guard-spawned-by": "spawnedBy",
  "per-input-guard-type": "type",
};

const FACTORY_WORKSTATION_GUARD_PATH =
  /factory\.workstations\[\d+\]\.guards\[(\d+)\]\.([a-zA-Z.]+)/;
const FACTORY_INPUT_GUARD_PATH =
  /factory\.workstations\[\d+\]\.inputs\[(\d+)\]\.guards\[\d+\]\.([a-zA-Z]+)/;

export function resolveWorkstationSaveValidationFieldName(
  target: FactoryValidationTargetLike,
): keyof EditableWorkstationSaveValidationErrors | null {
  const subjectID = target.subject.id.trim().toLowerCase();
  const subjectType = target.subject.type;
  const subjectLocation = target.subject.location;

  if (
    target.code === "factory.worker.danglingReference" &&
    subjectType === "WORKSTATION"
  ) {
    return "workerName";
  }
  if (
    subjectType === "WORKSTATION" &&
    (subjectLocation === "REFERENCE" || subjectLocation === "DEFINITION")
  ) {
    if (subjectID.endsWith("worker") || subjectID === "worker") {
      return "workerName";
    }
    if (subjectID === "behavior") {
      return "behavior";
    }
    if (subjectID === "body" || subjectID === "prompt") {
      return "prompt";
    }
    if (subjectID === "runner" || subjectID === "runnername") {
      return "runnerName";
    }
  }

  const guardFieldName = resolveWorkstationGuardSaveValidationFieldName(target);
  return guardFieldName as keyof EditableWorkstationSaveValidationErrors | null;
}

export function mapWorkstationSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableWorkstationSaveValidationErrors | undefined {
  const targetFieldErrors =
    mapFactoryValidationTargetsToFieldErrors(
      error,
      resolveWorkstationSaveValidationFieldName,
    ) ?? {};
  const messageFieldErrors = mapWorkstationSaveErrorMessageToFieldErrors(
    error.message,
  );

  const fieldErrors = {
    ...targetFieldErrors,
    ...messageFieldErrors,
  };

  return Object.keys(fieldErrors).length > 0
    ? (fieldErrors as EditableWorkstationSaveValidationErrors)
    : undefined;
}

function resolveWorkstationGuardSaveValidationFieldName(
  target: FactoryValidationTargetLike,
): string | null {
  const guardFieldName = WORKSTATION_GUARD_TARGET_FIELD_BY_CODE[target.code];
  if (!guardFieldName) {
    return null;
  }

  const inputIndex = resolveInputGuardIndexFromTarget(target);
  if (inputIndex != null) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return `inputs[${inputIndex}].guard.${guardFieldName}`;
  }

  const guardIndex = resolveWorkstationGuardIndexFromTarget(target);
  if (guardIndex != null) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return `guards[${guardIndex}].${guardFieldName}`;
  }

  return null;
}

function mapWorkstationSaveErrorMessageToFieldErrors(
  message: string,
): Record<string, string> {
  const workstationGuardMatch = message.match(FACTORY_WORKSTATION_GUARD_PATH);
  if (workstationGuardMatch) {
    const [, guardIndex, fieldName] = workstationGuardMatch;
    return {
      [`guards[${guardIndex}].${fieldName}`]: message,
    };
  }

  const inputGuardMatch = message.match(FACTORY_INPUT_GUARD_PATH);
  if (inputGuardMatch) {
    const [, slotIndex, fieldName] = inputGuardMatch;
    return {
      [`inputs[${slotIndex}].guard.${fieldName}`]: message,
    };
  }

  return {};
}

function resolveInputGuardIndexFromTarget(
  target: FactoryValidationTargetLike,
): number | null {
  const subjectMatch = target.subject.id.match(/inputs\[(\d+)\]/i);
  if (subjectMatch) {
    return Number.parseInt(subjectMatch[1], 10);
  }

  const pathMatch = target.subject.id.match(/\.inputs\[(\d+)\]\.guard/i);
  if (pathMatch) {
    return Number.parseInt(pathMatch[1], 10);
  }

  if (
    target.subject.type === "WORKSTATION" &&
    target.subject.location === "INPUTS"
  ) {
    const locationMatch = target.subject.id.match(/\[(\d+)\]/);
    if (locationMatch) {
      return Number.parseInt(locationMatch[1], 10);
    }
  }

  return null;
}

function resolveWorkstationGuardIndexFromTarget(
  target: FactoryValidationTargetLike,
): number | null {
  const subjectMatch = target.subject.id.match(/guards\[(\d+)\]/i);
  if (subjectMatch) {
    return Number.parseInt(subjectMatch[1], 10);
  }

  const pathMatch = target.subject.id.match(/\.guards\[(\d+)\]/i);
  if (pathMatch) {
    return Number.parseInt(pathMatch[1], 10);
  }

  return null;
}
