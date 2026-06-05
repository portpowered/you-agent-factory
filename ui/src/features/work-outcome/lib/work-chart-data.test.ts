import { describe, expect, it } from "vitest";
import type { WorkChartModel } from "./trends";
import { buildWorkChartData, type WorkChartSeriesDefinition } from "./work-chart-data";

const SERIES: readonly WorkChartSeriesDefinition[] = [
  {
    key: "queued",
    label: "Queued",
    lineClassName: "stroke-[var(--color-queued)]",
    lineColor: "var(--color-queued)",
  },
];

describe("buildWorkChartData", () => {
  it("maps series values by point order instead of the visible row index", () => {
    const model: WorkChartModel = {
      delta: { queued: 3, inFlight: 0, completed: 0, failed: 0 },
      failureGroups: [],
      points: [
        { label: "Tick 340", observedAt: 1000, order: 340, tick: 340 },
        { label: "Tick 342", observedAt: 2000, order: 342, tick: 342 },
        { label: "Tick 343", observedAt: 3000, order: 343, tick: 343 },
      ],
      rangeID: "session",
      rangeLabel: "Session",
      samples: [],
      series: [
        {
          key: "queued",
          label: "Queued",
          unit: "count",
          points: [
            { label: "Queued: 1", observedAt: 1000, order: 340, value: 1 },
            { label: "Queued: 2", observedAt: 2000, order: 342, value: 2 },
            { label: "Queued: 3", observedAt: 3000, order: 343, value: 3 },
          ],
        },
      ],
    };

    const result = buildWorkChartData(model, SERIES);

    expect(result.status).toBe("ready");
    if (result.status !== "ready") {
      return;
    }

    expect(result.data.rows).toEqual([
      { label: "Tick 340", queued: 1, tick: 340 },
      { label: "Tick 342", queued: 2, tick: 342 },
      { label: "Tick 343", queued: 3, tick: 343 },
    ]);
    expect(result.data.series.map((series) => series.key)).toEqual(["queued"]);
  });
});
