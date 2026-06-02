import { getWorkOutcomeMessages } from "../messages/work-outcome";
import {
  expectWorkChartAxisLabelsVisible as expectWorkChartAxisLabelsVisibleWithLabels,
  expectWorkChartCompactLegendContract,
  expectWorkChartLegendClearOfCardTitle,
} from "./work-chart-legend-contract";

export {
  expectWorkChartCompactLegendContract,
  expectWorkChartLegendClearOfCardTitle,
};

export function expectWorkChartAxisLabelsVisible(
  chart: HTMLElement,
  locale?: string,
): void {
  const chartMessages = getWorkOutcomeMessages(locale).chart;
  expectWorkChartAxisLabelsVisibleWithLabels(chart, {
    xAxisLabel: chartMessages.xAxisLabel,
    yAxisLabel: chartMessages.yAxisLabel,
  });
}
