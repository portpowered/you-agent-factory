import { describe, expect, it } from "vitest";

import type { FactoryWorkItem } from "../../../../api/events";
import { applyWorkStateChange } from "./replayWorldStateWorkStateChange";
import type { WorkStateChangeEvent } from "./replayWorldStateTypes";
import { emptyWorldRuntime, type ReplayWorldState } from "./types";

function workStateChangeEvent(
  payload: WorkStateChangeEvent["payload"],
): WorkStateChangeEvent {
  return {
    context: {
      eventTime: "2026-05-30T12:00:00.000Z",
      sequence: 1,
      tick: 1,
    },
    id: "event-work-state-change",
    payload,
    type: "WORK_STATE_CHANGE",
  };
}

function replayStateWithWork(
  workID: string,
  placeID: string,
  category: "FAILED" | "TERMINAL" | "PROCESSING",
): ReplayWorldState {
  const item: FactoryWorkItem = {
    id: workID,
    place_id: placeID,
    work_type_id: "task",
  };
  return {
    activeDispatches: {},
    completedDispatches: [],
    factory_state: "RUNNING",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: category === "FAILED" ? { [workID]: item } : {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {
      [placeID]: { placeID, resourceTokenIDs: [], workItemIDs: [workID] },
    },
    providerSessions: [],
    relationsByWorkID: {},
    runtime: emptyWorldRuntime(),
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: category === "TERMINAL" ? { [workID]: { status: "TERMINAL", work_item: item } } : {},
    tick_count: 1,
    topology: {
      places: [{ category, id: placeID, name: placeID }],
    },
    tracesByID: {},
    tracesByWorkID: {},
    uptime_seconds: 0,
    workItemsByID: { [workID]: item },
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

describe("applyWorkStateChange", () => {
  it("no-ops when workId is missing", () => {
    const state = replayStateWithWork("work-1", "task:init", "PROCESSING");
    applyWorkStateChange(state, workStateChangeEvent({
      fromPlaceId: "task:init",
      fromState: "init",
      source: "cli",
      toPlaceId: "task:review",
      toState: "review",
      workId: "",
      workTypeName: "task",
    }));
    expect(state.occupancyByID["task:init"]?.workItemIDs).toEqual(["work-1"]);
  });

  it("clears failed and terminal indexes when leaving those places", () => {
    const failedState = replayStateWithWork("work-failed", "task:failed", "FAILED");
    applyWorkStateChange(failedState, workStateChangeEvent({
      fromPlaceId: "task:failed",
      fromState: "failed",
      source: "api",
      toPlaceId: "task:review",
      toState: "review",
      workId: "work-failed",
      workTypeName: "task",
    }));
    expect(failedState.failedWorkItemsByID["work-failed"]).toBeUndefined();
    expect(failedState.occupancyByID["task:review"]?.workItemIDs).toEqual(["work-failed"]);

    const terminalState = replayStateWithWork("work-terminal", "task:done", "TERMINAL");
    applyWorkStateChange(terminalState, workStateChangeEvent({
      fromPlaceId: "task:done",
      fromState: "done",
      source: "cli",
      toPlaceId: "task:review",
      toState: "review",
      workId: "work-terminal",
      workTypeName: "task",
    }));
    expect(terminalState.terminalWorkByID["work-terminal"]).toBeUndefined();
    expect(terminalState.occupancyByID["task:review"]?.workItemIDs).toEqual(["work-terminal"]);
  });

  it("records failed and terminal occupancy at destination places", () => {
    const state = replayStateWithWork("work-move", "task:init", "PROCESSING");
    state.topology.places = [
      { category: "PROCESSING", id: "task:init", name: "task:init" },
      { category: "FAILED", id: "task:failed", name: "task:failed" },
      { category: "TERMINAL", id: "task:done", name: "task:done" },
    ];
    applyWorkStateChange(state, workStateChangeEvent({
      fromPlaceId: "task:init",
      fromState: "init",
      source: "cli",
      toPlaceId: "task:failed",
      toState: "failed",
      workId: "work-move",
      workTypeName: "task",
    }));
    expect(state.failedWorkItemsByID["work-move"]).toBeDefined();
    expect(state.occupancyByID["task:failed"]?.workItemIDs).toEqual(["work-move"]);

    applyWorkStateChange(state, workStateChangeEvent({
      fromPlaceId: "task:failed",
      fromState: "failed",
      source: "cli",
      toPlaceId: "task:done",
      toState: "done",
      workId: "work-move",
      workTypeName: "task",
    }));
    expect(state.failedWorkItemsByID["work-move"]).toBeUndefined();
    expect(state.terminalWorkByID["work-move"]).toBeDefined();
  });

  it("removes work tokens when fromPlaceId is omitted", () => {
    const state = replayStateWithWork("work-orphan", "task:init", "PROCESSING");
    state.topology.places = [
      { category: "PROCESSING", id: "task:init", name: "task:init" },
      { category: "PROCESSING", id: "task:review", name: "task:review" },
    ];
    applyWorkStateChange(state, workStateChangeEvent({
      fromState: "init",
      source: "api",
      toPlaceId: "task:review",
      toState: "review",
      workId: "work-orphan",
      workTypeName: "task",
    }));
    expect(state.occupancyByID["task:init"]?.workItemIDs ?? []).toEqual([]);
    expect(state.occupancyByID["task:review"]?.workItemIDs).toEqual(["work-orphan"]);
  });
});
