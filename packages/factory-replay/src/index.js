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
