import {
  type FactorySaveValidationErrorLike,
  type FactoryValidationTargetLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableWorkstationSaveValidationErrors } from "./detail-card-types";

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
    const cronFieldName = resolveCronSaveValidationFieldName(subjectID);
    if (cronFieldName != null) {
      return cronFieldName;
    }
  }

  return null;
}

function resolveCronSaveValidationFieldName(
  subjectID: string,
): keyof EditableWorkstationSaveValidationErrors | null {
  if (matchesCronValidationSubject(subjectID, "cron.schedule")) {
    return "cronSchedule";
  }
  if (matchesCronValidationSubject(subjectID, "cron.jitter")) {
    return "cronJitter";
  }
  if (
    matchesCronValidationSubject(subjectID, "cron.expiry_window") ||
    matchesCronValidationSubject(subjectID, "cron.expirywindow")
  ) {
    return "cronExpiryWindow";
  }
  if (
    matchesCronValidationSubject(subjectID, "cron.trigger_at_start") ||
    matchesCronValidationSubject(subjectID, "cron.triggeratstart")
  ) {
    return "cronTriggerAtStart";
  }

  return null;
}

function matchesCronValidationSubject(
  subjectID: string,
  fieldPath: string,
): boolean {
  return subjectID === fieldPath || subjectID.endsWith(`.${fieldPath}`);
}

export function mapWorkstationSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableWorkstationSaveValidationErrors | undefined {
  return mapFactoryValidationTargetsToFieldErrors(
    error,
    resolveWorkstationSaveValidationFieldName,
  );
}
