import type {
  DashboardSnapshot,
  DashboardTrace,
} from "../../api/dashboard/types";

export type ThroughputRangeID = "5m" | "15m" | "session";

export interface ThroughputRangeOption {
  id: ThroughputRangeID;
  label: string;
  durationMillis: number | null;
}

export interface ThroughputSample {
  completedCount: number;
  dispatchedCount: number;
  failedByWorkType: Record<string, number>;
  failedCount: number;
  failedWorkLabels: string[];
  inFlightCount: number;
  observedAt: number;
  queuedCount: number;
  tick: number;
}

export type WorkChartSeriesKey = "queued" | "inFlight" | "completed" | "failed";
export type WorkChartValueUnit = "count";

export interface WorkChartSample {
  label: string;
  observedAt: number;
  order: number;
  tick: number;
}

export interface WorkChartSeriesPoint {
  label: string;
  observedAt: number;
  order: number;
  value: number;
}

export interface WorkChartSeries {
  key: WorkChartSeriesKey;
  label: string;
  unit: WorkChartValueUnit;
  points: WorkChartSeriesPoint[];
}

export interface WorkChartModel {
  delta: Record<WorkChartSeriesKey, number>;
  failureGroups: FailureCauseGroup[];
  points: WorkChartSample[];
  rangeID: ThroughputRangeID;
  rangeLabel: string;
  samples: ThroughputSample[];
  series: WorkChartSeries[];
}

export interface ThroughputTrendPoint {
  completedCount: number;
  dispatchedCount: number;
  failedCount: number;
  label: string;
  x: number;
  y: number;
}

export interface ThroughputTrendModel {
  completedDelta: number;
  currentCompleted: number;
  currentDispatched: number;
  currentFailed: number;
  failedDelta: number;
  failureGroups: FailureCauseGroup[];
  path: string;
  points: ThroughputTrendPoint[];
  rangeLabel: string;
}

export interface FailureCauseGroup {
  count: number;
  label: string;
}

export interface FailureTrendPoint {
  failedCount: number;
  label: string;
  x: number;
  y: number;
}

export interface FailureTrendModel {
  currentFailed: number;
  failureDelta: number;
  groups: FailureCauseGroup[];
  path: string;
  points: FailureTrendPoint[];
  rangeLabel: string;
}

export interface ReworkTrendPoint {
  dispatchLabel: string;
  reworkCount: number;
  x: number;
  y: number;
}

export interface ReworkTrendModel {
  currentWorkLabel: string;
  path: string;
  points: ReworkTrendPoint[];
  retryOrReworkCount: number;
  terminalOutcome: string;
}

export interface TimingTrendPoint {
  dispatchLabel: string;
  durationMillis: number;
  x: number;
  y: number;
}

export interface TimingTrendModel {
  averageDurationMillis: number;
  currentWorkLabel: string;
  fastestDurationMillis: number;
  latestDurationMillis: number;
  path: string;
  points: TimingTrendPoint[];
  slowestDurationMillis: number;
}

const TREND_WIDTH = 320;
const TREND_HEIGHT = 120;
const TREND_PADDING = 14;
const MAX_RETAINED_SAMPLE_AGE_MILLIS = 60 * 60 * 1000;

export const WORK_CHART_SERIES_DEFINITIONS: readonly Omit<WorkChartSeries, "points">[] = [
  { key: "queued", label: "Queued", unit: "count" },
  { key: "inFlight", label: "In-flight", unit: "count" },
  { key: "completed", label: "Completed", unit: "count" },
  { key: "failed", label: "Failed/retried", unit: "count" },
];

const WORK_CHART_SERIES_VALUE_ACCESSORS: Record<
  WorkChartSeriesKey,
  (sample: ThroughputSample) => number
