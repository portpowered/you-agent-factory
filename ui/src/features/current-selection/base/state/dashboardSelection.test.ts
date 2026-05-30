// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: selection-resolution regression coverage stays in one place for timeline readability.
import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";

import { buildFactoryTimelineSnapshot } from "../../../timeline/state/factoryTimelineStore";
import {
  findFactoryWorkerInSnapshot,
  resolveDashboardSelection,
  workstationNamesReferencingWorkerInSnapshot,
  type DashboardWorkItemSelection,
  type DashboardWorkstationRequestSelection,
} from "./dashboardSelection";

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = event("event-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
  factory: {
    workTypes: [{
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
      ],
    }],
    workstations: [
      {
        id: "review",
        inputs: [{ state: "new", workType: "story" }],
        name: "Review",
        outputs: [{ state: "review", workType: "story" }],
        worker: "reviewer",
      },
    ],
  },
});

const workRequest = event("event-2", 2, FACTORY_EVENT_TYPES.workRequest, {
  type: "FACTORY_REQUEST_BATCH",
  works: [{
    name: "Selection Story",
    request_id: "request-selection-1",
    trace_id: "trace-selection-1",
    work_id: "work-selection-1",
    work_type_id: "story",
  }],
});
workRequest.context.requestId = "request-selection-1";
workRequest.context.traceIds = ["trace-selection-1"];
workRequest.context.workIds = ["work-selection-1"];

const dispatchRequest = event("event-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
  dispatchId: "dispatch-selection-1",
  inputs: [{
    name: "Selection Story",
    request_id: "request-selection-1",
    trace_id: "trace-selection-1",
    work_id: "work-selection-1",
    work_type_id: "story",
  }],
  transitionId: "review",
  workstation: {
    id: "review",
    inputs: [{ state: "new", workType: "story" }],
    name: "Review",
    outputs: [{ state: "review", workType: "story" }],
    worker: "reviewer",
  },
});
dispatchRequest.context.dispatchId = "dispatch-selection-1";
dispatchRequest.context.traceIds = ["trace-selection-1"];
dispatchRequest.context.workIds = ["work-selection-1"];

