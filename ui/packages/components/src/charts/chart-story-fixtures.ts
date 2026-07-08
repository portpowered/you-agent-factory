import type { LegendPayload } from "recharts/types/component/DefaultLegendContent";

import type { ChartConfig } from "./chart";

export const sampleChartConfig: ChartConfig = {
  alpha: {
    color: "#3366cc",
    label: "Alpha series",
  },
  beta: {
    color: "#cc6633",
    label: "Beta series",
  },
};

export const sampleChartData = [
  { alpha: 4, beta: 2, tick: 1 },
  { alpha: 6, beta: 3, tick: 2 },
  { alpha: 5, beta: 4, tick: 3 },
];

export const sampleLegendPayload: LegendPayload[] = [
  {
    color: sampleChartConfig.alpha.color,
    dataKey: "alpha",
    type: "line",
    value: sampleChartConfig.alpha.label,
  },
  {
    color: sampleChartConfig.beta.color,
    dataKey: "beta",
    type: "line",
    value: sampleChartConfig.beta.label,
  },
];

export const sampleChartTitle = "Sample throughput chart";
