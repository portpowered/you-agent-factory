import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

export function serializeFactoryValidationDefinition(
  factoryDefinition: CanonicalFactoryDefinition | null,
): string | null {
  if (factoryDefinition == null) {
    return null;
  }

  return JSON.stringify(factoryDefinition);
}
