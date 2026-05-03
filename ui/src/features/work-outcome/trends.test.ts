import { describe, expect, it } from "vitest";

import {
  buildWorkChartModel,
  recordThroughputSample,
  type ThroughputSample,
} from "./trends";
import type {
  DashboardSessionRuntime,
  DashboardSnapshot,
  DashboardTrace,
} from "../../api/dashboard/types";

function session(
  dispatchedCount: number,
  completedCount: number,
  failedCount = 0,
): DashboardSessionRuntime {
  return {
    completed_count: completedCount,
    dispatched_count: dispatchedCount,
    failed_by_work_type: failedCount > 0 ? { task: failedCount } : {},
    failed_count: failedCount,
    failed_work_labels: failedCount > 0 ? ["task failed because review rejected it"] : [],
    has_data: true,
  };
}

function snapshot(
  runtimeSession: DashboardSessionRuntime,
  queuedCount = 0,
  inFlightCount = 0,
  tickCount = 1,
): DashboardSnapshot {
  return {
    factory_state: "RUNNING",
    runtime: {
      in_flight_dispatch_count: inFlightCount,
      place_token_counts: {
        "story:init": queuedCount,
      },
      session: runtimeSession,
    },
    tick_count: tickCount,
    topology: {
      edges: [],
      workstation_node_ids: ["plan"],
      workstation_nodes_by_id: {
        plan: {
          input_places: [
            {
              kind: "work_state",
              place_id: "story:init",
              state_category: "INITIAL",
              state_value: "init",
              type_id: "story",
            },
          ],
          node_id: "plan",
          transition_id: "plan",
          workstation_name: "Plan",
        },
      },
    },
    uptime_seconds: 1,
  };
}

describe("recordThroughputSample", () => {
  it("records changing website-visible session totals", () => {
    const samples = recordThroughputSample([], snapshot(session(2, 1), 2, 1), 1000);
    const nextSamples = recordThroughputSample(samples, snapshot(session(4, 3), 1, 2, 2), 2000);

    expect(nextSamples).toMatchObject([
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedCount: 0,
        inFlightCount: 1,
        observedAt: 1000,
        queuedCount: 2,
        tick: 1,
      },
      {
        completedCount: 3,
        dispatchedCount: 4,
        failedCount: 0,
        inFlightCount: 2,
        observedAt: 2000,
        queuedCount: 1,
        tick: 2,
      },
    ]);
  });

  it("does not append duplicate samples when totals are unchanged", () => {
    const samples = recordThroughputSample([], snapshot(session(2, 1), 2, 1), 1000);
    const nextSamples = recordThroughputSample(samples, snapshot(session(2, 1), 2, 1), 2000);

    expect(nextSamples).toEqual(samples);
  });

  it("keeps unchanged totals when they arrive on a new factory tick", () => {
    const samples = recordThroughputSample([], snapshot(session(2, 1), 2, 1, 1), 1000);
    const nextSamples = recordThroughputSample(samples, snapshot(session(2, 1), 2, 1, 2), 2000);

    expect(nextSamples.map((sample) => sample.tick)).toEqual([1, 2]);
  });
});

describe("buildWorkChartModel", () => {
  const sessionSamples: ThroughputSample[] = [
    {
      completedCount: 1,
      dispatchedCount: 4,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 10_000,
      queuedCount: 1,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 5,
      failedByWorkType: { task: 1 },
      failedCount: 1,
      failedWorkLabels: ["task validation failed"],
      inFlightCount: 1,
      observedAt: 0,
      queuedCount: 3,
      tick: 1,
    },
  ];

  it("maps throughput samples into deterministic work chart series", () => {
    const first = buildWorkChartModel(sessionSamples, "15m", 10_000);
    const second = buildWorkChartModel([...sessionSamples].reverse(), "15m", 10_000);
    const expectedSeriesOrder = ["queued", "inFlight", "completed", "failed"];

    expect(first.series.map((series) => series.key)).toEqual(expectedSeriesOrder);
    expect(first.series[0]?.label).toBe("Queued");
    expect(first.series[0]?.points.map((point) => point.value)).toEqual([3, 1]);
    expect(first.points.map((point) => point.tick)).toEqual([1, 10]);
    expect(first.points.map((point) => point.label)).toEqual(["Tick 1", "Tick 10"]);
    expect(first.series.every((series) => series.unit === "count")).toBe(true);
    expect(first).toEqual(second);
  });
});
