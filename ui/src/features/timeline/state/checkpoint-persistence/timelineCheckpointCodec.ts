import {
  MATERIALIZED_WORK_OUTCOME_RETENTION,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
  retainMaterializedWorkOutcomeState,
} from "../../../work-outcome/public/materializer";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import type { ReplayWorldState } from "../timeline/types";

export const CHECKPOINT_SCHEMA_VERSION_GUARDED = 4;

const MAX_COMPACT_TEXT_CHARS = 512;

export interface PersistedTimelineCheckpoint {
  afterEventId?: string;
  afterSequence?: number;
  materializedWorkOutcomeState: MaterializedWorkOutcomeState;
  replayState: ReplayWorldState;
  selectedTick: number;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
}

export function isSupportedPersistedTimelineCheckpoint(
  checkpoint: unknown,
): checkpoint is PersistedTimelineCheckpoint {
  return (
    isRecord(checkpoint) &&
    isOptionalString(checkpoint.afterEventId) &&
    isOptionalNonNegativeInteger(checkpoint.afterSequence) &&
    isMaterializedWorkOutcomeState(checkpoint.materializedWorkOutcomeState) &&
    isRecord(checkpoint.replayState) &&
    isNonNegativeInteger(checkpoint.selectedTick)
  );
}

export function buildPersistedCheckpoint(
  checkpoint: FactoryTimelineCheckpoint,
): PersistedTimelineCheckpoint | null {
  const materializedWorkOutcomeState = retainMaterializedWorkOutcomeState(
    checkpoint.materializedWorkOutcomeState,
  );
  if (!materializedWorkOutcomeState) {
    return null;
  }
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    materializedWorkOutcomeState,
    replayState: compactReplayState(checkpoint.replayState),
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  };
}

export function hydrateCheckpoint(
  checkpoint: PersistedTimelineCheckpoint,
): FactoryTimelineCheckpoint {
  return structuredClone({
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    materializedWorkOutcomeState: checkpoint.materializedWorkOutcomeState,
    replayState: checkpoint.replayState,
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  });
}

function compactReplayState(state: ReplayWorldState): ReplayWorldState {
  const compacted = structuredClone(state);

  for (const [textBlobID, value] of Object.entries(compacted.textBlobsByID)) {
    compacted.textBlobsByID[textBlobID] = compactText(value);
  }

  return compacted;
}

function compactText(value: string): string {
  if (value.length <= MAX_COMPACT_TEXT_CHARS) {
    return value;
  }
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `${value.slice(0, MAX_COMPACT_TEXT_CHARS)}\n\n[checkpoint truncated ${value.length - MAX_COMPACT_TEXT_CHARS} chars]`;
}

function isMaterializedWorkOutcomeState(
  value: unknown,
): value is MaterializedWorkOutcomeState {
  if (!isRecord(value) || value.version !== MATERIALIZED_WORK_OUTCOME_VERSION) {
    return false;
  }

  return (
    isMaterializedAccumulator(value.accumulator) &&
    isMaterializedCounts(value.counts) &&
    (value.cursor === null || isMaterializedCursor(value.cursor)) &&
    isBoundedNumberRecord(
      value.failedByWorkType,
      MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries,
    ) &&
    isBoundedStringArray(
      value.failedWorkLabels,
      MATERIALIZED_WORK_OUTCOME_RETENTION.labels,
    ) &&
    Array.isArray(value.samples) &&
    value.samples.length <= MATERIALIZED_WORK_OUTCOME_RETENTION.samples &&
    value.samples.every(isThroughputSample)
  );
}

function isMaterializedAccumulator(value: unknown): boolean {
  return (
    isRecord(value) &&
    isBoundedRecord(
      value.activeDispatchesByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
      isMaterializedActiveDispatch,
    ) &&
    isNonNegativeInteger(value.appliedEventCount) &&
    isNonNegativeInteger(value.completedAcceptedCount) &&
    isNonNegativeInteger(value.completedDispatchCount) &&
    isBoundedRecord(
      value.failedWorkItemsByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
      isMaterializedWorkItem,
    ) &&
    isBoundedStringArray(
      value.initialPlaceIDs,
      MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs,
    ) &&
    isBoundedRecord(
      value.workItemsByID,
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries,
      isMaterializedWorkItem,
    )
  );
}

function isMaterializedActiveDispatch(value: unknown): boolean {
  return (
    isRecord(value) &&
    isBoundedStringArray(
      value.inputWorkIDs,
      MATERIALIZED_WORK_OUTCOME_RETENTION.nestedIDs,
    ) &&
    typeof value.systemOnly === "boolean"
  );
}

function isMaterializedWorkItem(value: unknown): boolean {
  return (
    isRecord(value) &&
    isOptionalBoundedString(value.displayName) &&
    isBoundedString(value.id) &&
    isOptionalBoundedString(value.placeID) &&
    isOptionalBoundedString(value.traceID) &&
    isBoundedString(value.workTypeID)
  );
}

function isMaterializedCounts(value: unknown): boolean {
  return (
    isRecord(value) &&
    isNonNegativeInteger(value.completed) &&
    isNonNegativeInteger(value.dispatched) &&
    isNonNegativeInteger(value.failed) &&
    isNonNegativeInteger(value.inFlight) &&
    isNonNegativeInteger(value.queued)
  );
}

function isMaterializedCursor(value: unknown): boolean {
  return (
    isRecord(value) &&
    isBoundedString(value.eventID) &&
    isBoundedString(value.eventTime) &&
    isNonNegativeInteger(value.sequence) &&
    isNonNegativeInteger(value.tick)
  );
}

function isThroughputSample(value: unknown): boolean {
  return (
    isRecord(value) &&
    isNonNegativeInteger(value.completedCount) &&
    isNonNegativeInteger(value.dispatchedCount) &&
    isBoundedNumberRecord(
      value.failedByWorkType,
      MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries,
    ) &&
    isNonNegativeInteger(value.failedCount) &&
    isBoundedStringArray(
      value.failedWorkLabels,
      MATERIALIZED_WORK_OUTCOME_RETENTION.labels,
    ) &&
    isNonNegativeInteger(value.inFlightCount) &&
    isFiniteNumber(value.observedAt) &&
    isNonNegativeInteger(value.queuedCount) &&
    isNonNegativeInteger(value.tick)
  );
}

function isBoundedNumberRecord(value: unknown, limit: number): boolean {
  return isBoundedRecord(value, limit, isNonNegativeInteger);
}

function isBoundedRecord(
  value: unknown,
  limit: number,
  isValueValid: (entry: unknown) => boolean,
): boolean {
  if (!isRecord(value)) {
    return false;
  }
  const entries = Object.entries(value);
  return (
    entries.length <= limit &&
    entries.every(([key, entry]) => isBoundedString(key) && isValueValid(entry))
  );
}

function isBoundedStringArray(value: unknown, limit: number): boolean {
  return (
    Array.isArray(value) &&
    value.length <= limit &&
    value.every(isBoundedString)
  );
}

function isOptionalBoundedString(value: unknown): boolean {
  return value === undefined || isBoundedString(value);
}

function isBoundedString(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length <= MATERIALIZED_WORK_OUTCOME_RETENTION.textChars
  );
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string";
}

function isOptionalNonNegativeInteger(value: unknown): boolean {
  return value === undefined || isNonNegativeInteger(value);
}

function isNonNegativeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) >= 0;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