> = {
  queued: (sample) => sample.queuedCount,
  inFlight: (sample) => sample.inFlightCount,
  completed: (sample) => sample.completedCount,
  failed: (sample) => sample.failedCount,
};
const EMPTY_THROUGHPUT_SAMPLE: ThroughputSample = {
  completedCount: 0,
  dispatchedCount: 0,
  failedByWorkType: {},
  failedCount: 0,
  failedWorkLabels: [],
  inFlightCount: 0,
  observedAt: 0,
  queuedCount: 0,
  tick: 0,
};

export const THROUGHPUT_RANGE_OPTIONS: ThroughputRangeOption[] = [
  { id: "5m", label: "5m", durationMillis: 5 * 60 * 1000 },
  { id: "15m", label: "15m", durationMillis: 15 * 60 * 1000 },
  { id: "session", label: "Session", durationMillis: null },
];

export function recordThroughputSample(
  samples: ThroughputSample[],
  snapshot: DashboardSnapshot,
  observedAt: number,
): ThroughputSample[] {
  const session = snapshot.runtime.session;
  const nextSample: ThroughputSample = {
    completedCount: session.completed_count,
    dispatchedCount: session.dispatched_count,
    failedByWorkType: session.failed_by_work_type ?? {},
    failedCount: session.failed_count,
    failedWorkLabels: session.failed_work_labels ?? [],
    inFlightCount: snapshot.runtime.in_flight_dispatch_count,
    observedAt,
    queuedCount: countQueuedWork(snapshot),
    tick: snapshot.tick_count,
  };
  const lastSample = samples[samples.length - 1];
  const retainedSamples = samples.filter(
    (sample) => observedAt - sample.observedAt <= MAX_RETAINED_SAMPLE_AGE_MILLIS,
  );

  if (
    lastSample &&
    lastSample.completedCount === nextSample.completedCount &&
    lastSample.dispatchedCount === nextSample.dispatchedCount &&
    lastSample.failedCount === nextSample.failedCount &&
    lastSample.inFlightCount === nextSample.inFlightCount &&
    lastSample.queuedCount === nextSample.queuedCount &&
    lastSample.tick === nextSample.tick &&
    areStringRecordsEqual(lastSample.failedByWorkType, nextSample.failedByWorkType) &&
    areStringArraysEqual(lastSample.failedWorkLabels, nextSample.failedWorkLabels)
  ) {
    return retainedSamples.length === 0 ? [nextSample] : retainedSamples;
  }

  return [...retainedSamples, nextSample];
}

export function buildWorkChartModel(
  samples: ThroughputSample[],
  rangeID: ThroughputRangeID,
  now: number,
): WorkChartModel {
  const range = THROUGHPUT_RANGE_OPTIONS.find((option) => option.id === rangeID);
  const visibleSamples = selectVisibleSamples(samples, rangeID, now);
  const chartSamples = visibleSamples
    .map((sample, index) => ({ sample, index }))
    .sort((left, right) => {
      if (left.sample.tick !== right.sample.tick) {
        return left.sample.tick - right.sample.tick;
      }
      if (left.sample.observedAt !== right.sample.observedAt) {
        return left.sample.observedAt - right.sample.observedAt;
      }
      return left.index - right.index;
    });
  const firstSample = chartSamples[0]?.sample;
  const lastSample = chartSamples[chartSamples.length - 1]?.sample;

  const orderedPoints = chartSamples.map(({ sample }, order) => ({
    label: `Tick ${sample.tick}`,
    order,
    observedAt: sample.observedAt,
    tick: sample.tick,
  }));
  const orderedSamples = chartSamples.map(({ sample }) => sample);
  const series = WORK_CHART_SERIES_DEFINITIONS.map((definition) => ({
    ...definition,
    points: orderedPoints.map((point, index) => {
      const value = WORK_CHART_SERIES_VALUE_ACCESSORS[definition.key](
        orderedSamples[index] ?? (chartSamples[0]?.sample ?? EMPTY_THROUGHPUT_SAMPLE),
      );
      return {
        label: `${definition.label}: ${value}`,
        observedAt: point.observedAt,
        order: point.order,
        value,
      };
    }),
  }));

  const latestValues: Record<WorkChartSeriesKey, number> = {
    queued: lastSample?.queuedCount ?? 0,
    inFlight: lastSample?.inFlightCount ?? 0,
    completed: lastSample?.completedCount ?? 0,
    failed: lastSample?.failedCount ?? 0,
  };
  const delta: Record<WorkChartSeriesKey, number> = {
    queued: Math.max(0, latestValues.queued - (firstSample?.queuedCount ?? 0)),
    inFlight: Math.max(0, latestValues.inFlight - (firstSample?.inFlightCount ?? 0)),
    completed: Math.max(0, latestValues.completed - (firstSample?.completedCount ?? 0)),
    failed: Math.max(0, latestValues.failed - (firstSample?.failedCount ?? 0)),
  };

  return {
    delta,
    failureGroups:
      lastSample && hasWorkHistory(lastSample)
        ? buildFailureCauseGroups(lastSample)
        : [],
    points: orderedPoints,
    rangeID,
    rangeLabel: range?.label ?? "Session",
    samples: orderedSamples,
    series,
  };
}

