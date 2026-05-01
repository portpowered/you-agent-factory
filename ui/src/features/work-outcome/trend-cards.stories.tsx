import { expect, within } from "storybook/test";

import "../../styles.css";
import { getDashboardChartSemanticStyle } from "./chart-contract";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_WIDGET_SUBTITLE_CLASS,
} from "./typography";
import { FailureTrendCard, ReworkTrendCard, TimingTrendCard } from "./trend-cards";
import type { FailureTrendModel, ReworkTrendModel, TimingTrendModel } from "./trends";

const failureTrend = {
  currentFailed: 3,
  failureDelta: 2,
  groups: [{ count: 2, label: "Work type: story" }],
  path: "M 14 106 L 160 58 L 306 14",
  points: [
    { failedCount: 1, label: "Sample 1: 1 failed", x: 14, y: 106 },
    { failedCount: 2, label: "Sample 2: 2 failed", x: 160, y: 58 },
    { failedCount: 3, label: "Sample 3: 3 failed", x: 306, y: 14 },
  ],
  rangeLabel: "15m",
} satisfies FailureTrendModel;

const reworkTrend = {
  currentWorkLabel: "work-active-story",
  path: "M 14 106 L 160 70 L 306 14",
  points: [
    { dispatchLabel: "Review", reworkCount: 1, x: 14, y: 106 },
    { dispatchLabel: "Revise", reworkCount: 2, x: 160, y: 70 },
    { dispatchLabel: "Plan", reworkCount: 3, x: 306, y: 14 },
  ],
  retryOrReworkCount: 3,
  terminalOutcome: "REJECTED",
} satisfies ReworkTrendModel;

const timingTrend = {
  averageDurationMillis: 1_500,
  currentWorkLabel: "work-active-story",
  fastestDurationMillis: 450,
  latestDurationMillis: 3_000,
  path: "M 14 96 L 160 52 L 306 14",
  points: [
    { dispatchLabel: "Review", durationMillis: 450, x: 14, y: 96 },
    { dispatchLabel: "Revise", durationMillis: 1_200, x: 160, y: 52 },
    { dispatchLabel: "Plan", durationMillis: 3_000, x: 306, y: 14 },
  ],
  slowestDurationMillis: 3_000,
} satisfies TimingTrendModel;

function expectTrendChartContract(
  chart: HTMLElement,
  role: "failureTrend" | "reworkTrend" | "timingTrend",
): void {
  const chartStyle = getDashboardChartSemanticStyle(role);
  const path = chart.querySelector<SVGPathElement>("path");
  const point = chart.querySelector<SVGCircleElement>("circle");

  expect(path).not.toBeNull();
  expect(point).not.toBeNull();
  expect(path?.getAttribute("stroke")).toBe(chartStyle.color);
  expect(path?.getAttribute("class")).toContain("[stroke-width:2.25]");
  expect(point?.getAttribute("class")).toBe(chartStyle.pointClassName);
  expect(point?.getAttribute("r")).toBe(`${chartStyle.pointRadius}`);
}

function expectNoOverflowInStoryShell(canvasElement: HTMLElement): void {
  const shell = canvasElement.querySelector<HTMLElement>("[data-story-shell]");

  expect(shell).not.toBeNull();
  expect(shell ? shell.getBoundingClientRect().width : 0).toBeLessThanOrEqual(360);
  expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
}

export default {
  title: "Agent Factory/Dashboard/Trend Cards",
  component: FailureTrendCard,
};

export const TypographyScale = {
  render: () => (
    <div className="grid gap-4">
      <FailureTrendCard
        model={failureTrend}
        onRangeChange={() => undefined}
        rangeID="15m"
        widgetId="failure-trend-story"
      />
      <ReworkTrendCard model={reworkTrend} widgetId="rework-trend-story" />
      <TimingTrendCard model={timingTrend} widgetId="timing-trend-story" />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByRole("heading", { name: "Failure trend" })).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Retry and rework trend" }),
    ).toBeVisible();
    await expect(canvas.getByRole("heading", { name: "Timing trend" })).toBeVisible();

    const failureCard = canvas.getByRole("heading", { name: "Failure trend" }).closest("article");
    const timingCard = canvas.getByRole("heading", { name: "Timing trend" }).closest("article");
    const failureChart = canvas.getByRole("img", { name: "Failed work trend for 15m" });

    expect(failureCard).toBeTruthy();
    expect(timingCard).toBeTruthy();
    expectTrendChartContract(failureChart, "failureTrend");

    const failureScope = within(failureCard!);
    const timingScope = within(timingCard!);

    expect(failureScope.getByText("Time range").className).toContain(
      DASHBOARD_SUPPORTING_LABEL_CLASS,
    );
    expect(failureScope.getByLabelText("Time range").className).toContain(
      DASHBOARD_BODY_TEXT_CLASS,
    );
    expect(failureScope.getByText("Failed in range").closest("dl")?.className).toContain(
      DASHBOARD_SUPPORTING_LABELS_CLASS,
    );
    expect(
      failureScope.getByText("Failed in range").closest("div")?.querySelector("dd")?.className,
    ).toContain(DASHBOARD_WIDGET_SUBTITLE_CLASS);
    expect(timingScope.getByLabelText("Timing range").className).toContain(
      DASHBOARD_SUPPORTING_LABELS_CLASS,
    );
  },
};

export const FailureTrendConstrainedWidth = {
  render: () => (
    <div data-story-shell="failure-trend" style={{ maxWidth: "360px", padding: "1rem" }}>
      <FailureTrendCard
        model={failureTrend}
        onRangeChange={() => undefined}
        rangeID="15m"
        widgetId="failure-trend-narrow-story"
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const chart = await canvas.findByRole("img", { name: "Failed work trend for 15m" });

    expect(chart).toBeVisible();
    expectTrendChartContract(chart, "failureTrend");
    expectNoOverflowInStoryShell(canvasElement);
  },
};
