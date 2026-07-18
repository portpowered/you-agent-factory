import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  advanceFactoryReplay,
  canonicalizeFactoryEvents,
  createFactoryReplayCheckpoint,
  initializeFactoryReplay,
  projectFactoryTopology,
  projectFactoryTopologyAtTick,
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

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../..",
);

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

const completeFactory = {
  name: "publishing",
  resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
  workers: [
    {
      id: "writer-stable",
      name: "writer",
      resources: [{ capacity: 1, name: "gpu" }],
    },
  ],
  workTypes: [
    {
      id: "task-stable",
      name: "task",
      states: [
        { id: "queued-stable", name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workstations: [
    {
      id: "review-stable",
      inputs: [{ state: "queued", workType: "task" }],
      name: "review",
      onContinue: [{ state: "queued", workType: "task" }],
      onFailure: [{ state: "failed", workType: "task" }],
      onRejection: [{ state: "queued", workType: "task" }],
      outputs: [{ state: "done", workType: "task" }],
      resources: [{ capacity: 1, name: "gpu" }],
      worker: "writer",
    },
  ],
};

function topologyEvent(id, type, tick, sequence, factory) {
  return {
    context: {
      eventTime: `2026-07-18T05:00:0${sequence}Z`,
      sequence,
      tick,
    },
    id,
    payload: { factory },
    type,
  };
}

test("public topology projection is stable, ordered, and handle-complete", () => {
  const first = projectFactoryTopology({
    factory: completeFactory,
    selectedTick: 4,
  });
  const repeated = projectFactoryTopology({
    factory: structuredClone(completeFactory),
    selectedTick: 4,
  });

  assert.deepEqual(first, repeated);
  assert.deepEqual(
    first.nodes.map((node) => node.id),
    [...first.nodes.map((node) => node.id)].sort(),
  );
  assert.deepEqual(
    first.connections.map((connection) => connection.id),
    [...first.connections.map((connection) => connection.id)].sort(),
  );
  assert.deepEqual(
    new Set(first.nodes.map((node) => node.kind)),
    new Set(["resource", "worker", "work-state", "work-type", "workstation"]),
  );
  assert.deepEqual(
    new Set(first.connections.map((connection) => connection.kind)),
    new Set([
      "worker-assignment",
      "worker-resource",
      "workstation-input",
      "workstation-on-continue",
      "workstation-on-failure",
      "workstation-on-rejection",
      "workstation-output",
      "workstation-resource",
      "work-type-state",
    ]),
  );
  assert.equal(first.issues.length, 0);

  const nodesByID = new Map(first.nodes.map((node) => [node.id, node]));
  for (const connection of first.connections) {
    for (const endpoint of [connection.source, connection.target]) {
      const endpointNode = nodesByID.get(endpoint.nodeId);
      assert.ok(endpointNode, `missing node ${endpoint.nodeId}`);
      assert.ok(
        endpointNode.handles.some((handle) => handle.id === endpoint.handleId),
        `missing handle ${endpoint.handleId} on ${endpoint.nodeId}`,
      );
    }
  }
});

test("backend canonical topology snapshots preserve stable IDs and resource connections", () => {
  const events = JSON.parse(
    execFileSync(
      "go",
      ["run", "./packages/factory-replay/test/fixtures/canonical_topology_event"],
      { cwd: repositoryRoot, encoding: "utf8" },
    ),
  );

  const before = projectFactoryTopologyAtTick({ events, tick: 0 });
  const after = projectFactoryTopologyAtTick({ events, tick: 3 });

  assert.deepEqual(before.issues, []);
  assert.deepEqual(after.issues, []);
  assert.deepEqual(
    before.nodes.map(({ id }) => id),
    after.nodes.map(({ id }) => id),
  );
  assert.deepEqual(
    before.connections.map(({ id }) => id),
    after.connections.map(({ id }) => id),
  );
  assert.equal(before.nodes.find(({ id }) => id === "resource:gpu-stable")?.label, "gpu");
  assert.equal(after.nodes.find(({ id }) => id === "resource:gpu-stable")?.label, "accelerator");
  assert.deepEqual(
    new Set(before.connections.map(({ kind }) => kind)),
    new Set([
      "worker-resource",
      "work-type-state",
      "worker-assignment",
      "workstation-resource",
      "workstation-input",
      "workstation-output",
      "workstation-on-failure",
      "workstation-on-rejection",
    ]),
  );
  assert.ok(
    before.connections.some(
      ({ kind, source, target }) =>
        kind === "workstation-input" &&
        source.nodeId === "work-state:task-stable:queued-stable" &&
        target.nodeId === "workstation:review-stable",
    ),
  );
  assert.ok(
    before.connections.some(
      ({ kind, source, target }) =>
        kind === "workstation-output" &&
        source.nodeId === "workstation:review-stable" &&
        target.nodeId === "work-state:task-stable:done-stable",
    ),
  );
});

test("selected-tick topology follows canonical replacement order", () => {
  const replacement = structuredClone(completeFactory);
  replacement.workstations[0].outputs = [
    { state: "failed", workType: "task" },
  ];
  const events = [
    topologyEvent("replacement", "FACTORY_CHANGE", 3, 2, replacement),
    topologyEvent("initial", "INITIAL_STRUCTURE_REQUEST", 1, 0, completeFactory),
  ];

  const before = projectFactoryTopologyAtTick({ events, tick: 2 });
  const after = projectFactoryTopologyAtTick({ events, tick: 3 });

  assert.ok(
    before.connections.some((connection) =>
      connection.id.endsWith("->work-state:task-stable:done"),
    ),
  );
  assert.ok(
    after.connections.some((connection) =>
      connection.id.endsWith("->work-state:task-stable:failed"),
    ),
  );
  assert.deepEqual(
    before.nodes.map((node) => node.id),
    after.nodes.map((node) => node.id),
  );
  assert.equal(after.selectedTick, 3);
});

test("same-tick topology replacements use canonical sequence order", () => {
  const replacement = { ...completeFactory, name: "replacement" };
  const result = projectFactoryTopologyAtTick({
    events: [
      topologyEvent("later", "FACTORY_CHANGE", 7, 2, replacement),
      topologyEvent("earlier", "FACTORY_CHANGE", 7, 1, { name: "empty" }),
    ],
    tick: 7,
  });

  assert.equal(result.nodes.length, 7);
});

test("partial topology omits dangling connections and returns stable issues", () => {
  const partialFactory = {
    name: "partial",
    workers: [
      {
        name: "worker",
        resources: [{ capacity: 1, name: "missing-worker-resource" }],
      },
    ],
    workstations: [
      {
        inputs: [{ state: "missing", workType: "unknown" }],
        name: "workstation",
        resources: [{ capacity: 1, name: "missing-workstation-resource" }],
        worker: "missing-worker",
      },
    ],
  };

  const result = projectFactoryTopology({
    factory: partialFactory,
    selectedTick: 1,
  });

  assert.deepEqual(result.connections, []);
  assert.deepEqual(
    result.issues.map((issue) => issue.id),
    [...result.issues.map((issue) => issue.id)].sort(),
  );
  assert.equal(result.issues.length, 4);
  assert.ok(
    result.issues.every((issue) => issue.code === "UNRESOLVED_CONNECTION"),
  );
});

test("missing topology and optional collections return deterministic results", () => {
  assert.deepEqual(
    projectFactoryTopology({ factory: { name: "empty" }, selectedTick: 2 }),
    { connections: [], issues: [], nodes: [], selectedTick: 2 },
  );
  assert.deepEqual(projectFactoryTopologyAtTick({ events: [], tick: 2 }), {
    connections: [],
    issues: [
      {
        code: "MISSING_FACTORY",
        id: "missing-factory",
        message: "No Factory topology is available at the selected tick.",
      },
    ],
    nodes: [],
    selectedTick: 2,
  });
});
