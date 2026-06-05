import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { getDashboardChartSemanticStyle } from "../lib/chart-contract";
import type {
  FailureTrendModel,
  ReworkTrendModel,
  TimingTrendModel,
} from "../lib/trends";
import {
  FailureTrendCard,
  ReworkTrendCard,
  TimingTrendCard,
} from "./trend-cards";

const failureTrend: FailureTrendModel = {
  currentFailed: 3,
  failureDelta: 2,
  groups: [{ count: 2, label: "Work type: story" }],
  path: "M 14 106 L 306 14",
  points: [
    { failedCount: 1, label: "Sample 1: 1 failed", x: 14, y: 106 },
    { failedCount: 3, label: "Sample 2: 3 failed", x: 306, y: 14 },
  ],
  rangeLabel: "15m",
};

const reworkTrend: ReworkTrendModel = {
  currentWorkLabel: "work-active-story",
  path: "M 14 106 L 306 14",
  points: [
    { dispatchLabel: "Review", reworkCount: 1, x: 14, y: 106 },
    { dispatchLabel: "Plan", reworkCount: 2, x: 306, y: 14 },
  ],
  retryOrReworkCount: 2,
  terminalOutcome: "REJECTED",
};

const timingTrend: TimingTrendModel = {
  averageDurationMillis: 1_500,
  currentWorkLabel: "work-active-story",
  fastestDurationMillis: 450,
  latestDurationMillis: 3_000,
  path: "M 14 106 L 306 14",
  points: [
    { dispatchLabel: "Review", durationMillis: 450, x: 14, y: 106 },
    { dispatchLabel: "Plan", durationMillis: 3_000, x: 306, y: 14 },
  ],
  slowestDurationMillis: 3_000,
};

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

