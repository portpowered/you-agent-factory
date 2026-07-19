import type { FactoryEvent } from "@you-agent-factory/client";

export interface FactoryReplayFixedSelection {
  mode: "fixed";
  tick: number;
}

export type FactoryReplaySelection =
  | { mode: "current" }
  | FactoryReplayFixedSelection;

/** Domain interpretation stays with the consumer; the kernel owns replay semantics. */
export interface FactoryReplayReducer<State> {
  applyEvent(state: State, event: FactoryEvent): State;
  createState(selectedTick: number): State;
}

/** A reducer that also maps its domain state into a consumer-owned view. */
export interface FactoryReplayWorldReducer<State, World>
  extends FactoryReplayReducer<State> {
  projectWorld(state: State): World;
}

export interface FactoryReplayInitialization<State> {
  events: readonly FactoryEvent[];
  reducer: FactoryReplayReducer<State>;
  selection: FactoryReplaySelection;
}

export interface FactoryReplayCheckpoint<State> {
  acceptedEventIds: ReadonlySet<string>;
  appliedEvents: readonly FactoryEvent[];
  selectedTick: number;
  state: State;
}

export interface FactoryReplayCheckpointAdvancement<State> {
  checkpoint: FactoryReplayCheckpoint<State>;
  reducer: FactoryReplayReducer<State>;
  tail: readonly FactoryEvent[];
  tick: number;
}

export interface FactoryReplayResult<State>
  extends FactoryReplayCheckpoint<State> {
  acceptedEventIds: Set<string>;
  appliedEvents: FactoryEvent[];
  events: FactoryEvent[];
  latestTick: number;
  selectedTick: number;
  selection: FactoryReplaySelection;
  state: State;
}

export interface FactoryReplayWorldResult<State, World>
  extends FactoryReplayResult<State> {
  world: World;
}

export type FactoryReplayStateCloner<State> = (state: State) => State;

/**
 * A compact checkpoint for consumers that retain domain state but not replay
 * history. Event IDs make repeated tails idempotent without retaining events.
 */
export interface FactoryReplayWorldCheckpoint<State> {
  acceptedEventIDs: readonly string[];
  selectedTick: number;
  state: State;
}

export interface FactoryReplayWorldAdvanceInput<State, World> {
  checkpoint: FactoryReplayWorldCheckpoint<State>;
  cloneState: FactoryReplayStateCloner<State>;
  events: readonly FactoryEvent[];
  reducer: FactoryReplayWorldReducer<State, World>;
  setSelectedTick(state: State, tick: number): State;
  tick: number;
}

export interface FactoryReplayWorldAdvanceResult<State, World> {
  appliedEvents: FactoryEvent[];
  checkpoint: FactoryReplayWorldCheckpoint<State>;
  latestTick: number;
  selectedTick: number;
  state: State;
  world: World;
}

function cloneFactoryEvent(event: FactoryEvent): FactoryEvent {
  return JSON.parse(JSON.stringify(event)) as FactoryEvent;
}

function canonicalValue(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(canonicalValue).join(",")}]`;
  }
  if (value !== null && typeof value === "object") {
    const entries = Object.entries(value).sort(([left], [right]) =>
      left.localeCompare(right),
    );
    return `{${entries
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalValue(item)}`)
      .join(",")}}`;
  }
  return JSON.stringify(value) ?? "undefined";
}

function compareAcceptedEvents(
  left: FactoryEvent,
  right: FactoryEvent,
): number {
  const tickDifference = left.context.tick - right.context.tick;
  if (tickDifference !== 0) return tickDifference;

  const effectiveSequenceDifference =
    (left.context.sessionSequence ?? left.context.sequence) -
    (right.context.sessionSequence ?? right.context.sequence);
  if (effectiveSequenceDifference !== 0) return effectiveSequenceDifference;

  const eventLogSequenceDifference =
    left.context.sequence - right.context.sequence;
  if (eventLogSequenceDifference !== 0) return eventLogSequenceDifference;

  const timeDifference = left.context.eventTime.localeCompare(
    right.context.eventTime,
  );
  if (timeDifference !== 0) return timeDifference;

  const idDifference = left.id.localeCompare(right.id);
  if (idDifference !== 0) return idDifference;

  return canonicalValue(left).localeCompare(canonicalValue(right));
}

