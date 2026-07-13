import type { ThroughputSample } from "../trends";
import {
  MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES,
  type MaterializedActiveDispatch,
  type MaterializedTimelineWorkItem,
  type MaterializedWorkOutcomeState,
} from "./materialized-work-outcome-state";

/**
 * Checkpoints are disposable projections, so their detailed history has fixed
 * limits even though aggregate counts continue to describe the full stream.
 * Map-like collections retain the lexicographically first keys, set-like
 * arrays retain sorted unique values, and samples retain the newest window.
 */
export const MATERIALIZED_WORK_OUTCOME_RETENTION = {
  accumulatorMapEntries: 2048,
  breakdownEntries: 128,
  labels: 128,
  nestedIDs: 512,
  samples: MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES,
  textChars: 512,
} as const;

export function retainMaterializedWorkOutcomeState(
  state: MaterializedWorkOutcomeState,
): MaterializedWorkOutcomeState {
  return {
    accumulator: {
      activeDispatchesByID: retainRecord(
        state.accumulator.activeDispatchesByID,
        MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
        retainActiveDispatch,
      ),
      appliedEventCount: state.accumulator.appliedEventCount,
      completedAcceptedCount: state.accumulator.completedAcceptedCount,
      completedDispatchCount: state.accumulator.completedDispatchCount,
      failedWorkItemsByID: retainRecord(
        state.accumulator.failedWorkItemsByID,
        MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
        retainWorkItem,
      ),
      initialPlaceIDs: retainSortedText(
        state.accumulator.initialPlaceIDs,
        MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs,
      ),
      workItemsByID: retainRecord(
        state.accumulator.workItemsByID,
        MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
        retainWorkItem,
      ),
    },
    counts: { ...state.counts },
    cursor: state.cursor
      ? {
          eventID: retainText(state.cursor.eventID),
          eventTime: retainText(state.cursor.eventTime),
          sequence: state.cursor.sequence,
          tick: state.cursor.tick,
        }
      : null,
    failedByWorkType: retainNumberRecord(
      state.failedByWorkType,
      MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries,
    ),
    failedWorkLabels: retainSortedText(
      state.failedWorkLabels,
      MATERIALIZED_WORK_OUTCOME_RETENTION.labels,
    ),
    samples: state.samples
      .slice(-MATERIALIZED_WORK_OUTCOME_RETENTION.samples)
      .map(retainSample),
    version: state.version,
  };
}

function retainActiveDispatch(
  dispatch: MaterializedActiveDispatch,
): MaterializedActiveDispatch {
  return {
    inputWorkIDs: retainSortedText(
      dispatch.inputWorkIDs,
      MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs,
    ),
    systemOnly: dispatch.systemOnly,
  };
}

function retainWorkItem(
  item: MaterializedTimelineWorkItem,
): MaterializedTimelineWorkItem {
  return {
    ...(item.displayName ? { displayName: retainText(item.displayName) } : {}),
    id: retainText(item.id),
    ...(item.placeID ? { placeID: retainText(item.placeID) } : {}),
    ...(item.traceID ? { traceID: retainText(item.traceID) } : {}),
    workTypeID: retainText(item.workTypeID),
  };
}

function retainSample(sample: ThroughputSample): ThroughputSample {
  return {
    completedCount: sample.completedCount,
    dispatchedCount: sample.dispatchedCount,
    failedByWorkType: retainNumberRecord(
      sample.failedByWorkType,
      MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries,
    ),
    failedCount: sample.failedCount,
    failedWorkLabels: retainSortedText(
      sample.failedWorkLabels,
      MATERIALIZED_WORK_OUTCOME_RETENTION.labels,
    ),
    inFlightCount: sample.inFlightCount,
    observedAt: sample.observedAt,
    queuedCount: sample.queuedCount,
    tick: sample.tick,
  };
}

function retainNumberRecord(
  values: Record<string, number>,
  limit: number,
): Record<string, number> {
  return retainRecord(values, limit, (value) => value);
}

function retainRecord<T, R>(
  values: Record<string, T>,
  limit: number,
  retainValue: (value: T) => R,
): Record<string, R> {
  const retained: Record<string, R> = {};
  for (const key of Object.keys(values).sort().slice(0, limit)) {
    retained[retainText(key)] = retainValue(values[key] as T);
  }
  return retained;
}

function retainSortedText(values: string[], limit: number): string[] {
  return [...new Set(values.map(retainText))].sort().slice(0, limit);
}

function retainText(value: string): string {
  return value.slice(0, MATERIALIZED_WORK_OUTCOME_RETENTION.textChars);
}
