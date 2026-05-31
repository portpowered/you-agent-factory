import type { FactorySessionTarget } from "../factory-sessions";

export function extractNamedFactoryNamesFromSessionTargets(
  targets: FactorySessionTarget[] | undefined,
): string[] {
  if (!targets?.length) {
    return [];
  }

  const names = targets
    .map((target) => {
      if (target.ref.kind !== "named" || !target.ref.name?.trim()) {
        return null;
      }
      return target.ref.name.trim();
    })
    .filter((name): name is string => name !== null);

  return [...new Set(names)].sort((left, right) => left.localeCompare(right));
}

export function allocateFirstFreeSuffixedFactoryName(
  preferredName: string,
  existingNames: readonly string[],
): string {
  const trimmedPreferredName = preferredName.trim();
  if (!trimmedPreferredName) {
    return preferredName;
  }

  const existing = new Set(existingNames);
  if (!existing.has(trimmedPreferredName)) {
    return trimmedPreferredName;
  }

  let suffix = 2;
  while (existing.has(`${trimmedPreferredName}-${suffix}`)) {
    suffix += 1;
  }

  return `${trimmedPreferredName}-${suffix}`;
}

export function resolveImportCreateFactoryName(
  preferredName: string,
  existingNames: readonly string[],
): { factoryName: string; replacesExisting: boolean } {
  const factoryName = allocateFirstFreeSuffixedFactoryName(
    preferredName,
    existingNames,
  );

  return {
    factoryName,
    replacesExisting: existingNames.includes(factoryName),
  };
}
