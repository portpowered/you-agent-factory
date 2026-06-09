import type { CanonicalFactoryDefinition } from "./api";

/**
 * Some observer-mode factory sources omit portable layout metadata even when a
 * compatible persisted factory already has it. Keep the existing layout until
 * an explicit incoming layout is available.
 */
export function preserveExistingLayoutWhenAbsent(
  incoming: CanonicalFactoryDefinition,
  existing: CanonicalFactoryDefinition | null | undefined,
): CanonicalFactoryDefinition {
  if (existing?.layout === undefined || incoming.layout !== undefined) {
    return incoming;
  }

  return {
    ...incoming,
    layout: existing.layout,
  };
}
