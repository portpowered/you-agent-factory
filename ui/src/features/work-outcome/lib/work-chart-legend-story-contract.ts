import { expect, within } from "storybook/test";

const DEFAULT_WORK_CHART_SERIES_LABELS = [
  "Queued",
  "In-flight",
  "Completed",
  "Failed/retried",
] as const;

/** Max legend-to-plot height ratio for a graph-first ready work outcome chart. */
const MAX_LEGEND_TO_PLOT_HEIGHT_RATIO = 0.4;

export function expectWorkChartCompactLegendContract(
  chart: HTMLElement,
  {
    seriesLabels = DEFAULT_WORK_CHART_SERIES_LABELS,
  }: {
    seriesLabels?: readonly string[];
  } = {},
): void {
  expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
  expect(chart.getAttribute("data-work-chart-legend-placement")).toBe(
    "shell-row",
  );

  const legend = chart.querySelector<HTMLElement>(
    "[data-work-chart-legend='true']",
  );
  expect(legend).toBeTruthy();
  expect(legend?.getAttribute("data-work-chart-legend-density")).toBe(
    "compact",
  );
  expect(legend?.closest(".recharts-wrapper")).toBeNull();
  expect(chart.querySelector(".recharts-legend-wrapper")).toBeNull();

  const plot = chart.querySelector<HTMLElement>(
    ".recharts-responsive-container",
  );
  expect(plot).toBeTruthy();

  const legendHeight = legend?.getBoundingClientRect().height ?? 0;
  const plotHeight = plot?.getBoundingClientRect().height ?? 0;
  expect(legendHeight).toBeGreaterThan(0);
  expect(plotHeight).toBeGreaterThan(legendHeight);
  expect(legendHeight / plotHeight).toBeLessThan(
    MAX_LEGEND_TO_PLOT_HEIGHT_RATIO,
  );

  const legendControls = within(chart).getAllByRole("button", {
    name: / series$/,
  });
  expect(legendControls.length).toBe(seriesLabels.length);

  for (const label of seriesLabels) {
    expect(
      within(chart).getByRole("button", {
        name: `Hide ${label} series`,
      }),
    ).toBeVisible();
    expect(within(chart).getByText(label)).toBeVisible();
  }
}

export function expectWorkChartAxisLabelsVisible(chart: HTMLElement): void {
  const chartScope = within(chart);
  expect(chartScope.getByText("Ticks")).toBeVisible();
  expect(chartScope.getByText("Work count")).toBeVisible();

  const overlay = chart.querySelector<HTMLElement>(
    "[data-work-chart-overlay='true']",
  );
  expect(overlay).toBeTruthy();

  const chartRect = chart.getBoundingClientRect();
  const overlayRect = overlay?.getBoundingClientRect();
  expect(overlayRect).toBeTruthy();
  expect(overlayRect?.left ?? 0).toBeGreaterThanOrEqual(chartRect.left - 1);
  expect(overlayRect?.top ?? 0).toBeGreaterThanOrEqual(chartRect.top - 1);
  expect(overlayRect?.right ?? 0).toBeLessThanOrEqual(chartRect.right + 1);

  const yAxisLabel = within(overlay as HTMLElement).getByText("Work count");
  const yAxisLabelRect = yAxisLabel.getBoundingClientRect();
  expect(yAxisLabelRect.right).toBeLessThanOrEqual(chartRect.right + 1);
  expect(yAxisLabelRect.top).toBeGreaterThanOrEqual(chartRect.top - 1);
}

export function expectWorkChartLegendClearOfCardTitle(card: HTMLElement): void {
  const title = within(card).getByRole("heading", { level: 3 });
  const chart = within(card).getByRole("img");
  const legend = chart.querySelector<HTMLElement>(
    "[data-work-chart-legend='true']",
  );

  expect(legend).toBeTruthy();

  const titleRect = title.getBoundingClientRect();
  const legendRect = legend?.getBoundingClientRect();
  expect(legendRect).toBeTruthy();
  expect(legendRect?.top ?? 0).toBeGreaterThanOrEqual(titleRect.bottom - 1);
}
