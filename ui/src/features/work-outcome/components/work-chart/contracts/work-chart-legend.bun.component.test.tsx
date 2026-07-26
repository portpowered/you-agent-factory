import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, expect, it } from "bun:test";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { getDashboardWorkChartSeriesStyle } from "../../../lib/chart-contract";
import { getWorkOutcomeMessages } from "../../../messages/work-outcome";
import { WORK_CHART_MARGIN, WorkChart } from "../work-chart";
import {
  emptyWorkChartModel,
  OUTCOME_SERIES,
  sparseWorkChartModel,
} from "./work-chart.test.fixtures";

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