describe("resolveDashboardSelection", () => {
  it("retains workstation-request selections while the projected request remains present", () => {
    const activeTick = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest, dispatchRequest],
      3,
    );
    const request = activeTick.workstationRequestsByDispatchID["dispatch-selection-1"];
    if (!request) {
      throw new Error("expected workstation request projection");
    }

    const selection: DashboardWorkstationRequestSelection = {
      dispatchId: request.dispatch_id,
      kind: "workstation-request",
      nodeId: "stale-node-id",
      request,
    };
    const resolved = resolveDashboardSelection({
      selection,
      snapshot: activeTick,
      workstationRequestsByDispatchID: activeTick.workstationRequestsByDispatchID,
    });

    expect(resolved).toMatchObject({
      dispatchId: "dispatch-selection-1",
      kind: "workstation-request",
      nodeId: "review",
    });
  });

  it("falls back to the default dashboard selection when the projected request disappears", () => {
    const activeTick = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest, dispatchRequest],
      3,
    );
    const request = activeTick.workstationRequestsByDispatchID["dispatch-selection-1"];
    if (!request) {
      throw new Error("expected workstation request projection");
    }

    const beforeDispatch = buildFactoryTimelineSnapshot([initialStructureRequest, workRequest], 2);
    const resolved = resolveDashboardSelection({
      selection: {
        dispatchId: request.dispatch_id,
        kind: "workstation-request",
        nodeId: request.workstation_node_id,
        request,
      },
      snapshot: beforeDispatch,
      workstationRequestsByDispatchID: beforeDispatch.workstationRequestsByDispatchID,
    });

    expect(resolved).toEqual({ kind: "node", nodeId: "review" });
  });

  it("reanchors selected work-item detail to the current retained request node", () => {
    const workItem = {
      display_name: "Selection Story",
      trace_id: "trace-selection-1",
      work_id: "work-selection-1",
      work_type_id: "story",
    };
    const snapshot: DashboardSnapshot = {
      factory_state: "RUNNING",
      runtime: {
        active_executions_by_dispatch_id: {},
        current_work_items_by_place_id: {},
        place_occupancy_work_items_by_place_id: {
          "story:repair": [workItem],
        },
        session: {
          completed_count: 0,
          dispatched_count: 1,
          failed_count: 0,
          has_data: true,
          provider_sessions: [],
        },
        workstation_requests_by_dispatch_id: {
          "dispatch-repair-1": {
            dispatch_id: "dispatch-repair-1",
            started_at: "2026-04-16T12:00:03Z",
            transition_id: "repair",
            workstation_name: "Repair",
            workstation_node_id: "repair",
            work_items: [workItem],
          },
        },
      },
      tick_count: 4,
      topology: {
        edges: [],
        workstation_node_ids: ["review", "repair"],
        workstation_nodes_by_id: {
          repair: {
            input_places: [
              {
                kind: "work_state",
                place_id: "story:review",
                state_category: "PROCESSING",
                state_name: "review",
                work_type_name: "story",
              },
            ],
            node_id: "repair",
            output_places: [
              {
                kind: "work_state",
                place_id: "story:repair",
                state_category: "PROCESSING",
                state_name: "repair",
                work_type_name: "story",
              },
            ],
            transition_id: "repair",
            workstation_name: "Repair",
          },
          review: {
            input_places: [
              {
                kind: "work_state",
                place_id: "story:new",
                state_category: "INITIAL",
                state_name: "new",
                work_type_name: "story",
              },
            ],
            node_id: "review",
            output_places: [
              {
                kind: "work_state",
                place_id: "story:review",
                state_category: "PROCESSING",
                state_name: "review",
                work_type_name: "story",
              },
            ],
            transition_id: "review",
            workstation_name: "Review",
          },
        },
      },
      uptime_seconds: 12,
    };
    const selection: DashboardWorkItemSelection = {
      dispatchId: "dispatch-review-1",
      kind: "work-item",
      nodeId: "review",
      workItem,
    };

    const resolved = resolveDashboardSelection({
      selection,
      snapshot,
      workstationRequestsByDispatchID: {
        "dispatch-repair-1": {
          counts: {
            dispatched_count: 1,
            errored_count: 0,
            responded_count: 1,
          },
          dispatch_id: "dispatch-repair-1",
          dispatched_request_count: 1,
          errored_request_count: 0,
          inference_attempts: [],
          request_view: {
            input_work_items: [workItem],
            started_at: "2026-04-16T12:00:03Z",
            trace_ids: ["trace-selection-1"],
          },
          responded_request_count: 1,
          started_at: "2026-04-16T12:00:03Z",
          transition_id: "repair",
          work_items: [workItem],
          workstation_name: "Repair",
          workstation_node_id: "repair",
        },
      },
    });

    expect(resolved).toEqual({
      dispatchId: "dispatch-repair-1",
      kind: "work-item",
      nodeId: "repair",
      workItem,
    });
  });

  it("retains worker selections while the authored worker remains in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
    };

    const selection = {
      kind: "worker" as const,
      workerName: "reviewer",
    };

    expect(
      resolveDashboardSelection({
        selection,
        snapshot,
      }),
    ).toEqual(selection);
  });

  it("resolves authored workers and referencing workstation names from the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER", model: "gpt-5" }],
      workstations: [
        { name: "Review", worker: "reviewer" },
        { name: "Plan", worker: "reviewer" },
        { name: "Code", worker: "planner" },
      ],
    };

    expect(findFactoryWorkerInSnapshot(snapshot, "reviewer")).toEqual({
      name: "reviewer",
      type: "MODEL_WORKER",
      model: "gpt-5",
    });
    expect(workstationNamesReferencingWorkerInSnapshot(snapshot, "reviewer")).toEqual([
      "Review",
      "Plan",
    ]);
  });

  it("falls back to the default dashboard selection when the selected worker disappears", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
    };

    const resolved = resolveDashboardSelection({
      selection: {
        kind: "worker",
        workerName: "removed-worker",
      },
      snapshot,
    });

    expect(resolved).toEqual({
      kind: "node",
      nodeId: "review",
    });
  });
});
