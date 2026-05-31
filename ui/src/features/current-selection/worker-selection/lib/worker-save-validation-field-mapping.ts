import {
  type FactorySaveValidationErrorLike,
  type FactoryValidationTargetLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableWorkerSaveValidationErrors } from "./detail-card-types";

export function resolveWorkerSaveValidationFieldName(
  target: FactoryValidationTargetLike,
): keyof EditableWorkerSaveValidationErrors | null {
  if (target.subject.type !== "WORKER") {
    return null;
  }

  const subjectID = target.subject.id.trim().toLowerCase();

  if (subjectID === "type") {
    return "type";
  }
  if (subjectID === "modelprovider") {
    return "modelProvider";
  }
  if (subjectID === "model") {
    return "model";
  }
  if (subjectID === "modellocality") {
    return "modelLocality";
  }
  if (subjectID === "executorprovider") {
    return "executorProvider";
  }
  if (subjectID === "command") {
    return "command";
  }
  if (subjectID === "args") {
    return "args";
  }
  if (subjectID === "body") {
    return "body";
  }
  if (subjectID === "provider") {
    return "provider";
  }

  return null;
}

export function mapWorkerSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableWorkerSaveValidationErrors | undefined {
  return mapFactoryValidationTargetsToFieldErrors(
    error,
    resolveWorkerSaveValidationFieldName,
  );
}
