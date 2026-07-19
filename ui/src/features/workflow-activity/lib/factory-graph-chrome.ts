/** Optional presentation regions for the public, read-only Factory graph. */
export type FactoryGraphChromeRegion =
  | "legend"
  | "background"
  | "viewportControls"
  | "visibilityControls";

export type FactoryGraphChromePreset = "full" | "minimal" | "none";

export interface FactoryGraphChromeConfiguration {
  background?: boolean;
  legend?: boolean;
  preset?: FactoryGraphChromePreset;
  viewportControls?: boolean;
  visibilityControls?: boolean;
}

export type ResolvedFactoryGraphChrome = Readonly<
  Record<FactoryGraphChromeRegion, boolean>
>;

export const DEFAULT_FACTORY_GRAPH_CHROME_PRESET: FactoryGraphChromePreset =
  "full";

const FACTORY_GRAPH_CHROME_PRESETS: Readonly<
  Record<FactoryGraphChromePreset, ResolvedFactoryGraphChrome>
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

/**
 * Resolves graph presentation only. It deliberately never receives or changes
 * the canonical topology or runtime projection supplied to the visualizer.
 */
export function resolveFactoryGraphChrome(
  configuration: FactoryGraphChromeConfiguration = {},
): ResolvedFactoryGraphChrome {
  const preset = configuration.preset ?? DEFAULT_FACTORY_GRAPH_CHROME_PRESET;
  const resolvedPreset = FACTORY_GRAPH_CHROME_PRESETS[preset];

  return {
    background: configuration.background ?? resolvedPreset.background,
    legend: configuration.legend ?? resolvedPreset.legend,
    viewportControls:
      configuration.viewportControls ?? resolvedPreset.viewportControls,
    visibilityControls:
      configuration.visibilityControls ?? resolvedPreset.visibilityControls,
  };
}
