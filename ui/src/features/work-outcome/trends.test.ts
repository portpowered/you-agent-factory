import { describe, expect, it } from "vitest";

import {
  buildFailureTrendModel,
  buildReworkTrendModel,
  buildWorkChartModel,
  buildTimingTrendModel,
  buildThroughputTrendModel,
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

describe("buildThroughputTrendModel", () => {
  const samples: ThroughputSample[] = [
    {
      completedCount: 1,
      dispatchedCount: 4,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 0,
      queuedCount: 2,
      tick: 1,
    },
    {
      completedCount: 3,
      dispatchedCount: 5,
      failedByWorkType: { task: 1 },
      failedCount: 1,
      failedWorkLabels: ["task validation failed"],
      inFlightCount: 2,
      observedAt: 10_000,
      queuedCount: 1,
      tick: 2,
    },
    {
      completedCount: 6,
      dispatchedCount: 8,
      failedByWorkType: { task: 2, story: 1 },
      failedCount: 3,
      failedWorkLabels: ["task validation failed", "story review rejected"],
      inFlightCount: 0,
      observedAt: 20_000,
      queuedCount: 0,
      tick: 3,
    },
  ];

  it("builds combined completed, failed, and dispatched work points for the selected range", () => {
    const trend = buildThroughputTrendModel(samples, "15m", 20_000);

    expect(trend.currentCompleted).toBe(6);
    expect(trend.currentDispatched).toBe(8);
    expect(trend.currentFailed).toBe(3);
    expect(trend.completedDelta).toBe(5);
    expect(trend.failedDelta).toBe(3);
    expect(trend.failureGroups).toEqual([
      { count: 2, label: "Work type: task" },
      { count: 1, label: "Work type: story" },
    ]);
    expect(trend.points.map((point) => point.failedCount)).toEqual([0, 1, 3]);
    expect(trend.points).toHaveLength(3);
    expect(trend.path).toContain("M");
    expect(trend.rangeLabel).toBe("15m");
  });

  it("falls back to the latest sample when the selected window has no data", () => {
    const trend = buildThroughputTrendModel(samples, "5m", 900_000);

    expect(trend.currentCompleted).toBe(6);
    expect(trend.completedDelta).toBe(0);
    expect(trend.points).toHaveLength(1);
  });
});

describe("buildFailureTrendModel", () => {
  const samples: ThroughputSample[] = [
    {
      completedCount: 1,
      dispatchedCount: 4,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 0,
      queuedCount: 1,
      tick: 1,
    },
    {
      completedCount: 2,
      dispatchedCount: 6,
      failedByWorkType: { story: 2, task: 1 },
      failedCount: 3,
      failedWorkLabels: ["review rejected story", "provider timeout"],
      inFlightCount: 0,
      observedAt: 10_000,
      queuedCount: 0,
      tick: 2,
    },
  ];

  it("builds failed work points and operator cause groups from session totals", () => {
    const trend = buildFailureTrendModel(samples, "15m", 10_000);

    expect(trend.currentFailed).toBe(3);
    expect(trend.failureDelta).toBe(3);
    expect(trend.groups).toEqual([
      { count: 2, label: "Work type: story" },
      { count: 1, label: "Work type: task" },
    ]);
    expect(trend.points).toHaveLength(2);
    expect(trend.path).toContain("M");
  });
});

describe("buildReworkTrendModel", () => {
  const trace: DashboardTrace = {
    trace_id: "trace-1",
    work_ids: ["work-1"],
    transition_ids: ["plan", "review", "plan"],
    workstation_sequence: ["Plan", "Review", "Plan"],
    dispatches: [
      {
        dispatch_id: "dispatch-plan-1",
        duration_millis: 1000,
        end_time: "2026-04-08T12:00:01Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-08T12:00:00Z",
        transition_id: "plan",
      },
      {
        dispatch_id: "dispatch-review-1",
        duration_millis: 1000,
        end_time: "2026-04-08T12:00:02Z",
        outcome: "REJECTED",
        start_time: "2026-04-08T12:00:01Z",
        transition_id: "review",
      },
      {
        dispatch_id: "dispatch-plan-2",
        duration_millis: 1000,
        end_time: "2026-04-08T12:00:03Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-08T12:00:02Z",
        transition_id: "plan",
      },
    ],
  };

  it("builds retry and rework points from selected trace outcomes", () => {
    const trend = buildReworkTrendModel(trace);

    expect(trend.currentWorkLabel).toBe("work-1");
    expect(trend.retryOrReworkCount).toBe(1);
    expect(trend.terminalOutcome).toBe("ACCEPTED");
    expect(trend.points.map((point) => point.reworkCount)).toEqual([0, 1, 1]);
    expect(trend.path).toContain("M");
  });
});

describe("buildTimingTrendModel", () => {
  const trace: DashboardTrace = {
    trace_id: "trace-1",
    work_ids: ["work-1"],
    transition_ids: ["plan", "review", "ship"],
    workstation_sequence: ["Plan", "Review", "Ship"],
    dispatches: [
      {
        dispatch_id: "dispatch-plan-1",
        duration_millis: 450,
        end_time: "2026-04-08T12:00:00.450Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-08T12:00:00Z",
        transition_id: "plan",
        workstation_name: "Plan",
      },
      {
        dispatch_id: "dispatch-review-1",
        duration_millis: 192_000,
        end_time: "2026-04-08T12:03:12Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-08T12:00:00Z",
        transition_id: "review",
        workstation_name: "Review",
      },
      {
        dispatch_id: "dispatch-ship-1",
        duration_millis: 3_000,
        end_time: "2026-04-08T12:03:15Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-08T12:03:12Z",
        transition_id: "ship",
        workstation_name: "Ship",
      },
    ],
  };

  it("builds dispatch duration points and timing summaries from the selected trace", () => {
    const trend = buildTimingTrendModel(trace);

    expect(trend.currentWorkLabel).toBe("work-1");
    expect(trend.fastestDurationMillis).toBe(450);
    expect(trend.slowestDurationMillis).toBe(192_000);
    expect(trend.latestDurationMillis).toBe(3_000);
    expect(Math.round(trend.averageDurationMillis)).toBe(65_150);
    expect(trend.points.map((point) => point.durationMillis)).toEqual([450, 192_000, 3_000]);
    expect(trend.path).toContain("M");
  });
});
