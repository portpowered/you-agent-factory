import { expect, it } from "vitest";

import { projectRuntime } from "./projectRuntime";
import type { ReplayWorldState } from "./types";
import { emptyWorldRuntime } from "./types";

function buildReplayWorldState(): ReplayWorldState {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factory_state: "RUNNING",
    failedWorkDetailsByWorkID: {
      "work-plan-19": {
        dispatch_id: "dispatch-plan-19",
        failure_message: "workspace setup failed",
        failure_reason: "script_error",
        transition_id: "setup-workspace",
        work_item: {
          display_name: "retire-dispatch-result-hook-syncdispatch-cache",
          work_id: "work-plan-19",
          work_type_id: "plan",
        },
        workstation_name: "setup-workspace",
      },
    },
    failedWorkItemsByID: {
      "work-plan-19": {
        id: "work-plan-19",
        display_name: "retire-dispatch-result-hook-syncdispatch-cache",
        place_id: "plan:failed",
        work_type_id: "plan",
      },
    },
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {
      "plan:failed": {
        placeID: "plan:failed",
        resourceTokenIDs: [],
        tokenCount: 1,
        workItemIDs: ["work-plan-19"],
      },
      "task:failed": {
        placeID: "task:failed",
        resourceTokenIDs: [],
        tokenCount: 3,
        workItemIDs: ["batch-request-task-2", "work-task-2", "work-task-4"],
      },
    },
    providerSessions: [],
    relationsByWorkID: {},
    runtime: emptyWorldRuntime(),
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: {},
    tick_count: 1766,
    topology: {
      places: [
        { id: "plan:failed", category: "FAILED", type_id: "plan" },
        { id: "task:failed", category: "FAILED", type_id: "task" },
      ],
      work_types: [
        { id: "plan", name: "plan" },
        { id: "task", name: "task" },
      ],
    },
    tracesByID: {},
    tracesByWorkID: {},
    uptime_seconds: 0,
    workItemsByID: {
      "work-plan-19": {
        id: "work-plan-19",
        display_name: "retire-dispatch-result-hook-syncdispatch-cache",
        place_id: "plan:failed",
        work_type_id: "plan",
      },
      "batch-request-task-2": {
        id: "batch-request-task-2",
        display_name: "prd-functional-test-suite-decomposition",
        place_id: "task:failed",
        work_type_id: "task",
      },
      "work-task-2": {
        id: "work-task-2",
        display_name: "prd-functional-test-suite-decomposition",
        place_id: "task:failed",
        work_type_id: "task",
      },
      "work-task-4": {
        id: "work-task-4",
        display_name: "prd-api-model-contract-cleanup",
        place_id: "task:failed",
        work_type_id: "task",
      },
    },
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

it("counts failed work from failed-place occupancy without double-counting duplicate labels", () => {
  const runtime = projectRuntime(buildReplayWorldState());

  expect(runtime.session.failed_count).toBe(3);
  expect(runtime.session.failed_by_work_type).toEqual({
    plan: 1,
    task: 2,
  });
  expect(runtime.session.failed_work_labels).toEqual([
    "prd-api-model-contract-cleanup",
    "prd-functional-test-suite-decomposition",
    "retire-dispatch-result-hook-syncdispatch-cache",
  ]);
});

it("projects active customer runtime state while filtering system-only work", () => {
  const state = buildReplayWorldState();
  state.activeDispatches = {
    "dispatch-review": {
      consumedTokens: [],
      dispatchID: "dispatch-review",
      resources: [],
      startedAt: "2026-05-27T01:02:03Z",
      systemOnly: false,
      traceIDs: ["trace-review"],
      transitionID: "review",
      workItems: [
        {
          display_name: "Review Story",
          work_id: "work-story-active",
          work_type_id: "story",
        },
      ],
      workstationName: "Review",
    },
    "dispatch-system": {
      consumedTokens: [],
      dispatchID: "dispatch-system",
      resources: [],
      startedAt: "2026-05-27T01:02:04Z",
      systemOnly: true,
      traceIDs: ["trace-system"],
      transitionID: "__system_time:expire",
      workItems: [
        {
          work_id: "time-work",
          work_type_id: "__system_time",
        },
      ],
    },
  };
  state.completedDispatches = [
    {
      ...state.activeDispatches["dispatch-review"],
      dispatchID: "dispatch-complete",
      durationMillis: 2000,
      endTime: "2026-05-27T01:02:05Z",
      inputItems: [],
      outcome: "ACCEPTED",
      outputItems: [],
      outputMutations: [],
    },
  ];
  state.occupancyByID = {
    "story:new": {
      placeID: "story:new",
      resourceTokenIDs: [],
      tokenCount: 1,
      workItemIDs: ["work-story-active", "work-story-done"],
    },
    "__system_time:pending": {
      placeID: "__system_time:pending",
      resourceTokenIDs: [],
      tokenCount: 1,
      workItemIDs: ["time-work"],
    },
  };
  state.terminalWorkByID = {
    "work-story-done": {
      status: "TERMINAL",
      work_item: {
        id: "work-story-done",
        display_name: "Done Story",
        place_id: "story:done",
        work_type_id: "story",
      },
    },
  };
  state.topology.places = [
    { id: "story:new", category: "INITIAL", type_id: "story" },
    { id: "story:done", category: "TERMINAL", type_id: "story" },
    {
      id: "__system_time:pending",
      category: "PROCESSING",
      type_id: "__system_time",
    },
  ];
  state.topology.work_types = [{ id: "story", name: "story" }];
  state.workItemsByID = {
    "work-story-active": {
      id: "work-story-active",
      display_name: "Review Story",
      place_id: "story:new",
      work_type_id: "story",
    },
    "work-story-done": {
      id: "work-story-done",
      display_name: "Done Story",
      place_id: "story:done",
      work_type_id: "story",
    },
    "time-work": {
      id: "time-work",
      place_id: "__system_time:pending",
      work_type_id: "__system_time",
    },
  };

  const runtime = projectRuntime(state);

  expect(runtime.active_dispatch_ids).toEqual(["dispatch-review"]);
  expect(runtime.active_workstation_node_ids).toEqual([
    "__system_time:expire",
    "review",
  ]);
  expect(runtime.current_work_items_by_place_id?.["story:new"]).toEqual([
    expect.objectContaining({ work_id: "work-story-active" }),
  ]);
  expect(runtime.place_token_counts).toEqual({ "story:new": 1 });
  expect(runtime.session.completed_count).toBe(1);
  expect(runtime.session.completed_work_labels).toEqual(["Done Story"]);
  expect(runtime.session.dispatched_count).toBe(2);
  expect(runtime.session.has_data).toBe(true);
  expect(runtime.workstation_activity_by_node_id.review).toMatchObject({
    active_dispatch_ids: ["dispatch-review"],
    trace_ids: ["trace-review"],
  });
});
