import {
  type FactorySaveValidationErrorLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableDocSaveValidationErrors } from "./detail-card-types";

const BUNDLED_FILE_TARGET_PATH_RULES = new Set([
  "bundled-file-target-duplicate",
  "bundled-file-target-path",
  "bundled-file-target-root",
]);

const BUNDLED_FILE_INLINE_RULES = new Set([
  "bundled-file-content-inline",
  "bundled-file-content-encoding",
]);

export function mapDocSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableDocSaveValidationErrors | undefined {
  const fieldErrors: EditableDocSaveValidationErrors = {};

  for (const target of error.targets ?? []) {
    if (BUNDLED_FILE_TARGET_PATH_RULES.has(target.code)) {
      fieldErrors.fileName ??= target.message;
      continue;
    }

    if (BUNDLED_FILE_INLINE_RULES.has(target.code)) {
      fieldErrors.inlineContent ??= target.message;
    }
  }

  const mappedTargets = mapFactoryValidationTargetsToFieldErrors(
    error,
    () => null,
  );

  return Object.keys({ ...fieldErrors, ...mappedTargets }).length > 0
    ? { ...mappedTargets, ...fieldErrors }
    : undefined;
}
