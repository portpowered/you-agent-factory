import assert from "node:assert/strict";
import test from "node:test";

import {
  canonicalizeFactoryEvents,
  initializeFactoryReplay,
  projectFactoryWorldAtTick,
} from "../src/index.js";

function event(id, tick, sequence, eventTime = "2026-07-18T05:00:00Z") {
  return {
    context: { eventTime, sequence, tick },
    id,
    payload: {},
    type: "SESSION_STARTED",
  };
}

const reducer = {
  createState(selectedTick) {
    return { appliedIDs: [], selectedTick };
  },
  applyEvent(state, factoryEvent) {
    return { ...state, appliedIDs: [...state.appliedIDs, factoryEvent.id] };
  },
  projectWorld(state) {
    return { eventIDs: state.appliedIDs, logicalTick: state.selectedTick };
  },
};

test("canonicalization orders same-tick events and accepts each id once", () => {
  const events = Object.freeze([
    event("event-later", 3, 0),
    event("event-tie-b", 2, 1, "2026-07-18T05:00:02Z"),
    event("event-first", 1, 2),
    event("event-tie-a", 2, 1, "2026-07-18T05:00:01Z"),
    event("event-tie-a", 2, 1, "2026-07-18T05:00:03Z"),
  ]);

  const canonical = canonicalizeFactoryEvents(events);

  assert.deepEqual(
    canonical.map((factoryEvent) => factoryEvent.id),
    ["event-first", "event-tie-a", "event-tie-b", "event-later"],
  );
  assert.equal(events.length, 5);
});

test("current selection projects the accepted canonical history at the latest tick", () => {
  const result = initializeFactoryReplay({
    events: [event("event-2", 2, 0), event("event-1", 1, 0)],
    reducer,
    selection: { mode: "current" },
  });

  assert.equal(result.latestTick, 2);
  assert.equal(result.selectedTick, 2);
  assert.deepEqual(result.world, {
    eventIDs: ["event-1", "event-2"],
    logicalTick: 2,
  });
});

test("fixed selection reconstructs only the historical logical tick", () => {
  const result = projectFactoryWorldAtTick({
    events: [event("event-3", 3, 0), event("event-1", 1, 0)],
    reducer,
    tick: 1,
  });

  assert.deepEqual(
    result.appliedEvents.map((factoryEvent) => factoryEvent.id),
    ["event-1"],
  );
  assert.deepEqual(result.world, {
    eventIDs: ["event-1"],
    logicalTick: 1,
  });
});