describe("dashboard trend cards", () => {
  it("renders the failure trend card with range changes and cause groups", () => {
    const onRangeChange = vi.fn();
    const failureChartStyle = getDashboardChartSemanticStyle("failureTrend");

    render(
      <FailureTrendCard
        model={failureTrend}
        onRangeChange={onRangeChange}
        rangeID="15m"
      />,
    );

    expect(screen.getByRole("heading", { name: "Failure trend" })).toBeTruthy();
    expect(screen.getByText("Work type: story")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Time range"), {
      target: { value: "5m" },
    });

    const chart = screen.getByRole("img", { name: /Failed work trend/ });

    expect(onRangeChange).toHaveBeenCalledWith("5m");
    expect(chart.querySelector("path")?.getAttribute("class")).toBe(
      failureChartStyle.lineClassName,
    );
    expect(chart.querySelector("path")?.getAttribute("stroke")).toBe(
      failureChartStyle.color,
    );
    expect(chart.querySelector("circle")?.getAttribute("class")).toBe(
      failureChartStyle.pointClassName,
    );
    expect(chart.querySelector("circle")?.getAttribute("r")).toBe(
      `${failureChartStyle.pointRadius}`,
    );
  });

  it("renders retry and rework trend values from a selected trace model", () => {
    const reworkChartStyle = getDashboardChartSemanticStyle("reworkTrend");
    render(<ReworkTrendCard model={reworkTrend} />);

    expect(
      screen.getByRole("heading", { name: "Retry and rework trend" }),
    ).toBeTruthy();
    expect(screen.getByText("work-active-story")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    const chart = screen.getByRole("img", { name: /Retry and rework trend/ });

    expect(chart).toBeTruthy();
    expect(chart.querySelector("path")?.getAttribute("stroke")).toBe(
      reworkChartStyle.color,
    );
    expect(chart.querySelector("circle")?.getAttribute("class")).toBe(
      reworkChartStyle.pointClassName,
    );
  });

  it("renders localized retry and rework point titles", () => {
    render(<ReworkTrendCard locale="zh-CN" model={reworkTrend} />);

    const chart = screen.getByRole("img", { name: /重试与返工趋势/ });

    expect(within(chart).getByText("Review：1 个重试或返工事件")).toBeTruthy();
  });

  it("renders timing trend summaries with formatted durations", () => {
    const timingChartStyle = getDashboardChartSemanticStyle("timingTrend");
    render(<TimingTrendCard model={timingTrend} />);

    expect(screen.getByRole("heading", { name: "Timing trend" })).toBeTruthy();
    expect(screen.getByText("450ms")).toBeTruthy();
    expect(screen.getAllByText("3s").length).toBeGreaterThan(0);
    const chart = screen.getByRole("img", { name: /Timing trend/ });

    expect(chart).toBeTruthy();
    expect(chart.querySelector("path")?.getAttribute("stroke")).toBe(
      timingChartStyle.color,
    );
    expect(chart.querySelector("circle")?.getAttribute("class")).toBe(
      timingChartStyle.pointClassName,
    );
  });

  it("renders empty trend cards through shared empty-state copy primitives", () => {
    render(
      <>
        <FailureTrendCard
          model={{
            currentFailed: 0,
            failureDelta: 0,
            groups: [],
            path: null,
            points: [],
            rangeLabel: "15m",
          }}
          onRangeChange={() => undefined}
          rangeID="15m"
        />
        <ReworkTrendCard
          model={{
            currentWorkLabel: "",
            path: null,
            points: [],
            retryOrReworkCount: 0,
            terminalOutcome: "APPROVED",
          }}
        />
        <TimingTrendCard
          model={{
            averageDurationMillis: 0,
            currentWorkLabel: "",
            fastestDurationMillis: 0,
            latestDurationMillis: 0,
            path: null,
            points: [],
            slowestDurationMillis: 0,
          }}
        />
      </>,
    );

    expect(screen.getByText("No failure samples").className).toContain(
      "af-dashboard-section-heading",
    );
    expect(
      screen.getByText(
        "Failure trend data appears after the event stream receives work history.",
      ).className,
    ).toContain("af-dashboard-body-text");
    expect(screen.getAllByText("No selected trace")).toHaveLength(2);
    expect(
      screen.getByText(
        "Select active work with retained trace history to compare dispatch timing.",
      ).className,
    ).toContain("af-dashboard-body-text");
  });

  it("applies shared typography helpers to trend labels, summaries, and supporting copy", () => {
    render(
      <>
        <FailureTrendCard
          model={failureTrend}
          onRangeChange={() => undefined}
          rangeID="15m"
        />
        <ReworkTrendCard model={reworkTrend} />
        <TimingTrendCard model={timingTrend} />
      </>,
    );

    const failureCard = screen
      .getByRole("heading", { name: "Failure trend" })
      .closest("article");
    const reworkCard = screen
      .getByRole("heading", { name: "Retry and rework trend" })
      .closest("article");
    const timingCard = screen
      .getByRole("heading", { name: "Timing trend" })
      .closest("article");

    const resolvedFailureCard = requireValue(
      failureCard,
      "expected failure trend card",
    );
    const resolvedReworkCard = requireValue(
      reworkCard,
      "expected rework trend card",
    );
    const resolvedTimingCard = requireValue(
      timingCard,
      "expected timing trend card",
    );

    const failureScope = within(resolvedFailureCard);
    const reworkScope = within(resolvedReworkCard);
    const timingScope = within(resolvedTimingCard);
    const toolbar = failureScope.getByText("Time range").closest("div")
      ?.parentElement;
    const summary = failureScope.getByText("Failed in range").closest("dl");
    const failedInRangeMetric = failureScope
      .getByText("Failed in range")
      .closest("div");

    expect(failureScope.getByText("Time range").tagName).toBe("LABEL");
    expect(failureScope.getByLabelText("Time range").tagName).toBe("SELECT");
    expect(
      requireValue(toolbar, "expected failure trend toolbar").className,
    ).toContain("md:flex-row");
    expect(
      requireValue(summary, "expected failure summary").className,
    ).toContain("md:grid-cols-3");
    expect(failedInRangeMetric?.className).toContain("border-outline");
    expect(failedInRangeMetric?.className).toContain(
      "bg-surface-container-low",
    );
    expect(
      failureScope
        .getByText("Failed in range")
        .closest("div")
        ?.querySelector("dd")?.className,
    ).toContain("af-dashboard-widget-subtitle");
    expect(failureScope.getByText("Work type: story").className).toContain(
      "af-dashboard-body-text",
    );
    expect(failureScope.getByText("Work type: story").className).toContain(
      "text-on-surface-variant",
    );
    expect(failureScope.getByLabelText("Time range").className).toContain(
      "border-primary",
    );
    expect(failureScope.getByLabelText("Time range").className).toContain(
      "bg-surface-container-high",
    );

    expect(reworkScope.getByText("work-active-story").className).toContain(
      "af-dashboard-widget-subtitle",
    );
    expect(timingScope.getByLabelText("Timing range").tagName).toBe("DL");
    expect(timingScope.getByText("450ms").className).toContain(
      "af-dashboard-widget-subtitle",
    );
  });
});