export function buildFailureTrendModel(
  samples: ThroughputSample[],
  rangeID: ThroughputRangeID,
  now: number,
): FailureTrendModel {
  const range = THROUGHPUT_RANGE_OPTIONS.find((option) => option.id === rangeID);
  const chartSamples = selectVisibleSamples(samples, rangeID, now);
  const firstSample = chartSamples[0];
  const lastSample = chartSamples[chartSamples.length - 1];

  if (!firstSample || !lastSample) {
    return {
      currentFailed: 0,
      failureDelta: 0,
      groups: [],
      path: "",
      points: [],
      rangeLabel: range?.label ?? "Session",
    };
  }

  const maxFailed = Math.max(...chartSamples.map((sample) => sample.failedCount), 1);
  const points = buildTrendPoints(chartSamples, (sample, index) => ({
    failedCount: sample.failedCount,
    label: `Sample ${index + 1}: ${sample.failedCount} failed`,
  })).map((point) => ({
    ...point.value,
    x: point.x,
    y:
      TREND_HEIGHT -
      TREND_PADDING -
      (point.value.failedCount / maxFailed) * (TREND_HEIGHT - TREND_PADDING * 2),
  }));

  return {
    currentFailed: lastSample.failedCount,
    failureDelta: Math.max(0, lastSample.failedCount - firstSample.failedCount),
    groups: buildFailureCauseGroups(lastSample),
    path: buildPath(points),
    points,
    rangeLabel: range?.label ?? "Session",
  };
}

export function buildReworkTrendModel(trace?: DashboardTrace): ReworkTrendModel {
  if (!trace || trace.dispatches.length === 0) {
    return {
      currentWorkLabel: "No selected trace",
      path: "",
      points: [],
      retryOrReworkCount: 0,
      terminalOutcome: "Unavailable",
    };
  }

  let retryOrReworkCount = 0;
  const cumulativePoints = trace.dispatches.map((dispatch, index) => {
    if (isReworkDispatch(dispatch.outcome)) {
      retryOrReworkCount += 1;
    }

    return {
      dispatchLabel: dispatch.workstation_name || dispatch.transition_id || dispatch.dispatch_id,
      reworkCount: retryOrReworkCount,
      x:
        trace.dispatches.length === 1
          ? TREND_WIDTH / 2
          : TREND_PADDING +
            (index / Math.max(trace.dispatches.length - 1, 1)) *
              (TREND_WIDTH - TREND_PADDING * 2),
    };
  });
  const maxRework = Math.max(...cumulativePoints.map((point) => point.reworkCount), 1);
  const points = cumulativePoints.map((point) => ({
    ...point,
    y:
      TREND_HEIGHT -
      TREND_PADDING -
      (point.reworkCount / maxRework) * (TREND_HEIGHT - TREND_PADDING * 2),
  }));
  const lastDispatch = trace.dispatches[trace.dispatches.length - 1];

  return {
    currentWorkLabel: trace.work_ids.join(", ") || trace.trace_id || "Selected trace",
    path: buildPath(points),
    points,
    retryOrReworkCount,
    terminalOutcome: lastDispatch?.outcome ?? "Unavailable",
  };
}

