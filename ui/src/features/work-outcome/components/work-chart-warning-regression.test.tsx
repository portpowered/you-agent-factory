import { fireEvent, render, screen } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { WorkChart, type WorkChartSeriesDefinition } from "./work-chart";
import type { WorkChartModel } from "../lib/trends";
import { getDashboardWorkChartSeriesStyle } from "../lib/chart-contract";

const restoreBrowserShims = installDashboardBrowserTestShims();

const chartModel: WorkChartModel = {
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
  ],
};

const chartSeries: readonly WorkChartSeriesDefinition[] = [
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
];

const chartSizingWarning = "The width(-1) and height(-1) of chart should be greater than 0";

afterAll(() => {
  restoreBrowserShims();
});

describe("WorkChart warning regression", () => {
  it("zooms and resets without emitting the Recharts sizing warning", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    try {
      render(
        <WorkChart
          ariaLabel="Work chart warning regression"
          model={chartModel}
          series={chartSeries}
        />,
      );

      const chart = screen.getByRole("img", {
        name: "Work chart warning regression",
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

      expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20");

      fireEvent.click(
        screen.getByRole("button", {
          name: "Reset work outcome chart zoom",
        }),
      );

      expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20,40");
      for (const call of [...warnSpy.mock.calls, ...errorSpy.mock.calls]) {
        expect(call.join(" ")).not.toContain(chartSizingWarning);
      }
    } finally {
      warnSpy.mockRestore();
      errorSpy.mockRestore();
    }
  });
});