/**
 * Return one immutable, canonically ordered accepted history. When an ID is
 * repeated, the canonically earliest event owns that ID.
 */
export function canonicalizeFactoryEvents(
  events: readonly FactoryEvent[],
): FactoryEvent[] {
  const acceptedEventIds = new Set<string>();
  return [...events].sort(compareAcceptedEvents).filter((event) => {
    if (acceptedEventIds.has(event.id)) return false;
    acceptedEventIds.add(event.id);
    return true;
  });
}

/** Reconstruct reducer-owned state for a current or fixed logical tick. */
export function initializeFactoryReplay<State>(
  input: FactoryReplayInitialization<State>,
): FactoryReplayResult<State> {
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
    acceptedEventIds: new Set(appliedEvents.map(({ id }) => id)),
    appliedEvents,
    events,
    latestTick,
    selectedTick,
    selection: input.selection,
    state,
  };
}

/** Advance an immutable checkpoint with unseen accepted events through a tick. */
export function advanceFactoryReplayCheckpoint<State>(
  input: FactoryReplayCheckpointAdvancement<State>,
): FactoryReplayResult<State> {
  const checkpointEventIds = new Set(input.checkpoint.acceptedEventIds);
  const eligibleTail = canonicalizeFactoryEvents(input.tail).filter(
    (event) =>
      event.context.tick <= input.tick && !checkpointEventIds.has(event.id),
  );
  const events = canonicalizeFactoryEvents([
    ...input.checkpoint.appliedEvents.map(cloneFactoryEvent),
    ...eligibleTail.map(cloneFactoryEvent),
  ]).filter((event) => event.context.tick <= input.tick);
  let state = input.reducer.createState(input.tick);
  for (const event of events) {
    state = input.reducer.applyEvent(state, event);
  }

  const acceptedEventIds = new Set(events.map(({ id }) => id));

  return {
    acceptedEventIds,
    appliedEvents: [...events],
    events,
    latestTick: events.reduce(
      (latest, event) => Math.max(latest, event.context.tick),
      0,
    ),
    selectedTick: input.tick,
    selection: { mode: "fixed", tick: input.tick },
    state,
  };
}

/** Reconstruct reducer-owned state at one explicit logical tick. */
export function projectFactoryStateAtTick<State>(
  input: Omit<FactoryReplayInitialization<State>, "selection"> & {
    tick: number;
  },
): FactoryReplayResult<State> {
  return initializeFactoryReplay({
    events: input.events,
    reducer: input.reducer,
    selection: { mode: "fixed", tick: input.tick },
  });
}

/** Create an independent compact checkpoint from a selected-tick projection. */
export function createFactoryReplayWorldCheckpoint<State, World>(
  result: FactoryReplayWorldResult<State, World>,
  cloneState: FactoryReplayStateCloner<State>,
): FactoryReplayWorldCheckpoint<State> {
  return {
    acceptedEventIDs: result.appliedEvents.map((event) => event.id),
    selectedTick: result.selectedTick,
    state: cloneState(result.state),
  };
}

/**
 * Advance a compact checkpoint with an accepted tail without retaining the
 * checkpoint's historical events. The explicit clone and tick adapters keep
 * domain-state ownership with the consumer.
 */
export function advanceFactoryReplay<State, World>(
  input: FactoryReplayWorldAdvanceInput<State, World>,
): FactoryReplayWorldAdvanceResult<State, World> {
  const acceptedEventIDs = new Set(input.checkpoint.acceptedEventIDs);
  const appliedEvents = canonicalizeFactoryEvents(input.events).filter(
    (event) => {
      if (
        acceptedEventIDs.has(event.id) ||
        event.context.tick < input.checkpoint.selectedTick ||
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

/** Reconstruct a consumer-owned world at one explicit logical tick. */
export function projectFactoryWorldAtTick<State, World>(
  input: Omit<FactoryReplayInitialization<State>, "selection"> & {
    reducer: FactoryReplayWorldReducer<State, World>;
    tick: number;
  },
): FactoryReplayWorldResult<State, World> {
  const result = projectFactoryStateAtTick(input);
  return { ...result, world: input.reducer.projectWorld(result.state) };
}
