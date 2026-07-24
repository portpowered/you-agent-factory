/** Stable category path for `@you-agent-factory/components/charts`. */
export const COMPONENTS_CATEGORY = "charts" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  type ChartConfig,
  type ChartConfigEntry,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  type ChartPresentation,
  ChartTooltip,
  ChartTooltipContent,
} from "./chart";

export {
  ChartStatePanel,
  type ChartStatePanelProps,
  type ChartStateStatus,
} from "./chart-state-panel";
