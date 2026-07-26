import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { getDashboardWorkChartSeriesStyle } from "../lib/chart-contract";
import {
  createMaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
  selectMaterializedWorkOutcomeSamples,
} from "../lib/materializer/materialized-work-outcome";
import { buildWorkChartModel, type WorkChartModel } from "../lib/trends";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import {
  WORK_CHART_MARGIN,
  WorkChart,
  type WorkChartSeriesDefinition,
} from "./work-chart";

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

const edgeZoomWorkChartModel: WorkChartModel = {
  delta: { queued: -25, inFlight: 0, completed: 25, failed: 0 },
  failureGroups: [],
  points: Array.from({ length: 26 }, (_, index) => ({
    label: `Tick ${index}`,
    observedAt: index * 1000,
    order: index,
    tick: index,
  })),
  rangeID: "session",
  rangeLabel: "Session",
  samples: Array.from({ length: 26 }, (_, index) => ({
    completedCount: index,
    dispatchedCount: index,
    failedByWorkType: {},
    failedCount: 0,
    failedWorkLabels: [],
    inFlightCount: 1,
    observedAt: index * 1000,
    queuedCount: 25 - index,
    tick: index,
  })),
  series: [
    {
      key: "queued",
      label: "Queued",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: `Queued: ${25 - index}`,
        observedAt: index * 1000,
        order: index,
        value: 25 - index,
      })),
      unit: "count",
    },
    {
      key: "completed",
      label: "Completed",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: `Completed: ${index}`,
        observedAt: index * 1000,
        order: index,
        value: index,
      })),
      unit: "count",
    },
    {
      key: "inFlight",
      label: "In-flight",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: "In-flight: 1",
        observedAt: index * 1000,
        order: index,
        value: 1,
      })),
      unit: "count",
    },
    {
      key: "failed",
      label: "Failed/retried",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: "Failed: 0",
        observedAt: index * 1000,
        order: index,
        value: 0,
      })),
      unit: "count",
    },
  ],
};

const liveSessionLikeModel = buildWorkChartModel(
  selectMaterializedWorkOutcomeSamples(
    reduceMaterializedWorkOutcomeEvents(createMaterializedWorkOutcomeState(), [
      workOutcomeEvent("run-started", 0, FACTORY_EVENT_TYPES.runRequest, {
        factory: {
          resources: [{ capacity: 10, name: "executor-slot" }],
          workTypes: [
            {
              name: "plan",
              states: [
                { name: "init", type: "INITIAL" },
                { name: "complete", type: "TERMINAL" },
                { name: "failed", type: "FAILED" },
              ],
            },
          ],
          workers: [],
          workstations: [],
        },
        recordedAt: "2026-06-04T10:28:25.842914Z",
      }),
      workOutcomeEvent("work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Plan A",
            traceId: "trace-plan-a",
            workId: "plan-a",
            workTypeName: "plan",
          },
          {
            name: "Plan B",
            traceId: "trace-plan-b",
            workId: "plan-b",
            workTypeName: "plan",
          },
        ],
      }),
      workOutcomeEvent(
        "dispatch-request-plan-a",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-a" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-a" },
      ),
      workOutcomeEvent(
        "dispatch-request-plan-b",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-b" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-b" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-a",
        4,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "Plan A",
              state: "complete",
              traceId: "trace-plan-a",
              workId: "plan-a",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-a" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-b",
        6,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          failureMessage: "setup failed",
          failureReason: "workspace bootstrap failed",
          outcome: "FAILED",
          outputWork: [
            {
              name: "Plan B",
              state: "failed",
              traceId: "trace-plan-b",
              workId: "plan-b",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-b" },
      ),
      workOutcomeEvent("work-request-2", 7, FACTORY_EVENT_TYPES.workRequest, {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Plan C",
            traceId: "trace-plan-c",
            workId: "plan-c",
            workTypeName: "plan",
          },
        ],
      }),
      workOutcomeEvent(
        "dispatch-request-plan-c",
        11,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-c" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-c" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-c",
        12,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "Plan C",
              state: "complete",
              traceId: "trace-plan-c",
              workId: "plan-c",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-c" },
      ),
    ]),
    12,
  ),
  "session",
  0,
  "en",
);

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
const CHART_SIZING_WARNING_FRAGMENT =
  "The width(-1) and height(-1) of chart should be greater than 0";

