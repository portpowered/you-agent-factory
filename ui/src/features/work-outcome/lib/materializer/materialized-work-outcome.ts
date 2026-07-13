import type {
  DispatchRequestPayload,
  DispatchResponsePayload,
  FactoryDefinition,
  FactoryEvent,
  FactoryWork,
  InitialStructureRequestPayload,
  RunRequestPayload,
} from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import type { ThroughputSample } from "../trends";
import {
  compareEventToTimelinePosition,
  orderedFactoryEvents,
} from "./factory-event-ordering";
import {
  MATERIALIZED_WORK_OUTCOME_VERSION,
  MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES,
  type MaterializedTimelineWorkItem,
  type MaterializedWorkOutcomeAccumulator,
  type MaterializedWorkOutcomeCounts,
  type MaterializedWorkOutcomeState,
} from "./materialized-work-outcome-state";

export {
  createMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_VERSION,
  MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES,
  type MaterializedActiveDispatch,
  type MaterializedTimelineWorkItem,
  type MaterializedWorkOutcomeAccumulator,
  type MaterializedWorkOutcomeCounts,
  type MaterializedWorkOutcomeEventCursor,
  type MaterializedWorkOutcomeState,
  selectMaterializedWorkOutcomeSamples,
} from "./materialized-work-outcome-state";