export function buildTimingTrendModel(trace?: DashboardTrace): TimingTrendModel {
  if (!trace || trace.dispatches.length === 0) {
    return {
      averageDurationMillis: 0,
      currentWorkLabel: "No selected trace",
      fastestDurationMillis: 0,
      latestDurationMillis: 0,
      path: "",
      points: [],
      slowestDurationMillis: 0,
    };
  }

  const durations = trace.dispatches.map((dispatch) => Math.max(0, dispatch.duration_millis));
  const slowestDurationMillis = Math.max(...durations);
  const fastestDurationMillis = Math.min(...durations);
  const latestDurationMillis = durations[durations.length - 1] ?? 0;
  const averageDurationMillis =
    durations.reduce((total, durationMillis) => total + durationMillis, 0) / durations.length;
  const maxDurationMillis = Math.max(slowestDurationMillis, 1);
  const points = trace.dispatches.map((dispatch, index) => {
    const durationMillis = durations[index] ?? 0;

    return {
      dispatchLabel: dispatch.workstation_name || dispatch.transition_id || dispatch.dispatch_id,
      durationMillis,
      x:
        trace.dispatches.length === 1
          ? TREND_WIDTH / 2
          : TREND_PADDING +
            (index / Math.max(trace.dispatches.length - 1, 1)) *
              (TREND_WIDTH - TREND_PADDING * 2),
      y:
        TREND_HEIGHT -
        TREND_PADDING -
        (durationMillis / maxDurationMillis) * (TREND_HEIGHT - TREND_PADDING * 2),
    };
  });

  return {
    averageDurationMillis,
    currentWorkLabel: trace.work_ids.join(", ") || trace.trace_id || "Selected trace",
    fastestDurationMillis,
    latestDurationMillis,
    path: buildPath(points),
    points,
    slowestDurationMillis,
  };
}

export function buildThroughputTrendModel(
  samples: ThroughputSample[],
  rangeID: ThroughputRangeID,
  now: number,
): ThroughputTrendModel {
  const range = THROUGHPUT_RANGE_OPTIONS.find((option) => option.id === rangeID);
  const workChartModel = buildWorkChartModel(samples, rangeID, now);
  const firstSample = workChartModel.samples[0];
  const lastSample = workChartModel.samples[workChartModel.samples.length - 1];

  if (!firstSample || !lastSample) {
    return {
      completedDelta: 0,
      currentCompleted: 0,
      currentDispatched: 0,
      currentFailed: 0,
      failedDelta: 0,
      failureGroups: [],
      path: "",
      points: [],
      rangeLabel: range?.label ?? "Session",
    };
  }

  const maxValue = Math.max(
    ...workChartModel.samples.map((sample) =>
      Math.max(sample.completedCount, sample.failedCount, sample.dispatchedCount),
    ),
    1,
  );
  const points = buildTrendPoints(workChartModel.samples, (sample, index) => ({
    completedCount: sample.completedCount,
    dispatchedCount: sample.dispatchedCount,
    failedCount: sample.failedCount,
    label: `${workChartModel.points[index]?.label}: ${sample.completedCount} completed, ${sample.failedCount} failed, ${sample.dispatchedCount} dispatched`,
  })).map((point) => {
    const y =
      TREND_HEIGHT -
      TREND_PADDING -
      (point.value.completedCount / maxValue) * (TREND_HEIGHT - TREND_PADDING * 2);

    return {
      ...point.value,
      x: point.x,
      y,
    };
  });

  return {
    completedDelta: workChartModel.delta.completed,
    currentCompleted: lastSample.completedCount,
    currentDispatched: lastSample.dispatchedCount,
    currentFailed: lastSample.failedCount,
    failedDelta: workChartModel.delta.failed,
    failureGroups: buildFailureCauseGroups(lastSample),
    path: buildPath(points),
    points,
    rangeLabel: range?.label ?? "Session",
  };
}

