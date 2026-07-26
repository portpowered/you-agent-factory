import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import type { WorkChartModel } from "../lib/trends";
import {
  expectWorkChartAxisLabelsVisible,
  expectWorkChartCompactLegendContract,
  expectWorkChartLegendClearOfCardTitle,
} from "../lib/work-chart-legend-contract";
import {
  expectSingleWorkOutcomeCardHeader,
  expectWorkOutcomeCardFillsChartBody,
} from "../lib/work-outcome-card-header-contract";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import { WorkChartCard } from "./d3-information-card";

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

const zoomableTrend: WorkChartModel = {
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
        { label: "Completed: 3", observedAt: 2000, order: 1, value: 3 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [{ label: "Failed: 0", observedAt: 2000, order: 1, value: 0 }],
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

describe("WorkChartCard", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  afterEach(() => {
    cleanup();
  });

  it("keeps the bento header while leaving the chart region titleless", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard
        headerAction={<button type="button">Remove chart</button>}
        model={populatedTrend}
        widgetId="work-outcome-chart"
      />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: messages.chart.cardRegionLabel,
      cardTitle: messages.chart.cardTitle,
      headerActionLabel: "Remove chart",
    });
    expectWorkOutcomeCardFillsChartBody(card, messages.chart.cardRegionLabel);
  });

  it("renders zh-CN chart labels", () => {
    render(<WorkChartCard locale="zh-CN" model={populatedTrend} />);

    const chart = screen.getByRole("img", { name: "15m 的工作结果图表" });
    expect(within(chart).getByText("排队中")).toBeTruthy();
    expect(within(chart).getByText("进行中")).toBeTruthy();
    expect(within(chart).getByText("已完成")).toBeTruthy();
    const overlay = chart.querySelector<HTMLElement>(
      "[data-work-chart-overlay='true']",
    );
    expect(overlay).toBeTruthy();
    expect(within(chart).getByText("刻度")).toBeTruthy();
    expect(within(overlay as HTMLElement).getByText("工作计数")).toBeTruthy();
  });

  it("flattens ready-state chart chrome without duplicating the bento card title", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard model={populatedTrend} widgetId="work-outcome-chart" />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    const chartRegion = within(card).getByLabelText(
      messages.chart.cardRegionLabel,
    );
    const chart = within(chartRegion).getByRole("img", {
      name: messages.chart.ariaLabel(populatedTrend.rangeLabel),
    });

    expect(chart.getAttribute("data-chart-presentation")).toBe("embedded");
    expect(chart.getAttribute("data-work-chart-presentation")).toBe("embedded");
    expect(chart.className).not.toContain("rounded-2xl");
    expect(chart.className).not.toContain("border-outline");
    expect(chart.className).not.toContain("bg-surface-container-low");
    expect(chart.className).toContain("pt-0");
    expect(chart.className).not.toContain("pt-4");
    expect(
      within(chartRegion).queryByRole("heading", {
        level: 3,
        name: messages.chart.cardTitle,
      }),
    ).toBeNull();
    expect(
      within(chartRegion).queryByText(messages.chart.cardTitle),
    ).toBeNull();
  });

  it("renders a shared-chart accessible work outcome visualization from dashboard samples", () => {
    render(
      <WorkChartCard model={populatedTrend} widgetId="work-outcome-chart" />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(
      within(card).queryByRole("combobox", { name: "Time range" }),
    ).toBeNull();
    expect(
      within(card).queryByRole("list", { name: "Work outcome totals" }),
    ).toBeNull();
    expect(within(card).queryByText("Completed in range")).toBeNull();
    const chart = within(card).getByRole("img", {
      name: "Work outcome chart for 15m",
    });
    expect(chart).toBeTruthy();
    expect(card.querySelector(".recharts-wrapper")).toBeTruthy();
    expect(within(chart).getByText("Queued")).toBeTruthy();
    expect(within(chart).getByText("In-flight")).toBeTruthy();
    expect(within(chart).getByText("Completed")).toBeTruthy();
    expect(within(chart).getByText("Failed/retried")).toBeTruthy();
    expect(within(chart).getByText("Ticks")).toBeTruthy();
    expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
    const chartRegion = within(card).getByLabelText(
      "Work outcome chart region",
    );
    expectWorkOutcomeCardFillsChartBody(card, "Work outcome chart region");
    expect(chartRegion.className).toContain("flex");
    expect(chartRegion.className).toContain("flex-1");
    expect(chartRegion.className).toContain("h-full");
    expect(chartRegion.className).toContain("min-h-0");
    expect(chartRegion.className).toContain("px-4");
    expect(chartRegion.className).toContain("sm:px-5");
    expect(chart.className).toContain("h-full");
    expect(chart.className).toContain("w-full");
    expect(chart.className).toContain("min-w-0");
    expect(chart.className).toContain("min-h-[14rem]");
    expect(chart.className).toContain("px-0");
    expect(chart.className).toContain("pb-4");
    expect(chart.className).toContain("pt-0");
    expect(chart.className).not.toContain("px-5");
    expect(chart.className).not.toContain("pt-4");
    expect(chart.style.height).toBe("");
    expect(chart.style.minHeight).toBe("14rem");
    const overlay = chart.querySelector<HTMLElement>(
      "[data-work-chart-overlay='true']",
    );
    expect(overlay).toBeTruthy();
    expect(within(overlay as HTMLElement).getByText("Work count")).toBeTruthy();
    expect(overlay?.className).toContain("px-0");
    expect(overlay?.className).toContain("pb-3");
    expect(overlay?.className).toContain("pt-0");
    expect(overlay?.className).not.toContain("pt-4");
  });

  it("toggles chart lines when the legend controls are clicked", () => {
    render(
      <WorkChartCard model={populatedTrend} widgetId="work-outcome-chart" />,
    );

    const chart = screen.getByRole("img", {
      name: "Work outcome chart for 15m",
    });
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

  it("renders an explicit empty state without a chart when samples are unavailable", () => {
    render(<WorkChartCard model={emptyTrend} />);

    expect(
      screen.queryByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeNull();
    expect(screen.getByText("No work outcome samples")).toBeTruthy();
    expect(
      screen.getByText(
        "Work outcome data appears after the event stream receives work history.",
      ),
    ).toBeTruthy();
    const emptyState = screen.getByRole("status");
    expect(emptyState.getAttribute("data-work-chart-presentation")).toBe(
      "embedded",
    );
    expect(emptyState.className).toContain("h-full");
    expect(emptyState.className).toContain("w-full");
    expect(emptyState.className).toContain("min-w-0");
    expect(emptyState.className).toContain("flex-1");
    expect(emptyState.className).toContain("justify-center");
    expect(emptyState.className).toContain("min-h-[14rem]");
    expect(emptyState.className).not.toContain("border-dashed");
  });

  it("renders an explicit loading state without dropping chart summary controls", () => {
    render(
      <WorkChartCard chartState={{ status: "loading" }} model={emptyTrend} />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(
      within(card).queryByRole("combobox", { name: "Time range" }),
    ).toBeNull();
    const loadingState = within(card).getByRole("status");
    expect(loadingState).toBeTruthy();
    expect(loadingState.getAttribute("data-work-chart-presentation")).toBe(
      "embedded",
    );
    expect(within(card).getByText("Loading work outcome samples")).toBeTruthy();
    expect(loadingState.className).toContain("h-full");
    expect(loadingState.className).toContain("w-full");
    expect(loadingState.className).toContain("min-w-0");
    expect(loadingState.className).toContain("flex-1");
    expect(loadingState.className).toContain("justify-center");
    expect(loadingState.className).toContain("min-h-[14rem]");
    expect(loadingState.className).not.toContain("border-dashed");
    expect(
      within(card).queryByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeNull();
  });

  it("preserves compact legend, axis labels, and legend placement in embedded bento layout", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard model={zoomableTrend} widgetId="work-outcome-chart" />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    const chart = within(card).getByRole("img", {
      name: messages.chart.ariaLabel(zoomableTrend.rangeLabel),
    });

    expect(chart.getAttribute("data-work-chart-presentation")).toBe("embedded");

    const legend = chart.querySelector<HTMLElement>(
      "[data-work-chart-legend='true']",
    );
    const plot = chart.querySelector<HTMLElement>(
      ".recharts-responsive-container",
    );
    expect(legend).toBeTruthy();
    expect(plot).toBeTruthy();
    vi.spyOn(legend as HTMLElement, "getBoundingClientRect").mockReturnValue({
      bottom: 24,
      height: 24,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 0,
      width: 400,
      x: 0,
      y: 0,
    });
    vi.spyOn(plot as HTMLElement, "getBoundingClientRect").mockReturnValue({
      bottom: 200,
      height: 200,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 0,
      width: 400,
      x: 0,
      y: 0,
    });

    expectWorkChartCompactLegendContract(chart);
    expectWorkChartAxisLabelsVisible(chart, {
      xAxisLabel: messages.chart.xAxisLabel,
      yAxisLabel: messages.chart.yAxisLabel,
    });

    const titleHeading = within(card).getByRole("heading", { level: 3 });
    vi.spyOn(titleHeading, "getBoundingClientRect").mockReturnValue({
      bottom: 48,
      height: 24,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 24,
      width: 400,
      x: 0,
      y: 24,
    });
    vi.spyOn(legend as HTMLElement, "getBoundingClientRect").mockReturnValue({
      bottom: 72,
      height: 24,
      left: 0,
      right: 400,
      toJSON: () => ({}),
      top: 56,
      width: 400,
      x: 0,
      y: 56,
    });
    expectWorkChartLegendClearOfCardTitle(card);
  });

  it("preserves drag-to-zoom and keyboard reset outside the chart surface in embedded layout", async () => {
    const user = userEvent.setup();
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard model={zoomableTrend} widgetId="work-outcome-chart" />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    const chart = within(card).getByRole("img", {
      name: messages.chart.ariaLabel(zoomableTrend.rangeLabel),
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

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );

    fireEvent.mouseDown(chart, { clientX: 40, clientY: 168 });
    fireEvent.mouseMove(chart, { clientX: 200, clientY: 168 });
    fireEvent.mouseUp(chart, { clientX: 200, clientY: 168 });

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");

    const resetZoom = screen.getByRole("button", {
      name: messages.chart.resetZoomLabel,
    });
    expect(chart.contains(resetZoom)).toBe(false);

    resetZoom.focus();
    expect(document.activeElement).toBe(resetZoom);
    await user.keyboard("[Enter]");

    expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe(
      "10,20,40",
    );
    expect(
      screen.queryByRole("button", { name: messages.chart.resetZoomLabel }),
    ).toBeNull();
  });

  it("renders an explicit error state with the card landmark preserved", () => {
    render(
      <WorkChartCard chartState={{ status: "error" }} model={emptyTrend} />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    const alert = within(card).getByRole("alert");
    expect(alert).toBeTruthy();
    expect(
      within(alert).getByText("Work outcome chart unavailable"),
    ).toBeTruthy();
    expect(
      within(alert).getByText(
        "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.",
      ),
    ).toBeTruthy();
    expect(
      within(card).queryByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeNull();
    expect(alert.getAttribute("data-work-chart-presentation")).toBe("embedded");
    expect(alert.className).toContain("h-full");
    expect(alert.className).toContain("w-full");
    expect(alert.className).toContain("min-w-0");
    expect(alert.className).toContain("flex-1");
    expect(alert.className).toContain("justify-center");
    expect(alert.className).toContain("min-h-[14rem]");
    expect(alert.className).not.toContain("border-dashed");
  });
});
