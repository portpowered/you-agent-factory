/** Optional presentation regions for the public, read-only Factory topology. */
export type FactoryTopologyChromeRegion =
  | "legend"
  | "background"
  | "viewportControls"
  | "visibilityControls";

export type FactoryTopologyChromePreset = "full" | "minimal" | "none";

export interface FactoryTopologyChromeConfiguration {
  background?: boolean;
  legend?: boolean;
  preset?: FactoryTopologyChromePreset;
  viewportControls?: boolean;
  visibilityControls?: boolean;
}

export type ResolvedFactoryTopologyChrome = Readonly<
  Record<FactoryTopologyChromeRegion, boolean>
>;

export const DEFAULT_FACTORY_TOPOLOGY_CHROME_PRESET: FactoryTopologyChromePreset =
  "full";

const FACTORY_TOPOLOGY_CHROME_PRESETS: Readonly<
  Record<FactoryTopologyChromePreset, ResolvedFactoryTopologyChrome>
> = {
  full: {
    background: true,
    legend: true,
    viewportControls: true,
    visibilityControls: true,
  },
  minimal: {
    background: true,
    legend: false,
    viewportControls: true,
    visibilityControls: false,
  },
  none: {
    background: false,
    legend: false,
    viewportControls: false,
    visibilityControls: false,
  },
};

/** Resolves presentation without receiving or changing replay projection data. */
export function resolveFactoryTopologyChrome(
  configuration: FactoryTopologyChromeConfiguration = {},
): ResolvedFactoryTopologyChrome {
  const preset = configuration.preset ?? DEFAULT_FACTORY_TOPOLOGY_CHROME_PRESET;
  const resolvedPreset = FACTORY_TOPOLOGY_CHROME_PRESETS[preset];

  return {
    background: configuration.background ?? resolvedPreset.background,
    legend: configuration.legend ?? resolvedPreset.legend,
    viewportControls:
      configuration.viewportControls ?? resolvedPreset.viewportControls,
    visibilityControls:
      configuration.visibilityControls ?? resolvedPreset.visibilityControls,
  };
}
