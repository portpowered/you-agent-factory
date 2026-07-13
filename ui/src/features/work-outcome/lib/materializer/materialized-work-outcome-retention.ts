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
 * Presentation-only collections retain a deterministic subset. Continuation
 * accumulator fields are instead retained exactly or rejected: truncating a
 * work or dispatch identifier can change how a later event is reduced.
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
): MaterializedWorkOutcomeState | null {
  if (!isRetainableContinuationState(state)) {
    return null;
  }

  return {
    accumulator: {
      activeDispatchesByID: retainRecordExactly(
        state.accumulator.activeDispatchesByID,
        cloneActiveDispatch,
      ),
      appliedEventCount: state.accumulator.appliedEventCount,
      completedAcceptedCount: state.accumulator.completedAcceptedCount,
      completedDispatchCount: state.accumulator.completedDispatchCount,
      failedWorkItemsByID: retainRecordExactly(
        state.accumulator.failedWorkItemsByID,
        cloneWorkItem,
      ),
      initialPlaceIDs: [...state.accumulator.initialPlaceIDs],
      workItemsByID: retainRecordExactly(
        state.accumulator.workItemsByID,
        cloneWorkItem,
      ),
    },
    counts: { ...state.counts },
    cursor: state.cursor
      ? {
          eventID: state.cursor.eventID,
          eventTime: state.cursor.eventTime,
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

function cloneActiveDispatch(
  dispatch: MaterializedActiveDispatch,
): MaterializedActiveDispatch {
  return {
    inputWorkIDs: [...dispatch.inputWorkIDs],
    systemOnly: dispatch.systemOnly,
  };
}

function cloneWorkItem(
  item: MaterializedTimelineWorkItem,
): MaterializedTimelineWorkItem {
  return {
    ...(item.displayName !== undefined
      ? { displayName: item.displayName }
      : {}),
    id: item.id,
    ...(item.placeID !== undefined ? { placeID: item.placeID } : {}),
    ...(item.traceID !== undefined ? { traceID: item.traceID } : {}),
    workTypeID: item.workTypeID,
  };
}

function isRetainableContinuationState(
  state: MaterializedWorkOutcomeState,
): boolean {
  const { accumulator } = state;
  return (
    (state.cursor === null ||
      (isBoundedText(state.cursor.eventID) &&
        isBoundedText(state.cursor.eventTime))) &&
    hasAtMostEntries(
      accumulator.activeDispatchesByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
    ) &&
    Object.entries(accumulator.activeDispatchesByID).every(
      ([dispatchID, dispatch]) =>
        isBoundedText(dispatchID) &&
        dispatch.inputWorkIDs.length <=
          MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs &&
        dispatch.inputWorkIDs.every(isBoundedText),
    ) &&
    hasAtMostEntries(
      accumulator.failedWorkItemsByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
    ) &&
    Object.entries(accumulator.failedWorkItemsByID).every(
      ([workID, item]) => isBoundedText(workID) && isBoundedWorkItem(item),
    ) &&
    accumulator.initialPlaceIDs.length <=
      MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs &&
    accumulator.initialPlaceIDs.every(isBoundedText) &&
    hasAtMostEntries(
      accumulator.workItemsByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
    ) &&
    Object.entries(accumulator.workItemsByID).every(
      ([workID, item]) => isBoundedText(workID) && isBoundedWorkItem(item),
    )
  );
}

function hasAtMostEntries<T>(
  values: Record<string, T>,
  limit: number,
): boolean {
  return Object.keys(values).length <= limit;
}

function isBoundedWorkItem(item: MaterializedTimelineWorkItem): boolean {
  return (
    isBoundedText(item.id) &&
    isBoundedText(item.workTypeID) &&
    (item.displayName === undefined || isBoundedText(item.displayName)) &&
    (item.placeID === undefined || isBoundedText(item.placeID)) &&
    (item.traceID === undefined || isBoundedText(item.traceID))
  );
}

function isBoundedText(value: string): boolean {
  return value.length <= MATERIALIZED_WORK_OUTCOME_RETENTION.textChars;
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

function retainRecordExactly<T, R>(
  values: Record<string, T>,
  retainValue: (value: T) => R,
): Record<string, R> {
  return Object.fromEntries(
    Object.keys(values)
      .sort()
      .map((key) => [key, retainValue(values[key] as T)]),
  );
}

function retainSortedText(values: string[], limit: number): string[] {
  return [...new Set(values.map(retainText))].sort().slice(0, limit);
}

function retainText(value: string): string {
  return value.slice(0, MATERIALIZED_WORK_OUTCOME_RETENTION.textChars);
}
