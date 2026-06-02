import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";

export type FactoryValidationTargetLike = NonNullable<
  CurrentFactoryDefinitionError["targets"]
>[number];

export type FactorySaveValidationErrorLike = Pick<
  CurrentFactoryDefinitionError,
  "message" | "targets"
>;

export function mapFactoryValidationTargetsToFieldErrors<
  TFieldErrors extends Record<string, string>,
>(
  error: FactorySaveValidationErrorLike,
  resolveTargetFieldName: (
    target: FactoryValidationTargetLike,
  ) => (keyof TFieldErrors & string) | null,
): TFieldErrors | undefined {
  const fieldErrors: Record<string, string> = {};

  for (const target of error.targets ?? []) {
    const fieldName = resolveTargetFieldName(target);
    if (fieldName === null) {
      continue;
    }
    fieldErrors[fieldName] ??= error.message;
  }

  return Object.keys(fieldErrors).length > 0
    ? (fieldErrors as TFieldErrors)
    : undefined;
}
