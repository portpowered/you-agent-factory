import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";

import { canonicalizeFactoryEvents } from "./replay.js";

const SYSTEM_WORK_TYPE_ID = "__system_time";

export type FactoryWorkStateCategory =
  | "FAILED"
  | "INITIAL"
  | "PROCESSING"
  | "TERMINAL";

export type FactoryWorkProgressCategory =
  | "failed"
  | "completed"
  | "active"
  | "queued"
  | "unclassified";

export interface FactoryWorkProgressStateEvidence {
  category?: FactoryWorkStateCategory;
  id?: string;
  name?: string;
}

export interface FactoryWorkProgressEvidence {
  id: string;
  state?: FactoryWorkProgressStateEvidence;
  workTypeId?: string;
}

export interface FactoryWorkProgressItem {
  id: string;
  stateId?: string;
  stateName?: string;
  workTypeId?: string;
}

export type FactoryWorkProgressCounts = Record<
  FactoryWorkProgressCategory,
  number
>;

export interface FactoryWorkProgressProjection {
  active: FactoryWorkProgressItem[];
  completed: FactoryWorkProgressItem[];
  counts: FactoryWorkProgressCounts;
  failed: FactoryWorkProgressItem[];
  queued: FactoryWorkProgressItem[];
  selectedTick: number;
  total: number;
  unclassified: FactoryWorkProgressItem[];
}

export interface FactoryWorkProgressProjectionInput {
  activeWorkIds: readonly string[];
  factory?: FactoryDefinition;
  selectedTick: number;
  works: readonly FactoryWorkProgressEvidence[];
}

export interface FactoryWorkProgressAtTickInput {
  events: readonly FactoryEvent[];
  tick: number;
}

interface ProgressReplayState {
  activeDispatches: Map<string, string[]>;
  factory?: FactoryDefinition;
  initialWorkIds: Set<string>;
  works: Map<string, FactoryWorkProgressEvidence>;
}

interface IndexedWorkState {
  category: FactoryWorkStateCategory;
  id?: string;
  name: string;
}

/** Partition customer Work into one mutually exclusive progress category. */
export function projectFactoryWorkProgress(
  input: FactoryWorkProgressProjectionInput,
): FactoryWorkProgressProjection {
  const statesByWorkType = indexWorkStates(input.factory);
  const activeWorkIds = new Set(input.activeWorkIds);
  const worksById = new Map<string, FactoryWorkProgressEvidence>();
  for (const evidence of input.works) {
    if (!evidence.id || evidence.workTypeId === SYSTEM_WORK_TYPE_ID) continue;
    worksById.set(evidence.id, structuredClone(evidence));
  }

  const projection: FactoryWorkProgressProjection = {
    active: [],
    completed: [],
    counts: { active: 0, completed: 0, failed: 0, queued: 0, unclassified: 0 },
    failed: [],
    queued: [],
    selectedTick: input.selectedTick,
    total: worksById.size,
    unclassified: [],
  };
  const works = [...worksById.values()].sort((left, right) =>
    left.id.localeCompare(right.id),
  );
  for (const evidence of works) {
    const state = resolveState(evidence, statesByWorkType);
    const item: FactoryWorkProgressItem = {
      id: evidence.id,
      ...(evidence.workTypeId ? { workTypeId: evidence.workTypeId } : {}),
      ...(state?.id ? { stateId: state.id } : {}),
      ...(state?.name ? { stateName: state.name } : {}),
    };
    const category = classifyWork(
      state?.category,
      activeWorkIds.has(evidence.id),
    );
    projection[category].push(item);
    projection.counts[category] += 1;
  }
  return projection;
}

/** Reconstruct and exclusively classify customer Work at one logical tick. */
export function projectFactoryWorkProgressAtTick(
  input: FactoryWorkProgressAtTickInput,
): FactoryWorkProgressProjection {
  const state: ProgressReplayState = {
    activeDispatches: new Map(),
    initialWorkIds: new Set(),
    works: new Map(),
  };
  const events = canonicalizeFactoryEvents(input.events).filter(
    (event) => event.context.tick <= input.tick,
  );
  for (const event of events) applyProgressEvent(state, event);

  return projectFactoryWorkProgress({
    activeWorkIds: [...state.activeDispatches.values()].flat(),
    factory: state.factory,
    selectedTick: input.tick,
    works: [...state.works.values()].map((work) => {
      const initialState = state.initialWorkIds.has(work.id)
        ? initialStateForWork(state.factory, work.workTypeId)
        : undefined;
      return initialState ? { ...work, state: initialState } : work;
    }),
  });
}

