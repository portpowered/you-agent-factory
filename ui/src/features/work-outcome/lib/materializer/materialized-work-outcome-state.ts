import type { ThroughputSample } from "../trends";

export const MATERIALIZED_WORK_OUTCOME_VERSION = 1 as const;

/**
 * Materialized timelines retain the latest 512 tick samples. The continuation
 * accumulator is retained independently, so dropping an old sample never
 * changes how the next event is reduced.
 */
export const MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES = 512;

export interface MaterializedWorkOutcomeEventCursor {
  eventID: string;
  eventTime: string;
  sequence: number;
  tick: number;
}

export interface MaterializedWorkOutcomeCounts {
  completed: number;
  dispatched: number;
  failed: number;
  inFlight: number;
  queued: number;
}

export interface MaterializedTimelineWorkItem {
  displayName?: string;
  id: string;
  placeID?: string;
  traceID?: string;
  workTypeID: string;
}

export interface MaterializedActiveDispatch {
  inputWorkIDs: string[];
  systemOnly: boolean;
}

export interface MaterializedWorkOutcomeAccumulator {
  activeDispatchesByID: Record<string, MaterializedActiveDispatch>;
  appliedEventCount: number;
  completedAcceptedCount: number;
  completedDispatchCount: number;
  failedWorkItemsByID: Record<string, MaterializedTimelineWorkItem>;
  initialPlaceIDs: string[];
  workItemsByID: Record<string, MaterializedTimelineWorkItem>;
}

export interface MaterializedWorkOutcomeState {
  accumulator: MaterializedWorkOutcomeAccumulator;
  counts: MaterializedWorkOutcomeCounts;
  cursor: MaterializedWorkOutcomeEventCursor | null;
  failedByWorkType: Record<string, number>;
  failedWorkLabels: string[];
  samples: ThroughputSample[];
  version: typeof MATERIALIZED_WORK_OUTCOME_VERSION;
}

export function createMaterializedWorkOutcomeState(): MaterializedWorkOutcomeState {
  return {
    accumulator: {
      activeDispatchesByID: {},
      appliedEventCount: 0,
      completedAcceptedCount: 0,
      completedDispatchCount: 0,
      failedWorkItemsByID: {},
      initialPlaceIDs: [],
      workItemsByID: {},
    },
    counts: {
      completed: 0,
      dispatched: 0,
      failed: 0,
      inFlight: 0,
      queued: 0,
    },
    cursor: null,
    failedByWorkType: {},
    failedWorkLabels: [],
    samples: [],
    version: MATERIALIZED_WORK_OUTCOME_VERSION,
  };
}

export function selectMaterializedWorkOutcomeSamples(
  state: MaterializedWorkOutcomeState,
  selectedTick: number,
): ThroughputSample[] {
  return state.samples.filter((sample) => sample.tick <= selectedTick);
}