function selectVisibleSamples(
  samples: ThroughputSample[],
  rangeID: ThroughputRangeID,
  now: number,
): ThroughputSample[] {
  const range = THROUGHPUT_RANGE_OPTIONS.find((option) => option.id === rangeID);
  const rangeStart = range?.durationMillis === null ? 0 : now - (range?.durationMillis ?? 0);
  const visibleSamples =
    range?.durationMillis === null
      ? samples
      : samples.filter((sample) => sample.observedAt >= rangeStart);

  return visibleSamples.length > 0 ? visibleSamples : samples.slice(-1);
}

function buildTrendPoints<Value>(
  samples: ThroughputSample[],
  selectValue: (sample: ThroughputSample, index: number) => Value,
): { value: Value; x: number }[] {
  const minObservedAt = samples[0]?.observedAt ?? 0;
  const maxObservedAt = samples[samples.length - 1]?.observedAt ?? minObservedAt;
  const timeSpan = Math.max(maxObservedAt - minObservedAt, 1);

  return samples.map((sample, index) => ({
    value: selectValue(sample, index),
    x:
      samples.length === 1
        ? TREND_WIDTH / 2
        : TREND_PADDING +
          ((sample.observedAt - minObservedAt) / timeSpan) *
            (TREND_WIDTH - TREND_PADDING * 2),
  }));
}

function buildFailureCauseGroups(sample: ThroughputSample): FailureCauseGroup[] {
  const byWorkType = Object.entries(sample.failedByWorkType)
    .filter(([, count]) => count > 0)
    .map(([workType, count]) => ({
      count,
      label: `Work type: ${workType}`,
    }));

  if (byWorkType.length > 0) {
    return byWorkType.sort((left, right) => right.count - left.count);
  }

  return sample.failedWorkLabels.slice(0, 4).map((label) => ({
    count: 1,
    label,
  }));
}

function hasWorkHistory(sample: ThroughputSample): boolean {
  return (
    sample.completedCount > 0 ||
    sample.dispatchedCount > 0 ||
    sample.failedCount > 0 ||
    sample.inFlightCount > 0 ||
    sample.queuedCount > 0 ||
    sample.failedWorkLabels.length > 0 ||
    Object.keys(sample.failedByWorkType).length > 0
  );
}

function countQueuedWork(snapshot: DashboardSnapshot): number {
  const initialPlaceIDs = new Set<string>();

  for (const nodeID of snapshot.topology.workstation_node_ids) {
    const workstation = snapshot.topology.workstation_nodes_by_id[nodeID];
    for (const place of [
      ...(workstation?.input_places ?? []),
      ...(workstation?.output_places ?? []),
    ]) {
      if (place.kind === "work_state" && place.state_category === "INITIAL") {
        initialPlaceIDs.add(place.place_id);
      }
    }
  }

  return [...initialPlaceIDs].reduce(
    (total, placeID) => total + (snapshot.runtime.place_token_counts?.[placeID] ?? 0),
    0,
  );
}

function buildPath(points: { x: number; y: number }[]): string {
  return points
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`)
    .join(" ");
}

function isReworkDispatch(outcome: string): boolean {
  const normalizedOutcome = outcome.toLowerCase();
  return (
    normalizedOutcome.includes("reject") ||
    normalizedOutcome.includes("retry") ||
    normalizedOutcome.includes("rework")
  );
}

function areStringRecordsEqual(
  left: Record<string, number>,
  right: Record<string, number>,
): boolean {
  const leftEntries = Object.entries(left);
  const rightEntries = Object.entries(right);

  return (
    leftEntries.length === rightEntries.length &&
    leftEntries.every(([key, value]) => right[key] === value)
  );
}

function areStringArraysEqual(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => right[index] === value);
}
