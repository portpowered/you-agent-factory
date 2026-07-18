import { projectFactoryTopology } from "./topology.js";
import { projectFactoryActivity } from "./activity.js";
import { projectFactoryWorkProgress } from "./progress.js";

export { projectFactoryTopology } from "./topology.js";
export { projectFactoryActivity } from "./activity.js";
export { projectFactoryWorkProgress } from "./progress.js";

/**
 * Compare Factory events using the established dashboard replay order.
 *
 * The event timestamp and id remain deterministic tie-breakers for events
 * that share one logical tick and sequence.
 *
 * @param {import("@you-agent-factory/client").FactoryEvent} left
 * @param {import("@you-agent-factory/client").FactoryEvent} right
 * @returns {number}
 */
export function compareFactoryEvents(left, right) {
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

/**
 * Return the canonical accepted Factory event history without mutating input.
 * The first event in canonical order owns a duplicate id.
 *
 * @param {readonly import("@you-agent-factory/client").FactoryEvent[]} events
 * @returns {import("@you-agent-factory/client").FactoryEvent[]}
 */
export function canonicalizeFactoryEvents(events) {
  const acceptedIDs = new Set();
  return [...events]
    .sort(compareFactoryEvents)
    .filter((event) => {
      if (acceptedIDs.has(event.id)) {
        return false;
      }
      acceptedIDs.add(event.id);
      return true;
    });
}

/**
 * Build one deterministic Factory-event replay result for current or fixed
 * logical-tick selection. The reducer owns Factory domain interpretation;
 * this kernel owns canonical ordering, id acceptance, and tick selection.
 *
 * @template State, World
 * @param {import("./index.d.ts").FactoryReplayInitialization<State, World>} input
 * @returns {import("./index.d.ts").FactoryReplayResult<State, World>}
 */
export function initializeFactoryReplay(input) {
  const events = canonicalizeFactoryEvents(input.events);
  const latestTick = events.reduce(
    (latest, event) => Math.max(latest, event.context.tick),
    0,
  );
  const selectedTick =
    input.selection.mode === "current" ? latestTick : input.selection.tick;
  const appliedEvents = events.filter(
    (event) => event.context.tick <= selectedTick,
  );
  let state = input.reducer.createState(selectedTick);
  for (const event of appliedEvents) {
    state = input.reducer.applyEvent(state, event);
  }

  return {
    appliedEvents,
    events,
    latestTick,
    selectedTick,
    selection: input.selection,
    state,
    world: input.reducer.projectWorld(state),
  };
}

/**
 * Create an independent checkpoint from an already projected replay result.
 * The caller provides cloning because the kernel cannot know the shape of a
 * domain-specific replay state.
 *
 * @template State, World
 * @param {import("./index.d.ts").FactoryReplayResult<State, World>} result
 * @param {import("./index.d.ts").FactoryReplayStateCloner<State>} cloneState
 * @returns {import("./index.d.ts").FactoryReplayCheckpoint<State>}
 */
export function createFactoryReplayCheckpoint(result, cloneState) {
  return {
    acceptedEventIDs: result.appliedEvents.map((event) => event.id),
    selectedTick: result.selectedTick,
    state: cloneState(result.state),
  };
}

/**
 * Advance an immutable replay checkpoint with an accepted event tail.
 * Only previously unseen events at or before the target tick are applied.
 * Event IDs, rather than the checkpoint tick, define what has already been
 * applied so later canonical events at the checkpoint's tick are retained.
 * The returned checkpoint is independently cloned so callers can retain it
 * for another historical reconstruction.
 *
 * @template State, World
 * @param {import("./index.d.ts").FactoryReplayAdvanceInput<State, World>} input
 * @returns {import("./index.d.ts").FactoryReplayAdvanceResult<State, World>}
 */
export function advanceFactoryReplay(input) {
  const acceptedEventIDs = new Set(input.checkpoint.acceptedEventIDs);
  const appliedEvents = canonicalizeFactoryEvents(input.events).filter(
    (event) => {
      if (
        acceptedEventIDs.has(event.id) ||
        event.context.tick > input.tick
      ) {
        return false;
      }
      acceptedEventIDs.add(event.id);
      return true;
    },
  );
  let state = input.setSelectedTick(
    input.cloneState(input.checkpoint.state),
    input.tick,
  );
  for (const event of appliedEvents) {
    state = input.reducer.applyEvent(state, event);
  }

  return {
    appliedEvents,
    checkpoint: {
      acceptedEventIDs: [...acceptedEventIDs],
      selectedTick: input.tick,
      state: input.cloneState(state),
    },
    latestTick: appliedEvents.reduce(
      (latest, event) => Math.max(latest, event.context.tick),
      input.checkpoint.selectedTick,
    ),
    selectedTick: input.tick,
    state,
    world: input.reducer.projectWorld(state),
  };
}

/**
 * Reconstruct the canonical Factory world at one explicit logical tick.
 *
 * @template State, World
 * @param {Omit<import("./index.d.ts").FactoryReplayInitialization<State, World>, "selection"> & { tick: number }} input
 * @returns {import("./index.d.ts").FactoryReplayResult<State, World>}
 */
export function projectFactoryWorldAtTick(input) {
  return initializeFactoryReplay({
    ...input,
    selection: { mode: "fixed", tick: input.tick },
  });
}

/**
 * Reconstruct and project the public Factory topology at one logical tick.
 * Only canonical topology replacement events participate in this projection.
 *
 * @param {import("./index.d.ts").FactoryTopologyAtTickInput} input
 * @returns {import("./index.d.ts").FactoryTopologyProjection}
 */
export function projectFactoryTopologyAtTick(input) {
  const events = canonicalizeFactoryEvents(input.events).filter(
    (event) => event.context.tick <= input.tick,
  );
  /** @type {import("@you-agent-factory/client").FactoryDefinition | undefined} */
  let factory;
  for (const event of events) {
    if (
      event.type !== "INITIAL_STRUCTURE_REQUEST" &&
      event.type !== "FACTORY_CHANGE"
    ) {
      continue;
    }
    const payload = event.payload;
    if (
      payload &&
      typeof payload === "object" &&
      "factory" in payload &&
      payload.factory &&
      typeof payload.factory === "object"
    ) {
      factory = payload.factory;
    }
  }
  return projectFactoryTopology({ factory, selectedTick: input.tick });
}

/**
 * Reconstruct active customer Dispatches and resource occupancy at one tick.
 * A response removes its matching request in canonical sequence order, even
 * when both events share a logical tick.
 *
 * @param {import("./index.d.ts").FactoryActivityAtTickInput} input
 * @returns {import("./index.d.ts").FactoryActivityProjection}
 */
export function projectFactoryActivityAtTick(input) {
  const events = canonicalizeFactoryEvents(input.events).filter(
    (event) => event.context.tick <= input.tick,
  );
  let factory;
  const activeDispatches = new Map();
  for (const event of events) {
    if (
      event.type === "INITIAL_STRUCTURE_REQUEST" ||
      event.type === "FACTORY_CHANGE"
    ) {
      const payload = event.payload;
      if (
        payload &&
        typeof payload === "object" &&
        "factory" in payload &&
        payload.factory &&
        typeof payload.factory === "object"
      ) {
        factory = payload.factory;
      }
      continue;
    }
    if (event.type === "DISPATCH_RESPONSE") {
      if (event.context.dispatchId) {
        activeDispatches.delete(event.context.dispatchId);
      }
      continue;
    }
    if (event.type !== "DISPATCH_REQUEST" || !event.context.dispatchId) {
      continue;
    }
    const payload = event.payload;
    if (
      !payload ||
      typeof payload !== "object" ||
      !("transitionId" in payload) ||
      typeof payload.transitionId !== "string" ||
      payload.transitionId.startsWith("__system_time:")
    ) {
      continue;
    }
    const resources =
      "resources" in payload && Array.isArray(payload.resources)
        ? payload.resources
        : undefined;
    activeDispatches.set(event.context.dispatchId, {
      id: event.context.dispatchId,
      ...(resources
        ? {
            resourceNames: resources.flatMap((resource) =>
              resource &&
              typeof resource === "object" &&
              "name" in resource &&
              typeof resource.name === "string"
                ? [resource.name]
                : [],
            ),
          }
        : {}),
      startedTick: event.context.tick,
      transitionId: payload.transitionId,
      workIds: [...(event.context.workIds ?? [])],
    });
  }
  return projectFactoryActivity({
    activeDispatches: [...activeDispatches.values()],
    factory,
    selectedTick: input.tick,
  });
}

/**
 * Reconstruct and exclusively classify customer Work at one logical tick.
 * Canonical sequence order owns same-tick lifecycle and Dispatch changes.
 *
 * @param {import("./index.d.ts").FactoryWorkProgressAtTickInput} input
 * @returns {import("./index.d.ts").FactoryWorkProgressProjection}
 */
export function projectFactoryWorkProgressAtTick(input) {
  const events = canonicalizeFactoryEvents(input.events).filter(
    (event) => event.context.tick <= input.tick,
  );
  /** @type {import("@you-agent-factory/client").FactoryDefinition | undefined} */
  let factory;
  const works = new Map();
  const initialWorkIds = new Set();
  const activeDispatches = new Map();
  for (const event of events) {
    if (
      event.type === "INITIAL_STRUCTURE_REQUEST" ||
      event.type === "FACTORY_CHANGE"
    ) {
      const payload = objectPayload(event.payload);
      if (payload?.factory && typeof payload.factory === "object") {
        factory = /** @type {import("@you-agent-factory/client").FactoryDefinition} */ (
          payload.factory
        );
      }
      continue;
    }
    if (event.type === "WORK_REQUEST") {
      const payload = objectPayload(event.payload);
      for (const work of Array.isArray(payload?.works) ? payload.works : []) {
        mergeEventWork(works, work);
        const record = objectPayload(work);
        if (
          typeof record?.workId === "string" &&
          !objectPayload(record.state)
        ) {
          initialWorkIds.add(record.workId);
        }
      }
      continue;
    }
    if (event.type === "WORK_STATE_CHANGE") {
      const payload = objectPayload(event.payload);
      if (typeof payload?.workId !== "string" || !payload.workId) continue;
      const previous = works.get(payload.workId) ?? { id: payload.workId };
      initialWorkIds.delete(payload.workId);
      const state =
        typeof payload.toState === "string"
          ? { name: payload.toState }
          : undefined;
      works.set(payload.workId, {
        ...previous,
        ...(typeof payload.workTypeName === "string"
          ? { workTypeId: payload.workTypeName }
          : {}),
        ...(state ? { state } : {}),
      });
      continue;
    }
    if (event.type === "DISPATCH_REQUEST" && event.context.dispatchId) {
      const payload = objectPayload(event.payload);
      if (
        typeof payload?.transitionId === "string" &&
        !payload.transitionId.startsWith("__system_time:")
      ) {
        const inputIds = Array.isArray(payload.inputs)
          ? payload.inputs.flatMap((input) => {
              const record = objectPayload(input);
              return typeof record?.workId === "string"
                ? [record.workId]
                : [];
            })
          : [];
        activeDispatches.set(event.context.dispatchId, [
          ...new Set([...(event.context.workIds ?? []), ...inputIds]),
        ]);
      }
      continue;
    }
    if (event.type !== "DISPATCH_RESPONSE" || !event.context.dispatchId) {
      continue;
    }
    const consumedIds = activeDispatches.get(event.context.dispatchId) ?? [];
    activeDispatches.delete(event.context.dispatchId);
    const payload = objectPayload(event.payload);
    const outputWorks = Array.isArray(payload?.outputWork)
      ? payload.outputWork
      : [];
    const outputIds = new Set(
      outputWorks.flatMap((work) => {
        const record = objectPayload(work);
        return typeof record?.workId === "string" ? [record.workId] : [];
      }),
    );
    for (const workId of consumedIds) {
      const previous = works.get(workId);
      if (!previous || outputIds.has(workId)) continue;
      initialWorkIds.delete(workId);
      const category = previous.state?.category;
      if (category !== "FAILED" && category !== "TERMINAL") {
        works.set(workId, {
          id: previous.id,
          ...(previous.workTypeId ? { workTypeId: previous.workTypeId } : {}),
        });
      }
    }
    for (const work of outputWorks) {
      mergeEventWork(works, work);
      const record = objectPayload(work);
      if (typeof record?.workId === "string") {
        initialWorkIds.delete(record.workId);
      }
    }
  }

  return projectFactoryWorkProgress({
    activeWorkIds: [...activeDispatches.values()].flat(),
    factory,
    selectedTick: input.tick,
    works: [...works.values()].map((work) => {
      const initialState = initialWorkIds.has(work.id)
        ? initialStateForWork(factory, work.workTypeId)
        : undefined;
      return initialState ? { ...work, state: initialState } : work;
    }),
  });
}

/**
 * @param {import("@you-agent-factory/client").FactoryDefinition | undefined} factory
 * @param {string | undefined} workTypeId
 * @returns {import("./index.d.ts").FactoryWorkProgressStateEvidence | undefined}
 */
function initialStateForWork(factory, workTypeId) {
  if (!workTypeId) return undefined;
  const workType = (factory?.workTypes ?? []).find(
    (candidate) => candidate.name === workTypeId || candidate.id === workTypeId,
  );
  const state = workType?.states.find((candidate) => candidate.type === "INITIAL");
  return state
    ? {
        category: state.type,
        ...(state.id ? { id: state.id } : {}),
        name: state.name,
      }
    : undefined;
}

/** @param {unknown} payload @returns {Record<string, unknown> | undefined} */
function objectPayload(payload) {
  return payload && typeof payload === "object"
    ? /** @type {Record<string, unknown>} */ (payload)
    : undefined;
}

/** @param {Map<string, import("./index.d.ts").FactoryWorkProgressEvidence>} works @param {unknown} value */
function mergeEventWork(works, value) {
  const record = objectPayload(value);
  if (typeof record?.workId !== "string" || !record.workId) return;
  const previous = works.get(record.workId) ?? { id: record.workId };
  const stateRecord = objectPayload(record.state);
  const state =
    stateRecord
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

/** @param {unknown} value @returns {value is import("./index.d.ts").FactoryWorkStateCategory} */
function isWorkStateCategory(value) {
  return (
    value === "FAILED" ||
    value === "INITIAL" ||
    value === "PROCESSING" ||
    value === "TERMINAL"
  );
}
