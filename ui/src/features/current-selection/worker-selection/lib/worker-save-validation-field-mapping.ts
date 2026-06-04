import {
  type FactorySaveValidationErrorLike,
  type FactoryValidationTargetLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableWorkerSaveValidationErrors } from "./detail-card-types";

const FACTORY_WORKER_RUNTIME_FIELD_PATH =
  /factory\.workers\[\d+\]\.(timeout|stopToken|skipPermissions)/;

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
  if (subjectID === "timeout") {
    return "timeout";
  }
  if (subjectID === "stoptoken") {
    return "stopToken";
  }
  if (subjectID === "skippermissions") {
    return "skipPermissions";
  }

  return null;
}

export function mapWorkerSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableWorkerSaveValidationErrors | undefined {
  const targetFieldErrors =
    mapFactoryValidationTargetsToFieldErrors(
      error,
      resolveWorkerSaveValidationFieldName,
    ) ?? {};
  const messageFieldErrors = mapWorkerSaveErrorMessageToFieldErrors(
    error.message,
  );

  const fieldErrors = {
    ...targetFieldErrors,
    ...messageFieldErrors,
  };

  return Object.keys(fieldErrors).length > 0 ? fieldErrors : undefined;
}

function mapWorkerSaveErrorMessageToFieldErrors(
  message: string,
): EditableWorkerSaveValidationErrors {
  const runtimeFieldMatch = message.match(FACTORY_WORKER_RUNTIME_FIELD_PATH);
  if (!runtimeFieldMatch) {
    return {};
  }

  const [, fieldName] = runtimeFieldMatch;
  switch (fieldName) {
    case "timeout":
      return { timeout: message };
    case "stopToken":
      return { stopToken: message };
    case "skipPermissions":
      return { skipPermissions: message };
    default:
      return {};
  }
}
