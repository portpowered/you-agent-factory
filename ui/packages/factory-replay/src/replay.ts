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
  cloneState(state: State, selectedTick: number): State;
  createState(selectedTick: number): State;
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

function compareAcceptedEvents(left: FactoryEvent, right: FactoryEvent): number {
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
  let state = input.reducer.cloneState(input.checkpoint.state, input.tick);
  for (const event of eligibleTail) {
    state = input.reducer.applyEvent(state, event);
  }

  const events = [
    ...input.checkpoint.appliedEvents.map(cloneFactoryEvent),
    ...eligibleTail.map(cloneFactoryEvent),
  ];
  const acceptedEventIds = new Set(input.checkpoint.acceptedEventIds);
  for (const event of eligibleTail) acceptedEventIds.add(event.id);

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
