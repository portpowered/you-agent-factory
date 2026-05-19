import userEvent from "@testing-library/user-event";
import { fireEvent, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../components/dashboard/test-browser-shims";
import { WorkChartCard } from "./d3-information-card";
import { WorkChart, type WorkChartSeriesDefinition } from "./work-chart";
import type { WorkChartModel } from "./trends";
import { getDashboardWorkChartSeriesStyle } from "./chart-contract";

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
          points: [{ label: "Failed: 0", observedAt: 2000, order: 1, value: 0 }],
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

    expect(screen.getByRole("img", { name: "Zero work chart" }).textContent).toContain("Failed");
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
      screen.getByText("Work outcome data appears after the event stream receives work history."),
    ).toBeTruthy();
    expect(screen.queryByRole("img", { name: "Work chart empty" })).toBeNull();
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
    expect(screen.queryByRole("img", { name: "Work chart zero series" })).toBeNull();
  });

  it("renders zh-CN chart labels", () => {
    render(
      <WorkChartCard
        locale="zh-CN"
        model={sparseWorkChartModel}
      />,
    );

    const chart = screen.getByRole("img", { name: "15m 的工作结果图表" });
    expect(within(chart).getByText("排队中")).toBeTruthy();
    expect(within(chart).getByText("进行中")).toBeTruthy();
    expect(within(chart).getByText("已完成")).toBeTruthy();
    expect(within(chart).getByText("刻度")).toBeTruthy();
    expect(within(chart).getByText("工作计数")).toBeTruthy();
  });

  it("supports localized zoom and reset interactions", async () => {
    const user = userEvent.setup();

    render(
      <WorkChart
        ariaLabel="工作结果图表"
        locale="zh-CN"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "工作结果图表" });
    vi.spyOn(chart, "getBoundingClientRect").mockReturnValue({
      bottom: 240,
      height: 240,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 0,
      width: 400,
      x: 0,
      y: 0,
    });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20,40");

    fireEvent.mouseDown(chart, { clientX: 40, clientY: 168 });
    fireEvent.mouseMove(chart, { clientX: 200, clientY: 168 });
    fireEvent.mouseUp(chart, { clientX: 200, clientY: 168 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");
    expect(screen.getByText("已缩放到刻度 10-20")).toBeTruthy();

    const resetZoom = screen.getByRole("button", {
      name: "重置工作结果图表缩放",
    });
    expect(resetZoom).toBeTruthy();
    expect(chart.contains(resetZoom)).toBe(false);

    await user.click(resetZoom);

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20,40");
    expect(screen.queryByText("已缩放到刻度 10-20")).toBeNull();
  });

  it("keeps the reset zoom button above the chart interaction surface and keyboard operable", async () => {
    const user = userEvent.setup();

    render(
      <WorkChart
        ariaLabel="Work chart keyboard zoom"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Work chart keyboard zoom" });
    vi.spyOn(chart, "getBoundingClientRect").mockReturnValue({
      bottom: 240,
      height: 240,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 0,
      width: 400,
      x: 0,
      y: 0,
    });

    fireEvent.mouseDown(chart, { clientX: 40, clientY: 168 });
    fireEvent.mouseMove(chart, { clientX: 200, clientY: 168 });
    fireEvent.mouseUp(chart, { clientX: 200, clientY: 168 });

    const resetZoom = screen.getByRole("button", {
      name: "Reset work outcome chart zoom",
    });
    expect(chart.contains(resetZoom)).toBe(false);

    resetZoom.focus();
    expect(document.activeElement).toBe(resetZoom);
    await user.keyboard("[Enter]");

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20,40");
    expect(
      screen.queryByRole("button", { name: "Reset work outcome chart zoom" }),
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
    expect(screen.getByText("Waiting for dashboard timeline data.")).toBeTruthy();
    expect(loadingState.querySelector(".animate-pulse")).toBeTruthy();
    expect(screen.queryByRole("img", { name: "Work chart loading" })).toBeNull();
  });

  it("uses caller-provided loading, empty, and error copy", () => {
    const { rerender } = render(
      <WorkChart
        ariaLabel="Work chart custom states"
        series={OUTCOME_SERIES}
        state={{
          message: "Loading custom message",
          status: "loading",
          title: "Loading custom title",
        }}
      />,
    );

    expect(screen.getByText("Loading custom title")).toBeTruthy();
    expect(screen.getByText("Loading custom message")).toBeTruthy();

    rerender(
      <WorkChart
        ariaLabel="Work chart custom states"
        emptyMessage="Empty custom message"
        emptyTitle="Empty custom title"
        model={emptyWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    expect(screen.getByText("Empty custom title")).toBeTruthy();
    expect(screen.getByText("Empty custom message")).toBeTruthy();

    rerender(
      <WorkChart
        ariaLabel="Work chart custom states"
        series={OUTCOME_SERIES}
        state={{
          message: "Error custom message",
          status: "error",
          title: "Error custom title",
        }}
      />,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("Error custom title")).toBeTruthy();
    expect(screen.getByText("Error custom message")).toBeTruthy();
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
    expect(screen.queryByRole("img", { name: "Work chart malformed" })).toBeNull();
  });
});
