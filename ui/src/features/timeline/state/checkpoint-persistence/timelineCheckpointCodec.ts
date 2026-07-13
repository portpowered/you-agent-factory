import {
  MATERIALIZED_WORK_OUTCOME_VERSION,
  type MaterializedWorkOutcomeState,
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
  checkpoint: PersistedTimelineCheckpoint | null | undefined,
): checkpoint is PersistedTimelineCheckpoint {
  return (
    checkpoint?.replayState !== undefined &&
    checkpoint.materializedWorkOutcomeState?.version ===
      MATERIALIZED_WORK_OUTCOME_VERSION
  );
}

export function buildPersistedCheckpoint(
  checkpoint: FactoryTimelineCheckpoint,
): PersistedTimelineCheckpoint {
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    materializedWorkOutcomeState: structuredClone(
      checkpoint.materializedWorkOutcomeState,
    ),
    replayState: compactReplayState(checkpoint.replayState),
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  };
}

export function hydrateCheckpoint(
  checkpoint: PersistedTimelineCheckpoint,
): FactoryTimelineCheckpoint {
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    materializedWorkOutcomeState: checkpoint.materializedWorkOutcomeState,
    replayState: checkpoint.replayState,
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  };
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
