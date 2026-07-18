import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";

import { projectFactoryLoad } from "./load.js";
import type {
  FactoryActiveResourceClaimsEvidence,
  FactoryLoadAtTickInput,
  FactoryLoadProjection,
  FactoryWorkStateOccupancyEvidence,
} from "./load-contract.js";
import { canonicalizeFactoryEvents } from "./replay.js";

interface FactoryLoadReplayState {
  activeDispatches: Map<string, FactoryActiveResourceClaimsEvidence>;
  factory?: FactoryDefinition;
  works: Map<string, FactoryWorkStateOccupancyEvidence>;
}

/** Reconstruct Work State counts and resource occupancy at one logical tick. */
export function projectFactoryLoadAtTick(
  input: FactoryLoadAtTickInput,
): FactoryLoadProjection {
  const state: FactoryLoadReplayState = {
    activeDispatches: new Map(),
    works: new Map(),
  };
  for (const event of canonicalizeFactoryEvents(input.events)) {
    if (event.context.tick > input.tick) break;
    applyLoadEvent(state, event);
  }
  return projectFactoryLoad({
    activeDispatches: [...state.activeDispatches.values()],
    factory: state.factory,
    selectedTick: input.tick,
    works: [...state.works.values()],
  });
}

function applyLoadEvent(
  state: FactoryLoadReplayState,
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
    applyWorkRequest(state, payload);
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
  if (event.type === "DISPATCH_RESPONSE") {
    applyDispatchResponse(state, event, payload);
  }
}

function applyWorkRequest(
  state: FactoryLoadReplayState,
  payload: Record<string, unknown> | undefined,
): void {
  for (const value of arrayValue(payload?.works)) {
    const work = workEvidence(value);
    if (!work) continue;
    state.works.set(work.id, withInitialState(work, state.factory));
  }
}

function applyWorkStateChange(
  state: FactoryLoadReplayState,
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
  state: FactoryLoadReplayState,
  event: FactoryEvent,
  payload: Record<string, unknown> | undefined,
): void {
  const dispatchId = event.context.dispatchId;
  const inputIds = arrayValue(payload?.inputs).flatMap((value) => {
    const input = objectRecord(value);
    return typeof input?.workId === "string" && input.workId
      ? [input.workId]
      : [];
  });
  for (const workId of new Set([
    ...(event.context.workIds ?? []),
    ...inputIds,
  ])) {
    state.works.delete(workId);
  }
  if (!dispatchId) {
    state.activeDispatches.set(`incomplete:${event.id}`, {
      id: `incomplete:${event.id}`,
    });
    return;
  }
  const resourceValues = Array.isArray(payload?.resources)
    ? payload.resources
    : undefined;
  const resources = resourceValues
    ? resourceValues.flatMap((value) => {
        const resource = objectRecord(value);
        return typeof resource?.name === "string" && resource.name
          ? [{ resourceName: resource.name }]
          : [];
      })
    : undefined;
  const hasCompleteResourceEvidence =
    resources !== undefined && resources.length === resourceValues?.length;
  state.activeDispatches.set(dispatchId, {
    id: dispatchId,
    ...(hasCompleteResourceEvidence ? { resourceClaims: resources } : {}),
  });
}

function applyDispatchResponse(
  state: FactoryLoadReplayState,
  event: FactoryEvent,
  payload: Record<string, unknown> | undefined,
): void {
  if (event.context.dispatchId) {
    state.activeDispatches.delete(event.context.dispatchId);
  } else {
    state.activeDispatches.set(`incomplete:${event.id}`, {
      id: `incomplete:${event.id}`,
    });
  }
  for (const value of arrayValue(payload?.outputWork)) {
    const work = workEvidence(value);
    if (work) state.works.set(work.id, withInitialState(work, state.factory));
  }
}

function workEvidence(
  value: unknown,
): FactoryWorkStateOccupancyEvidence | undefined {
  const work = objectRecord(value);
  if (typeof work?.workId !== "string" || !work.workId) return undefined;
  const state = objectRecord(work.state);
  return {
    id: work.workId,
    ...(typeof state?.id === "string" ? { stateId: state.id } : {}),
    ...(typeof state?.name === "string" ? { stateName: state.name } : {}),
    ...(typeof work.workTypeName === "string"
      ? { workTypeId: work.workTypeName }
      : {}),
  };
}

function withInitialState(
  work: FactoryWorkStateOccupancyEvidence,
  factory: FactoryDefinition | undefined,
): FactoryWorkStateOccupancyEvidence {
  if (work.stateId || work.stateName || !work.workTypeId) return work;
  const workType = factory?.workTypes?.find(
    (candidate) =>
      candidate.name === work.workTypeId || candidate.id === work.workTypeId,
  );
  const initial = workType?.states.find((state) => state.type === "INITIAL");
  return initial
    ? {
        ...work,
        ...(initial.id?.trim() ? { stateId: initial.id } : {}),
        stateName: initial.name,
      }
    : work;
}

function objectRecord(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}
