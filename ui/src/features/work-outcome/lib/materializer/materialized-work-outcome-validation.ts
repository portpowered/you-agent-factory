import type { ThroughputSample } from "../trends";
import {
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
} from "./materialized-work-outcome-state";

export function isSupportedMaterializedWorkOutcomeState(
  value: unknown,
): value is MaterializedWorkOutcomeState {
  if (!isRecord(value) || value.version !== MATERIALIZED_WORK_OUTCOME_VERSION) {
    return false;
  }

  return (
    isAccumulator(value.accumulator) &&
    isCounts(value.counts) &&
    isCursor(value.cursor) &&
    isCountRecord(value.failedByWorkType) &&
    isStringArray(value.failedWorkLabels) &&
    isOrderedSampleArray(value.samples)
  );
}

function isAccumulator(value: unknown): boolean {
  return (
    isRecord(value) &&
    isActiveDispatchRecord(value.activeDispatchesByID) &&
    isNonNegativeInteger(value.appliedEventCount) &&
    isNonNegativeInteger(value.completedAcceptedCount) &&
    isNonNegativeInteger(value.completedDispatchCount) &&
    isWorkItemRecord(value.failedWorkItemsByID) &&
    isStringArray(value.initialPlaceIDs) &&
    isWorkItemRecord(value.workItemsByID)
  );
}

function isActiveDispatchRecord(value: unknown): boolean {
  return (
    isRecord(value) &&
    Object.values(value).every(
      (dispatch) =>
        isRecord(dispatch) &&
        isStringArray(dispatch.inputWorkIDs) &&
        typeof dispatch.systemOnly === "boolean",
    )
  );
}

function isWorkItemRecord(value: unknown): boolean {
  return isRecord(value) && Object.values(value).every(isWorkItem);
}

function isWorkItem(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.workTypeID === "string" &&
    isOptionalString(value.displayName) &&
    isOptionalString(value.placeID) &&
    isOptionalString(value.traceID)
  );
}

function isCounts(value: unknown): boolean {
  return (
    isRecord(value) &&
    isNonNegativeInteger(value.completed) &&
    isNonNegativeInteger(value.dispatched) &&
    isNonNegativeInteger(value.failed) &&
    isNonNegativeInteger(value.inFlight) &&
    isNonNegativeInteger(value.queued)
  );
}

function isCursor(value: unknown): boolean {
  return (
    value === null ||
    (isRecord(value) &&
      typeof value.eventID === "string" &&
      typeof value.eventTime === "string" &&
      isNonNegativeInteger(value.sequence) &&
      isNonNegativeInteger(value.tick))
  );
}

function isOrderedSampleArray(value: unknown): value is ThroughputSample[] {
  if (!Array.isArray(value) || !value.every(isSample)) {
    return false;
  }
  return value.every(
    (sample, index) => index === 0 || value[index - 1].tick <= sample.tick,
  );
}

function isSample(value: unknown): value is ThroughputSample {
  return (
    isRecord(value) &&
    isNonNegativeInteger(value.completedCount) &&
    isNonNegativeInteger(value.dispatchedCount) &&
    isCountRecord(value.failedByWorkType) &&
    isNonNegativeInteger(value.failedCount) &&
    isStringArray(value.failedWorkLabels) &&
    isNonNegativeInteger(value.inFlightCount) &&
    isFiniteNumber(value.observedAt) &&
    isNonNegativeInteger(value.queuedCount) &&
    isNonNegativeInteger(value.tick)
  );
}

function isCountRecord(value: unknown): boolean {
  return isRecord(value) && Object.values(value).every(isNonNegativeInteger);
}

function isStringArray(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((item) => typeof item === "string")
  );
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string";
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
