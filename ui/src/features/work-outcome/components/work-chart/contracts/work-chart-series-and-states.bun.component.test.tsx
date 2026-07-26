import { render, screen, within } from "@testing-library/react";
import { afterAll, expect, it } from "bun:test";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import type { WorkChartModel } from "../../../lib/trends";
import { WorkChart } from "../work-chart";
import {
  emptyWorkChartModel,
  liveSessionLikeModel,
  OUTCOME_SERIES,
  sparseWorkChartModel,
  zeroValuedFailedSeriesModel,
} from "./work-chart.test.fixtures";

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
