import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { installDashboardBrowserTestShims } from "../../components/dashboard/test-browser-shims";
import { getDashboardWorkChartSeriesStyle } from "./chart-contract";
import type { WorkChartModel } from "./trends";
import { WorkChart, type WorkChartSeriesDefinition } from "./work-chart";

const sparseWorkChartModel: WorkChartModel = {
  delta: {
    queued: 1,
    inFlight: 2,
    completed: 3,
    failed: 0,
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
      dispatchedCount: 0,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 3,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 1,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 2,
      tick: 20,
    },
    {
      completedCount: 5,
      dispatchedCount: 2,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 3000,
      queuedCount: 1,
      tick: 40,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 3", observedAt: 1000, order: 0, value: 3 },
        { label: "Queued: 1", observedAt: 3000, order: 2, value: 1 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Completed: 3", observedAt: 2000, order: 2, value: 3 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [],
    },
  ],
};

const emptyWorkChartModel: WorkChartModel = {
  delta: { queued: 0, inFlight: 0, completed: 0, failed: 0 },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

const zeroValuedFailedSeriesModel: WorkChartModel = {
  ...sparseWorkChartModel,
  series: sparseWorkChartModel.series.map((seriesEntry) =>
    seriesEntry.key === "failed"
      ? {
          ...seriesEntry,
          points: [
            { label: "Failed: 0", observedAt: 2000, order: 1, value: 0 },
          ],
        }
      : seriesEntry,
  ),
};

const OUTCOME_SERIES: readonly WorkChartSeriesDefinition[] = [
  {
    key: "queued",
    label: "Queued",
    ...getDashboardWorkChartSeriesStyle("queued"),
  },
  {
    key: "completed",
    label: "Completed",
    ...getDashboardWorkChartSeriesStyle("completed"),
  },
  {
    key: "inFlight",
    label: "In-flight",
    ...getDashboardWorkChartSeriesStyle("inFlight"),
  },
  {
    key: "failed",
    label: "Failed",
    ...getDashboardWorkChartSeriesStyle("failed"),
  },
];

describe("WorkChart", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  it("renders reusable paths for sparse outcome series without crashing", () => {
    render(
      <WorkChart
        ariaLabel="Work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Work chart" });
    expect(chart).toBeTruthy();
    expect(chart.querySelector(".recharts-wrapper")).toBeTruthy();
    expect(within(chart).getByText("Queued")).toBeTruthy();
    expect(within(chart).getByText("In-flight")).toBeTruthy();
    expect(within(chart).getByText("Completed")).toBeTruthy();
    expect(within(chart).queryByText("Failed")).toBeNull();
    expect(within(chart).getByText("Ticks")).toBeTruthy();
    expect(within(chart).getByText("Work count")).toBeTruthy();
  });

  it("zooms the ready chart to the selected tick range after a horizontal drag", () => {
    render(
      <WorkChart
        ariaLabel="Zoomable work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Zoomable work chart" });
    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );

    dragWorkChart(chart, { endX: 650, startX: 60 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");
    expect(within(chart).getByText("Queued")).toBeTruthy();
    expect(within(chart).getByText("In-flight")).toBeTruthy();
    expect(within(chart).getByText("Completed")).toBeTruthy();
    expect(within(chart).getByText("Ticks")).toBeTruthy();
    expect(within(chart).getByText("Work count")).toBeTruthy();
  });

  it("applies the same selected tick range when dragged from right to left", () => {
    render(
      <WorkChart
        ariaLabel="Reverse zoomable work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", {
      name: "Reverse zoomable work chart",
    });

    dragWorkChart(chart, { endX: 60, startX: 650 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");
  });

  it("leaves the current domain unchanged for click-only selections", () => {
    render(
      <WorkChart
        ariaLabel="Click-only work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Click-only work chart" });

    dragWorkChart(chart, { endX: 60, startX: 60 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
  });

  it("shows active zoom context and resets the chart range when reset is clicked", async () => {
    const user = userEvent.setup();

    render(
      <WorkChart
        ariaLabel="Resettable work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Resettable work chart" });
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();

    dragWorkChart(chart, { endX: 650, startX: 60 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");
    expect(screen.getByText("Zoomed to ticks 10-20")).toBeTruthy();

    await user.click(
      screen.getByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    );

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
    expect(screen.queryByText("Zoomed to ticks 10-20")).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();
  });

  it("lets keyboard users focus and activate reset zoom with Enter and Space", async () => {
    const user = userEvent.setup();

    const { rerender } = render(
      <WorkChart
        ariaLabel="Keyboard reset work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    let chart = screen.getByRole("img", { name: "Keyboard reset work chart" });
    dragWorkChart(chart, { endX: 650, startX: 60 });

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    );

    await user.keyboard("{Enter}");
    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();

    rerender(
      <WorkChart
        ariaLabel="Keyboard reset work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    chart = screen.getByRole("img", { name: "Keyboard reset work chart" });
    dragWorkChart(chart, { endX: 650, startX: 60 });
    await user.tab();
    await user.keyboard(" ");

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();
  });

  it("keeps missing series points absent instead of fabricating zero-valued rows", () => {
    render(
      <WorkChart
        ariaLabel="Sparse work chart"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    expect(screen.queryByText("Failed")).toBeNull();
  });

  it("still renders a series when the retained sample value is explicitly zero", () => {
    render(
      <WorkChart
        ariaLabel="Zero work chart"
        model={zeroValuedFailedSeriesModel}
        series={OUTCOME_SERIES}
      />,
    );

    expect(
      screen.getByRole("img", { name: "Zero work chart" }).textContent,
    ).toContain("Failed");
  });

  it("renders explicit no-data state when timeline points are unavailable", () => {
    render(
      <WorkChart
        ariaLabel="Work chart empty"
        model={emptyWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    expect(screen.getByText("No work outcome samples")).toBeTruthy();
    expect(
      screen.getByText(
        "Work outcome data appears after the event stream receives work history.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("img", { name: "Work chart empty" })).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();
  });

  it("renders explicit no-data state when series definitions are unavailable", () => {
    render(
      <WorkChart
        ariaLabel="Work chart zero series"
        model={sparseWorkChartModel}
        series={[]}
      />,
    );

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.getByText("No work outcome samples")).toBeTruthy();
    expect(
      screen.queryByRole("img", { name: "Work chart zero series" }),
    ).toBeNull();
  });

  it("renders an accessible loading placeholder before chart data is ready", () => {
    render(
      <WorkChart
        ariaLabel="Work chart loading"
        series={OUTCOME_SERIES}
        state={{ status: "loading" }}
      />,
    );

    const loadingState = screen.getByRole("status");
    expect(loadingState.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByText("Loading work outcome samples")).toBeTruthy();
    expect(
      screen.getByText("Waiting for dashboard timeline data."),
    ).toBeTruthy();
    expect(loadingState.querySelector(".animate-pulse")).toBeTruthy();
    expect(
      screen.queryByRole("img", { name: "Work chart loading" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();
  });

  it("renders an error-safe fallback when the chart model shape is incomplete", () => {
    const malformedModel = {
      ...sparseWorkChartModel,
      series: [{ key: "completed", label: "Completed", unit: "count" }],
    } as unknown as WorkChartModel;

    expect(() => {
      render(
        <WorkChart
          ariaLabel="Work chart malformed"
          model={malformedModel}
          series={OUTCOME_SERIES}
        />,
      );
    }).not.toThrow();

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("Work outcome chart unavailable")).toBeTruthy();
    expect(
      screen.getByText(
        "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("img", { name: "Work chart malformed" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Reset work outcome chart zoom",
      }),
    ).toBeNull();
  });
});

function dragWorkChart(
  chart: HTMLElement,
  { endX, startX }: { endX: number; startX: number },
): void {
  const surface = chart.querySelector(".recharts-surface");
  expect(surface).toBeTruthy();

  fireEvent.mouseDown(surface as Element, { clientX: startX, clientY: 280 });
  fireEvent.mouseMove(surface as Element, { clientX: endX, clientY: 280 });
  fireEvent.mouseUp(surface as Element, { clientX: endX, clientY: 280 });
}
