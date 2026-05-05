import type { ComponentProps } from "react";
import { expect, within } from "storybook/test";

import { getDashboardWorkChartSeriesStyle } from "./chart-contract";
import { WorkChart, type WorkChartSeriesDefinition } from "./work-chart";
import type { WorkChartModel } from "./trends";

const populatedModel = {
  delta: {
    queued: 2,
    inFlight: 3,
    completed: 4,
    failed: 1,
  },
  failureGroups: [],
  points: [
    { label: "Tick 10", observedAt: 1000, order: 0, tick: 10 },
    { label: "Tick 20", observedAt: 2000, order: 1, tick: 20 },
    { label: "Tick 40", observedAt: 3000, order: 2, tick: 40 },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 1,
      dispatchedCount: 3,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["story-review-retry"],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 4,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 5,
      failedByWorkType: { story: 0 },
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 3,
      tick: 20,
    },
    {
      completedCount: 6,
      dispatchedCount: 8,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["story-review-retry"],
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
      points: [
        { label: "Queued: 4", observedAt: 1000, order: 0, value: 4 },
        { label: "Queued: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Queued: 2", observedAt: 3000, order: 2, value: 2 },
      ],
      unit: "count",
    },
    {
      key: "inFlight",
      label: "In-flight",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
        { label: "In-flight: 3", observedAt: 3000, order: 2, value: 3 },
      ],
      unit: "count",
    },
    {
      key: "completed",
      label: "Completed",
      points: [
        { label: "Completed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Completed: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Completed: 6", observedAt: 3000, order: 2, value: 6 },
      ],
      unit: "count",
    },
    {
      key: "failed",
      label: "Failed/retried",
      points: [
        { label: "Failed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Failed: 0", observedAt: 2000, order: 1, value: 0 },
        { label: "Failed: 1", observedAt: 3000, order: 2, value: 1 },
      ],
      unit: "count",
    },
  ],
} satisfies WorkChartModel;

const emptyModel = {
  delta: { queued: 0, inFlight: 0, completed: 0, failed: 0 },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
} satisfies WorkChartModel;

const WORK_CHART_SERIES: readonly WorkChartSeriesDefinition[] = [
  {
    key: "queued",
    label: "Queued",
    ...getDashboardWorkChartSeriesStyle("queued"),
  },
  {
    key: "inFlight",
    label: "In-flight",
    ...getDashboardWorkChartSeriesStyle("inFlight"),
  },
  {
    key: "completed",
    label: "Completed",
    ...getDashboardWorkChartSeriesStyle("completed"),
  },
  {
    key: "failed",
    label: "Failed/retried",
    ...getDashboardWorkChartSeriesStyle("failed"),
  },
];

function expectWorkChartOverlayContract(chart: HTMLElement): void {
  expect(within(chart).getByText("Ticks")).toBeVisible();
  expect(within(chart).getByText("Work count")).toBeVisible();
}

function expectWorkChartPaddingContract(chart: HTMLElement): void {
  expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
  expect(chart.className).toContain("px-5");
  expect(chart.className).toContain("pb-5");
  expect(chart.className).toContain("pt-4");
  expect(chart.className).toContain("sm:px-6");
  expect(chart.className).toContain("sm:pb-6");
  expect(chart.className).toContain("sm:pt-5");

  const overlay = chart.querySelector<HTMLElement>("[data-work-chart-overlay='true']");

  expect(overlay).not.toBeNull();
  expect(overlay?.className).toContain("px-5");
  expect(overlay?.className).toContain("pb-4");
  expect(overlay?.className).toContain("pt-4");
  expect(overlay?.className).toContain("sm:px-6");
  expect(overlay?.className).toContain("sm:pb-5");
  expect(overlay?.className).toContain("sm:pt-5");
}

function expectNoOverflowInStoryShell(canvasElement: HTMLElement): void {
  const shell = canvasElement.querySelector<HTMLElement>("[data-story-shell]");

  expect(shell).not.toBeNull();
  expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
  const card = shell?.querySelector<HTMLElement>("[data-chart-container]");
  expect(card).not.toBeNull();
  expect((card?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
}

function renderWorkChartStoryShell(
  args: ComponentProps<typeof WorkChart>,
  maxWidth: string,
) {
  return (
    <div data-story-shell="work-chart" style={{ maxWidth, padding: "1rem", width: "100%" }}>
      <WorkChart {...args} />
    </div>
  );
}

export default {
  title: "Agent Factory/Dashboard/Work Chart",
  component: WorkChart,
  tags: ["website-consistency-shared-primitive"],
  args: {
    ariaLabel: "Work outcome chart",
  },
};

export const Populated = {
  render: (args: ComponentProps<typeof WorkChart>) =>
    renderWorkChartStoryShell(args, "640px"),
  args: {
    model: populatedModel,
    series: WORK_CHART_SERIES,
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const chart = await canvas.findByRole("img", { name: "Work outcome chart" });

    expectWorkChartOverlayContract(chart);
    expectWorkChartPaddingContract(chart);
  },
};

export const EmptyData = {
  args: {
    model: emptyModel,
    series: WORK_CHART_SERIES,
  },
};

export const LoadingData = {
  args: {
    series: WORK_CHART_SERIES,
    state: { status: "loading" },
  },
};

export const IncompleteData = {
  args: {
    model: {
      ...populatedModel,
      series: [{ key: "completed", label: "Completed", unit: "count" }],
    } as unknown as WorkChartModel,
    series: WORK_CHART_SERIES,
  },
};

export const ConstrainedWidth = {
  render: (args: ComponentProps<typeof WorkChart>) =>
    renderWorkChartStoryShell(args, "360px"),
  args: {
    model: populatedModel,
    series: WORK_CHART_SERIES,
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const chart = await canvas.findByRole("img", { name: "Work outcome chart" });

    expectWorkChartOverlayContract(chart);
    expectWorkChartPaddingContract(chart);
    expectNoOverflowInStoryShell(canvasElement);
  },
};
