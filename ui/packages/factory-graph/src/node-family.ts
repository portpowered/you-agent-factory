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

export interface FactoryGraphNodeDimensionBounds {
  readonly maximum: FactoryGraphNodeDimensions;
  readonly minimum: FactoryGraphNodeDimensions;
}

/** The dimensions that a future editor resize affordance may change. */
export interface FactoryGraphNodeResizeAxes {
  readonly height: boolean;
  readonly width: boolean;
}

/** Text surfaces used by the pure fitted-size resolver. */
export type FactoryGraphNodeSizingContent =
  | string
  | readonly (string | null | undefined)[];

/**
 * The package-owned family contract.
 *
 * `defaultDimensions`, bounds, and axis policy are the stable graph contract.
 * Content fit and authored-size normalization are resolved without changing
 * the family identity or copying this table into a host.
 */
export interface FactoryGraphNodeFamilyRole {
  readonly allowedAxes: FactoryGraphNodeResizeAxes;
  readonly defaultDimensions: FactoryGraphNodeDimensions;
  readonly family: FactoryGraphNodeFamily;
  readonly maximumDimensions: FactoryGraphNodeDimensions;
  readonly minimumDimensions: FactoryGraphNodeDimensions;
  readonly shape: FactoryGraphNodeShape;
}

export type FactoryGraphNodeDimensionSource = "default" | "fitted" | "resolved";

export interface FactoryGraphNodeDimensionResolutionOptions {
  readonly authoredDimensions?: FactoryGraphNodeDimensions | null;
  readonly content?: FactoryGraphNodeSizingContent;
}

/**
 * The complete pure sizing result. Hosts may use the resolved dimensions for
 * projection while retaining authored layout and interaction ownership.
 */
export interface FactoryGraphNodeDimensionResolution {
  readonly allowedAxes: FactoryGraphNodeResizeAxes;
  readonly bounds: FactoryGraphNodeDimensionBounds;
  readonly defaultDimensions: FactoryGraphNodeDimensions;
  readonly fittedDimensions: FactoryGraphNodeDimensions;
  readonly maximumDimensions: FactoryGraphNodeDimensions;
  readonly minimumDimensions: FactoryGraphNodeDimensions;
  readonly resolvedDimensions: FactoryGraphNodeDimensions;
  readonly source: FactoryGraphNodeDimensionSource;
}

const FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS: Record<
  FactoryGraphNodeFamily,
  FactoryGraphNodeFamilyRole
> = {
  constraint: {
    allowedAxes: { height: false, width: true },
    defaultDimensions: { height: 58, width: 156 },
    family: "constraint",
    maximumDimensions: { height: 176, width: 360 },
    minimumDimensions: { height: 58, width: 128 },
    shape: "constraint",
  },
  doc: {
    allowedAxes: { height: true, width: true },
    defaultDimensions: { height: 86, width: 168 },
    family: "doc",
    maximumDimensions: { height: 320, width: 480 },
    minimumDimensions: { height: 86, width: 168 },
    shape: "document",
  },
  resource: {
    allowedAxes: { height: false, width: true },
    defaultDimensions: { height: 86, width: 168 },
    family: "resource",
    maximumDimensions: { height: 240, width: 420 },
    minimumDimensions: { height: 86, width: 168 },
    shape: "resource",
  },
  worker: {
    allowedAxes: { height: false, width: true },
    defaultDimensions: { height: 58, width: 156 },
    family: "worker",
    maximumDimensions: { height: 144, width: 360 },
    minimumDimensions: { height: 58, width: 156 },
    shape: "worker",
  },
  "work-state": {
    allowedAxes: { height: false, width: true },
    defaultDimensions: { height: 86, width: 164 },
    family: "work-state",
    maximumDimensions: { height: 240, width: 440 },
    minimumDimensions: { height: 86, width: 164 },
    shape: "work-state",
  },
  "work-type": {
    allowedAxes: { height: false, width: true },
    defaultDimensions: { height: 58, width: 156 },
    family: "work-type",
    maximumDimensions: { height: 144, width: 360 },
    minimumDimensions: { height: 58, width: 156 },
    shape: "work-type",
  },
  workstation: {
    allowedAxes: { height: true, width: true },
    defaultDimensions: { height: 156, width: 156 },
    family: "workstation",
    maximumDimensions: { height: 720, width: 520 },
    minimumDimensions: { height: 156, width: 156 },
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
    allowedAxes: { ...role.allowedAxes },
    maximumDimensions: { ...role.maximumDimensions },
    defaultDimensions: { ...role.defaultDimensions },
    minimumDimensions: { ...role.minimumDimensions },
  };
}

