import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";

export function serializeFactoryValidationDefinition(
  factoryDefinition: CanonicalFactoryDefinition | null,
): string | null {
  if (factoryDefinition == null) {
    return null;
  }

  return JSON.stringify(factoryDefinition);
}
