import { cleanup, render, screen, within } from "@testing-library/react";

import { D3CompletionInformationCard } from "./d3-information-card";
import type { WorkChartModel } from "./trends";

const populatedTrend: WorkChartModel = {
  delta: {
    queued: 1,
    inFlight: 2,
    completed: 3,
    failed: 1,
  },
  failureGroups: [{ count: 2, label: "Work type: story" }],
  points: [
    {
      label: "Tick 7",
      observedAt: 1000,
      order: 0,
      tick: 7,
    },
    {
      label: "Tick 9",
      observedAt: 2000,
      order: 1,
      tick: 9,
    },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 2,
      dispatchedCount: 4,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["task validation failed"],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 3,
      tick: 7,
    },
    {
      completedCount: 5,
      dispatchedCount: 8,
      failedByWorkType: { story: 2 },
      failedCount: 2,
      failedWorkLabels: ["story review rejected"],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 1,
      tick: 9,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 3", observedAt: 1000, order: 0, value: 3 },
        { label: "Queued: 1", observedAt: 2000, order: 1, value: 1 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 2", observedAt: 1000, order: 0, value: 2 },
        { label: "Completed: 5", observedAt: 2000, order: 1, value: 5 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [
        { label: "Failed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Failed: 2", observedAt: 2000, order: 1, value: 2 },
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

describe("D3CompletionInformationCard", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders a D3-backed accessible work outcome chart from dashboard samples", () => {
    render(
      <D3CompletionInformationCard
        model={populatedTrend}
        widgetId="work-outcome-chart"
      />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(within(card).queryByRole("combobox", { name: "Time range" })).toBeNull();
    expect(within(card).queryByRole("list", { name: "Work outcome totals" })).toBeNull();
    expect(within(card).queryByText("Completed in range")).toBeNull();
    expect(
      within(card).getByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeTruthy();
    expect(card.querySelector("[data-chart-series='queued']")).toBeTruthy();
    expect(card.querySelector("[data-chart-series='inFlight']")).toBeTruthy();
    expect(card.querySelector("[data-chart-series='completed']")).toBeTruthy();
    expect(card.querySelector("[data-chart-series='failed']")).toBeTruthy();
    expect(
      card.querySelector("[data-chart-series='queued']")?.getAttribute("data-chart-series-color"),
    ).toBe("var(--color-af-chart-queued)");
    expect(
      card.querySelector("[data-chart-series='inFlight']")?.getAttribute("data-chart-series-color"),
    ).toBe("var(--color-af-chart-in-flight)");
    expect(
      card.querySelector("[data-chart-series='completed']")?.getAttribute("data-chart-series-color"),
    ).toBe("var(--color-af-chart-completed)");
    expect(
      card.querySelector("[data-chart-series='failed']")?.getAttribute("data-chart-series-color"),
    ).toBe("var(--color-af-chart-failed)");
    expect(
      card.querySelector("[data-chart-series='failed']")?.getAttribute("class"),
    ).toContain("[stroke-width:2.25]");
    expect(card.querySelector("circle")).toBeNull();
    expect(card.querySelector("[data-axis-tick='x'][data-axis-tick-value='7']")).toBeTruthy();
    expect(card.querySelector("[data-axis-tick='y'][data-axis-tick-value='0']")).toBeTruthy();
    expect(within(card).getByText("Ticks")).toBeTruthy();
    expect(within(card).getByText("Work count")).toBeTruthy();
  });

  it("renders an explicit empty state without a chart when samples are unavailable", () => {
    render(
      <D3CompletionInformationCard
        model={emptyTrend}
      />,
    );

    expect(screen.queryByRole("img", { name: "Work outcome chart for 15m" })).toBeNull();
    expect(screen.getByText("No work outcome samples")).toBeTruthy();
    expect(
      screen.getByText("Work outcome data appears after the event stream receives work history."),
    ).toBeTruthy();
  });

  it("renders an explicit loading state without dropping chart summary controls", () => {
    render(
      <D3CompletionInformationCard
        chartState={{ status: "loading" }}
        model={emptyTrend}
      />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(within(card).queryByRole("combobox", { name: "Time range" })).toBeNull();
    expect(within(card).getByRole("status")).toBeTruthy();
    expect(within(card).getByText("Loading work outcome samples")).toBeTruthy();
    expect(
      within(card).queryByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeNull();
  });
});