const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
const SYSTEM_TIME_EXPIRY_TRANSITION_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:expire`;

export function applyMaterializedWorkOutcomeEvent(
  state: MaterializedWorkOutcomeState,
  event: FactoryEvent,
): MaterializedWorkOutcomeState {
  if (
    state.cursor &&
    compareEventToTimelinePosition(event, state.cursor) <= 0
  ) {
    return state;
  }

  const accumulator = cloneAccumulator(state.accumulator);
  const fallbackObservedAt = accumulator.appliedEventCount;

  applyTimelineEvent(accumulator, event);
  accumulator.appliedEventCount += 1;

  const { counts, failedByWorkType, failedWorkLabels } =
    materializeOutcome(accumulator);
  const sample = sampleFromMaterializedOutcome(
    event.context.tick,
    observedAtForEvent(event, fallbackObservedAt),
    counts,
    failedByWorkType,
    failedWorkLabels,
  );

  return {
    accumulator,
    counts,
    cursor: {
      eventID: event.id,
      eventTime: event.context.eventTime,
      sequence: event.context.sequence,
      tick: event.context.tick,
    },
    failedByWorkType,
    failedWorkLabels,
    samples: retainSample(state.samples, sample),
    version: MATERIALIZED_WORK_OUTCOME_VERSION,
  };
}

/**
 * Applies only events strictly after the saved cursor. Incoming events are
 * canonically ordered by tick, sequence, event time, and event ID; repeated
 * event IDs and stale events are no-ops.
 */
export function reduceMaterializedWorkOutcomeEvents(
  state: MaterializedWorkOutcomeState,
  events: FactoryEvent[],
): MaterializedWorkOutcomeState {
  const seenEventIDs = new Set<string>();
  let nextState = state;

  for (const event of orderedFactoryEvents(events)) {
    if (seenEventIDs.has(event.id)) {
      continue;
    }
    seenEventIDs.add(event.id);
    nextState = applyMaterializedWorkOutcomeEvent(nextState, event);
  }

  return nextState;
}

function cloneAccumulator(
  accumulator: MaterializedWorkOutcomeAccumulator,
): MaterializedWorkOutcomeAccumulator {
  return {
    activeDispatchesByID: Object.fromEntries(
      Object.entries(accumulator.activeDispatchesByID).map(
        ([dispatchID, dispatch]) => [
          dispatchID,
          { ...dispatch, inputWorkIDs: [...dispatch.inputWorkIDs] },
        ],
      ),
    ),
    appliedEventCount: accumulator.appliedEventCount,
    completedAcceptedCount: accumulator.completedAcceptedCount,
    completedDispatchCount: accumulator.completedDispatchCount,
    failedWorkItemsByID: cloneWorkItems(accumulator.failedWorkItemsByID),
    initialPlaceIDs: [...accumulator.initialPlaceIDs],
    workItemsByID: cloneWorkItems(accumulator.workItemsByID),
  };
}

function cloneWorkItems(
  workItems: Record<string, MaterializedTimelineWorkItem>,
): Record<string, MaterializedTimelineWorkItem> {
  return Object.fromEntries(
    Object.entries(workItems).map(([workID, item]) => [workID, { ...item }]),
  );
}

function applyTimelineEvent(
  accumulator: MaterializedWorkOutcomeAccumulator,
  event: FactoryEvent,
): void {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.initialStructureRequest:
      applyFactoryDefinition(
        accumulator,
        (event.payload as InitialStructureRequestPayload).factory,
      );
      return;
    case FACTORY_EVENT_TYPES.runRequest:
      applyFactoryDefinition(
        accumulator,
        (event.payload as RunRequestPayload).factory,
      );
      return;
    case FACTORY_EVENT_TYPES.workRequest:
      applyWorkRequest(accumulator, event);
      return;
    case FACTORY_EVENT_TYPES.dispatchRequest:
      applyDispatchRequest(accumulator, event);
      return;
    case FACTORY_EVENT_TYPES.dispatchResponse:
      applyDispatchResponse(accumulator, event);
      return;
    default:
      return;
  }
}

function applyFactoryDefinition(
  accumulator: MaterializedWorkOutcomeAccumulator,
  factory: FactoryDefinition | undefined,
): void {
  if (!factory) {
    return;
  }
  const workTypes =
    factory.workTypes ??
    (
      factory as FactoryDefinition & {
        work_types?: FactoryDefinition["workTypes"];
      }
    ).work_types ??
    [];
  const initialPlaceIDs = new Set(accumulator.initialPlaceIDs);

  for (const workType of workTypes) {
    if (workType.name === SYSTEM_TIME_WORK_TYPE_ID) {
      continue;
    }
    for (const workState of workType.states ?? []) {
      if (workState.type === "INITIAL") {
        initialPlaceIDs.add(placeID(workType.name, workState.name));
      }
    }
  }

  accumulator.initialPlaceIDs = [...initialPlaceIDs];
}

function applyWorkRequest(
  accumulator: MaterializedWorkOutcomeAccumulator,
  event: FactoryEvent,
): void {
  const payload = event.payload as { works?: FactoryWork[] };
  for (const work of payload.works ?? []) {
    const workTypeID = workTypeIDFromWork(work);
    const workID = timelineWorkID(work);
    if (!workTypeID || workTypeID === SYSTEM_TIME_WORK_TYPE_ID || !workID) {
      continue;
    }
    accumulator.workItemsByID[workID] = timelineWorkItem({
      displayName: work.name,
      id: workID,
      placeID: firstMatchingInitialPlaceID(
        accumulator.initialPlaceIDs,
        workTypeID,
      ),
      traceID: timelineWorkTraceID(work),
      workTypeID,
    });
  }
}

function applyDispatchRequest(
  accumulator: MaterializedWorkOutcomeAccumulator,
  event: FactoryEvent,
): void {
  const payload = event.payload as DispatchRequestPayload & {
    dispatchId?: string;
  };
  const dispatchID = event.context.dispatchId ?? payload.dispatchId;
  if (!dispatchID) {
    return;
  }
  const inputWorkIDs = (payload.inputs ?? [])
    .map((input) => input.workId)
    .filter(
      (workID): workID is string =>
        typeof workID === "string" && workID.length > 0,
    );

  for (const workID of inputWorkIDs) {
    const workItem = accumulator.workItemsByID[workID];
    if (workItem) {
      delete workItem.placeID;
    }
  }

  const publicWorkIDs = inputWorkIDs.filter(
    (workID) => accumulator.workItemsByID[workID] !== undefined,
  );
  accumulator.activeDispatchesByID[dispatchID] = {
    inputWorkIDs: publicWorkIDs,
    systemOnly:
      payload.transitionId === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
      publicWorkIDs.length === 0,
  };
}

function applyDispatchResponse(
  accumulator: MaterializedWorkOutcomeAccumulator,
  event: FactoryEvent,
): void {
  const payload = event.payload as DispatchResponsePayload & {
    dispatchId?: string;
  };
  const dispatchID = event.context.dispatchId ?? payload.dispatchId;
  if (!dispatchID) {
    return;
  }
  const activeDispatch = accumulator.activeDispatchesByID[dispatchID];
  delete accumulator.activeDispatchesByID[dispatchID];

  if (!activeDispatch?.systemOnly) {
    accumulator.completedDispatchCount += 1;
    if (payload.outcome === "ACCEPTED") {
      accumulator.completedAcceptedCount += 1;
    }
  }

  const outputWorkItems = (payload.outputWork ?? [])
    .map(timelineWorkItemFromOutputWork)
    .filter(
      (item): item is MaterializedTimelineWorkItem =>
        item !== undefined && item.workTypeID !== SYSTEM_TIME_WORK_TYPE_ID,
    );
  for (const item of outputWorkItems) {
    accumulator.workItemsByID[item.id] = item;
  }

  if (payload.outcome !== "FAILED") {
    return;
  }
  const failedItems =
    outputWorkItems.length > 0
      ? outputWorkItems
      : (activeDispatch?.inputWorkIDs ?? [])
          .map((workID) => accumulator.workItemsByID[workID])
          .filter(
            (item): item is MaterializedTimelineWorkItem => item !== undefined,
          );
  for (const item of failedItems) {
    accumulator.failedWorkItemsByID[item.id] = item;
  }
}

function materializeOutcome(accumulator: MaterializedWorkOutcomeAccumulator): {
  counts: MaterializedWorkOutcomeCounts;
  failedByWorkType: Record<string, number>;
  failedWorkLabels: string[];
} {
  const activeDispatches = Object.values(accumulator.activeDispatchesByID);
  const customerActiveDispatchCount = activeDispatches.filter(
    (dispatch) => !dispatch.systemOnly,
  ).length;
  const failedWorkItems = Object.values(accumulator.failedWorkItemsByID);
  const failedByWorkType = Object.fromEntries(
    [...new Set(failedWorkItems.map((item) => item.workTypeID))]
      .sort()
      .map((workTypeID) => [
        workTypeID,
        failedWorkItems.filter((item) => item.workTypeID === workTypeID).length,
      ]),
  );

  return {
    counts: {
      completed: accumulator.completedAcceptedCount,
      dispatched:
        customerActiveDispatchCount + accumulator.completedDispatchCount,
      failed: failedWorkItems.length,
      inFlight: customerActiveDispatchCount,
      queued: Object.values(accumulator.workItemsByID).filter((item) =>
        item.placeID
          ? accumulator.initialPlaceIDs.includes(item.placeID)
          : false,
      ).length,
    },
    failedByWorkType,
    failedWorkLabels: uniqueSorted(
      failedWorkItems.map((item) => item.displayName ?? item.id),
    ),
  };
}

function sampleFromMaterializedOutcome(
  tick: number,
  observedAt: number,
  counts: MaterializedWorkOutcomeCounts,
  failedByWorkType: Record<string, number>,
  failedWorkLabels: string[],
): ThroughputSample {
  return {
    completedCount: counts.completed,
    dispatchedCount: counts.dispatched,
    failedByWorkType: { ...failedByWorkType },
    failedCount: counts.failed,
    failedWorkLabels: [...failedWorkLabels],
    inFlightCount: counts.inFlight,
    observedAt,
    queuedCount: counts.queued,
    tick,
  };
}

function retainSample(
  existingSamples: ThroughputSample[],
  sample: ThroughputSample,
): ThroughputSample[] {
  const samples = existingSamples.map((existingSample) => ({
    ...existingSample,
    failedByWorkType: { ...existingSample.failedByWorkType },
    failedWorkLabels: [...existingSample.failedWorkLabels],
  }));
  if (samples.at(-1)?.tick === sample.tick) {
    samples[samples.length - 1] = sample;
  } else {
    samples.push(sample);
  }
  return samples.slice(-MAX_MATERIALIZED_WORK_OUTCOME_SAMPLES);
}

function observedAtForEvent(event: FactoryEvent, fallback: number): number {
  const observedAt = Date.parse(event.context.eventTime);
  return Number.isFinite(observedAt) ? observedAt : fallback;
}

function timelineWorkItemFromOutputWork(
  work: FactoryWork,
): MaterializedTimelineWorkItem | undefined {
  const workTypeID = workTypeIDFromWork(work);
  const workID = timelineWorkID(work);
  if (!workTypeID || !workID) {
    return undefined;
  }
  const placeIDValue =
    typeof work.state === "object" && work.state?.name
      ? placeID(workTypeID, work.state.name)
      : undefined;
  return timelineWorkItem({
    displayName: work.name,
    id: workID,
    placeID: placeIDValue,
    traceID: timelineWorkTraceID(work),
    workTypeID,
  });
}

function timelineWorkItem({
  displayName,
  id,
  placeID: placeIDValue,
  traceID,
  workTypeID,
}: MaterializedTimelineWorkItem): MaterializedTimelineWorkItem {
  return {
    ...(displayName ? { displayName } : {}),
    id,
    ...(placeIDValue ? { placeID: placeIDValue } : {}),
    ...(traceID ? { traceID } : {}),
    workTypeID,
  };
}

function workTypeIDFromWork(work: FactoryWork): string | undefined {
  return work.workTypeName;
}

function timelineWorkID(work: FactoryWork): string | undefined {
  return work.workId;
}

function timelineWorkTraceID(work: FactoryWork): string | undefined {
  return work.traceId;
}

function placeID(workTypeID: string, workState: string): string {
  return `${workTypeID}:${workState}`;
}

function firstMatchingInitialPlaceID(
  initialPlaceIDs: string[],
  workTypeID: string,
): string | undefined {
  return initialPlaceIDs.find((initialPlaceID) =>
    initialPlaceID.startsWith(`${workTypeID}:`),
  );
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.filter((value) => value.length > 0))].sort();
}
