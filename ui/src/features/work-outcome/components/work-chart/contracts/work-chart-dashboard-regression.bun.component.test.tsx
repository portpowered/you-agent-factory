import { afterAll, expect, it } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import { StrictMode } from "react";

import { dashboardRegressionChartStates } from "../../../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import type { WorkChartModel } from "../../../lib/trends";
import { WorkChart } from "../work-chart";
import {
  emptyWorkChartModel,
  OUTCOME_SERIES,
} from "./work-chart.test.fixtures";

const restoreBrowserShims = installDashboardBrowserTestShims();
const dashboardRegressionSuccess = dashboardRegressionChartStates.success;
const dashboardRegressionCompleted =
  dashboardRegressionSuccess.series.completed;
const dashboardRegressionFailed = dashboardRegressionSuccess.series.failed;

const dashboardRegressionPoints = dashboardRegressionCompleted.map(
  (point, index) => ({
    label: `Tick ${point.sequence}`,
    observedAt: point.sequence * 1000,
    order: index,
    tick: point.sequence,
  }),
);

const dashboardRegressionChartModel: WorkChartModel = {
  delta: {
    completed: dashboardRegressionCompleted[1].value,
    failed: dashboardRegressionFailed[1].value,
    inFlight: 0,
    queued: 0,
  },
  failureGroups: [],
  points: dashboardRegressionPoints,
  rangeID: "session",
  rangeLabel: "Dashboard regression fixture",
  samples: dashboardRegressionPoints.map((point, index) => ({
    completedCount: dashboardRegressionCompleted[index].value,
    dispatchedCount: 0,
    failedByWorkType: {},
    failedCount: dashboardRegressionFailed[index].value,
    failedWorkLabels: [],
    inFlightCount: 0,
    observedAt: point.observedAt,
    queuedCount: 0,
    tick: point.tick,
  })),
  series: [
    buildChartSeries("completed", "Completed", dashboardRegressionCompleted),
    buildChartSeries("failed", "Failed", dashboardRegressionFailed),
  ],
};

afterAll(() => {
  restoreBrowserShims();
});

it("keeps one chart treatment and an accessible value description through fixture remounts", () => {
  const { rerender } = render(
    <StrictMode>
      <WorkChart
        ariaLabel={dashboardRegressionSuccess.accessibleDescription}
        key="dashboard-regression-mount-001"
        model={dashboardRegressionChartModel}
        series={OUTCOME_SERIES}
      />
    </StrictMode>,
  );

  expectSingleChartTreatment();

  rerender(
    <StrictMode>
      <WorkChart
        ariaLabel={dashboardRegressionSuccess.accessibleDescription}
        key="dashboard-regression-mount-002"
        model={dashboardRegressionChartModel}
        series={OUTCOME_SERIES}
      />
    </StrictMode>,
  );

  expectSingleChartTreatment();
});

it("keeps each fixture chart state explicit without mounting a misleading plot", () => {
  const { rerender } = render(
    <WorkChart
      ariaLabel={dashboardRegressionChartStates.loading.accessibleDescription}
      series={OUTCOME_SERIES}
      state={{
        message: dashboardRegressionChartStates.loading.accessibleDescription,
        status: "loading",
      }}
    />,
  );

  expect(screen.getByRole("status")).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();

  rerender(
    <WorkChart
      ariaLabel={dashboardRegressionChartStates.empty.accessibleDescription}
      emptyMessage={dashboardRegressionChartStates.empty.accessibleDescription}
      model={emptyWorkChartModel}
      series={OUTCOME_SERIES}
    />,
  );
  expect(screen.getByRole("status")).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();

  rerender(
    <WorkChart
      ariaLabel={dashboardRegressionChartStates.error.accessibleDescription}
      series={OUTCOME_SERIES}
      state={{
        message: dashboardRegressionChartStates.error.accessibleDescription,
        status: "error",
      }}
    />,
  );
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.queryByRole("img")).toBeNull();

  rerender(
    <WorkChart
      ariaLabel={dashboardRegressionSuccess.accessibleDescription}
      model={dashboardRegressionChartModel}
      series={OUTCOME_SERIES}
    />,
  );
  expect(screen.getByRole("img")).toBeTruthy();
});

function expectSingleChartTreatment(): void {
  const chart = screen.getByRole("img", {
    name: dashboardRegressionSuccess.accessibleDescription,
  });

  expect(chart.querySelectorAll(".recharts-wrapper")).toHaveLength(1);
  expect(chart.querySelectorAll("svg")).toHaveLength(1);
  expect(chart.querySelectorAll(".recharts-cartesian-grid")).toHaveLength(1);
  expect(within(chart).getAllByText("Work count")).toHaveLength(1);
  expect(chart.getAttribute("aria-describedby")).toBeTruthy();

  const description = document.getElementById(
    chart.getAttribute("aria-describedby") ?? "",
  );
  expect(description?.getAttribute("data-work-chart-accessible-data")).toBe(
    "true",
  );
  expect(description?.textContent).toContain("Completed: 3");
  expect(description?.textContent).toContain("Failed: 1");
  expect(
    within(chart).getAllByRole("button", { name: / series$/ }),
  ).toHaveLength(2);

  const completedLine = chart.querySelector(
    '.recharts-curve[data-chart-series="completed"]',
  );
  const failedLine = chart.querySelector(
    '.recharts-curve[data-chart-series="failed"]',
  );
  expect(completedLine?.getAttribute("stroke-dasharray")).not.toBe(
    failedLine?.getAttribute("stroke-dasharray"),
  );
}

function buildChartSeries(
  key: "completed" | "failed",
  label: string,
  points: typeof dashboardRegressionCompleted,
): WorkChartModel["series"][number] {
  return {
    key,
    label,
    points: points.map((point, index) => ({
      label: `${label}: ${point.value}`,
      observedAt: dashboardRegressionPoints[index].observedAt,
      order: index,
      value: point.value,
    })),
    unit: "count",
  };
}
