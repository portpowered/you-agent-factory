import { FACTORY_EVENT_TYPES } from "../../../api/events";
import type { FactoryEvent } from "../../../api/events";

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

const reviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", workType: "story" }],
  name: "Review",
  onFailure: [{ state: "failed", workType: "story" }],
  outputs: [{ state: "done", workType: "story" }],
  worker: "reviewer",
};

export const graphStateSmokeTimelineEvents: FactoryEvent[] = [
  factoryEvent("graph-state-smoke-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      workTypes: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "review", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      }],
      workstations: [reviewWorkstation],
    },
  }),
  factoryEvent("graph-state-smoke-2", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Completed Smoke Story One",
      trace_id: "trace-smoke-complete-one",
      work_id: "work-smoke-complete-one",
      work_type_name: "story",
    }],
  }),
  factoryEvent("graph-state-smoke-3", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Completed Smoke Story Two",
      trace_id: "trace-smoke-complete-two",
      work_id: "work-smoke-complete-two",
      work_type_name: "story",
    }],
  }),
  factoryEvent("graph-state-smoke-4", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Failed Smoke Story",
      trace_id: "trace-smoke-failed",
      work_id: "work-smoke-failed",
      work_type_name: "story",
    }],
  }),
  factoryEvent("graph-state-smoke-5", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: "dispatch-smoke-complete-one",
    inputs: [
      {
        name: "Completed Smoke Story One",
        trace_id: "trace-smoke-complete-one",
        work_id: "work-smoke-complete-one",
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-6", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: "dispatch-smoke-complete-one",
    durationMillis: 700,
    outcome: "ACCEPTED",
    outputWork: [
      {
        name: "Completed Smoke Story One",
        trace_id: "trace-smoke-complete-one",
        work_id: "work-smoke-complete-one",
        work_type_name: "story",
      },
    ],
    providerSession: {
      id: "sess-smoke-complete-one",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-7", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: "dispatch-smoke-failed",
    inputs: [
      {
        name: "Failed Smoke Story",
        trace_id: "trace-smoke-failed",
        work_id: "work-smoke-failed",
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-8", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: "dispatch-smoke-failed",
    durationMillis: 900,
    failureMessage: "Provider rate limit exceeded while running the graph-state smoke.",
    failureReason: "provider_rate_limit",
    outcome: "FAILED",
    outputWork: [
      {
        name: "Failed Smoke Story",
        trace_id: "trace-smoke-failed",
        work_id: "work-smoke-failed",
        work_type_name: "story",
      },
    ],
    providerSession: {
      id: "sess-smoke-failed",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-9", 7, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: "dispatch-smoke-complete-two",
    inputs: [
      {
        name: "Completed Smoke Story Two",
        trace_id: "trace-smoke-complete-two",
        work_id: "work-smoke-complete-two",
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-10", 8, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: "dispatch-smoke-complete-two",
    durationMillis: 650,
    outcome: "ACCEPTED",
    outputWork: [
      {
        name: "Completed Smoke Story Two",
        trace_id: "trace-smoke-complete-two",
        work_id: "work-smoke-complete-two",
        work_type_name: "story",
      },
    ],
    providerSession: {
      id: "sess-smoke-complete-two",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("graph-state-smoke-11", 9, FACTORY_EVENT_TYPES.factoryStateResponse, {
    previousState: "RUNNING",
    reason: "smoke complete",
    state: "FINISHED",
  }),
];

graphStateSmokeTimelineEvents[1].context.requestId = "request-smoke-complete-one";
graphStateSmokeTimelineEvents[1].context.traceIds = ["trace-smoke-complete-one"];
graphStateSmokeTimelineEvents[1].context.workIds = ["work-smoke-complete-one"];
graphStateSmokeTimelineEvents[2].context.requestId = "request-smoke-complete-two";
graphStateSmokeTimelineEvents[2].context.traceIds = ["trace-smoke-complete-two"];
graphStateSmokeTimelineEvents[2].context.workIds = ["work-smoke-complete-two"];
graphStateSmokeTimelineEvents[3].context.requestId = "request-smoke-failed";
graphStateSmokeTimelineEvents[3].context.traceIds = ["trace-smoke-failed"];
graphStateSmokeTimelineEvents[3].context.workIds = ["work-smoke-failed"];
graphStateSmokeTimelineEvents[4].context.dispatchId = "dispatch-smoke-complete-one";
graphStateSmokeTimelineEvents[4].context.traceIds = ["trace-smoke-complete-one"];
graphStateSmokeTimelineEvents[4].context.workIds = ["work-smoke-complete-one"];
graphStateSmokeTimelineEvents[5].context.dispatchId = "dispatch-smoke-complete-one";
graphStateSmokeTimelineEvents[5].context.traceIds = ["trace-smoke-complete-one"];
graphStateSmokeTimelineEvents[5].context.workIds = ["work-smoke-complete-one"];
graphStateSmokeTimelineEvents[6].context.dispatchId = "dispatch-smoke-failed";
graphStateSmokeTimelineEvents[6].context.traceIds = ["trace-smoke-failed"];
graphStateSmokeTimelineEvents[6].context.workIds = ["work-smoke-failed"];
graphStateSmokeTimelineEvents[7].context.dispatchId = "dispatch-smoke-failed";
graphStateSmokeTimelineEvents[7].context.traceIds = ["trace-smoke-failed"];
graphStateSmokeTimelineEvents[7].context.workIds = ["work-smoke-failed"];
graphStateSmokeTimelineEvents[8].context.dispatchId = "dispatch-smoke-complete-two";
graphStateSmokeTimelineEvents[8].context.traceIds = ["trace-smoke-complete-two"];
graphStateSmokeTimelineEvents[8].context.workIds = ["work-smoke-complete-two"];
graphStateSmokeTimelineEvents[9].context.dispatchId = "dispatch-smoke-complete-two";
graphStateSmokeTimelineEvents[9].context.traceIds = ["trace-smoke-complete-two"];
graphStateSmokeTimelineEvents[9].context.workIds = ["work-smoke-complete-two"];
