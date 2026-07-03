/** Stable category path for `@you-agent-factory/components/charts`. */
export const COMPONENTS_CATEGORY = "charts" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
  type ChartConfigEntry,
  type ChartPresentation,
} from "./chart";

export {
  ChartStatePanel,
  type ChartStatePanelProps,
  type ChartStateStatus,
} from "./chart-state-panel";
