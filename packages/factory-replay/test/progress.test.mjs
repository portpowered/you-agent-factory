import assert from "node:assert/strict";
import test from "node:test";

import {
  projectFactoryWorkProgress,
  projectFactoryWorkProgressAtTick,
} from "../src/index.js";

const factory = {
  name: "publishing",
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" },
        { id: "editing-stable", name: "editing", type: "PROCESSING" },
        { id: "done-stable", name: "done", type: "TERMINAL" },
        { id: "failed-stable", name: "failed", type: "FAILED" },
      ],
    },
  ],
};

function event(id, type, tick, sequence, payload, context = {}) {
  return {
    context: {
      eventTime: `2026-07-18T09:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

function topologyEvent() {
  return event(
    "topology",
    "INITIAL_STRUCTURE_REQUEST",
    1,
    0,
    { factory },
  );
}

function workRequest(id, tick, sequence, works) {
  return event(id, "WORK_REQUEST", tick, sequence, {
    type: "FACTORY_REQUEST_BATCH",
    works,
  });
}

function dispatch(id, type, tick, sequence, workIds, outputWork) {
  return event(
    id,
    type,
    tick,
    sequence,
    type === "DISPATCH_REQUEST"
      ? { inputs: workIds.map((workId) => ({ workId })), transitionId: "edit" }
      : { outcome: "CONTINUE", outputWork, transitionId: "edit" },
    { dispatchId: "dispatch-1", workIds },
  );
}

test("partitions Work once with failed, completed, active, queued precedence", () => {
  const input = {
    activeWorkIds: ["active", "failed", "completed", "active"],
    factory,
    selectedTick: 7,
    works: [
      { id: "queued", state: { name: "editing" }, workTypeId: "story" },
      { id: "active", state: { name: "ready" }, workTypeId: "story" },
      { id: "completed", state: { name: "done" }, workTypeId: "story" },
      { id: "failed", state: { name: "failed" }, workTypeId: "story" },
      { id: "unknown", state: { name: "missing" }, workTypeId: "story" },
      { id: "missing-type", state: { name: "ready" } },
      { id: "system", workTypeId: "__system_time" },
    ],
  };
  const before = structuredClone(input);
  const result = projectFactoryWorkProgress(input);

  assert.deepEqual(input, before);
  assert.deepEqual(result.counts, {
    active: 1,
    completed: 1,
    failed: 1,
    queued: 1,
    unclassified: 2,
  });
  assert.equal(result.total, 6);
  assert.deepEqual(result.failed.map((work) => work.id), ["failed"]);
  assert.deepEqual(result.completed.map((work) => work.id), ["completed"]);
  assert.deepEqual(result.active.map((work) => work.id), ["active"]);
  assert.deepEqual(result.queued.map((work) => work.id), ["queued"]);
  assert.deepEqual(result.unclassified.map((work) => work.id), [
    "missing-type",
    "unknown",
  ]);
  assert.equal(
    Object.values(result.counts).reduce((total, count) => total + count, 0),
    result.total,
  );
});

test("deduplicates Work identity and uses explicit state categories without topology", () => {
  const result = projectFactoryWorkProgress({
    activeWorkIds: ["work-1"],
    selectedTick: 1,
    works: [
      { id: "work-1", state: { category: "PROCESSING", name: "editing" } },
      { id: "work-1", state: { category: "FAILED", name: "failed" } },
      { id: "work-2", state: { category: "TERMINAL", name: "done" } },
    ],
  });

  assert.equal(result.total, 2);
  assert.deepEqual(result.failed.map((work) => work.id), ["work-1"]);
  assert.deepEqual(result.completed.map((work) => work.id), ["work-2"]);
});

test("replays Work through queued, active, queued, completed, and recovery states", () => {
  const events = [
    topologyEvent(),
    workRequest("request", 1, 1, [
      { name: "Draft", workId: "work-1", workTypeName: "story" },
    ]),
    dispatch("start", "DISPATCH_REQUEST", 2, 0, ["work-1"]),
    dispatch("finish", "DISPATCH_RESPONSE", 3, 0, ["work-1"], [
      {
        name: "Draft",
        state: { name: "editing", type: "PROCESSING" },
        workId: "work-1",
        workTypeName: "story",
      },
    ]),
    event("complete", "WORK_STATE_CHANGE", 4, 0, {
      fromPlaceId: "story:editing",
      fromState: "editing",
      source: "api",
      toPlaceId: "story:done",
      toState: "done",
      workId: "work-1",
      workTypeName: "story",
    }),
    event("fail", "WORK_STATE_CHANGE", 5, 0, {
      fromPlaceId: "story:done",
      fromState: "done",
      source: "api",
      toPlaceId: "story:failed",
      toState: "failed",
      workId: "work-1",
      workTypeName: "story",
    }),
    event("recover", "WORK_STATE_CHANGE", 6, 0, {
      fromPlaceId: "story:failed",
      fromState: "failed",
      source: "api",
      toPlaceId: "story:editing",
      toState: "editing",
      workId: "work-1",
      workTypeName: "story",
    }),
  ];

  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 1 }).queued.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 2 }).active.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 3 }).queued.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 4 }).completed.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 5 }).failed.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
  assert.deepEqual(
    projectFactoryWorkProgressAtTick({ events, tick: 6 }).queued.map(
      (work) => work.id,
    ),
    ["work-1"],
  );
});

test("same-tick canonical order recomputes completed Dispatches and state changes", () => {
  const base = [
    topologyEvent(),
    workRequest("request", 1, 1, [
      { name: "Draft", workId: "work-1", workTypeName: "story" },
    ]),
    dispatch("start", "DISPATCH_REQUEST", 2, 1, ["work-1"]),
  ];
  const active = projectFactoryWorkProgressAtTick({ events: base, tick: 2 });
  assert.equal(active.counts.active, 1);

  const completedSameTick = projectFactoryWorkProgressAtTick({
    events: [
      ...base,
      dispatch("finish", "DISPATCH_RESPONSE", 2, 2, ["work-1"], [
        {
          name: "Draft",
          state: { name: "done", type: "TERMINAL" },
          workId: "work-1",
          workTypeName: "story",
        },
      ]),
    ],
    tick: 2,
  });
  assert.equal(completedSameTick.counts.active, 0);
  assert.equal(completedSameTick.counts.completed, 1);
});

test("missing completion output becomes unclassified and system Work stays excluded", () => {
  const result = projectFactoryWorkProgressAtTick({
    events: [
      topologyEvent(),
      workRequest("request", 1, 1, [
        { name: "Draft", workId: "work-1", workTypeName: "story" },
        { name: "Timer", workId: "timer", workTypeName: "__system_time" },
      ]),
      dispatch("start", "DISPATCH_REQUEST", 2, 0, ["work-1"]),
      dispatch("finish", "DISPATCH_RESPONSE", 2, 1, ["work-1"], undefined),
    ],
    tick: 2,
  });

  assert.equal(result.total, 1);
  assert.deepEqual(result.unclassified.map((work) => work.id), ["work-1"]);
});