function expectNoChartSizingWarnings(
  warnSpy: ReturnType<typeof vi.spyOn>,
  errorSpy: ReturnType<typeof vi.spyOn>,
) {
  for (const call of [...warnSpy.mock.calls, ...errorSpy.mock.calls]) {
    expect(call.join(" ")).not.toContain(CHART_SIZING_WARNING_FRAGMENT);
  }
}

describe("WorkChart", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  it("renders the series legend in a content-sized shell row outside Recharts", () => {
    render(
      <WorkChart
        ariaLabel="Work chart legend placement"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", {
      name: "Work chart legend placement",
    });
    expect(chart.getAttribute("data-work-chart-legend-placement")).toBe(
      "shell-row",
    );
    expect(chart.getAttribute("data-work-chart-plot-margin-bottom")).toBe(
      String(WORK_CHART_MARGIN.bottom),
    );
    expect(WORK_CHART_MARGIN.bottom).toBeLessThan(40);

    const legend = chart.querySelector("[data-work-chart-legend='true']");
    expect(legend).toBeTruthy();
    expect(legend?.closest(".recharts-wrapper")).toBeNull();
    expect(chart.querySelector(".recharts-legend-wrapper")).toBeNull();
    expect(within(chart).getByText("Queued")).toBeTruthy();
  });

  it("renders a compact single-row legend at default work outcome bento width", () => {
    render(
      <div style={{ width: "24rem" }}>
        <WorkChart
          ariaLabel="Work chart compact legend"
          model={sparseWorkChartModel}
          series={OUTCOME_SERIES}
        />
      </div>,
    );

    const chart = screen.getByRole("img", {
      name: "Work chart compact legend",
    });
    const legend = chart.querySelector("[data-work-chart-legend='true']");
    expect(legend?.getAttribute("data-work-chart-legend-density")).toBe(
      "compact",
    );

    const legendContent = legend?.firstElementChild as HTMLElement | null;
    expect(legendContent?.className).toContain("flex-nowrap");
    expect(legendContent?.className).toContain("gap-x-2");
    expect(legendContent?.className).toContain("pt-0");

    const swatch = legendContent?.querySelector("[aria-hidden='true']");
    expect(swatch?.className).toContain("h-2");
    expect(swatch?.className).toContain("w-2");

    const legendControls = within(chart).getAllByRole("button", {
      name: / series$/,
    });
    expect(legendControls.length).toBeGreaterThan(1);
    const rowTops = legendControls.map(
      (control) => control.getBoundingClientRect().top,
    );
    expect(new Set(rowTops).size).toBe(1);
  });

  it("toggles series visibility from shell-row legend controls", () => {
    render(
      <WorkChart
        ariaLabel="Work chart legend toggle"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Work chart legend toggle" });
    const queuedLegendControl = within(chart).getByRole("button", {
      name: "Hide Queued series",
    });

    expect(queuedLegendControl.getAttribute("aria-pressed")).toBe("true");
    expect(chart.getAttribute("data-work-chart-hidden-series")).toBe("");
    expect(chart.getAttribute("data-work-chart-visible-series")).toContain(
      "queued",
    );

    fireEvent.click(queuedLegendControl);

    expect(
      within(chart).getByRole("button", { name: "Show Queued series" }),
    ).toBeTruthy();
    expect(queuedLegendControl.getAttribute("aria-pressed")).toBe("false");
    expect(chart.getAttribute("data-work-chart-hidden-series")).toBe("queued");
    expect(chart.getAttribute("data-work-chart-visible-series")).not.toContain(
      "queued",
    );

    fireEvent.click(
      within(chart).getByRole("button", { name: "Show Queued series" }),
    );

    expect(
      within(chart).getByRole("button", { name: "Hide Queued series" }),
    ).toBeTruthy();
    expect(chart.getAttribute("data-work-chart-hidden-series")).toBe("");
    expect(chart.getAttribute("data-work-chart-visible-series")).toContain(
      "queued",
    );
  });

  it("exposes localized hide and show aria labels on legend toggles", () => {
    const chartMessages = getWorkOutcomeMessages("zh-CN").chart;

    render(
      <WorkChart
        ariaLabel="工作结果图表"
        locale="zh-CN"
        model={sparseWorkChartModel}
        series={[
          {
            key: "queued",
            label: chartMessages.seriesLabels.queued,
            ...getDashboardWorkChartSeriesStyle("queued"),
          },
          {
            key: "completed",
            label: chartMessages.seriesLabels.completed,
            ...getDashboardWorkChartSeriesStyle("completed"),
          },
          {
            key: "inFlight",
            label: chartMessages.seriesLabels.inFlight,
            ...getDashboardWorkChartSeriesStyle("inFlight"),
          },
          {
            key: "failed",
            label: chartMessages.seriesLabels.failed,
            ...getDashboardWorkChartSeriesStyle("failed"),
          },
        ]}
      />,
    );

    const chart = screen.getByRole("img", { name: "工作结果图表" });
    const queuedLegendControl = within(chart).getByRole("button", {
      name: chartMessages.hideSeriesLabel(chartMessages.seriesLabels.queued),
    });

    expect(queuedLegendControl.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(queuedLegendControl);
    expect(
      within(chart).getByRole("button", {
        name: chartMessages.showSeriesLabel(chartMessages.seriesLabels.queued),
      }),
    ).toBeTruthy();
  });

  it("keeps legend toggles keyboard focusable with visible focus treatment", async () => {
    const user = userEvent.setup();

    render(
      <WorkChart
        ariaLabel="Work chart legend keyboard"
        model={sparseWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", {
      name: "Work chart legend keyboard",
    });
    const queuedLegendControl = within(chart).getByRole("button", {
      name: "Hide Queued series",
    });

    queuedLegendControl.focus();
    expect(document.activeElement).toBe(queuedLegendControl);
    expect(queuedLegendControl.className).toContain("focus-visible:outline-2");
    expect(queuedLegendControl.className).toContain(
      "focus-visible:outline-af-focus",
    );

    await user.keyboard("[Enter]");
    expect(chart.getAttribute("data-work-chart-hidden-series")).toBe("queued");
    expect(queuedLegendControl.getAttribute("aria-pressed")).toBe("false");
  });

  it("does not render a legend for loading, empty, or error chart states", () => {
    const { rerender } = render(
      <WorkChart
        ariaLabel="Work chart non-ready legend"
        series={OUTCOME_SERIES}
        state={{ status: "loading" }}
      />,
    );

    expect(
      document.querySelector("[data-work-chart-legend='true']"),
    ).toBeNull();

    rerender(
      <WorkChart
        ariaLabel="Work chart non-ready legend"
        emptyMessage="Empty"
        emptyTitle="Empty title"
        model={emptyWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );
    expect(
      document.querySelector("[data-work-chart-legend='true']"),
    ).toBeNull();

    rerender(
      <WorkChart
        ariaLabel="Work chart non-ready legend"
        series={OUTCOME_SERIES}
        state={{ status: "error" }}
      />,
    );
    expect(
      document.querySelector("[data-work-chart-legend='true']"),
    ).toBeNull();
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
    const overlay = chart.querySelector<HTMLElement>(
      "[data-work-chart-overlay='true']",
    );
    expect(overlay).toBeTruthy();
    expect(within(chart).getByText("Ticks")).toBeTruthy();
    expect(within(overlay as HTMLElement).getByText("Work count")).toBeTruthy();
  });

  it("renders visible SVG line paths for a live-session-like event timeline model", () => {
    render(
      <WorkChart
        ariaLabel="Live session work chart"
        model={liveSessionLikeModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Live session work chart" });
    const svg = chart.querySelector("svg");

    expect(svg).toBeTruthy();
    expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
    expect(chart.className).toContain("select-none");
    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "0,1,2,4,6,7,11,12",
    );
    expect(chart.textContent).toContain("Completed");
    expect(chart.textContent).toContain("Failed");

    const renderedSeries = Array.from(
      chart.querySelectorAll<HTMLElement>("[data-chart-series]"),
    );
    expect(
      renderedSeries.map((node) => node.getAttribute("data-chart-series")),
    ).toEqual(["queued", "completed", "inFlight", "failed"]);

    const visibleCurves = Array.from(
      chart.querySelectorAll<SVGPathElement>("path.recharts-curve"),
    ).filter((path) => {
      const d = path.getAttribute("d");
      return typeof d === "string" && d.length > 0;
    });

    expect(visibleCurves.length).toBeGreaterThanOrEqual(4);
    expect(
      visibleCurves.some((path) => path.getAttribute("d")?.includes("L")),
    ).toBe(true);
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

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );

    fireEvent.mouseDown(chart, { clientX: 40, clientY: 168 });
    fireEvent.mouseMove(chart, { clientX: 200, clientY: 168 });
    fireEvent.mouseUp(chart, { clientX: 200, clientY: 168 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");
    expect(screen.queryByText("已缩放到刻度 10-20")).toBeNull();

    const resetZoom = screen.getByRole("button", {
      name: "重置工作结果图表缩放",
    });
    expect(resetZoom).toBeTruthy();
    expect(chart.contains(resetZoom)).toBe(false);

    await user.click(resetZoom);

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
    expect(screen.queryByText("已缩放到刻度 10-20")).toBeNull();
  });

  it("maps drag selection to the rendered plot bounds so the final tick is reachable", () => {
    render(
      <WorkChart
        ariaLabel="Work chart edge zoom"
        model={edgeZoomWorkChartModel}
        series={OUTCOME_SERIES}
      />,
    );

    const chart = screen.getByRole("img", { name: "Work chart edge zoom" });
    const svg = chart.querySelector<SVGSVGElement>("svg");
    const gridLine = chart.querySelector<SVGLineElement>(
      ".recharts-cartesian-grid-horizontal line",
    );

    expect(svg).toBeTruthy();
    expect(gridLine).toBeTruthy();
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
    vi.spyOn(svg as SVGSVGElement, "getBoundingClientRect").mockReturnValue({
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
    Object.defineProperty(svg, "viewBox", {
      configurable: true,
      value: { baseVal: { width: 400 } },
    });
    gridLine?.setAttribute("x1", "70");
    gridLine?.setAttribute("x2", "372");

    fireEvent.mouseDown(chart, { clientX: 221, clientY: 168 });
    fireEvent.mouseMove(chart, { clientX: 372, clientY: 168 });
    fireEvent.mouseUp(chart, { clientX: 372, clientY: 168 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      Array.from({ length: 13 }, (_, index) => String(index + 13)).join(","),
    );
  });

  it("keeps the reset zoom button above the chart interaction surface and keyboard operable", async () => {
    const user = userEvent.setup();
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    try {
      render(
        <WorkChart
          ariaLabel="Work chart keyboard zoom"
          model={sparseWorkChartModel}
          series={OUTCOME_SERIES}
        />,
      );

      const chart = screen.getByRole("img", {
        name: "Work chart keyboard zoom",
      });
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

      expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
        "10,20,40",
      );
      expect(
        screen.queryByRole("button", { name: "Reset work outcome chart zoom" }),
      ).toBeNull();
      expectNoChartSizingWarnings(warnSpy, errorSpy);
    } finally {
      warnSpy.mockRestore();
      errorSpy.mockRestore();
    }
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
    expect(
      screen.queryByRole("img", { name: "Work chart malformed" }),
    ).toBeNull();
  });
});

function workOutcomeEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-06-04T10:28:${String(tick).padStart(2, "0")}.000Z`,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}
