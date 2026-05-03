import { describe, expect, it } from "vitest";

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
        workItemIDs: [
          "batch-request-task-2",
          "work-task-2",
          "work-task-4",
        ],
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

describe("projectRuntime", () => {
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
});
