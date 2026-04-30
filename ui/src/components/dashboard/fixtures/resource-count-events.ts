import { FACTORY_EVENT_TYPES } from "../../../api/events";
import type { FactoryEvent } from "../../../api/events";

export const resourceCountAvailablePlaceID = "agent-slot:available";

function factoryEvent(
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

const resourceWorkstation = {
  id: "implement",
  inputs: [
    { state: "new", workType: "story" },
    { state: "available", workType: "agent-slot" },
  ],
  name: "Implement",
  outputs: [{ state: "done", workType: "story" }],
  worker: "agent",
};

export const resourceCountTimelineEvents: FactoryEvent[] = [
  factoryEvent("resource-count-structure", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      resources: [{ capacity: 2, name: "agent-slot" }],
      workTypes: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      }],
      workstations: [resourceWorkstation],
    },
  }),
  factoryEvent("resource-count-work-input", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Resource Occupancy Story",
      trace_id: "trace-resource-count",
      work_id: "work-resource-count",
      work_type_name: "story",
    }],
  }),
  factoryEvent("resource-count-request", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: "dispatch-resource-count",
    inputs: [
      {
        name: "Resource Occupancy Story",
        trace_id: "trace-resource-count",
        work_id: "work-resource-count",
        work_type_name: "story",
      },
    ],
    resources: [{ capacity: 2, name: "agent-slot" }],
    transitionId: "implement",
    workstation: resourceWorkstation,
  }),
  factoryEvent("resource-count-response", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: "dispatch-resource-count",
    durationMillis: 1000,
    outcome: "ACCEPTED",
    outputResources: [{ capacity: 2, name: "agent-slot" }],
    outputWork: [
      {
        name: "Resource Occupancy Story",
        trace_id: "trace-resource-count",
        work_id: "work-resource-count",
        work_type_name: "story",
      },
    ],
    transitionId: "implement",
    workstation: resourceWorkstation,
  }),
];

resourceCountTimelineEvents[1].context.requestId = "request-resource-count";
resourceCountTimelineEvents[1].context.traceIds = ["trace-resource-count"];
resourceCountTimelineEvents[1].context.workIds = ["work-resource-count"];
resourceCountTimelineEvents[2].context.dispatchId = "dispatch-resource-count";
resourceCountTimelineEvents[2].context.traceIds = ["trace-resource-count"];
resourceCountTimelineEvents[2].context.workIds = ["work-resource-count"];
resourceCountTimelineEvents[3].context.dispatchId = "dispatch-resource-count";
resourceCountTimelineEvents[3].context.traceIds = ["trace-resource-count"];
resourceCountTimelineEvents[3].context.workIds = ["work-resource-count"];
