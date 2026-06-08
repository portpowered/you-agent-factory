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
 * factory definition save payloads outside the portable top-level layout
 * contract.
 */
export function findGraphLayoutPropertyPaths(
  value: unknown,
  path = "",
  insidePortableLayout = false,
): string[] {
  if (value === null || typeof value !== "object") {
    return [];
  }

  if (Array.isArray(value)) {
    return value.flatMap((entry, index) =>
      findGraphLayoutPropertyPaths(
        entry,
        `${path}[${index}]`,
        insidePortableLayout,
      ),
    );
  }

  const paths: string[] = [];
  for (const [key, entry] of Object.entries(value)) {
    const nextPath = path.length > 0 ? `${path}.${key}` : key;
    const nextInsidePortableLayout =
      insidePortableLayout || (path.length === 0 && key === "layout");
    if (GRAPH_LAYOUT_PROPERTY_NAMES.has(key) && !nextInsidePortableLayout) {
      paths.push(nextPath);
    }
    if ((key === "x" || key === "y") && !insidePortableLayout) {
      const sibling = value as Record<string, unknown>;
      const hasCoordinatePair =
        typeof sibling.x === "number" && typeof sibling.y === "number";
      if (hasCoordinatePair) {
        paths.push(nextPath);
      }
    }
    paths.push(
      ...findGraphLayoutPropertyPaths(
        entry,
        nextPath,
        nextInsidePortableLayout,
      ),
    );
  }

  return paths;
}

export function factoryDefinitionSavePayloadHasGraphLayoutFields(
  factoryDefinition: unknown,
): boolean {
  return findGraphLayoutPropertyPaths(factoryDefinition).length > 0;
}
