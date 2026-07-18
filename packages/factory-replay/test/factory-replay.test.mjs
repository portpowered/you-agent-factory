import assert from "node:assert/strict";
import test from "node:test";

import {
  advanceFactoryReplay,
  canonicalizeFactoryEvents,
  createFactoryReplayCheckpoint,
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

const mutableReducer = {
  createState(selectedTick) {
    return { appliedIDs: [], eventKinds: {}, selectedTick };
  },
  applyEvent(state, factoryEvent) {
    state.appliedIDs.push(factoryEvent.id);
    state.eventKinds[factoryEvent.type] =
      (state.eventKinds[factoryEvent.type] ?? 0) + 1;
    return state;
  },
  projectWorld(state) {
    return {
      eventKinds: { ...state.eventKinds },
      logicalTick: state.selectedTick,
      processed: [...state.appliedIDs],
    };
  },
};

const cloneMutableState = (state) => structuredClone(state);
const setMutableStateTick = (state, tick) => ({ ...state, selectedTick: tick });

test("accepted tails advance a cloned checkpoint in canonical order", () => {
  const baseEvents = [event("topology", 1, 0)];
  const checkpoint = createFactoryReplayCheckpoint(
    initializeFactoryReplay({
      events: baseEvents,
      reducer: mutableReducer,
      selection: { mode: "current" },
    }),
    cloneMutableState,
  );
  const checkpointBeforeAdvance = structuredClone(checkpoint);
  const acceptedTail = [
    { ...event("lifecycle", 6, 0), type: "SESSION_STOPPED" },
    { ...event("complete", 5, 1), type: "DISPATCH_RESPONSE" },
    { ...event("work", 3, 0), type: "WORK_REQUEST" },
    { ...event("dispatch", 5, 0), type: "DISPATCH_REQUEST" },
    { ...event("topology", 1, 1), type: "RUN_REQUEST" },
    { ...event("work", 3, 1), type: "WORK_REQUEST" },
  ];

  const advanced = advanceFactoryReplay({
    checkpoint,
    cloneState: cloneMutableState,
    events: acceptedTail,
    reducer: mutableReducer,
    setSelectedTick: setMutableStateTick,
    tick: 5,
  });
  const full = initializeFactoryReplay({
    events: [...baseEvents, ...acceptedTail],
    reducer: mutableReducer,
    selection: { mode: "fixed", tick: 5 },
  });

  assert.deepEqual(
    advanced.appliedEvents.map((factoryEvent) => factoryEvent.id),
    ["work", "dispatch", "complete"],
  );
  assert.deepEqual(advanced.world, full.world);
  assert.deepEqual(checkpoint, checkpointBeforeAdvance);

  advanced.state.appliedIDs.push("caller-mutation");
  advanced.world.processed.push("caller-mutation");
  assert.deepEqual(checkpoint, checkpointBeforeAdvance);
  assert.deepEqual(advanced.checkpoint.state, full.state);
});

test("accepted tails retain later canonical events at the checkpoint tick", () => {
  const checkpoint = createFactoryReplayCheckpoint(
    initializeFactoryReplay({
      events: [event("initial", 2, 1)],
      reducer,
      selection: { mode: "current" },
    }),
    structuredClone,
  );
  const acceptedTail = [event("same-tick-tail", 2, 2)];

  const advanced = advanceFactoryReplay({
    checkpoint,
    cloneState: structuredClone,
    events: acceptedTail,
    reducer,
    setSelectedTick: setMutableStateTick,
    tick: 2,
  });

  assert.deepEqual(
    advanced.appliedEvents.map((factoryEvent) => factoryEvent.id),
    ["same-tick-tail"],
  );
  assert.deepEqual(advanced.world.eventIDs, ["initial", "same-tick-tail"]);
});