export function factoryGraphNodeFamilyDimensions(
  family: FactoryGraphNodeFamily,
): FactoryGraphNodeDimensions {
  return { ...factoryGraphNodeFamilyRole(family).defaultDimensions };
}

/** Resolve a deterministic, family-bounded size from content or authored data. */
export function resolveFactoryGraphNodeDimensions(
  family: FactoryGraphNodeFamily,
  request?:
    | FactoryGraphNodeDimensionResolutionOptions
    | FactoryGraphNodeDimensions
    | null,
): FactoryGraphNodeDimensionResolution {
  const role = factoryGraphNodeFamilyRole(family);
  const options = normalizeDimensionRequest(request);
  const fittedDimensions = fitFactoryGraphNodeDimensionsForRole(
    role,
    options.content,
  );
  const resolvedDimensions = resolveAuthoredDimensions(
    role,
    fittedDimensions,
    options.authoredDimensions,
  );
  const source = options.authoredDimensions
    ? isFinitePositiveDimensions(options.authoredDimensions)
      ? "resolved"
      : "fitted"
    : options.content
      ? dimensionsEqual(fittedDimensions, role.defaultDimensions)
        ? "default"
        : "fitted"
      : "default";

  return {
    allowedAxes: { ...role.allowedAxes },
    bounds: {
      maximum: { ...role.maximumDimensions },
      minimum: { ...role.minimumDimensions },
    },
    defaultDimensions: { ...role.defaultDimensions },
    fittedDimensions,
    maximumDimensions: { ...role.maximumDimensions },
    minimumDimensions: { ...role.minimumDimensions },
    resolvedDimensions,
    source,
  };
}

export function fitFactoryGraphNodeDimensions(
  family: FactoryGraphNodeFamily,
  content?: FactoryGraphNodeSizingContent,
): FactoryGraphNodeDimensions {
  return resolveFactoryGraphNodeDimensions(family, { content })
    .fittedDimensions;
}

/** Normalize an interactive resize request to the family's safe geometry. */
export function resolveFactoryGraphNodeResizeDimensions(
  family: FactoryGraphNodeFamily,
  requestedDimensions: FactoryGraphNodeDimensions,
): FactoryGraphNodeDimensions {
  const resolution = resolveFactoryGraphNodeDimensions(family, {
    authoredDimensions: requestedDimensions,
  });
  const role = factoryGraphNodeFamilyRole(family);

  return {
    height: role.allowedAxes.height
      ? resolution.resolvedDimensions.height
      : resolution.fittedDimensions.height,
    width: role.allowedAxes.width
      ? resolution.resolvedDimensions.width
      : resolution.fittedDimensions.width,
  };
}

function normalizeDimensionRequest(
  request:
    | FactoryGraphNodeDimensionResolutionOptions
    | FactoryGraphNodeDimensions
    | null
    | undefined,
): FactoryGraphNodeDimensionResolutionOptions {
  if (!request) return {};
  if (hasDimensionFields(request)) return { authoredDimensions: request };
  return request;
}

function hasDimensionFields(
  request:
    | FactoryGraphNodeDimensionResolutionOptions
    | FactoryGraphNodeDimensions,
): request is FactoryGraphNodeDimensions {
  return "width" in request || "height" in request;
}

