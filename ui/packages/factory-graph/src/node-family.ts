/**
 * Stable semantic families used by every Factory graph renderer.
 *
 * The family is intentionally separate from a rendered node type. A renderer
 * may use `statePosition` or `workType` as its library-specific type while
 * the Factory graph still has one `work-state` or `work-type` family.
 */
export const FACTORY_GRAPH_NODE_FAMILIES = [
  "constraint",
  "doc",
  "resource",
  "worker",
  "work-state",
  "work-type",
  "workstation",
] as const;

export type FactoryGraphNodeFamily =
  (typeof FACTORY_GRAPH_NODE_FAMILIES)[number];

/** Semantic shape vocabulary; dimensions and CSS are resolved by renderers. */
export type FactoryGraphNodeShape =
  | "constraint"
  | "document"
  | "resource"
  | "worker"
  | "work-state"
  | "work-type"
  | "workstation";

export interface FactoryGraphNodeDimensions {
  readonly height: number;
  readonly width: number;
}

/**
 * The package-owned family contract.
 *
 * `defaultDimensions` are the stable graph defaults. A future fit/resize
 * projection can supply resolved dimensions without changing this family
 * identity or copying this table.
 */
export interface FactoryGraphNodeFamilyRole {
  readonly defaultDimensions: FactoryGraphNodeDimensions;
  readonly family: FactoryGraphNodeFamily;
  readonly shape: FactoryGraphNodeShape;
}

export type FactoryGraphNodeDimensionSource = "default" | "resolved";

/**
 * Dimension seam for later authored/fit/resize resolution.
 *
 * This story only selects an externally resolved size when one is supplied;
 * it does not fit labels, persist dimensions, or add resize controls.
 */
export interface FactoryGraphNodeDimensionResolution {
  readonly defaultDimensions: FactoryGraphNodeDimensions;
  readonly resolvedDimensions: FactoryGraphNodeDimensions;
  readonly source: FactoryGraphNodeDimensionSource;
}

const FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS: Record<
  FactoryGraphNodeFamily,
  FactoryGraphNodeFamilyRole
> = {
  constraint: {
    defaultDimensions: { height: 58, width: 156 },
    family: "constraint",
    shape: "constraint",
  },
  doc: {
    defaultDimensions: { height: 86, width: 168 },
    family: "doc",
    shape: "document",
  },
  resource: {
    defaultDimensions: { height: 86, width: 168 },
    family: "resource",
    shape: "resource",
  },
  worker: {
    defaultDimensions: { height: 58, width: 156 },
    family: "worker",
    shape: "worker",
  },
  "work-state": {
    defaultDimensions: { height: 86, width: 164 },
    family: "work-state",
    shape: "work-state",
  },
  "work-type": {
    defaultDimensions: { height: 58, width: 156 },
    family: "work-type",
    shape: "work-type",
  },
  workstation: {
    defaultDimensions: { height: 196, width: 156 },
    family: "workstation",
    shape: "workstation",
  },
};

/** Read-only family table for package consumers that need the full role. */
export const FACTORY_GRAPH_NODE_FAMILY_ROLES =
  FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS;

export function factoryGraphNodeFamilyRole(
  family: FactoryGraphNodeFamily,
): FactoryGraphNodeFamilyRole {
  const role = FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS[family];
  return {
    ...role,
    defaultDimensions: { ...role.defaultDimensions },
  };
}

export function factoryGraphNodeFamilyDimensions(
  family: FactoryGraphNodeFamily,
): FactoryGraphNodeDimensions {
  return { ...factoryGraphNodeFamilyRole(family).defaultDimensions };
}

/**
 * Select a resolved dimension set while retaining the family default.
 * Resolution remains an explicit seam for the later resize lane.
 */
export function resolveFactoryGraphNodeDimensions(
  family: FactoryGraphNodeFamily,
  resolvedDimensions?: FactoryGraphNodeDimensions | null,
): FactoryGraphNodeDimensionResolution {
  const defaultDimensions = factoryGraphNodeFamilyDimensions(family);
  const resolved = resolvedDimensions
    ? { ...resolvedDimensions }
    : { ...defaultDimensions };

  return {
    defaultDimensions,
    resolvedDimensions: resolved,
    source: resolvedDimensions ? "resolved" : "default",
  };
}

export type FactoryGraphNodeShellType =
  | "constraint"
  | "doc"
  | "resource"
  | "statePosition"
  | "worker"
  | "workType"
  | "workstation";

const FAMILY_BY_SHELL_TYPE: Record<
  FactoryGraphNodeShellType,
  FactoryGraphNodeFamily
> = {
  constraint: "constraint",
  doc: "doc",
  resource: "resource",
  statePosition: "work-state",
  worker: "worker",
  workType: "work-type",
  workstation: "workstation",
};

export function factoryGraphNodeFamilyForShellType(
  nodeType: FactoryGraphNodeShellType,
): FactoryGraphNodeFamily {
  return FAMILY_BY_SHELL_TYPE[nodeType];
}
