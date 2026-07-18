import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
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
  type:
    | "DISPATCH_REQUEST"
    | "DISPATCH_RESPONSE"
    | "INITIAL_STRUCTURE_REQUEST"
    | "WORK_REQUEST",
  tick: number,
  sequence: number,
  payload: unknown,
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T09:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  } as FactoryEvent;
}

function dispatch(
  id: string,
  type: "DISPATCH_REQUEST" | "DISPATCH_RESPONSE",
  tick: number,
  outputWork?: unknown[],
): FactoryEvent {
  return event(
    id,
    type,
    tick,
    0,
    type === "DISPATCH_REQUEST"
      ? { inputs: [{ workId: "work-1" }], transitionId: "edit" }
      : { outcome: "CONTINUE", outputWork, transitionId: "edit" },
    { dispatchId: "dispatch-1", workIds: ["work-1"] },
  );
}

describe("selected-tick projection consistency", () => {
  it("agrees with Work State counts and active Dispatch overlays", () => {
    const work = {
      name: "Draft",
      state: { name: "editing", type: "PROCESSING" },
      workId: "work-1",
      workTypeName: "story",
    };
    const events = [
      event("topology", "INITIAL_STRUCTURE_REQUEST", 1, 0, { factory }),
      event("request", "WORK_REQUEST", 1, 1, {
        type: "FACTORY_REQUEST_BATCH",
        works: [work],
      }),
      dispatch("start", "DISPATCH_REQUEST", 2),
      dispatch("finish", "DISPATCH_RESPONSE", 3, [work]),
    ];

    const activeProgress = projectFactoryWorkProgressAtTick({
      events,
      tick: 2,
    });
    const activeLoad = projectFactoryLoadAtTick({ events, tick: 2 });
    const activeActivity = projectFactoryActivityAtTick({ events, tick: 2 });
    expect(activeProgress.active.map(({ id }) => id)).toEqual(["work-1"]);
    expect(
      activeActivity.activeDispatchOverlays.flatMap(
        (overlay) => overlay.workIds ?? [],
      ),
    ).toEqual(["work-1"]);
    expect(activeLoad.workStateCounts.map(({ count }) => count)).toEqual([
      0, 0, 0, 0,
    ]);

    const queuedProgress = projectFactoryWorkProgressAtTick({
      events,
      tick: 3,
    });
    const queuedLoad = projectFactoryLoadAtTick({ events, tick: 3 });
    const queuedActivity = projectFactoryActivityAtTick({ events, tick: 3 });
    expect(queuedProgress.queued.map(({ id }) => id)).toEqual(["work-1"]);
    expect(queuedActivity.activeDispatchOverlays).toEqual([]);
    expect(
      queuedLoad.workStateCounts.find(
        ({ workStateId }) => workStateId === "story-stable:editing-stable",
      )?.workIds,
    ).toEqual(["work-1"]);
  });
});
