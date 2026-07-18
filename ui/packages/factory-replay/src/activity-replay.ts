import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { projectFactoryActivity } from "./activity.js";
import type {
  FactoryActiveDispatchEvidence,
  FactoryActivityAtTickInput,
  FactoryActivityProjection,
  FactoryDispatchRouteEvidence,
} from "./activity-contract.js";
import { canonicalizeFactoryEvents } from "./replay.js";

interface WorkRouteEvidence extends FactoryDispatchRouteEvidence {
  id: string;
}

interface ActivityReplayState {
  activeDispatches: Map<string, FactoryActiveDispatchEvidence>;
  factory?: FactoryDefinition;
  works: Map<string, WorkRouteEvidence>;
}

/** Reconstruct active Dispatch overlays after all canonical events at one tick. */
export function projectFactoryActivityAtTick(
  input: FactoryActivityAtTickInput,
): FactoryActivityProjection {
  const state: ActivityReplayState = {
    activeDispatches: new Map(),
    works: new Map(),
  };
  for (const event of canonicalizeFactoryEvents(input.events)) {
    if (event.context.tick > input.tick) break;
    applyActivityEvent(state, event);
  }
  return projectFactoryActivity({
    activeDispatches: [...state.activeDispatches.values()],
    factory: state.factory,
    selectedTick: input.tick,
  });
}

function applyActivityEvent(
  state: ActivityReplayState,
  event: FactoryEvent,
): void {
  const payload = objectRecord(event.payload);
  if (
    event.type === "INITIAL_STRUCTURE_REQUEST" ||
    event.type === "FACTORY_CHANGE"
  ) {
    const factory = objectRecord(payload?.factory);
    if (factory) state.factory = structuredClone(factory) as FactoryDefinition;
    return;
  }
  if (event.type === "WORK_REQUEST") {
    for (const value of arrayValue(payload?.works)) {
      const work = workRouteEvidence(value, state.factory);
      if (work) state.works.set(work.id, work);
    }
    return;
  }
  if (event.type === "WORK_STATE_CHANGE") {
    applyWorkStateChange(state, payload);
    return;
  }
  if (event.type === "DISPATCH_REQUEST") {
    applyDispatchRequest(state, event, payload);
    return;
  }
  if (
    event.type === "DISPATCH_RESPONSE" ||
    event.type === "DISPATCH_INTERRUPTED"
  ) {
    if (event.context.dispatchId) {
      state.activeDispatches.delete(event.context.dispatchId);
    }
    return;
  }
  if (event.type === "DISPATCH_RECONCILED") {
    const status = payload?.reconciledStatus;
    if (
      event.context.dispatchId &&
      (status === "COMPLETED" ||
        status === "FAILED" ||
        status === "INTERRUPTED")
    ) {
      state.activeDispatches.delete(event.context.dispatchId);
    }
  }
}

function applyWorkStateChange(
  state: ActivityReplayState,
  payload: Record<string, unknown> | undefined,
): void {
  if (typeof payload?.workId !== "string" || !payload.workId) return;
  const previous = state.works.get(payload.workId);
  state.works.set(payload.workId, {
    id: payload.workId,
    ...(typeof payload.toState === "string"
      ? { stateName: payload.toState }
      : {}),
    ...(typeof payload.workTypeName === "string"
      ? { workTypeId: payload.workTypeName }
      : previous?.workTypeId
        ? { workTypeId: previous.workTypeId }
        : {}),
  });
}

function applyDispatchRequest(
  state: ActivityReplayState,
  event: FactoryEvent,
  payload: Record<string, unknown> | undefined,
): void {
  const transitionId =
    typeof payload?.transitionId === "string"
      ? payload.transitionId
      : undefined;
  if (transitionId?.startsWith("__system_time:")) return;
  const inputValues = Array.isArray(payload?.inputs)
    ? payload.inputs
    : undefined;
  const inputIds = (inputValues ?? []).flatMap((value) => {
    const input = objectRecord(value);
    return typeof input?.workId === "string" && input.workId
      ? [input.workId]
      : [];
  });
  const hasCompleteInputEvidence =
    inputValues !== undefined && inputIds.length === inputValues.length;
  const hasWorkEvidence =
    event.context.workIds !== undefined || hasCompleteInputEvidence;
  const workIds = hasWorkEvidence
    ? [...new Set([...(event.context.workIds ?? []), ...inputIds])]
    : undefined;
  const resourceValues = Array.isArray(payload?.resources)
    ? payload.resources
    : undefined;
  const resourceNames = resourceValues?.flatMap((value) => {
    const resource = objectRecord(value);
    return typeof resource?.name === "string" && resource.name
      ? [resource.name]
      : [];
  });
  const completeResources =
    resourceNames !== undefined &&
    resourceNames.length === resourceValues?.length;
  const inputRoutes = workIds?.flatMap((workId) => {
    const route = state.works.get(workId);
    return route ? [withoutId(route)] : [];
  });
  const completeRoutes =
    inputRoutes !== undefined && inputRoutes.length === workIds?.length;
  const dispatchId = event.context.dispatchId ?? `incomplete:${event.id}`;
  state.activeDispatches.set(dispatchId, {
    id: dispatchId,
    ...(completeRoutes ? { inputRoutes } : {}),
    ...(completeResources ? { resourceNames } : {}),
    startedTick: event.context.tick,
    ...(transitionId ? { transitionId } : {}),
    ...(workIds ? { workIds } : {}),
  });
}

function workRouteEvidence(
  value: unknown,
  factory: FactoryDefinition | undefined,
): WorkRouteEvidence | undefined {
  const work = objectRecord(value);
  if (typeof work?.workId !== "string" || !work.workId) return undefined;
  const state = objectRecord(work.state);
  const evidence: WorkRouteEvidence = {
    id: work.workId,
    ...(typeof state?.id === "string" ? { stateId: state.id } : {}),
    ...(typeof state?.name === "string" ? { stateName: state.name } : {}),
    ...(typeof work.workTypeName === "string"
      ? { workTypeId: work.workTypeName }
      : {}),
  };
  if (evidence.stateId || evidence.stateName || !evidence.workTypeId) {
    return evidence;
  }
  const workType = factory?.workTypes?.find(
    (candidate) =>
      candidate.name === evidence.workTypeId ||
      candidate.id === evidence.workTypeId,
  );
  const initial = workType?.states.find(
    (candidate) => candidate.type === "INITIAL",
  );
  return initial
    ? {
        ...evidence,
        ...(initial.id?.trim() ? { stateId: initial.id } : {}),
        stateName: initial.name,
      }
    : evidence;
}

function withoutId(work: WorkRouteEvidence): FactoryDispatchRouteEvidence {
  return {
    ...(work.stateId ? { stateId: work.stateId } : {}),
    ...(work.stateName ? { stateName: work.stateName } : {}),
    ...(work.workTypeId ? { workTypeId: work.workTypeId } : {}),
  };
}

function objectRecord(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}
