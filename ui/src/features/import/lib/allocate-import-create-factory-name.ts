function normalizeFactoryName(name: string): string {
  return name.trim();
}

function isFactoryNameTaken(name: string, existingNames: ReadonlySet<string>): boolean {
  const normalized = normalizeFactoryName(name);
  return normalized.length > 0 && existingNames.has(normalized);
}

/**
 * Picks the first unused factory name for import create-new-named flows.
 * When `embeddedName` collides with `existingNames`, returns `base-2`, `base-3`, …
 */
export function allocateImportCreateFactoryName(
  embeddedName: string,
  existingNames: readonly string[],
): string {
  const base = normalizeFactoryName(embeddedName);
  if (base.length === 0) {
    return base;
  }

  const takenNames = new Set(
    existingNames.map(normalizeFactoryName).filter((name) => name.length > 0),
  );
  if (!isFactoryNameTaken(base, takenNames)) {
    return base;
  }

  let suffix = 2;
  while (isFactoryNameTaken(`${base}-${suffix}`, takenNames)) {
    suffix += 1;
  }

  return `${base}-${suffix}`;
}
