import { useStore } from "@xyflow/react";

export const FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS = [
  "neutral",
  "primary",
  "info",
  "success",
  "warning",
  "danger",
  "outline",
] as const;

export type FactoryGraphGroupRegionColorToken =
  (typeof FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS)[number];

export type FactoryGraphGroupRegionResolvedColor = Exclude<
  FactoryGraphGroupRegionColorToken,
  "outline"
>;

export interface FactoryGraphGroupRegionBounds {
  height: number;
  width: number;
  x: number;
  y: number;
}

/**
 * Read-only presentation input for a saved layout group.
 *
 * This is intentionally a structural view contract rather than a durable
 * group schema. Hosts can pass the existing Factory layout group directly;
 * persistence remains owned by the host's layout model.
 */
export interface FactoryGraphGroupRegionInput {
  bounds: FactoryGraphGroupRegionBounds;
  color?: string;
  id: string;
  label?: string;
}

export interface FactoryGraphGroupRegionView {
  bounds: FactoryGraphGroupRegionBounds;
  color: FactoryGraphGroupRegionResolvedColor;
  id: string;
  label: string;
}

export interface FactoryGraphGroupRegionColorStyle {
  accent: string;
  fill: string;
}

const FACTORY_GRAPH_GROUP_REGION_COLOR_STYLES: Record<
  FactoryGraphGroupRegionResolvedColor,
  FactoryGraphGroupRegionColorStyle
> = {
  danger: {
    accent: "var(--color-error)",
    fill: "var(--color-error-container)",
  },
  info: {
    accent: "var(--color-info)",
    fill: "var(--color-info-container)",
  },
  neutral: {
    accent: "var(--color-outline-variant)",
    fill: "var(--color-surface-container-low)",
  },
  primary: {
    accent: "var(--color-primary)",
    fill: "var(--color-primary-container)",
  },
  success: {
    accent: "var(--color-success)",
    fill: "var(--color-success-container)",
  },
  warning: {
    accent: "var(--color-warning)",
    fill: "var(--color-warning-container)",
  },
};

const SUPPORTED_FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS = new Set<string>([
  "danger",
  "info",
  "neutral",
  "primary",
  "success",
  "warning",
]);

/** Resolve legacy outline and unknown values without interpolating raw CSS. */
export function resolveFactoryGraphGroupRegionColor(
  color: string | undefined,
): FactoryGraphGroupRegionResolvedColor {
  if (color === "outline") {
    return "neutral";
  }

  return color !== undefined &&
    SUPPORTED_FACTORY_GRAPH_GROUP_REGION_COLOR_TOKENS.has(color)
    ? (color as FactoryGraphGroupRegionResolvedColor)
    : "neutral";
}

export function factoryGraphGroupRegionColorStyle(
  color: string | undefined,
): FactoryGraphGroupRegionColorStyle {
  return FACTORY_GRAPH_GROUP_REGION_COLOR_STYLES[
    resolveFactoryGraphGroupRegionColor(color)
  ];
}

export function isValidFactoryGraphGroupRegionBounds(
  bounds: FactoryGraphGroupRegionBounds,
): boolean {
  return (
    Number.isFinite(bounds.height) &&
    Number.isFinite(bounds.width) &&
    Number.isFinite(bounds.x) &&
    Number.isFinite(bounds.y) &&
    bounds.height > 0 &&
    bounds.width > 0
  );
}

/** Project saved groups into safe, render-ready read-only view data. */
export function projectFactoryGraphGroupRegions(
  groups: readonly FactoryGraphGroupRegionInput[] | undefined,
): FactoryGraphGroupRegionView[] {
  if (!groups) {
    return [];
  }

  return groups.flatMap((group) => {
    const id = group.id.trim();
    if (!id || !isValidFactoryGraphGroupRegionBounds(group.bounds)) {
      return [];
    }

    return [
      {
        bounds: { ...group.bounds },
        color: resolveFactoryGraphGroupRegionColor(group.color),
        id,
        label: group.label?.trim() || id,
      },
    ];
  });
}

export function projectFactoryGraphGroupRegionBounds(
  bounds: FactoryGraphGroupRegionBounds,
  transform: readonly [number, number, number],
): FactoryGraphGroupRegionBounds {
  const [translateX, translateY, zoom] = transform;
  return {
    height: bounds.height * zoom,
    width: bounds.width * zoom,
    x: bounds.x * zoom + translateX,
    y: bounds.y * zoom + translateY,
  };
}

export interface FactoryGraphGroupRegionLayerProps {
  groupAriaLabel?: (group: FactoryGraphGroupRegionView) => string;
  groups: readonly FactoryGraphGroupRegionInput[] | undefined;
}

/**
 * Render saved groups as decorative graph regions.
 *
 * The layer and every region are pointer-transparent by design. Hosts that
 * need editing compose their own narrowly scoped affordances above this
 * layer; this component never creates edit controls or owns layout state.
 */
export function FactoryGraphGroupRegionLayer({
  groupAriaLabel,
  groups,
}: FactoryGraphGroupRegionLayerProps) {
  const transform = useStore((state) => state.transform);
  const regions = projectFactoryGraphGroupRegions(groups);

  if (regions.length === 0) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-0 z-0"
      data-factory-graph-group-region-layer=""
    >
      {regions.map((region, index) => {
        const projectedBounds = projectFactoryGraphGroupRegionBounds(
          region.bounds,
          transform,
        );
        const colorStyle = factoryGraphGroupRegionColorStyle(region.color);
        const accessibleLabel =
          groupAriaLabel?.(region) ?? (region.label || region.id);

        return (
          <section
            aria-label={accessibleLabel}
            className="pointer-events-none absolute overflow-visible rounded-2xl border-2"
            data-factory-graph-group-region={region.id}
            key={region.id}
            style={{
              backgroundColor: colorStyle.fill,
              borderColor: colorStyle.accent,
              boxShadow: `0 0 0 1px ${colorStyle.fill}, 0 0 1rem ${colorStyle.fill}`,
              height: projectedBounds.height,
              left: projectedBounds.x,
              top: projectedBounds.y,
              width: projectedBounds.width,
              zIndex: index,
            }}
          >
            <span className="pointer-events-none absolute top-0 right-3 left-3 -translate-y-1/2">
              <span
                aria-hidden="true"
                className="block max-w-full truncate rounded-lg border px-2 py-1 text-xs font-semibold leading-5 text-on-surface shadow-sm backdrop-blur-sm"
                data-factory-graph-group-region-label=""
                title={region.label}
                style={{
                  backgroundColor: colorStyle.fill,
                  borderColor: colorStyle.accent,
                }}
              >
                {region.label}
              </span>
            </span>
          </section>
        );
      })}
    </div>
  );
}
