import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";

export function workTypeHasDefaultHandling(
  factory: CanonicalFactoryDefinition | null | undefined,
  workTypeName: string,
): boolean {
  const workType = factory?.workTypes?.find(
    (entry) => entry.name === workTypeName,
  );
  return workType?.handlingBehavior?.includes("DEFAULT") ?? false;
}