function fitFactoryGraphNodeDimensionsForRole(
  role: FactoryGraphNodeFamilyRole,
  content?: FactoryGraphNodeSizingContent,
): FactoryGraphNodeDimensions {
  const labels = normalizeContent(content);
  if (labels.length === 0) return { ...role.defaultDimensions };

  const contentPadding = contentPaddingForFamily(role.family);
  const width = clamp(
    Math.max(
      role.defaultDimensions.width,
      Math.ceil(Math.max(...labels.map(estimatedTextWidth)) + contentPadding),
    ),
    role.minimumDimensions.width,
    role.maximumDimensions.width,
  );
  const availableTextWidth = Math.max(1, width - contentPadding);
  const lineCount = labels.reduce(
    (total, label) => total + wrappedLineCount(label, availableTextWidth),
    0,
  );
  const height = clamp(
    Math.max(
      role.defaultDimensions.height,
      role.defaultDimensions.height +
        Math.max(0, lineCount - baselineLineCount(role.family)) * 16,
    ),
    role.minimumDimensions.height,
    role.maximumDimensions.height,
  );

  return { height, width };
}

function resolveAuthoredDimensions(
  role: FactoryGraphNodeFamilyRole,
  fittedDimensions: FactoryGraphNodeDimensions,
  authoredDimensions?: FactoryGraphNodeDimensions | null,
): FactoryGraphNodeDimensions {
  if (!authoredDimensions || !isFinitePositiveDimensions(authoredDimensions)) {
    return { ...fittedDimensions };
  }

  return {
    height: clamp(
      authoredDimensions.height,
      role.minimumDimensions.height,
      role.maximumDimensions.height,
    ),
    width: clamp(
      authoredDimensions.width,
      role.minimumDimensions.width,
      role.maximumDimensions.width,
    ),
  };
}

function normalizeContent(content?: FactoryGraphNodeSizingContent): string[] {
  const labels = typeof content === "string" ? [content] : (content ?? []);
  return labels
    .filter((label): label is string => typeof label === "string")
    .map((label) => label.trim())
    .filter((label) => label.length > 0);
}

function isFinitePositiveDimensions(
  dimensions: FactoryGraphNodeDimensions,
): boolean {
  return (
    Number.isFinite(dimensions.width) &&
    Number.isFinite(dimensions.height) &&
    dimensions.width > 0 &&
    dimensions.height > 0
  );
}

function dimensionsEqual(
  left: FactoryGraphNodeDimensions,
  right: FactoryGraphNodeDimensions,
): boolean {
  return left.width === right.width && left.height === right.height;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function contentPaddingForFamily(family: FactoryGraphNodeFamily): number {
  return family === "workstation" ? 56 : family === "doc" ? 48 : 44;
}

function baselineLineCount(family: FactoryGraphNodeFamily): number {
  return family === "workstation" ? 4 : family === "resource" ? 3 : 2;
}

function wrappedLineCount(label: string, availableWidth: number): number {
  return label
    .split(/\r?\n/u)
    .reduce(
      (total, line) =>
        total +
        Math.max(1, Math.ceil(estimatedTextWidth(line) / availableWidth)),
      0,
    );
}

function estimatedTextWidth(label: string): number {
  return [...label].reduce((total, character) => {
    if (/\s/u.test(character)) return total + 4;
    if (isWideCharacter(character)) return total + 14;
    if (/[ilIjtfr.,:'`|!]/u.test(character)) return total + 4.5;
    if (/[A-Z0-9]/u.test(character)) return total + 8;
    return total + 7;
  }, 0);
}

function isWideCharacter(character: string): boolean {
  const codePoint = character.codePointAt(0) ?? 0;
  return (
    (codePoint >= 0x1100 && codePoint <= 0x11ff) ||
    (codePoint >= 0x2e80 && codePoint <= 0x9fff) ||
    (codePoint >= 0xac00 && codePoint <= 0xd7ff) ||
    (codePoint >= 0xf900 && codePoint <= 0xfaff) ||
    (codePoint >= 0x1f300 && codePoint <= 0x1faff)
  );
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
