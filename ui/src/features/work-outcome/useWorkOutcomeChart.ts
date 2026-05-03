import { useMemo } from "react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
import type {
  DispatchRequestPayload,
  DispatchResponsePayload,
  FactoryDefinition,
  FactoryEvent,
  FactoryWork,
  InitialStructureRequestPayload,
  RunRequestPayload,
} from "../../api/events/index";
import { FACTORY_EVENT_TYPES } from "../../api/events";
import type { WorldState } from "../timeline/state/factoryTimelineStore";
import {
  buildWorkChartModel,
  recordThroughputSample,
  type ThroughputSample,
} from "./trends";

const WORK_OUTCOME_RANGE_ID = "session";
const SESSION_WORK_CHART_NOW = 0;
const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
const SYSTEM_TIME_EXPIRY_TRANSITION_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:expire`;

interface TimelineWorkItem {
  displayName?: string;
  id: string;
  placeID?: string;
  traceID?: string;
  workTypeID: string;
}

interface LegacyTimelineWorkCompat {
  trace_id?: string;
  work_id?: string;
  work_type_name?: string;
}

interface ActiveDispatch {
  inputWorkIDs: string[];
  systemOnly: boolean;
}

interface WorkOutcomeTimelineState {
  activeDispatches: Record<string, ActiveDispatch>;
  completedAcceptedCount: number;
  completedDispatchCount: number;
  failedWorkItemsByID: Record<string, TimelineWorkItem>;
  initialPlaceIDs: Set<string>;
  workItemsByID: Record<string, TimelineWorkItem>;
}

export function useWorkOutcomeChart({
  selectedTimelineTick,
  timelineEvents,
  worldViewCache,
}: {
  selectedTimelineTick: number;
  timelineEvents: FactoryEvent[];
  worldViewCache: Record<number, WorldState | DashboardSnapshot | unknown>;
}) {
  const workOutcomeSamples = useMemo(
    () => {
      if (timelineEvents.length > 0) {
        return buildWorkOutcomeTimelineSamplesFromEvents(
          timelineEvents,
          selectedTimelineTick,
        );
      }
      return buildWorkOutcomeTimelineSamplesFromCachedSnapshots(
        worldViewCache,
        selectedTimelineTick,
      );
    },
    [selectedTimelineTick, timelineEvents, worldViewCache],
  );

  return useMemo(
    () =>
      buildWorkChartModel(
        workOutcomeSamples,
        WORK_OUTCOME_RANGE_ID,
        SESSION_WORK_CHART_NOW,
      ),
    [workOutcomeSamples],
  );
}

export function buildWorkOutcomeTimelineSamplesFromEvents(
  events: FactoryEvent[],
  selectedTick: number,
): ThroughputSample[] {
  const orderedEvents = [...events]
    .filter((event) => event.context.tick <= selectedTick)
    .sort(compareFactoryEvents);

  if (orderedEvents.length === 0) {
    return [];
  }

  const state: WorkOutcomeTimelineState = {
    activeDispatches: {},
    completedAcceptedCount: 0,
    completedDispatchCount: 0,
    failedWorkItemsByID: {},
    initialPlaceIDs: new Set<string>(),
    workItemsByID: {},
  };
  const samples: ThroughputSample[] = [];
  let currentTick = orderedEvents[0]?.context.tick ?? selectedTick;
  let currentObservedAt = observedAtForEvent(orderedEvents[0], 0);

  for (let index = 0; index < orderedEvents.length; index += 1) {
    const event = orderedEvents[index];
    if (event.context.tick !== currentTick) {
      samples.push(snapshotThroughputState(state, currentTick, currentObservedAt));
      currentTick = event.context.tick;
    }
    currentObservedAt = observedAtForEvent(event, index);
    applyTimelineEvent(state, event);
  }

  samples.push(snapshotThroughputState(state, currentTick, currentObservedAt));
  return samples;
}

function compareFactoryEvents(left: FactoryEvent, right: FactoryEvent): number {
  if (left.context.tick !== right.context.tick) {
    return left.context.tick - right.context.tick;
  }
  if (left.context.sequence !== right.context.sequence) {
    return left.context.sequence - right.context.sequence;
  }
  if (left.context.eventTime !== right.context.eventTime) {
    return left.context.eventTime.localeCompare(right.context.eventTime);
  }
  return left.id.localeCompare(right.id);
}

function buildWorkOutcomeTimelineSamplesFromCachedSnapshots(
  worldViewCache: Record<number, WorldState | DashboardSnapshot | unknown>,
  selectedTick: number,
): ThroughputSample[] {
  const ticks = Object.keys(worldViewCache)
    .map((value) => Number(value))
    .filter((tick) => Number.isFinite(tick) && tick <= selectedTick)
    .sort((left, right) => left - right);

  return ticks.reduce<ThroughputSample[]>((samples, tick, index) => {
    const snapshot = worldViewCache[tick] as DashboardSnapshot | undefined;
    if (!snapshot) {
      return samples;
    }
    return recordThroughputSample(samples, snapshot, index);
  }, []);
}

function observedAtForEvent(event: FactoryEvent, fallback: number): number {
  const observedAt = Date.parse(event.context.eventTime);
  return Number.isFinite(observedAt) ? observedAt : fallback;
}

function applyTimelineEvent(
  state: WorkOutcomeTimelineState,
  event: FactoryEvent,
): void {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.initialStructureRequest:
      applyFactoryDefinition(
        state,
        (event.payload as InitialStructureRequestPayload).factory,
      );
      return;
    case FACTORY_EVENT_TYPES.runRequest:
      applyFactoryDefinition(
        state,
        (event.payload as RunRequestPayload).factory,
      );
      return;
    case FACTORY_EVENT_TYPES.workRequest:
      applyWorkRequest(state, event);
      return;
    case FACTORY_EVENT_TYPES.dispatchRequest:
      applyDispatchRequest(state, event);
      return;
    case FACTORY_EVENT_TYPES.dispatchResponse:
      applyDispatchResponse(state, event);
      return;
    default:
      return;
  }
}

function applyFactoryDefinition(
  state: WorkOutcomeTimelineState,
  factory: FactoryDefinition | undefined,
): void {
  if (!factory) {
    return;
  }
  const workTypes =
    factory.workTypes ??
    ((factory as FactoryDefinition & { work_types?: FactoryDefinition["workTypes"] }).work_types ??
      []);
  for (const workType of workTypes) {
    if (workType.name === SYSTEM_TIME_WORK_TYPE_ID) {
      continue;
    }
    for (const workState of workType.states ?? []) {
      if (workState.type === "INITIAL") {
        state.initialPlaceIDs.add(placeID(workType.name, workState.name));
      }
    }
  }
}

function applyWorkRequest(
  state: WorkOutcomeTimelineState,
  event: FactoryEvent,
): void {
  const payload = event.payload as { works?: FactoryWork[] };
  for (const work of payload.works ?? []) {
    const workTypeID = workTypeIDFromWork(work);
    const workID = timelineWorkID(work);
    if (!workTypeID || workTypeID === SYSTEM_TIME_WORK_TYPE_ID || !workID) {
      continue;
    }
    const workItem: TimelineWorkItem = {
      displayName: work.name,
      id: workID,
      placeID: state.initialPlaceIDs.has(placeID(workTypeID, "init"))
        ? placeID(workTypeID, "init")
        : firstMatchingInitialPlaceID(state.initialPlaceIDs, workTypeID),
      traceID: timelineWorkTraceID(work),
      workTypeID,
    };
    state.workItemsByID[workItem.id] = workItem;
  }
}

function applyDispatchRequest(
  state: WorkOutcomeTimelineState,
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
    .map((input) => input.workId ?? (input as { work_id?: string }).work_id)
    .filter((workID): workID is string => typeof workID === "string" && workID.length > 0);

  for (const workID of inputWorkIDs) {
    const workItem = state.workItemsByID[workID];
    if (workItem) {
      workItem.placeID = undefined;
    }
  }

  const publicWorkIDs = inputWorkIDs.filter((workID) => {
    const workItem = state.workItemsByID[workID];
    return workItem && workItem.workTypeID !== SYSTEM_TIME_WORK_TYPE_ID;
  });

  state.activeDispatches[dispatchID] = {
    inputWorkIDs: publicWorkIDs,
    systemOnly:
      payload.transitionId === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
      publicWorkIDs.length === 0,
  };
}

function applyDispatchResponse(
  state: WorkOutcomeTimelineState,
  event: FactoryEvent,
): void {
  const payload = event.payload as DispatchResponsePayload & {
    dispatchId?: string;
  };
  const dispatchID = event.context.dispatchId ?? payload.dispatchId;
  if (!dispatchID) {
    return;
  }

  const activeDispatch = state.activeDispatches[dispatchID];
  delete state.activeDispatches[dispatchID];

  if (!activeDispatch?.systemOnly) {
    state.completedDispatchCount += 1;
    if (payload.outcome === "ACCEPTED") {
      state.completedAcceptedCount += 1;
    }
  }

  const outputWorkItems = (payload.outputWork ?? [])
    .map((work) => timelineWorkItemFromOutputWork(work))
    .filter(
      (item): item is TimelineWorkItem =>
        item !== undefined && item.workTypeID !== SYSTEM_TIME_WORK_TYPE_ID,
    );

  for (const item of outputWorkItems) {
    state.workItemsByID[item.id] = item;
  }

  if (payload.outcome !== "FAILED") {
    return;
  }

  const failedItems =
    outputWorkItems.length > 0
      ? outputWorkItems
      : (activeDispatch?.inputWorkIDs ?? [])
          .map((workID) => state.workItemsByID[workID])
          .filter((item): item is TimelineWorkItem => item !== undefined);

  for (const item of failedItems) {
    state.failedWorkItemsByID[item.id] = item;
  }
}

function snapshotThroughputState(
  state: WorkOutcomeTimelineState,
  tick: number,
  observedAt: number,
): ThroughputSample {
  const failedWorkItems = Object.values(state.failedWorkItemsByID);
  const failedByWorkType = failedWorkItems.reduce<Record<string, number>>(
    (counts, item) => {
      counts[item.workTypeID] = (counts[item.workTypeID] ?? 0) + 1;
      return counts;
    },
    {},
  );

  return {
    completedCount: state.completedAcceptedCount,
    dispatchedCount:
      Object.values(state.activeDispatches).filter((dispatch) => !dispatch.systemOnly)
        .length + state.completedDispatchCount,
    failedByWorkType,
    failedCount: failedWorkItems.length,
    failedWorkLabels: uniqueSorted(
      failedWorkItems.map((item) => item.displayName ?? item.id),
    ),
    inFlightCount: Object.values(state.activeDispatches).filter(
      (dispatch) => !dispatch.systemOnly,
    ).length,
    observedAt,
    queuedCount: Object.values(state.workItemsByID).filter((item) =>
      item.placeID ? state.initialPlaceIDs.has(item.placeID) : false,
    ).length,
    tick,
  };
}

function timelineWorkItemFromOutputWork(
  work: FactoryWork,
): TimelineWorkItem | undefined {
  const workTypeID = workTypeIDFromWork(work);
  const workID = timelineWorkID(work);
  if (!workTypeID || !workID) {
    return undefined;
  }
  const placeIDValue =
    typeof work.state === "string" && work.state.length > 0
      ? placeID(workTypeID, work.state)
      : undefined;

  return {
    displayName: work.name,
    id: workID,
    placeID: placeIDValue,
    traceID: timelineWorkTraceID(work),
    workTypeID,
  };
}

function workTypeIDFromWork(work: FactoryWork): string | undefined {
  const legacyWork = work as FactoryWork & LegacyTimelineWorkCompat;
  return work.workTypeName ?? legacyWork.work_type_name;
}

function timelineWorkID(work: FactoryWork): string | undefined {
  const legacyWork = work as FactoryWork & LegacyTimelineWorkCompat;
  return work.workId ?? legacyWork.work_id;
}

function timelineWorkTraceID(work: FactoryWork): string | undefined {
  const legacyWork = work as FactoryWork & LegacyTimelineWorkCompat;
  return work.traceId ?? legacyWork.trace_id;
}

function placeID(workTypeID: string, workState: string): string {
  return `${workTypeID}:${workState}`;
}

function firstMatchingInitialPlaceID(
  initialPlaceIDs: Set<string>,
  workTypeID: string,
): string | undefined {
  for (const initialPlaceID of initialPlaceIDs) {
    if (initialPlaceID.startsWith(`${workTypeID}:`)) {
      return initialPlaceID;
    }
  }
  return undefined;
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.filter((value) => value.length > 0))].sort();
}


