import { expect, within } from "storybook/test";

import "../../styles.css";
import { getDashboardWorkChartSeriesDefinitions } from "./chart-contract";
import { D3CompletionInformationCard } from "./d3-information-card";
import type { WorkChartModel } from "./trends";

const populatedTrend: WorkChartModel = {
  delta: {
    queued: 2,
    inFlight: 3,
    completed: 4,
    failed: 2,
  },
  failureGroups: [{ count: 2, label: "Work type: story" }],
  points: [
    {
      label: "Tick 10",
      observedAt: 1000,
      order: 0,
      tick: 10,
    },
    {
      label: "Tick 20",
      observedAt: 2000,
      order: 1,
      tick: 20,
    },
    {
      label: "Tick 40",
      observedAt: 3000,
      order: 2,
      tick: 40,
    },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 2,
      dispatchedCount: 3,
      failedByWorkType: { story: 0 },
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 4,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 5,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["story review rejected"],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 3,
      tick: 20,
    },
    {
      completedCount: 6,
      dispatchedCount: 9,
      failedByWorkType: { story: 2 },
      failedCount: 2,
      failedWorkLabels: ["story review rejected"],
      inFlightCount: 3,
      observedAt: 3000,
      queuedCount: 2,
      tick: 40,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 4", observedAt: 1000, order: 0, value: 4 },
        { label: "Queued: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Queued: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
        { label: "In-flight: 3", observedAt: 3000, order: 2, value: 3 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 2", observedAt: 1000, order: 0, value: 2 },
        { label: "Completed: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Completed: 6", observedAt: 3000, order: 2, value: 6 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [
        { label: "Failed: 0", observedAt: 1000, order: 0, value: 0 },
        { label: "Failed: 1", observedAt: 2000, order: 1, value: 1 },
        { label: "Failed: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
  ],
};

const emptyTrend: WorkChartModel = {
  delta: {
    queued: 0,
    inFlight: 0,
    completed: 0,
    failed: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

const WORK_OUTCOME_CHART_SERIES = getDashboardWorkChartSeriesDefinitions([
  { key: "queued", label: "Queued" },
  { key: "inFlight", label: "In-flight" },
  { key: "completed", label: "Completed" },
  { key: "failed", label: "Failed/retried" },
]);

function expectWorkOutcomeChartContract(card: HTMLElement): void {
  const chart = within(card).getByRole("img", { name: "Work outcome chart for 15m" });

  expect(chart).toBeVisible();
  expect(within(card).getByText("Ticks")).toBeVisible();
  expect(within(card).getByText("Work count")).toBeVisible();

  for (const series of WORK_OUTCOME_CHART_SERIES) {
    const path = chart.querySelector<SVGPathElement>(`[data-chart-series='${series.key}']`);

    expect(path).not.toBeNull();
    expect(path?.getAttribute("data-chart-series-color")).toBe(series.lineColor);
    expect(path?.getAttribute("class")).toContain("[stroke-width:2.25]");
    expect(path ? window.getComputedStyle(path).strokeWidth : "").toBe("2.25px");
  }
}

function expectNoOverflowInStoryShell(canvasElement: HTMLElement): void {
  const shell = canvasElement.querySelector<HTMLElement>("[data-story-shell]");

  expect(shell).not.toBeNull();
  expect(shell ? shell.getBoundingClientRect().width : 0).toBeLessThanOrEqual(360);
  expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
}

export default {
  title: "Agent Factory/Dashboard/D3 Work Outcome Information Card",
  component: D3CompletionInformationCard,
};

export const Populated = {
  args: {
    model: populatedTrend,
    widgetId: "work-outcome-chart-story",
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work outcome chart" });

    expectWorkOutcomeChartContract(card);
  },
};

export const EmptyData = {
  args: {
    model: emptyTrend,
    widgetId: "work-outcome-chart-empty-story",
  },
};

export const LoadingData = {
  args: {
    chartState: { status: "loading" },
    model: emptyTrend,
    widgetId: "work-outcome-chart-loading-story",
  },
};

export const ConstrainedWidth = {
  render: () => (
    <div data-story-shell="work-outcome" style={{ maxWidth: "360px", padding: "1rem" }}>
      <D3CompletionInformationCard
        model={populatedTrend}
        widgetId="work-outcome-chart-narrow-story"
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work outcome chart" });

    expectWorkOutcomeChartContract(card);
    expectNoOverflowInStoryShell(canvasElement);
  },
};