function applyProgressEvent(
  state: ProgressReplayState,
  event: FactoryEvent,
): void {
  const payload = objectRecord(event.payload);
  if (
    event.type === "INITIAL_STRUCTURE_REQUEST" ||
    event.type === "FACTORY_CHANGE"
  ) {
    if (objectRecord(payload?.factory)) {
      state.factory = structuredClone(payload?.factory) as FactoryDefinition;
    }
    return;
  }
  if (event.type === "WORK_REQUEST") {
    for (const work of arrayValue(payload?.works)) {
      mergeEventWork(state.works, work);
      const record = objectRecord(work);
      if (typeof record?.workId === "string") {
        if (objectRecord(record.state)) {
          state.initialWorkIds.delete(record.workId);
        } else {
          state.initialWorkIds.add(record.workId);
        }
      }
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
  if (event.type === "DISPATCH_RESPONSE") {
    applyDispatchResponse(state, event, payload);
    return;
  }
  if (event.type === "DISPATCH_INTERRUPTED") {
    endDispatch(state, event);
    return;
  }
  if (event.type === "DISPATCH_RECONCILED") {
    const status = payload?.reconciledStatus;
    if (
      status === "COMPLETED" ||
      status === "FAILED" ||
      status === "INTERRUPTED"
    ) {
      endDispatch(state, event);
    }
  }
}

function applyWorkStateChange(
  state: ProgressReplayState,
  payload: Record<string, unknown> | undefined,
): void {
  if (typeof payload?.workId !== "string" || !payload.workId) return;
  const previous = state.works.get(payload.workId) ?? { id: payload.workId };
  state.initialWorkIds.delete(payload.workId);
  state.works.set(payload.workId, {
    ...previous,
    ...(typeof payload.workTypeName === "string"
      ? { workTypeId: payload.workTypeName }
      : {}),
    ...(typeof payload.toState === "string"
      ? { state: { name: payload.toState } }
      : {}),
  });
}

function applyDispatchRequest(
  state: ProgressReplayState,
  event: FactoryEvent,
  payload: Record<string, unknown> | undefined,
): void {
  const dispatchId = event.context.dispatchId;
  const transitionId = payload?.transitionId;
  if (
    typeof transitionId === "string" &&
    transitionId.startsWith("__system_time:")
  ) {
    return;
  }
  const inputIds = arrayValue(payload?.inputs).flatMap((input) => {
    const record = objectRecord(input);
    return typeof record?.workId === "string" && record.workId
      ? [record.workId]
      : [];
  });
  const workIds = [
    ...new Set([...(event.context.workIds ?? []), ...inputIds].filter(Boolean)),
  ];
  for (const workId of workIds) {
    if (!state.works.has(workId)) state.works.set(workId, { id: workId });
  }
  if (dispatchId) state.activeDispatches.set(dispatchId, workIds);
}

function applyDispatchResponse(
  state: ProgressReplayState,
  event: FactoryEvent,
  payload: Record<string, unknown> | undefined,
): void {
  const dispatchId = event.context.dispatchId;
  const consumedIds = dispatchId
    ? (state.activeDispatches.get(dispatchId) ?? [])
    : [];
  if (dispatchId) state.activeDispatches.delete(dispatchId);

  const transitionId = payload?.transitionId;
  if (
    typeof transitionId !== "string" ||
    !transitionId.startsWith("__system_time:")
  ) {
    for (const workId of event.context.workIds ?? []) {
      if (workId && !state.works.has(workId)) {
        state.works.set(workId, { id: workId });
      }
    }
  }

  const outputWorks = arrayValue(payload?.outputWork);
  const outputIds = new Set(
    outputWorks.flatMap((work) => {
      const record = objectRecord(work);
      return typeof record?.workId === "string" ? [record.workId] : [];
    }),
  );
  for (const workId of consumedIds) {
    const previous = state.works.get(workId);
    if (!previous || outputIds.has(workId)) continue;
    state.initialWorkIds.delete(workId);
    if (
      previous.state?.category !== "FAILED" &&
      previous.state?.category !== "TERMINAL"
    ) {
      state.works.set(workId, {
        id: previous.id,
        ...(previous.workTypeId ? { workTypeId: previous.workTypeId } : {}),
      });
    }
  }
  for (const work of outputWorks) {
    mergeEventWork(state.works, work);
    const record = objectRecord(work);
    if (typeof record?.workId === "string") {
      state.initialWorkIds.delete(record.workId);
    }
  }
}

function endDispatch(state: ProgressReplayState, event: FactoryEvent): void {
  if (event.context.dispatchId) {
    state.activeDispatches.delete(event.context.dispatchId);
  }
}

function indexWorkStates(
  factory: FactoryDefinition | undefined,
): Map<string, Map<string, IndexedWorkState>> {
  const statesByWorkType = new Map<string, Map<string, IndexedWorkState>>();
  for (const workType of factory?.workTypes ?? []) {
    const states = new Map<string, IndexedWorkState>();
    for (const state of workType.states) {
      const indexed = { ...state, category: state.type };
      states.set(state.name, indexed);
      if (state.id?.trim()) states.set(state.id, indexed);
    }
    statesByWorkType.set(workType.name, states);
    if (workType.id?.trim()) statesByWorkType.set(workType.id, states);
  }
  return statesByWorkType;
}

function resolveState(
  evidence: FactoryWorkProgressEvidence,
  statesByWorkType: Map<string, Map<string, IndexedWorkState>>,
): FactoryWorkProgressStateEvidence | undefined {
  if (evidence.state?.category || !evidence.workTypeId) return evidence.state;
  const states = statesByWorkType.get(evidence.workTypeId);
  const indexed = evidence.state?.name
    ? states?.get(evidence.state.name)
    : evidence.state?.id
      ? states?.get(evidence.state.id)
      : undefined;
  return indexed ?? evidence.state;
}

function initialStateForWork(
  factory: FactoryDefinition | undefined,
  workTypeId: string | undefined,
): FactoryWorkProgressStateEvidence | undefined {
  if (!workTypeId) return undefined;
  const workType = factory?.workTypes?.find(
    (candidate) => candidate.name === workTypeId || candidate.id === workTypeId,
  );
  const state = workType?.states.find(
    (candidate) => candidate.type === "INITIAL",
  );
  return state ? { ...state, category: state.type } : undefined;
}

function mergeEventWork(
  works: Map<string, FactoryWorkProgressEvidence>,
  value: unknown,
): void {
  const record = objectRecord(value);
  if (typeof record?.workId !== "string" || !record.workId) return;
  const previous = works.get(record.workId) ?? { id: record.workId };
  const stateRecord = objectRecord(record.state);
  const state = stateRecord
    ? {
        ...(typeof stateRecord.id === "string" ? { id: stateRecord.id } : {}),
        ...(typeof stateRecord.name === "string"
          ? { name: stateRecord.name }
          : {}),
        ...(isWorkStateCategory(stateRecord.type)
          ? { category: stateRecord.type }
          : {}),
      }
    : previous.state;
  works.set(record.workId, {
    ...previous,
    ...(typeof record.workTypeName === "string"
      ? { workTypeId: record.workTypeName }
      : {}),
    ...(state ? { state } : {}),
  });
}

function classifyWork(
  stateCategory: FactoryWorkStateCategory | undefined,
  active: boolean,
): FactoryWorkProgressCategory {
  if (stateCategory === "FAILED") return "failed";
  if (stateCategory === "TERMINAL") return "completed";
  if (active) return "active";
  if (stateCategory === "INITIAL" || stateCategory === "PROCESSING") {
    return "queued";
  }
  return "unclassified";
}

function isWorkStateCategory(
  value: unknown,
): value is FactoryWorkStateCategory {
  return (
    value === "FAILED" ||
    value === "INITIAL" ||
    value === "PROCESSING" ||
    value === "TERMINAL"
  );
}

function objectRecord(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}
