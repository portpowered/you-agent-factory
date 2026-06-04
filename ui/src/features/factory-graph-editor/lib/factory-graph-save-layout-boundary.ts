const GRAPH_LAYOUT_PROPERTY_NAMES = new Set([
  "coordinates",
  "graphLayout",
  "graphPosition",
  "graphPositions",
  "layout",
  "nodeLayout",
  "nodePosition",
  "nodePositions",
  "position",
  "positions",
]);

/**
 * Returns dotted paths to graph-layout metadata that must not appear in
 * factory definition save payloads. Positions belong in the UI position store.
 */
export function findGraphLayoutPropertyPaths(
  value: unknown,
  path = "",
): string[] {
  if (value === null || typeof value !== "object") {
    return [];
  }

  if (Array.isArray(value)) {
    return value.flatMap((entry, index) =>
      findGraphLayoutPropertyPaths(entry, `${path}[${index}]`),
    );
  }

  const paths: string[] = [];
  for (const [key, entry] of Object.entries(value)) {
    const nextPath = path.length > 0 ? `${path}.${key}` : key;
    if (GRAPH_LAYOUT_PROPERTY_NAMES.has(key)) {
      paths.push(nextPath);
    }
    if (key === "x" || key === "y") {
      const sibling = value as Record<string, unknown>;
      const hasCoordinatePair =
        typeof sibling.x === "number" && typeof sibling.y === "number";
      if (hasCoordinatePair) {
        paths.push(nextPath);
      }
    }
    paths.push(...findGraphLayoutPropertyPaths(entry, nextPath));
  }

  return paths;
}

export function factoryDefinitionSavePayloadHasGraphLayoutFields(
  factoryDefinition: unknown,
): boolean {
  return findGraphLayoutPropertyPaths(factoryDefinition).length > 0;
}
