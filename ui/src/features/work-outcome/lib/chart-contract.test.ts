import {
  getDashboardChartSemanticStyle,
  getDashboardWorkChartSeriesDefinitions,
  type DashboardChartSemanticRole,
} from "./chart-contract";

const EXPECTED_CHART_COLORS: Record<DashboardChartSemanticRole, string> = {
  queued: "var(--color-af-chart-queued)",
  inFlight: "var(--color-af-chart-in-flight)",
  completed: "var(--color-af-chart-completed)",
  failed: "var(--color-af-chart-failed)",
  failureTrend: "var(--color-af-chart-failure-trend)",
  reworkTrend: "var(--color-af-chart-rework-trend)",
  timingTrend: "var(--color-af-chart-timing-trend)",
};

describe("dashboard chart contract", () => {
  it("maps each semantic role onto the canonical dashboard chart token family", () => {
    for (const [role, color] of Object.entries(EXPECTED_CHART_COLORS)) {
      expect(getDashboardChartSemanticStyle(role as DashboardChartSemanticRole).color).toBe(color);
    }
  });

  it("keeps the shared line and point defaults lighter than the previous chart baseline", () => {
    const completedStyle = getDashboardChartSemanticStyle("completed");
    const failureTrendStyle = getDashboardChartSemanticStyle("failureTrend");

    expect(completedStyle.lineClassName).toContain("[stroke-width:2.25]");
    expect(failureTrendStyle.pointClassName).toContain("[stroke-width:1.5]");
    expect(failureTrendStyle.pointRadius).toBe(3.25);
  });

  it("builds work outcome series definitions from the shared semantic contract", () => {
    const seriesDefinitions = getDashboardWorkChartSeriesDefinitions([
      { key: "queued", label: "Queued" },
      { key: "completed", label: "Completed" },
    ]);

    expect(seriesDefinitions).toEqual([
      {
        key: "queued",
        label: "Queued",
        lineClassName: expect.stringContaining("[stroke-width:2.25]"),
        lineColor: "var(--color-af-chart-queued)",
        pointClassName: expect.stringContaining("fill-af-chart-queued"),
        pointRadius: 3.25,
      },
      {
        key: "completed",
        label: "Completed",
        lineClassName: expect.stringContaining("[stroke-width:2.25]"),
        lineColor: "var(--color-af-chart-completed)",
        pointClassName: expect.stringContaining("fill-af-chart-completed"),
        pointRadius: 3.25,
      },
    ]);
  });
});

