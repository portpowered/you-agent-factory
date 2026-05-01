import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";

import { buildFactoryTimelineSnapshot } from "./factoryTimelineStore";
import {
  resolveDashboardSelection,
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
      snapshot: activeTick.dashboard,
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
      snapshot: beforeDispatch.dashboard,
      workstationRequestsByDispatchID: beforeDispatch.workstationRequestsByDispatchID,
    });

    expect(resolved).toEqual({ kind: "node", nodeId: "review" });
  });
});
