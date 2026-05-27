import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import type { WorkChartModel } from "../lib/trends";
import { D3CompletionInformationCard } from "./d3-information-card";

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
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders a shared-chart accessible work outcome visualization from dashboard samples", () => {
    render(
      <D3CompletionInformationCard
        model={populatedTrend}
        widgetId="work-outcome-chart"
      />,
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
    expect(within(chart).getByText("Work count")).toBeTruthy();
    expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
    const chartRegion = within(card).getByLabelText(
      "Work outcome chart region",
    );
    expect(chartRegion.className).toContain("flex");
    expect(chartRegion.className).toContain("flex-1");
    expect(chartRegion.className).toContain("min-h-0");
    expect(chartRegion.className).toContain("px-4");
    expect(chartRegion.className).toContain("sm:px-5");
    expect(chart.className).toContain("h-full");
    expect(chart.className).toContain("w-full");
    expect(chart.className).toContain("min-w-0");
    expect(chart.className).toContain("min-h-[14rem]");
    expect(chart.className).toContain("px-5");
    expect(chart.className).toContain("pb-5");
    expect(chart.className).toContain("pt-4");
    expect(chart.className).toContain("sm:px-6");
    expect(chart.className).toContain("sm:pb-6");
    expect(chart.className).toContain("sm:pt-5");
    expect(chart.style.height).toBe("");
    expect(chart.style.minHeight).toBe("14rem");
    const overlay = chart.querySelector<HTMLElement>(
      "[data-work-chart-overlay='true']",
    );
    expect(overlay).toBeTruthy();
    expect(overlay?.className).toContain("px-5");
    expect(overlay?.className).toContain("pb-4");
    expect(overlay?.className).toContain("pt-4");
    expect(overlay?.className).toContain("sm:px-6");
    expect(overlay?.className).toContain("sm:pb-5");
    expect(overlay?.className).toContain("sm:pt-5");
  });

  it("toggles chart lines when the legend controls are clicked", () => {
    render(
      <D3CompletionInformationCard
        model={populatedTrend}
        widgetId="work-outcome-chart"
      />,
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
    render(<D3CompletionInformationCard model={emptyTrend} />);

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
    expect(emptyState.className).toContain("h-full");
    expect(emptyState.className).toContain("w-full");
    expect(emptyState.className).toContain("min-w-0");
    expect(emptyState.className).toContain("flex-1");
    expect(emptyState.className).toContain("justify-center");
    expect(emptyState.className).toContain("min-h-[14rem]");
  });

  it("renders an explicit loading state without dropping chart summary controls", () => {
    render(
      <D3CompletionInformationCard
        chartState={{ status: "loading" }}
        model={emptyTrend}
      />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(
      within(card).queryByRole("combobox", { name: "Time range" }),
    ).toBeNull();
    const loadingState = within(card).getByRole("status");
    expect(loadingState).toBeTruthy();
    expect(within(card).getByText("Loading work outcome samples")).toBeTruthy();
    expect(loadingState.className).toContain("h-full");
    expect(loadingState.className).toContain("w-full");
    expect(loadingState.className).toContain("min-w-0");
    expect(loadingState.className).toContain("flex-1");
    expect(loadingState.className).toContain("justify-center");
    expect(loadingState.className).toContain("min-h-[14rem]");
    expect(
      within(card).queryByRole("img", { name: "Work outcome chart for 15m" }),
    ).toBeNull();
  });

  it("renders an explicit error state with the card landmark preserved", () => {
    render(
      <D3CompletionInformationCard
        chartState={{ status: "error" }}
        model={emptyTrend}
      />,
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
    expect(alert.className).toContain("h-full");
    expect(alert.className).toContain("w-full");
    expect(alert.className).toContain("min-w-0");
    expect(alert.className).toContain("flex-1");
    expect(alert.className).toContain("justify-center");
    expect(alert.className).toContain("min-h-[14rem]");
  });
});
