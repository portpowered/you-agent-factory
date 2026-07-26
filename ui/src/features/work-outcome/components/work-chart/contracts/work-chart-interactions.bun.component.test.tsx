import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, expect, it, spyOn } from "bun:test";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { WorkChart } from "../work-chart";
import {
  edgeZoomWorkChartModel,
  OUTCOME_SERIES,
  sparseWorkChartModel,
} from "./work-chart.test.fixtures";

const CHART_SIZING_WARNING_FRAGMENT =
  "The width(-1) and height(-1) of chart should be greater than 0";

function expectNoChartSizingWarnings(
  warnSpy: ReturnType<typeof spyOn>,
  errorSpy: ReturnType<typeof spyOn>,
) {
  for (const call of [...warnSpy.mock.calls, ...errorSpy.mock.calls]) {
    expect(call.join(" ")).not.toContain(CHART_SIZING_WARNING_FRAGMENT);
  }
}

const restoreBrowserShims = installDashboardBrowserTestShims();

afterAll(() => {
  restoreBrowserShims();
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
    spyOn(chart, "getBoundingClientRect").mockReturnValue({
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
    spyOn(chart, "getBoundingClientRect").mockReturnValue({
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
    spyOn(svg as SVGSVGElement, "getBoundingClientRect").mockReturnValue({
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
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {});
    const errorSpy = spyOn(console, "error").mockImplementation(() => {});

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
      spyOn(chart, "getBoundingClientRect").mockReturnValue({
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
