import assert from "node:assert/strict";
import test from "node:test";

import {
  projectFactoryActivity,
  projectFactoryActivityAtTick,
} from "../src/index.js";

const completeFactory = {
  name: "publishing",
  resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
  workers: [{ id: "writer-stable", name: "writer" }],
  workstations: [
    {
      id: "review-stable",
      inputs: [],
      name: "review",
      resources: [{ capacity: 1, name: "gpu" }],
      worker: "writer",
    },
  ],
};

function topologyEvent(factory = completeFactory) {
  return {
    context: { eventTime: "2026-07-18T06:00:00Z", sequence: 0, tick: 1 },
    id: "topology",
    payload: { factory },
    type: "INITIAL_STRUCTURE_REQUEST",
  };
}

function dispatchEvent(
  id,
  type,
  tick,
  sequence,
  { dispatchId, resources, transitionId = "review", workIds = [] },
) {
  return {
    context: {
      dispatchId,
      eventTime: `2026-07-18T06:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      workIds,
    },
    id,
    payload:
      type === "DISPATCH_REQUEST"
        ? { inputs: [], resources, transitionId }
        : { durationMillis: 1, outcome: "CONTINUE", transitionId },
    type,
  };
}

test("active Dispatch projection resolves topology and resource occupancy", () => {
  const input = {
    events: [
      topologyEvent(),
      dispatchEvent("start-b", "DISPATCH_REQUEST", 2, 2, {
        dispatchId: "dispatch-b",
        resources: [{ capacity: 2, name: "gpu" }],
        workIds: ["work-b"],
      }),
      dispatchEvent("start-a", "DISPATCH_REQUEST", 2, 1, {
        dispatchId: "dispatch-a",
        resources: [{ capacity: 2, name: "gpu" }],
        workIds: ["work-a", "work-a"],
      }),
    ],
    tick: 2,
  };
  const before = structuredClone(input);
  const result = projectFactoryActivityAtTick(input);

  assert.deepEqual(input, before);
  assert.deepEqual(
    result.activeDispatches.map((dispatch) => dispatch.id),
    ["dispatch-a", "dispatch-b"],
  );
  assert.deepEqual(result.activeDispatches[0], {
    id: "dispatch-a",
    resourceIds: ["gpu-stable"],
    startedTick: 2,
    transitionId: "review",
    workerId: "writer-stable",
    workstationId: "review-stable",
    workstationNodeId: "workstation:review-stable",
    workIds: ["work-a"],
  });
  assert.deepEqual(result.activeWorkstationIds, ["review-stable"]);
  assert.deepEqual(result.resourceOccupancy, [
    {
      availableQuantity: 0,
      capacity: 2,
      evidence: "known",
      occupiedQuantity: 2,
      resourceId: "gpu-stable",
      resourceNodeId: "resource:gpu-stable",
    },
  ]);
  assert.deepEqual(result.issues, []);
});

test("completion and same-tick ordering recompute historical activity", () => {
  const events = [
    topologyEvent(),
    dispatchEvent("start", "DISPATCH_REQUEST", 2, 0, {
      dispatchId: "dispatch-1",
      resources: [{ capacity: 2, name: "gpu" }],
      workIds: ["work-1"],
    }),
    dispatchEvent("finish", "DISPATCH_RESPONSE", 3, 1, {
      dispatchId: "dispatch-1",
    }),
    dispatchEvent("restart", "DISPATCH_REQUEST", 3, 2, {
      dispatchId: "dispatch-1",
      resources: [],
      workIds: ["work-1"],
    }),
    dispatchEvent("finish-again", "DISPATCH_RESPONSE", 3, 3, {
      dispatchId: "dispatch-1",
    }),
  ];

  assert.equal(
    projectFactoryActivityAtTick({ events, tick: 2 }).activeDispatches.length,
    1,
  );
  assert.equal(
    projectFactoryActivityAtTick({ events: events.slice(0, 4), tick: 3 })
      .activeDispatches.length,
    1,
  );
  assert.deepEqual(
    projectFactoryActivityAtTick({ events, tick: 3 }).activeDispatches,
    [],
  );
});

test("partial activity retains identity and reports unresolved references", () => {
  const result = projectFactoryActivity({
    activeDispatches: [
      {
        id: "dispatch-partial",
        resourceNames: ["missing"],
        startedTick: 4,
        transitionId: "missing-transition",
        workIds: ["work-partial"],
      },
    ],
    factory: completeFactory,
    selectedTick: 4,
  });

  assert.deepEqual(result.activeDispatches, [
    {
      id: "dispatch-partial",
      resourceIds: [],
      startedTick: 4,
      transitionId: "missing-transition",
      workIds: ["work-partial"],
    },
  ]);
  assert.deepEqual(
    result.issues.map((issue) => issue.code),
    ["UNRESOLVED_RESOURCE", "UNRESOLVED_WORKSTATION"],
  );
});

test("occupancy distinguishes unavailable evidence from known zero", () => {
  const unavailable = projectFactoryActivity({
    activeDispatches: [
      {
        id: "dispatch-unknown-resources",
        startedTick: 5,
        transitionId: "review",
        workIds: [],
      },
    ],
    factory: completeFactory,
    selectedTick: 5,
  });
  const knownZero = projectFactoryActivity({
    activeDispatches: [],
    factory: completeFactory,
    selectedTick: 5,
  });

  assert.deepEqual(unavailable.resourceOccupancy, [
    {
      capacity: 2,
      evidence: "unavailable",
      resourceId: "gpu-stable",
      resourceNodeId: "resource:gpu-stable",
    },
  ]);
  assert.equal(knownZero.resourceOccupancy[0].evidence, "known");
  assert.equal(knownZero.resourceOccupancy[0].occupiedQuantity, 0);
  assert.equal(knownZero.resourceOccupancy[0].availableQuantity, 2);
});

test("over-capacity evidence never produces negative availability", () => {
  const result = projectFactoryActivity({
    activeDispatches: [
      {
        id: "dispatch-over-capacity",
        resourceNames: ["gpu", "gpu", "gpu"],
        startedTick: 6,
        transitionId: "review",
        workIds: [],
      },
    ],
    factory: completeFactory,
    selectedTick: 6,
  });

  assert.equal(result.resourceOccupancy[0].occupiedQuantity, 3);
  assert.equal(result.resourceOccupancy[0].availableQuantity, 0);
  assert.equal(result.issues[0].code, "RESOURCE_CAPACITY_EXCEEDED");
});

test("system-only Dispatches are omitted from public activity", () => {
  const result = projectFactoryActivityAtTick({
    events: [
      dispatchEvent("system", "DISPATCH_REQUEST", 1, 0, {
        dispatchId: "system-dispatch",
        resources: [],
        transitionId: "__system_time:expire",
      }),
    ],
    tick: 1,
  });

  assert.deepEqual(result.activeDispatches, []);
});
