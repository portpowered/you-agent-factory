import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  projectFactoryWorkProgress,
  projectFactoryWorkProgressAtTick,
} from "./index.js";

const factory: FactoryDefinition = {
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

function event(
  id: string,
  type: FactoryEventType,
  tick: number,
  sequence: number,
  payload: unknown,
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type,
    context: {
      eventTime: `2026-07-18T09:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      ...context,
    },
    payload,
  } as FactoryEvent;
}

function topologyEvent(): FactoryEvent {
  return event("topology", "INITIAL_STRUCTURE_REQUEST", 1, 0, { factory });
}

function workRequest(
  id: string,
  tick: number,
  sequence: number,
  works: unknown[],
): FactoryEvent {
  return event(id, "WORK_REQUEST", tick, sequence, {
    type: "FACTORY_REQUEST_BATCH",
    works,
  });
}

function dispatch(
  id: string,
  type: "DISPATCH_REQUEST" | "DISPATCH_RESPONSE",
  tick: number,
  sequence: number,
  workIds: string[],
  outputWork?: unknown[],
): FactoryEvent {
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

describe("projectFactoryWorkProgress", () => {
  it("partitions each customer Work once using lifecycle precedence", () => {
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
    } as const;
    const before = structuredClone(input);

    const result = projectFactoryWorkProgress(input);

    expect(input).toEqual(before);
    expect(result.counts).toEqual({
      active: 1,
      completed: 1,
      failed: 1,
      queued: 1,
      unclassified: 2,
    });
    expect(result.failed.map(({ id }) => id)).toEqual(["failed"]);
    expect(result.completed.map(({ id }) => id)).toEqual(["completed"]);
    expect(result.active.map(({ id }) => id)).toEqual(["active"]);
    expect(result.queued.map(({ id }) => id)).toEqual(["queued"]);
    expect(result.unclassified.map(({ id }) => id)).toEqual([
      "missing-type",
      "unknown",
    ]);
    expect(
      Object.values(result.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(result.total);
  });

  it("deduplicates Work identity with deterministic last-evidence ownership", () => {
    const result = projectFactoryWorkProgress({
      activeWorkIds: ["work-1"],
      selectedTick: 1,
      works: [
        { id: "work-1", state: { category: "PROCESSING" } },
        { id: "work-1", state: { category: "FAILED", name: "failed" } },
      ],
    });

    expect(result.total).toBe(1);
    expect(result.failed).toEqual([{ id: "work-1", stateName: "failed" }]);
    expect(result.active).toEqual([]);
  });
});

describe("projectFactoryWorkProgressAtTick", () => {
  it("replays queued, active, completed, failed, and recovered lifecycle states", () => {
    const events = [
      topologyEvent(),
      workRequest("request", 1, 1, [
        { name: "Draft", workId: "work-1", workTypeName: "story" },
      ]),
      dispatch("start", "DISPATCH_REQUEST", 2, 0, ["work-1"]),
      dispatch(
        "finish",
        "DISPATCH_RESPONSE",
        3,
        0,
        ["work-1"],
        [
          {
            name: "Draft",
            state: { name: "editing", type: "PROCESSING" },
            workId: "work-1",
            workTypeName: "story",
          },
        ],
      ),
      event("complete", "WORK_STATE_CHANGE", 4, 0, {
        toState: "done",
        workId: "work-1",
        workTypeName: "story",
      }),
      event("fail", "WORK_STATE_CHANGE", 5, 0, {
        toState: "failed",
        workId: "work-1",
        workTypeName: "story",
      }),
      event("recover", "WORK_STATE_CHANGE", 6, 0, {
        toState: "editing",
        workId: "work-1",
        workTypeName: "story",
      }),
    ];

    expect(categoryIds(events, 1, "queued")).toEqual(["work-1"]);
    expect(categoryIds(events, 2, "active")).toEqual(["work-1"]);
    expect(categoryIds(events, 3, "queued")).toEqual(["work-1"]);
    expect(categoryIds(events, 4, "completed")).toEqual(["work-1"]);
    expect(categoryIds(events, 5, "failed")).toEqual(["work-1"]);
    expect(categoryIds(events, 6, "queued")).toEqual(["work-1"]);
  });

  it("uses canonical same-tick order for Dispatch completion and duplicate evidence", () => {
    const events = [
      dispatch(
        "finish",
        "DISPATCH_RESPONSE",
        2,
        3,
        ["work-1"],
        [
          {
            name: "Draft",
            state: { name: "done", type: "TERMINAL" },
            workId: "work-1",
            workTypeName: "story",
          },
        ],
      ),
      dispatch("start", "DISPATCH_REQUEST", 2, 2, ["work-1"]),
      workRequest("duplicate-work", 1, 2, [
        { name: "Draft revised", workId: "work-1", workTypeName: "story" },
      ]),
      workRequest("request", 1, 1, [
        { name: "Draft", workId: "work-1", workTypeName: "story" },
      ]),
      topologyEvent(),
    ];

    const forward = projectFactoryWorkProgressAtTick({ events, tick: 2 });
    const reverse = projectFactoryWorkProgressAtTick({
      events: [...events].reverse(),
      tick: 2,
    });

    expect(forward).toEqual(reverse);
    expect(forward.total).toBe(1);
    expect(forward.completed.map(({ id }) => id)).toEqual(["work-1"]);
    expect(forward.active).toEqual([]);
  });
});

describe("projectFactoryWorkProgressAtTick edge evidence", () => {
  it("ends active classification for interrupted and terminally reconciled Dispatches", () => {
    const start = event(
      "start",
      "DISPATCH_REQUEST",
      2,
      0,
      { inputs: [{ workId: "work-1" }] },
      { dispatchId: "dispatch-1", workIds: ["work-1"] },
    );
    const interrupted = event(
      "interrupted",
      "DISPATCH_INTERRUPTED",
      3,
      0,
      { reason: "cancelled" },
      { dispatchId: "dispatch-1", workIds: ["work-1"] },
    );
    const restarted = event(
      "restarted",
      "DISPATCH_REQUEST",
      4,
      0,
      { inputs: [{ workId: "work-1" }], transitionId: "edit" },
      { dispatchId: "dispatch-2", workIds: ["work-1"] },
    );
    const reconciled = event(
      "reconciled",
      "DISPATCH_RECONCILED",
      5,
      0,
      { reconciledStatus: "FAILED", reconciliationSource: "RECOVERY" },
      { dispatchId: "dispatch-2", workIds: ["work-1"] },
    );
    const events = [start, interrupted, restarted, reconciled];

    expect(categoryIds(events, 2, "active")).toEqual(["work-1"]);
    expect(categoryIds(events, 3, "unclassified")).toEqual(["work-1"]);
    expect(categoryIds(events, 4, "active")).toEqual(["work-1"]);
    expect(categoryIds(events, 5, "unclassified")).toEqual(["work-1"]);
  });

  it("retains terminal and failed output Work without Dispatch correlation", () => {
    const result = projectFactoryWorkProgressAtTick({
      events: [
        event(
          "response",
          "DISPATCH_RESPONSE",
          2,
          0,
          {
            outcome: "CONTINUE",
            outputWork: [
              {
                state: { name: "done", type: "TERMINAL" },
                workId: "completed-output",
                workTypeName: "story",
              },
              {
                state: { name: "failed", type: "FAILED" },
                workId: "failed-output",
                workTypeName: "story",
              },
            ],
            transitionId: "edit",
          },
          { workIds: [] },
        ),
      ],
      tick: 2,
    });

    expect(result.total).toBe(2);
    expect(result.completed.map(({ id }) => id)).toEqual(["completed-output"]);
    expect(result.failed.map(({ id }) => id)).toEqual(["failed-output"]);
    expect(
      Object.values(result.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(result.total);
  });

  it("retains response-only context Work without Dispatch correlation", () => {
    const result = projectFactoryWorkProgressAtTick({
      events: [
        event(
          "response",
          "DISPATCH_RESPONSE",
          2,
          0,
          { outcome: "CONTINUE", transitionId: "edit" },
          { workIds: ["context-only", "context-only"] },
        ),
      ],
      tick: 2,
    });

    expect(result.total).toBe(1);
    expect(result.unclassified).toEqual([{ id: "context-only" }]);
    expect(
      Object.values(result.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(result.total);
  });

  it("keeps Dispatch-only Work identifiable during and after active evidence", () => {
    const events = [
      dispatch("start", "DISPATCH_REQUEST", 2, 0, ["dispatch-only"]),
      dispatch("finish", "DISPATCH_RESPONSE", 3, 0, ["dispatch-only"]),
    ];

    const active = projectFactoryWorkProgressAtTick({ events, tick: 2 });
    expect(active.total).toBe(1);
    expect(active.active).toEqual([{ id: "dispatch-only" }]);
    expect(
      Object.values(active.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(active.total);

    const completed = projectFactoryWorkProgressAtTick({ events, tick: 3 });
    expect(completed.total).toBe(1);
    expect(completed.active).toEqual([]);
    expect(completed.unclassified).toEqual([{ id: "dispatch-only" }]);
    expect(
      Object.values(completed.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(completed.total);
  });
});

describe("projectFactoryWorkProgressAtTick request evidence", () => {
  it("retains request input and context Work without Dispatch correlation", () => {
    const result = projectFactoryWorkProgressAtTick({
      events: [
        event(
          "request",
          "DISPATCH_REQUEST",
          2,
          0,
          {
            inputs: [{ workId: "input-work" }, { workId: "input-work" }],
            transitionId: "review",
          },
          { workIds: ["context-work", "context-work"] },
        ),
      ],
      tick: 2,
    });

    expect(result.total).toBe(2);
    expect(result.active).toEqual([]);
    expect(result.unclassified).toEqual([
      { id: "context-work" },
      { id: "input-work" },
    ]);
    expect(
      Object.values(result.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(result.total);
  });

  it("uses same-tick Work-state order and failed precedence over active evidence", () => {
    const events = [
      event("fail", "WORK_STATE_CHANGE", 2, 4, {
        toState: "failed",
        workId: "work-1",
        workTypeName: "story",
      }),
      event("complete", "WORK_STATE_CHANGE", 2, 3, {
        toState: "done",
        workId: "work-1",
        workTypeName: "story",
      }),
      dispatch("start", "DISPATCH_REQUEST", 2, 2, ["work-1"]),
      workRequest("request", 1, 1, [
        { name: "Draft", workId: "work-1", workTypeName: "story" },
      ]),
      topologyEvent(),
    ];

    const result = projectFactoryWorkProgressAtTick({ events, tick: 2 });

    expect(result.failed.map(({ id }) => id)).toEqual(["work-1"]);
    expect(result.completed).toEqual([]);
    expect(result.active).toEqual([]);
  });

  it("keeps incomplete and unresolved Work identifiable while excluding system Work", () => {
    const result = projectFactoryWorkProgressAtTick({
      events: [
        topologyEvent(),
        workRequest("request", 1, 1, [
          { name: "Draft", workId: "work-1", workTypeName: "missing" },
          { name: "No type", workId: "work-2" },
          { name: "Timer", workId: "timer", workTypeName: "__system_time" },
          { name: "Missing identity", workTypeName: "story" },
        ]),
        dispatch("start", "DISPATCH_REQUEST", 2, 0, ["work-1"]),
        dispatch("finish", "DISPATCH_RESPONSE", 2, 1, ["work-1"]),
      ],
      tick: 2,
    });

    expect(result.total).toBe(2);
    expect(result.unclassified).toEqual([
      { id: "work-1", workTypeId: "missing" },
      { id: "work-2" },
    ]);
    expect(
      Object.values(result.counts).reduce((sum, count) => sum + count, 0),
    ).toBe(result.total);
  });
});

function categoryIds(
  events: readonly FactoryEvent[],
  tick: number,
  category: "active" | "completed" | "failed" | "queued" | "unclassified",
): string[] {
  return projectFactoryWorkProgressAtTick({ events, tick })[category].map(
    ({ id }) => id,
  );
}
